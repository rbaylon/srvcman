package updatesmodel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// This file is the client of the update server's software update API
// (https://<update server>/api/v1/updates/<product>/..., documented in the
// specz.net README). The server is addressed with a bearer API token that an
// operator creates by hand on the website and pastes into srvcmanui.
//
// What the server tells us depends on the plan of the account the token
// belongs to (Pro gets updates at once, Free on a schedule); this client does
// not try to work that out, it only asks for the latest update and checks
// what comes back before trusting it.

const (
	// DefaultProduct is the product slug asked for when none is configured.
	DefaultProduct = "speczisp"

	// maxFileSize bounds one update file. Updates carry only changed
	// binaries, not an OS image, so this is generous.
	maxFileSize = int64(1) << 30

	// metadataTimeout bounds one metadata request. Downloads are bounded
	// by the job's own context instead.
	metadataTimeout = 20 * time.Second
)

// Errors a check or download reports. They are written for the operator
// reading them in the UI: what is wrong and what to do about it.
var (
	ErrNotConfigured = errors.New("the update server and API token are not set up yet")
	ErrBadToken      = errors.New("the update server did not accept the API token: it may be wrong or revoked. Create a new token on the website and enter it again")
	ErrUnreachable   = errors.New("could not reach the update server")
	ErrNoRelease     = errors.New("the update server has no such release for this product")
	ErrNoProduct     = errors.New("the update server does not know this product")
	ErrRateLimited   = errors.New("the update server is busy. Try again in a minute")
)

// File is one file of an update, as listed by the update server.
type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url"`
}

// Update is the latest update the token's account may pull right now.
type Update struct {
	Version       string   `json:"version"`
	Notes         string   `json:"notes"`
	Channels      []string `json:"channels"`
	PublishedAt   string   `json:"published_at"`
	AvailableFrom string   `json:"available_from"`
	Files         []File   `json:"files"`
}

// Scheduled is a later update a Free account has been promised.
type Scheduled struct {
	Version       string `json:"version"`
	AvailableFrom string `json:"available_from"`
}

// Latest is the answer of GET /api/v1/updates/<product>/latest.
type Latest struct {
	Product   string     `json:"product"`
	Tier      string     `json:"tier"`
	Release   string     `json:"release"`
	Latest    *Update    `json:"latest"`
	Scheduled *Scheduled `json:"scheduled,omitempty"`
}

// Client talks to one update server with one token.
type Client struct {
	Server  string // base URL, e.g. https://www.specz.net
	Product string
	Token   string
	HTTP    *http.Client
}

// NewClient returns a client for the configured server. HTTP is left nil:
// httpClient() supplies one that never follows redirects, so the token
// cannot be sent anywhere but the configured server.
func NewClient(server, product, token string) *Client {
	if product == "" {
		product = DefaultProduct
	}
	return &Client{Server: strings.TrimRight(server, "/"), Product: product, Token: token}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

var productRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidProduct reports whether s can be a product slug.
func ValidProduct(s string) bool { return len(s) <= 60 && productRE.MatchString(s) }

var tokenRE = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

// ValidToken reports whether s looks like an API token (letters, digits, _ and -).
func ValidToken(s string) bool { return tokenRE.MatchString(s) }

func (c *Client) get(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	return resp, nil
}

// apiError turns a non-2xx answer into an error the operator can act on.
func apiError(resp *http.Response) error {
	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	_ = json.Unmarshal(b, &body)
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrBadToken
	case resp.StatusCode == http.StatusTooManyRequests:
		return ErrRateLimited
	case resp.StatusCode == http.StatusNotFound && body.Error == "release_not_found":
		return ErrNoRelease
	case resp.StatusCode == http.StatusNotFound && body.Error == "product_not_found":
		return ErrNoProduct
	}
	if body.Message != "" {
		return fmt.Errorf("the update server answered %d: %s", resp.StatusCode, body.Message)
	}
	return fmt.Errorf("the update server answered %d", resp.StatusCode)
}

// Latest asks for the latest update of the product. release is the release
// the box runs; "" asks for the product's latest release.
func (c *Client) Latest(ctx context.Context, release string) (*Latest, error) {
	ctx, cancel := context.WithTimeout(ctx, metadataTimeout)
	defer cancel()
	u := c.Server + "/api/v1/updates/" + url.PathEscape(c.Product) + "/latest"
	if release != "" {
		u += "?release=" + url.QueryEscape(release)
	}
	resp, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apiError(resp)
	}
	var l Latest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&l); err != nil {
		return nil, fmt.Errorf("the update server sent an answer that could not be read: %w", err)
	}
	if l.Latest != nil {
		if err := c.checkUpdate(l.Latest); err != nil {
			return nil, err
		}
	}
	return &l, nil
}

var (
	versionRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,39}$`)
	sha256RE  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// SafeFileName reports whether name can be used as a file name inside the
// staging directory: no path parts, no leading dot, plain characters.
func SafeFileName(name string) bool {
	if name == "" || len(name) > 150 || strings.HasPrefix(name, ".") {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', strings.ContainsRune("._+-", r):
		default:
			return false
		}
	}
	return name != "MANIFEST" && name != "VERSION"
}

// checkUpdate refuses an update description that is unsafe to act on: a
// version or file name that is not plain, a checksum that is not a SHA-256,
// a size that is not believable, or a download URL that is not on the
// configured update server. The last one matters because the token is sent
// with every download: it must never be sent to another host.
func (c *Client) checkUpdate(u *Update) error {
	bad := func(why string) error {
		return fmt.Errorf("the update server sent an update that was refused: %s", why)
	}
	if !versionRE.MatchString(u.Version) {
		return bad("its version is not a plain version number")
	}
	if len(u.Files) == 0 {
		return bad("it has no files")
	}
	if len(u.Files) > 20 {
		return bad("it has too many files")
	}
	seen := map[string]bool{}
	for _, f := range u.Files {
		switch {
		case !SafeFileName(f.Name) || seen[f.Name]:
			return bad("a file name is not usable")
		case !sha256RE.MatchString(f.SHA256):
			return bad("a checksum is not a SHA-256")
		case f.Size <= 0 || f.Size > maxFileSize:
			return bad("a file size is not acceptable")
		case !c.ownURL(f.URL):
			return bad("a download address is not on the update server")
		}
		seen[f.Name] = true
	}
	return nil
}

// ownURL reports whether raw is a file URL of this client's product on this
// client's server (same scheme and host).
func (c *Client) ownURL(raw string) bool {
	base, err := url.Parse(c.Server)
	if err != nil {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	prefix := strings.TrimRight(base.Path, "/") + "/api/v1/updates/" + c.Product + "/files/"
	id, ok := strings.CutPrefix(u.Path, prefix)
	return u.Scheme == base.Scheme && u.Host == base.Host && ok && fileIDRE.MatchString(id) && path.Clean(u.Path) == u.Path
}

var fileIDRE = regexp.MustCompile(`^[0-9]{1,18}$`)

// Download fetches f into dir under its own name and checks its size and
// SHA-256 against what the server announced. A file that does not match is
// removed.
func (c *Client) Download(ctx context.Context, f File, dir string) error {
	if !SafeFileName(f.Name) || !c.ownURL(f.URL) {
		return fmt.Errorf("refusing to download %q", f.Name)
	}
	resp, err := c.get(ctx, f.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return apiError(resp)
	}
	dest := filepath.Join(dir, f.Name)
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(resp.Body, f.Size+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n != f.Size {
		err = fmt.Errorf("%s has the wrong size (%d bytes, expected %d)", f.Name, n, f.Size)
	}
	if err == nil && hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
		err = fmt.Errorf("%s does not match its checksum: the download is damaged or was altered", f.Name)
	}
	if err != nil {
		os.Remove(dest)
		if ctx.Err() != nil {
			return fmt.Errorf("the download was interrupted: %w", ctx.Err())
		}
		return err
	}
	return nil
}
