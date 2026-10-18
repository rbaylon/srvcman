package updatesmodel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// memStore is Store without a database.
type memStore struct {
	mu sync.Mutex
	u  Updates
}

func (m *memStore) Load() (*Updates, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.u
	return &c, nil
}

func (m *memStore) Save(u *Updates) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.u = *u
	return nil
}

// fakeRoot records what would be handed to arkgated.
type fakeRoot struct {
	mu      sync.Mutex
	err     error
	started [][3]string
}

func (r *fakeRoot) Start(script, job, stage string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.started = append(r.started, [3]string{script, job, stage})
	return nil
}

func (r *fakeRoot) count() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.started) }

type rig struct {
	m  *Manager
	st *memStore
	r  *fakeRoot
	f  *fakeSpecz
}

func newRig(t *testing.T) *rig {
	t.Helper()
	t.Setenv("RELEASE_FILE", filepath.Join(t.TempDir(), "RELEASE"))
	f := newFakeSpecz(t)
	r := &fakeRoot{}
	m := &Manager{Root: r, Dir: t.TempDir()}
	return &rig{m: m, st: &memStore{u: Updates{UpdateServer: f.URL, ApiToken: f.token}}, r: r, f: f}
}

// waitDownloaded waits until the job has left the downloading step.
func (g *rig) waitDownloaded(t *testing.T) *Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if j := g.m.JobStatus(g.st); !(j.State == JobRunning && j.Step == StepDownloading) {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the download never finished")
	return nil
}

// runnerWrites plays the root runner: it writes the status file the way specz-update.sh does.
func (g *rig) runnerWrites(t *testing.T, job string, kv map[string]string) {
	t.Helper()
	dir := filepath.Join(g.m.dir(), "jobs", job)
	var b strings.Builder
	for _, k := range []string{"state", "step", "message", "reboot_pending", "pid", "updated"} {
		if v, ok := kv[k]; ok {
			b.WriteString(k + "=" + v + "\n")
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "status"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func now() string { return itoa(time.Now().Unix()) }

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func TestCheckSoftware(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()

	// Nothing set up: not an error, just not configured.
	g.st.u = Updates{}
	s, err := g.m.CheckSoftware(ctx, g.st)
	if err != nil || s.Configured || s.Error != "" || s.Product != "speczisp" {
		t.Fatalf("unconfigured: %v %+v", err, s)
	}
	g.st.u = Updates{UpdateServer: g.f.URL} // a server but no token
	if s, _ = g.m.CheckSoftware(ctx, g.st); s.Configured {
		t.Fatal("configured without a token")
	}

	// An update is available; the box's own release is sent along.
	g.st.u = Updates{UpdateServer: g.f.URL, ApiToken: g.f.token}
	os.WriteFile(os.Getenv("RELEASE_FILE"), []byte("1.0.0\n"), 0o644)
	g.f.setLatest("1.0.1", [3]string{"7", "u.tgz", "package"})
	s, err = g.m.CheckSoftware(ctx, g.st)
	if err != nil || !s.Configured || !s.UpdateAvailable || s.Latest == nil || s.Latest.Version != "1.0.1" || s.Tier != "pro" || s.Release != "1.0.0" || s.Installed != "" {
		t.Fatalf("available: %v %+v", err, s)
	}
	if g.f.releases[len(g.f.releases)-1] != "1.0.0" {
		t.Fatalf("release sent: %q", g.f.releases)
	}
	// Already installed: nothing to do.
	g.st.u.InstalledVersion = "1.0.1"
	if s, _ = g.m.CheckSoftware(ctx, g.st); s.UpdateAvailable || s.Installed != "1.0.1" || s.Latest == nil {
		t.Fatalf("up to date: %+v", s)
	}
	// An unknown release file means no release parameter; the server's answer names it.
	os.Remove(os.Getenv("RELEASE_FILE"))
	if s, _ = g.m.CheckSoftware(ctx, g.st); s.Release != "1.0.0" || g.f.releases[len(g.f.releases)-1] != "" {
		t.Fatalf("unknown release: %+v %q", s, g.f.releases)
	}
	// A garbage release file is ignored.
	os.WriteFile(os.Getenv("RELEASE_FILE"), []byte("not a version; rm -rf /\n"), 0o644)
	if installedRelease() != "" {
		t.Fatal("a bad release file was believed")
	}
	// A failed check is reported in the answer, in words.
	g.st.u.ApiToken = "spz_revokedrevokedrevoked"
	s, err = g.m.CheckSoftware(ctx, g.st)
	if err != nil || s.Error == "" || !strings.Contains(s.Error, "API token") || s.UpdateAvailable {
		t.Fatalf("bad token: %v %+v", err, s)
	}
	// The product setting decides which product is asked for.
	g.st.u = Updates{UpdateServer: g.f.URL, ApiToken: g.f.token, Product: "other"}
	if s, _ = g.m.CheckSoftware(ctx, g.st); s.Product != "other" || s.Error == "" {
		t.Fatalf("other product: %+v", s)
	}
}

func TestInstallHappyPath(t *testing.T) {
	g := newRig(t)
	g.f.setLatest("1.0.1", [3]string{"7", "update.tgz", "package bytes"}, [3]string{"8", "notes.txt", "hello"})
	ctx := context.Background()

	j, err := g.m.StartInstall(ctx, g.st)
	if err != nil || j.State != JobRunning || j.Version != "1.0.1" {
		t.Fatalf("start: %v %+v", err, j)
	}
	j = g.waitDownloaded(t)
	if g.r.count() != 1 {
		t.Fatalf("the runner was started %d times", g.r.count())
	}
	script, job, stage := g.r.started[0][0], g.r.started[0][1], g.r.started[0][2]
	if script != filepath.Join(g.m.dir(), "specz-update.sh") || !strings.HasPrefix(stage, filepath.Join(g.m.dir(), "staging")) || !strings.HasSuffix(stage, job) {
		t.Fatalf("runner args: %q %q %q", script, job, stage)
	}
	// The runner script is on disk, executable, and is what is embedded.
	if b, _ := os.ReadFile(script); string(b) != RunnerScript() || !strings.HasPrefix(string(b), "#!/bin/sh") {
		t.Fatal("the runner script was not written")
	}
	if fi, _ := os.Stat(script); fi.Mode().Perm()&0o100 == 0 && os.PathSeparator == '/' {
		t.Fatal("the runner script is not executable")
	}
	// Staging has the verified files, the manifest and the version.
	for name, want := range map[string]string{"update.tgz": "package bytes", "notes.txt": "hello", "VERSION": "1.0.1\n",
		"MANIFEST": sum("package bytes") + "  update.tgz\n" + sum("hello") + "  notes.txt\n"} {
		if b, err := os.ReadFile(filepath.Join(stage, name)); err != nil || string(b) != want {
			t.Errorf("%s: %q %v", name, b, err)
		}
	}
	// Handed over: until the runner writes its status the job is "preparing".
	if j.State != JobRunning || j.Step != StepPreparing {
		t.Fatalf("after hand-over: %+v", j)
	}
	// A second click while it runs starts nothing new.
	if j2, err := g.m.StartInstall(ctx, g.st); err != nil || j2.State != JobRunning || g.r.count() != 1 {
		t.Fatalf("second start: %v %+v (runner started %d times)", err, j2, g.r.count())
	}

	// The runner reports progress: OS step then install step, both "running" to the operator.
	g.runnerWrites(t, job, map[string]string{"state": "running", "step": "os", "message": "Preparing the system", "pid": "1", "updated": now()})
	if j = g.m.JobStatus(g.st); j.State != JobRunning || j.Step != StepPreparing {
		t.Fatalf("os step: %+v", j)
	}
	g.runnerWrites(t, job, map[string]string{"state": "running", "step": "install", "message": "Installing the update", "pid": "1", "updated": now()})
	if j = g.m.JobStatus(g.st); j.State != JobRunning || j.Step != StepInstalling {
		t.Fatalf("install step: %+v", j)
	}
	if u, _ := g.st.Load(); u.InstalledVersion != "" {
		t.Fatal("installed before the runner finished")
	}
	// Success, with a restart pending: recorded once.
	g.runnerWrites(t, job, map[string]string{"state": "success", "step": "done", "message": "The update was installed.", "reboot_pending": "1", "pid": "1", "updated": now()})
	j = g.m.JobStatus(g.st)
	if j.State != JobSuccess || !j.RebootPending || j.FinishedAt == nil || j.Version != "1.0.1" {
		t.Fatalf("success: %+v", j)
	}
	u, _ := g.st.Load()
	if u.InstalledVersion != "1.0.1" || u.InstalledAt == nil || !u.JobRecorded {
		t.Fatalf("not recorded: %+v", u)
	}
	// It now counts as installed: nothing left to do, and a second install is refused.
	if s, _ := g.m.CheckSoftware(ctx, g.st); s.UpdateAvailable || s.Installed != "1.0.1" {
		t.Fatalf("after install: %+v", s)
	}
	if _, err := g.m.StartInstall(ctx, g.st); !errors.Is(err, ErrNothingToInstall) {
		t.Fatalf("second install: %v", err)
	}
	// A srvcman restart in the middle (a new manager, no memory) reads the same truth.
	m2 := &Manager{Root: g.r, Dir: g.m.Dir}
	if j := m2.JobStatus(g.st); j.State != JobSuccess || !j.RebootPending {
		t.Fatalf("after a restart: %+v", j)
	}
}

func TestInstallRefusals(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	g.st.u = Updates{}
	if _, err := g.m.StartInstall(ctx, g.st); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("not configured: %v", err)
	}
	g.st.u = Updates{UpdateServer: g.f.URL, ApiToken: "spz_wrongwrongwrongwrong"}
	if _, err := g.m.StartInstall(ctx, g.st); !errors.Is(err, ErrBadToken) {
		t.Fatalf("bad token: %v", err)
	}
	g.st.u = Updates{UpdateServer: g.f.URL, ApiToken: g.f.token}
	g.f.latest = `{"product":"speczisp","tier":"free","release":"1.0.0","latest":null}`
	if _, err := g.m.StartInstall(ctx, g.st); !errors.Is(err, ErrNothingToInstall) {
		t.Fatalf("nothing to install: %v", err)
	}
	g.f.setLatest("1.0.1", [3]string{"7", "u.tgz", "x"})
	g.st.u.InstalledVersion = "1.0.1"
	if _, err := g.m.StartInstall(ctx, g.st); !errors.Is(err, ErrNothingToInstall) {
		t.Fatalf("already installed: %v", err)
	}
	if g.r.count() != 0 {
		t.Fatal("the runner was started for a refused install")
	}
	if j := g.m.JobStatus(g.st); j.State != JobIdle {
		t.Fatalf("refusals must leave no job behind: %+v", j)
	}
}

func TestInstallFailuresBeforeHandOver(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	g.f.setLatest("1.0.1", [3]string{"7", "u.tgz", "package bytes"})
	g.f.files["7"] = "packagX bytes" // damaged in transit: same size, wrong checksum
	if _, err := g.m.StartInstall(ctx, g.st); err != nil {
		t.Fatal(err)
	}
	j := g.waitDownloaded(t)
	if j.State != JobFailed || !strings.Contains(j.Message, "checksum") || g.r.count() != 0 {
		t.Fatalf("damaged download: %+v (runner started %d times)", j, g.r.count())
	}
	u, _ := g.st.Load()
	if entries, _ := os.ReadDir(filepath.Join(g.m.dir(), "staging", u.JobID)); len(entries) != 0 {
		if _, err := os.Stat(filepath.Join(g.m.dir(), "staging", u.JobID)); err == nil {
			t.Fatalf("staging left behind after a failed download: %v", entries)
		}
	}
	// A failed job does not block the next attempt.
	g.f.files["7"] = "package bytes"
	if j, err := g.m.StartInstall(ctx, g.st); err != nil || j.State != JobRunning {
		t.Fatalf("retry: %v %+v", err, j)
	}
	if j = g.waitDownloaded(t); j.State != JobRunning || g.r.count() != 1 {
		t.Fatalf("retry result: %+v", j)
	}

	// arkgated refuses or is unreachable.
	g2 := newRig(t)
	g2.r.err = errors.New("arkgated unreachable")
	g2.f.setLatest("1.0.1", [3]string{"7", "u.tgz", "package bytes"})
	if _, err := g2.m.StartInstall(ctx, g2.st); err != nil {
		t.Fatal(err)
	}
	if j := g2.waitDownloaded(t); j.State != JobFailed || !strings.Contains(j.Message, "could not start the installation") && !strings.Contains(j.Message, "Could not start the installation") {
		t.Fatalf("arkgated down: %+v", j)
	}
	// The update server vanishing mid-way.
	g3 := newRig(t)
	g3.f.setLatest("1.0.1", [3]string{"7", "u.tgz", "package bytes"})
	delete(g3.f.files, "7")
	if _, err := g3.m.StartInstall(ctx, g3.st); err != nil {
		t.Fatal(err)
	}
	if j := g3.waitDownloaded(t); j.State != JobFailed || g3.r.count() != 0 {
		t.Fatalf("file gone: %+v", j)
	}
}

func TestRunnerStatusEdgeCases(t *testing.T) {
	g := newRig(t)
	ctx := context.Background()
	g.f.setLatest("1.0.1", [3]string{"7", "u.tgz", "package bytes"})
	if _, err := g.m.StartInstall(ctx, g.st); err != nil {
		t.Fatal(err)
	}
	g.waitDownloaded(t)
	u, _ := g.st.Load()
	job := u.JobID
	logPath := filepath.Join(g.m.dir(), "jobs", job, "log")

	// The runner failed: the message and the tail of its log are reported, nothing is recorded.
	var lines []string
	for i := 1; i <= 60; i++ {
		lines = append(lines, "line "+itoa(int64(i)))
	}
	os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	g.runnerWrites(t, job, map[string]string{"state": "failed", "step": "os", "message": "The system could not be updated.", "pid": "1", "updated": now()})
	j := g.m.JobStatus(g.st)
	if j.State != JobFailed || j.Message != "The system could not be updated." || !strings.HasSuffix(j.Log, "line 60") || strings.Contains(j.Log, "line 10\n") || len(strings.Split(j.Log, "\n")) != 40 {
		t.Fatalf("failed job: %+v", j)
	}
	if u, _ := g.st.Load(); u.InstalledVersion != "" {
		t.Fatal("a failed job was recorded as installed")
	}

	// Running, but the runner has not been heard from for far too long: interrupted.
	old := itoa(time.Now().Add(-2 * staleAfter).Unix())
	g.runnerWrites(t, job, map[string]string{"state": "running", "step": "install", "message": "Installing the update", "pid": "1", "updated": old})
	if j = g.m.JobStatus(g.st); j.State != JobFailed || !strings.Contains(j.Message, "interrupted") {
		t.Fatalf("stale runner: %+v", j)
	}
	// A reboot_pending flag survives to the failure too (the OS step may have done its work).
	g.runnerWrites(t, job, map[string]string{"state": "failed", "step": "install", "message": "x", "reboot_pending": "1", "pid": "1", "updated": now()})
	if j = g.m.JobStatus(g.st); !j.RebootPending {
		t.Fatal("reboot_pending lost on failure")
	}

	// No status file at all: starting up for a short while, then "did not start".
	os.Remove(filepath.Join(g.m.dir(), "jobs", job, "status"))
	if j = g.m.JobStatus(g.st); j.State != JobRunning || j.Step != StepPreparing {
		t.Fatalf("just started: %+v", j)
	}
	late := &Manager{Root: g.r, Dir: g.m.Dir, Now: func() time.Time { return time.Now().Add(2 * startGrace) }}
	if j = late.JobStatus(g.st); j.State != JobFailed || !strings.Contains(j.Message, "did not start") {
		t.Fatalf("never started: %+v", j)
	}
	// A junk status file is treated like none.
	os.WriteFile(filepath.Join(g.m.dir(), "jobs", job, "status"), []byte("garbage\n"), 0o644)
	if j = late.JobStatus(g.st); j.State != JobFailed {
		t.Fatalf("junk status: %+v", j)
	}
	// No job ever: idle.
	g.st.u.JobID = ""
	if j = g.m.JobStatus(g.st); j.State != JobIdle {
		t.Fatalf("idle: %+v", j)
	}
}

func TestOldJobsArePruned(t *testing.T) {
	g := newRig(t)
	for _, sub := range []string{"jobs", "staging"} {
		for i := 1; i <= 9; i++ {
			os.MkdirAll(filepath.Join(g.m.dir(), sub, "2026010"+itoa(int64(i))+"T000000Z-aaaa"), 0o755)
		}
	}
	g.m.prune("20260109T000000Z-aaaa") // the job just started is always the newest
	for _, sub := range []string{"jobs", "staging"} {
		entries, _ := os.ReadDir(filepath.Join(g.m.dir(), sub))
		names := map[string]bool{}
		for _, e := range entries {
			names[e.Name()] = true
		}
		if len(names) != 5 || !names["20260105T000000Z-aaaa"] || !names["20260109T000000Z-aaaa"] || names["20260104T000000Z-aaaa"] {
			t.Errorf("%s after pruning: %v", sub, names)
		}
	}
}

// The command sent to arkgated must stay small (its default buffer is 1 KiB) and must not
// contain the paths or the job id spliced into shell text.
func TestRootStartCommandShape(t *testing.T) {
	const wrapper = `nohup /bin/sh "$0" "$@" >/dev/null 2>&1 </dev/null &`
	if len(wrapper)+len("/usr/local/arkgate/updates/specz-update.sh")+len("20261013T112319Z-1a2b3c4d")+len("/usr/local/arkgate/updates/staging/20261013T112319Z-1a2b3c4d")+200 > 1024 {
		t.Fatal("the arkgated command could exceed arkgated's default 1024 byte buffer")
	}
	if strings.Contains(wrapper, "$1") || strings.Contains(wrapper, "%s") {
		t.Fatal("the wrapper must pass arguments positionally")
	}
}
