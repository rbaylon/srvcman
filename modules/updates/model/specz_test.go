package updatesmodel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func sum(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

// fakeSpecz is a stand-in for the update server's software update API.
type fakeSpecz struct {
	*httptest.Server
	mu       sync.Mutex
	token    string
	latest   string // JSON body of the latest endpoint
	status   int    // when not 200: answer the latest endpoint with it
	errBody  string
	files    map[string]string // file id -> content
	releases []string          // release query values seen
	auths    []string
	fileHits int
}

func newFakeSpecz(t *testing.T) *fakeSpecz {
	f := &fakeSpecz{token: "spz_testtoken1234567890", files: map[string]string{}, status: 200}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/updates/speczisp/latest", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auths = append(f.auths, r.Header.Get("Authorization"))
		f.releases = append(f.releases, r.URL.Query().Get("release"))
		if r.Header.Get("Authorization") != "Bearer "+f.token {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":"invalid_token","message":"nope"}`)
			return
		}
		if f.status != 200 {
			w.WriteHeader(f.status)
			fmt.Fprint(w, f.errBody)
			return
		}
		fmt.Fprint(w, f.latest)
	})
	mux.HandleFunc("/api/v1/updates/speczisp/files/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.fileHits++
		f.auths = append(f.auths, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") != "Bearer "+f.token {
			w.WriteHeader(401)
			return
		}
		body, ok := f.files[strings.TrimPrefix(r.URL.Path, "/api/v1/updates/speczisp/files/")]
		if !ok {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":"file_not_found"}`)
			return
		}
		fmt.Fprint(w, body)
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

// setLatest makes the latest endpoint offer one update with the given files (id -> name, content).
func (f *fakeSpecz) setLatest(version string, files ...[3]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var parts []string
	for _, fl := range files {
		id, name, content := fl[0], fl[1], fl[2]
		f.files[id] = content
		parts = append(parts, fmt.Sprintf(`{"name":%q,"size":%d,"sha256":%q,"url":"%s/api/v1/updates/speczisp/files/%s"}`, name, len(content), sum(content), f.URL, id))
	}
	f.latest = fmt.Sprintf(`{"product":"speczisp","tier":"pro","release":"1.0.0","latest":{"version":%q,"notes":"Fixes","channels":["free","pro"],"published_at":"2026-10-13T11:23:19Z","available_from":"2026-10-13T11:23:19Z","files":[%s]}}`,
		version, strings.Join(parts, ","))
}

func (f *fakeSpecz) client() *Client { return NewClient(f.URL, "", f.token) }

func TestLatestParsesAndAsksForTheRelease(t *testing.T) {
	f := newFakeSpecz(t)
	f.setLatest("1.0.1", [3]string{"7", "u.tgz", "package"})
	l, err := f.client().Latest(context.Background(), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if l.Tier != "pro" || l.Release != "1.0.0" || l.Latest == nil || l.Latest.Version != "1.0.1" || len(l.Latest.Files) != 1 || l.Latest.Files[0].Name != "u.tgz" || l.Scheduled != nil {
		t.Fatalf("parsed %+v / %+v", l, l.Latest)
	}
	if f.releases[0] != "1.0.0" || f.auths[0] != "Bearer "+f.token {
		t.Fatalf("request: release %q auth %q", f.releases[0], f.auths[0])
	}
	// No known release: the parameter is left out.
	if _, err := f.client().Latest(context.Background(), ""); err != nil || f.releases[1] != "" {
		t.Fatalf("no release: %v %q", err, f.releases[1])
	}
	// Nothing to offer is not an error.
	f.latest = `{"product":"speczisp","tier":"free","release":"1.0.0","latest":null,"scheduled":{"version":"1.0.2","available_from":"2026-11-03T09:00:00Z"}}`
	l, err = f.client().Latest(context.Background(), "")
	if err != nil || l.Latest != nil || l.Scheduled == nil || l.Scheduled.Version != "1.0.2" {
		t.Fatalf("scheduled only: %v %+v", err, l)
	}
}

func TestLatestErrorsAreWrittenForTheOperator(t *testing.T) {
	f := newFakeSpecz(t)
	cases := []struct {
		name   string
		client *Client
		status int
		body   string
		want   error
	}{
		{"wrong token", NewClient(f.URL, "", "spz_wrongwrongwrong"), 200, "", ErrBadToken},
		{"rate limited", f.client(), 429, `{"error":"rate_limited"}`, ErrRateLimited},
		{"no release", f.client(), 404, `{"error":"release_not_found"}`, ErrNoRelease},
		{"no product", f.client(), 404, `{"error":"product_not_found"}`, ErrNoProduct},
	}
	for _, c := range cases {
		f.status, f.errBody = c.status, c.body
		if _, err := c.client.Latest(context.Background(), ""); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	f.status, f.errBody = 500, `{"error":"server_error","message":"Something went wrong on our side."}`
	if _, err := f.client().Latest(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "Something went wrong") {
		t.Errorf("500: %v", err)
	}
	f.status = 200
	f.latest = "not json"
	if _, err := f.client().Latest(context.Background(), ""); err == nil {
		t.Error("garbage accepted")
	}
	// Unreachable server.
	dead := f.client()
	f.Close()
	if _, err := dead.Latest(context.Background(), ""); !errors.Is(err, ErrUnreachable) {
		t.Errorf("unreachable: %v", err)
	}
}

func TestLatestRefusesUnsafeUpdates(t *testing.T) {
	f := newFakeSpecz(t)
	good := func() Update {
		return Update{Version: "1.0.1", Files: []File{{Name: "u.tgz", Size: 10, SHA256: sum("x"), URL: f.URL + "/api/v1/updates/speczisp/files/3"}}}
	}
	c := f.client()
	if err := c.checkUpdate(ptr(good())); err != nil {
		t.Fatalf("a good update was refused: %v", err)
	}
	mutate := map[string]func(*Update){
		"version with a slash": func(u *Update) { u.Version = "1.0/../x" },
		"empty version":        func(u *Update) { u.Version = "" },
		"no files":             func(u *Update) { u.Files = nil },
		"too many files":       func(u *Update) { u.Files = make([]File, 21) },
		"path in a file name":  func(u *Update) { u.Files[0].Name = "../u.tgz" },
		"hidden file name":     func(u *Update) { u.Files[0].Name = ".profile" },
		"reserved name":        func(u *Update) { u.Files[0].Name = "MANIFEST" },
		"shell characters":     func(u *Update) { u.Files[0].Name = "a;b.tgz" },
		"duplicate name":       func(u *Update) { u.Files = append(u.Files, u.Files[0]) },
		"checksum not sha256":  func(u *Update) { u.Files[0].SHA256 = "abc" },
		"upper case checksum":  func(u *Update) { u.Files[0].SHA256 = strings.ToUpper(sum("x")) },
		"zero size":            func(u *Update) { u.Files[0].Size = 0 },
		"huge size":            func(u *Update) { u.Files[0].Size = maxFileSize + 1 },
		"another host":         func(u *Update) { u.Files[0].URL = "https://evil.example/api/v1/updates/speczisp/files/3" },
		"another product":      func(u *Update) { u.Files[0].URL = f.URL + "/api/v1/updates/other/files/3" },
		"other path":           func(u *Update) { u.Files[0].URL = f.URL + "/download/file" },
		"path traversal":       func(u *Update) { u.Files[0].URL = f.URL + "/api/v1/updates/speczisp/files/../../../x" },
		"non-numeric file id":  func(u *Update) { u.Files[0].URL = f.URL + "/api/v1/updates/speczisp/files/abc" },
		"query string":         func(u *Update) { u.Files[0].URL = f.URL + "/api/v1/updates/speczisp/files/3?x=1" },
		"credentials in a url": func(u *Update) {
			u.Files[0].URL = strings.Replace(f.URL, "http://", "http://u:p@", 1) + "/api/v1/updates/speczisp/files/3"
		},
		"downgraded scheme": func(u *Update) {
			u.Files[0].URL = strings.Replace(f.URL, "http://", "https://", 1) + "/api/v1/updates/speczisp/files/3"
		},
		"double slash in a url": func(u *Update) { u.Files[0].URL = f.URL + "/api/v1/updates/speczisp//files/3" },
	}
	for name, m := range mutate {
		u := good()
		m(&u)
		if err := c.checkUpdate(&u); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	// And Latest itself refuses such an answer.
	f.latest = `{"product":"speczisp","tier":"pro","release":"1.0.0","latest":{"version":"1.0.1","files":[{"name":"u.tgz","size":10,"sha256":"` + sum("x") + `","url":"https://evil.example/api/v1/updates/speczisp/files/3"}]}}`
	if _, err := c.Latest(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Errorf("Latest accepted a foreign download address: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }

func TestDownloadChecksSizeAndSum(t *testing.T) {
	f := newFakeSpecz(t)
	f.setLatest("1.0.1", [3]string{"7", "u.tgz", "package bytes"})
	c := f.client()
	l, _ := c.Latest(context.Background(), "")
	file := l.Latest.Files[0]
	dir := t.TempDir()

	if err := c.Download(context.Background(), file, dir); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "u.tgz")); string(b) != "package bytes" {
		t.Fatalf("content %q", b)
	}
	if f.auths[len(f.auths)-1] != "Bearer "+f.token {
		t.Fatal("the download did not carry the token")
	}
	// Downloading it again over the same name is refused rather than overwriting.
	if err := c.Download(context.Background(), file, dir); err == nil {
		t.Fatal("an existing file was overwritten")
	}

	// A different body than announced: removed, with a reason.
	dir2 := t.TempDir()
	f.files["7"] = "packagX bytes"
	err := c.Download(context.Background(), file, dir2)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("tampered file: %v", err)
	}
	if _, serr := os.Stat(filepath.Join(dir2, "u.tgz")); serr == nil {
		t.Fatal("a file that failed its checksum was kept")
	}
	// A body of the wrong length.
	f.files["7"] = "package bytes plus more"
	if err := c.Download(context.Background(), file, t.TempDir()); err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("wrong size: %v", err)
	}
	f.files["7"] = "short"
	if err := c.Download(context.Background(), file, t.TempDir()); err == nil {
		t.Fatal("a short file was accepted")
	}
	// Refusals before any request is made.
	hits := f.fileHits
	for _, bad := range []File{
		{Name: "../x", Size: 1, SHA256: sum("x"), URL: file.URL},
		{Name: "u.tgz", Size: 1, SHA256: sum("x"), URL: "https://evil.example/api/v1/updates/speczisp/files/7"},
	} {
		if err := c.Download(context.Background(), bad, t.TempDir()); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if f.fileHits != hits {
		t.Fatal("a refused download still made a request")
	}
	// The server says no.
	f.token = "something-else-entirely"
	if err := c.Download(context.Background(), file, t.TempDir()); !errors.Is(err, ErrBadToken) {
		t.Fatalf("revoked token: %v", err)
	}
	// Cancelled.
	f.token = "spz_testtoken1234567890"
	f.files["7"] = "package bytes"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Download(ctx, file, t.TempDir()); err == nil {
		t.Fatal("a cancelled download succeeded")
	}
}

// The token is only ever sent to the configured server: a redirect is not followed.
func TestTokenIsNotSentAcrossRedirects(t *testing.T) {
	seen := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = true }))
	defer other.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer redir.Close()
	c := NewClient(redir.URL, "", "spz_testtoken1234567890")
	if _, err := c.Latest(context.Background(), ""); err == nil {
		t.Fatal("a redirect answer was accepted")
	}
	if seen {
		t.Fatal("the client followed a redirect to another host")
	}
}

func TestValidators(t *testing.T) {
	for _, ok := range []string{"speczisp", "specz-cloud", "a1"} {
		if !ValidProduct(ok) {
			t.Errorf("%q should be a valid product", ok)
		}
	}
	for _, bad := range []string{"", "Specz", "a b", "a/b", "-a", "a-", strings.Repeat("a", 61)} {
		if ValidProduct(bad) {
			t.Errorf("%q should not be a valid product", bad)
		}
	}
	for _, ok := range []string{"spz_" + strings.Repeat("a", 43), "abcdefgh", "A_b-9aaa"} {
		if !ValidToken(ok) {
			t.Errorf("%q should be a valid token", ok)
		}
	}
	for _, bad := range []string{"", "short", "has space 12345", "semi;colon12345", "new\nline12345", strings.Repeat("a", 129)} {
		if ValidToken(bad) {
			t.Errorf("%q should not be a valid token", bad)
		}
	}
}
