package updatesmodel

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests run the real specz-update.sh with sh, with stand-ins for the OpenBSD tools it
// calls (syspatch, stat -f, sha256 -q) put first on PATH. They cannot prove it works on
// OpenBSD, but they do run every branch of the script's logic.

type runnerEnv struct {
	t     *testing.T
	base  string // the updates dir, with forward slashes
	shims string
	stage string
	job   string
}

func slash(p string) string { return filepath.ToSlash(p) }

func newRunnerEnv(t *testing.T) *runnerEnv {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this machine")
	}
	_ = sh
	_, haveSha256 := exec.LookPath("sha256") // OpenBSD's own: used as it is
	_, haveSha256sum := exec.LookPath("sha256sum")
	if haveSha256 != nil && haveSha256sum != nil {
		t.Skip("no sha256 or sha256sum on this machine")
	}
	e := &runnerEnv{t: t, base: slash(t.TempDir()), shims: slash(t.TempDir()), job: "20261013T112319Z-1a2b3c4d"}
	e.stage = e.base + "/staging/" + e.job
	for _, d := range []string{e.stage, e.base + "/jobs/" + e.job} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(e.base+"/specz-update.sh", []byte(RunnerScript()), 0o755); err != nil {
		t.Fatal(err)
	}
	e.shim("syspatch", `#!/bin/sh
echo "syspatch $*" >> "$SHIM_DIR/calls"
rounds=$(cat "$SHIM_DIR/rounds" 2>/dev/null || echo 0)
case "$1" in
-c)
	[ -f "$SHIM_DIR/check_fails" ] && { echo "syspatch: cannot reach the mirror" >&2; exit 1; }
	want=$(cat "$SHIM_DIR/patch_rounds" 2>/dev/null || echo 0)
	if [ "$rounds" -lt "$want" ]; then echo "001_fake"; echo "002_fake"; fi
	exit 0 ;;
*)
	rounds=$((rounds + 1)); echo "$rounds" > "$SHIM_DIR/rounds"
	echo "Installing patch 001_fake"
	[ -f "$SHIM_DIR/apply_fails" ] && { echo "syspatch: install failed" >&2; exit 1; }
	[ -f "$SHIM_DIR/kernel_patch" ] && echo 2000 > "$SHIM_DIR/bsd_mtime"
	exit 0 ;;
esac
`)
	e.shim("rdate", `#!/bin/sh
echo "rdate $*" >> "$SHIM_DIR/calls"
[ -f "$SHIM_DIR/rdate_fails" ] && { echo "rdate: $2: no answer" >&2; exit 1; }
[ -f "$SHIM_DIR/rdate_first_fails" ] && [ "$2" = "time.cloudflare.com" ] && exit 1
date
exit 0
`)
	e.shim("stat", `#!/bin/sh
# only "stat -f %m /bsd" is used
cat "$SHIM_DIR/bsd_mtime" 2>/dev/null || echo 1000
`)
	if haveSha256 != nil {
		// Not OpenBSD: stand in for "sha256 -q FILE".
		e.shim("sha256", `#!/bin/sh
sha256sum "$2" | cut -d' ' -f1
`)
	}
	if runtime.GOOS == "windows" {
		// GNU tar on Windows reads "C:/dir" as host:path; the runner is not changed for that.
		if tar, err := exec.LookPath("tar"); err == nil {
			e.shim("tar", "#!/bin/sh\nexec '"+slash(tar)+"' --force-local \"$@\"\n")
		}
	}
	return e
}

func (e *runnerEnv) shim(name, body string) {
	e.t.Helper()
	if err := os.WriteFile(e.shims+"/"+name, []byte(body), 0o755); err != nil {
		e.t.Fatal(err)
	}
}

func (e *runnerEnv) flag(name string)   { os.WriteFile(e.shims+"/"+name, []byte("1"), 0o644) }
func (e *runnerEnv) set(name, v string) { os.WriteFile(e.shims+"/"+name, []byte(v), 0o644) }

// stageFiles puts files in the staging dir with a matching MANIFEST and VERSION.
func (e *runnerEnv) stageFiles(version string, files map[string][]byte) {
	e.t.Helper()
	var manifest strings.Builder
	for name, b := range files {
		if err := os.WriteFile(e.stage+"/"+name, b, 0o600); err != nil {
			e.t.Fatal(err)
		}
		manifest.WriteString(sum(string(b)) + "  " + name + "\n")
	}
	os.WriteFile(e.stage+"/MANIFEST", []byte(manifest.String()), 0o600)
	os.WriteFile(e.stage+"/VERSION", []byte(version+"\n"), 0o600)
}

// run executes the runner; it returns its exit status.
func (e *runnerEnv) run(args ...string) int {
	e.t.Helper()
	if len(args) == 0 {
		args = []string{e.job, e.stage}
	}
	cmd := exec.Command("sh", append([]string{e.base + "/specz-update.sh"}, args...)...)
	cmd.Env = append(os.Environ(), "PATH="+e.shims+string(os.PathListSeparator)+os.Getenv("PATH"), "SHIM_DIR="+e.shims)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0
	}
	if x, ok := err.(*exec.ExitError); ok {
		e.t.Logf("runner output: %s", out)
		return x.ExitCode()
	}
	e.t.Fatalf("running sh: %v", err)
	return -1
}

func (e *runnerEnv) status() map[string]string {
	e.t.Helper()
	st, ok := readStatus(e.base + "/jobs/" + e.job + "/status")
	if !ok {
		e.t.Fatal("the runner wrote no status")
	}
	return st
}

func (e *runnerEnv) log() string {
	b, _ := os.ReadFile(e.base + "/jobs/" + e.job + "/log")
	return string(b)
}

func (e *runnerEnv) calls() string { b, _ := os.ReadFile(e.shims + "/calls"); return string(b) }

func (e *runnerEnv) exists(rel string) bool { _, err := os.Stat(e.base + "/" + rel); return err == nil }

// tarball builds a .tar.gz with the given files (name -> content, mode 0755 for *.sh).
func tarball(t *testing.T, files [][2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		mode := int64(0o644)
		if strings.HasSuffix(f[0], ".sh") {
			mode = 0o755
		}
		if err := tw.WriteHeader(&tar.Header{Name: f[0], Mode: mode, Size: int64(len(f[1]))}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(f[1]))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

const markerScript = `#!/bin/sh
echo "installing $SPECZ_UPDATE_VERSION in job $SPECZ_UPDATE_JOB from $(pwd)"
echo "$SPECZ_UPDATE_VERSION" > "$SPECZ_UPDATES_DIR/installed-marker"
echo "payload: $(cat payload.txt 2>/dev/null)"
`

func TestRunnerHappyPath(t *testing.T) {
	e := newRunnerEnv(t)
	e.set("patch_rounds", "2") // syspatch has patches for two rounds, then none
	pkg := tarball(t, [][2]string{{"install.sh", markerScript}, {"payload.txt", "new binaries"}})
	e.stageFiles("1.0.1", map[string][]byte{"update-1.0.1.tgz": pkg, "NOTES.txt": []byte("hello")})

	if code := e.run(); code != 0 {
		t.Fatalf("exit %d\nlog:\n%s", code, e.log())
	}
	st := e.status()
	if st["state"] != "success" || st["step"] != "done" || st["reboot_pending"] != "0" || st["message"] != "The update was installed." || st["updated"] == "" || st["pid"] == "" {
		t.Fatalf("status %v", st)
	}
	// The OS was patched (twice: two rounds had patches) before the install script ran.
	calls := e.calls()
	if strings.Count(calls, "syspatch -c") != 3 || strings.Count(calls, "syspatch \n")+strings.Count(calls, "syspatch\n") != 2 {
		t.Fatalf("syspatch calls:\n%s", calls)
	}
	log := e.log()
	if strings.Index(log, "syspatch round 1") > strings.Index(log, "running ") || !strings.Contains(log, "installing 1.0.1 in job "+e.job) || !strings.Contains(log, "payload: new binaries") {
		t.Fatalf("log:\n%s", log)
	}
	// The install script ran with the version, the job and the updates dir in its environment.
	if b, _ := os.ReadFile(e.base + "/installed-marker"); strings.TrimSpace(string(b)) != "1.0.1" {
		t.Fatalf("install script did not run: %q", b)
	}
	// Staged files and the unpacked package are cleaned up; the job's log and status stay.
	if e.exists("staging/"+e.job) || e.exists("jobs/"+e.job+"/pkg") || !e.exists("jobs/"+e.job+"/log") {
		t.Fatal("cleanup")
	}
}

func TestRunnerNoPatchesNeeded(t *testing.T) {
	e := newRunnerEnv(t)
	e.stageFiles("1.0.1", map[string][]byte{"install.sh": []byte(markerScript)}) // loose script, no archive
	if code := e.run(); code != 0 {
		t.Fatalf("exit %d\n%s", code, e.log())
	}
	if strings.Contains(e.calls(), "syspatch \n") || strings.Count(e.calls(), "syspatch -c") != 1 {
		t.Fatalf("calls:\n%s", e.calls())
	}
	if e.status()["state"] != "success" {
		t.Fatalf("status %v", e.status())
	}
}

func TestRunnerKernelPatchNeedsARestartButDoesNotRestart(t *testing.T) {
	e := newRunnerEnv(t)
	e.set("patch_rounds", "1")
	e.flag("kernel_patch")
	e.stageFiles("1.0.1", map[string][]byte{"install.sh": []byte(markerScript)})
	if code := e.run(); code != 0 {
		t.Fatalf("exit %d\n%s", code, e.log())
	}
	st := e.status()
	if st["state"] != "success" || st["reboot_pending"] != "1" || !strings.Contains(e.log(), "a restart is pending") {
		t.Fatalf("status %v\n%s", st, e.log())
	}
	if strings.Contains(e.calls(), "reboot") {
		t.Fatal("the runner must never restart the machine")
	}
}

func TestRunnerFailuresStopBeforeInstalling(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(*runnerEnv)
		step    string
		message string
	}{
		{"damaged file", func(e *runnerEnv) {
			e.stageFiles("1.0.1", map[string][]byte{"install.sh": []byte(markerScript)})
			os.WriteFile(e.stage+"/install.sh", []byte("#!/bin/sh\necho changed\n"), 0o600)
		}, "prepare", "does not match its checksum"},
		{"missing file", func(e *runnerEnv) {
			e.stageFiles("1.0.1", map[string][]byte{"install.sh": []byte(markerScript)})
			os.Remove(e.stage + "/install.sh")
		}, "prepare", "is missing"},
		{"no manifest", func(e *runnerEnv) {
			e.stageFiles("1.0.1", map[string][]byte{"install.sh": []byte(markerScript)})
			os.Remove(e.stage + "/MANIFEST")
		}, "prepare", "files are missing"},
		{"manifest with a path", func(e *runnerEnv) {
			e.stageFiles("1.0.1", map[string][]byte{"install.sh": []byte(markerScript)})
			os.WriteFile(e.stage+"/MANIFEST", []byte(sum("x")+"  ../../etc/passwd\n"), 0o600)
		}, "prepare", "unusable name"},
		{"syspatch cannot check", func(e *runnerEnv) {
			e.flag("check_fails")
			e.stageFiles("1.0.1", map[string][]byte{"install.sh": []byte(markerScript)})
		}, "os", "could not be checked"},
		{"syspatch cannot apply", func(e *runnerEnv) {
			e.set("patch_rounds", "1")
			e.flag("apply_fails")
			e.stageFiles("1.0.1", map[string][]byte{"install.sh": []byte(markerScript)})
		}, "os", "could not be updated"},
		{"no install script", func(e *runnerEnv) {
			e.stageFiles("1.0.1", map[string][]byte{"readme.txt": []byte("nothing to run")})
		}, "install", "no install script"},
		{"install script fails", func(e *runnerEnv) {
			e.stageFiles("1.0.1", map[string][]byte{"install.sh": []byte("#!/bin/sh\necho 'disk full' >&2\nexit 3\n")})
		}, "install", "install script failed"},
		{"unsafe archive", func(e *runnerEnv) {
			e.stageFiles("1.0.1", map[string][]byte{"u.tgz": tarball(t, [][2]string{{"../escape.sh", "x"}, {"install.sh", markerScript}})})
		}, "install", "unsafe file names|could not be unpacked"},
		{"absolute path in archive", func(e *runnerEnv) {
			e.stageFiles("1.0.1", map[string][]byte{"u.tgz": tarball(t, [][2]string{{"/tmp/specz-escape.sh", "x"}})})
		}, "install", "unsafe file names|no install script"},
		{"broken archive", func(e *runnerEnv) {
			e.stageFiles("1.0.1", map[string][]byte{"u.tgz": []byte("this is not a tarball")})
		}, "install", "could not be unpacked"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newRunnerEnv(t)
			c.prepare(e)
			if code := e.run(); code != 1 {
				t.Fatalf("exit %d, want 1\n%s", code, e.log())
			}
			st := e.status()
			// c.message may list alternatives ("a|b"): the script's own check names unsafe
			// archives, but OpenBSD's tar neutralises such names itself, so there the failure
			// shows up one step later. What matters, checked below, is that nothing escaped.
			matched := false
			for _, alt := range strings.Split(c.message, "|") {
				matched = matched || strings.Contains(st["message"], alt)
			}
			if st["state"] != "failed" || st["step"] != c.step || !matched {
				t.Fatalf("status %v, want failed at %s with %q", st, c.step, c.message)
			}
			for _, escaped := range []string{e.base + "/jobs/escape.sh", e.base + "/jobs/" + e.job + "/escape.sh", "/tmp/specz-escape.sh"} {
				if _, err := os.Stat(escaped); err == nil {
					os.Remove(escaped)
					t.Fatalf("a file from the archive escaped to %s", escaped)
				}
			}
			if strings.Contains(st["message"], "\n") {
				t.Fatal("the message must be one line")
			}
			if e.exists("installed-marker") {
				t.Fatal("an install script ran after a failure")
			}
			// Nothing is deleted on failure: the files stay for looking into.
			if !e.exists("staging/" + e.job) {
				t.Fatal("staging removed after a failure")
			}
		})
	}
}

// A failure in the OS step must not leave the install script running afterwards, and the checksum
// check comes first: nothing touches the OS when the files are not what was announced.
func TestRunnerChecksFilesBeforeTouchingTheOS(t *testing.T) {
	e := newRunnerEnv(t)
	e.set("patch_rounds", "1")
	e.stageFiles("1.0.1", map[string][]byte{"install.sh": []byte(markerScript)})
	os.WriteFile(e.stage+"/install.sh", []byte("tampered"), 0o600)
	if code := e.run(); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if e.calls() != "" {
		t.Fatalf("syspatch was called although the files were damaged:\n%s", e.calls())
	}
}

func TestRunnerPatchesAtMostThreeRounds(t *testing.T) {
	e := newRunnerEnv(t)
	e.set("patch_rounds", "99") // syspatch always has something new
	e.stageFiles("1.0.1", map[string][]byte{"install.sh": []byte(markerScript)})
	if code := e.run(); code != 0 {
		t.Fatalf("exit %d\n%s", code, e.log())
	}
	if n := strings.Count(e.calls(), "syspatch -c"); n != 3 {
		t.Fatalf("%d check rounds, want 3", n)
	}
}

func TestRunnerPackageInASubdirectory(t *testing.T) {
	e := newRunnerEnv(t)
	pkg := tarball(t, [][2]string{{"specz-1.0.1/install.sh", markerScript}, {"specz-1.0.1/payload.txt", "nested"}})
	e.stageFiles("1.0.1", map[string][]byte{"u.tar.gz": pkg})
	if code := e.run(); code != 0 {
		t.Fatalf("exit %d\n%s", code, e.log())
	}
	if !strings.Contains(e.log(), "payload: nested") {
		t.Fatalf("log:\n%s", e.log())
	}
}

func TestRunnerRefusesBadArguments(t *testing.T) {
	e := newRunnerEnv(t)
	e.stageFiles("1.0.1", map[string][]byte{"install.sh": []byte(markerScript)})
	for name, args := range map[string][]string{
		"no arguments":             {"x"},
		"job id with a slash":      {"../x", e.stage},
		"job id with a space":      {"a b", e.stage},
		"staging outside the base": {e.job, "/tmp/other"},
		"staging with ..":          {e.job, e.base + "/staging/../../etc"},
		"staging is the base":      {e.job, e.base},
	} {
		if code := e.run(args...); code != 64 {
			t.Errorf("%s: exit %d, want 64", name, code)
		}
	}
	if e.calls() != "" || e.exists("installed-marker") {
		t.Fatal("something ran for a refused call")
	}
	// A job whose directory srvcman did not make is refused too.
	os.RemoveAll(e.base + "/jobs/" + e.job)
	if code := e.run(); code != 65 {
		t.Errorf("no job dir: exit %d, want 65", code)
	}
}

// The runner finds its own directory from where it was started, so a custom UPDATES_DIR works.
func TestRunnerBaseComesFromItsOwnLocation(t *testing.T) {
	e := newRunnerEnv(t)
	e.stageFiles("1.0.1", map[string][]byte{"install.sh": []byte(markerScript)})
	if code := e.run(); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if b, _ := os.ReadFile(e.base + "/installed-marker"); strings.TrimSpace(string(b)) != "1.0.1" {
		t.Fatalf("marker %q", b)
	}
	_ = fmt.Sprint
}

// The clock is set before anything is fetched, and a server that does not answer
// is skipped.
func TestRunnerSetsTheClockBeforeCheckingTheSystem(t *testing.T) {
	e := newRunnerEnv(t)
	e.stageFiles("1.0.1", map[string][]byte{"install.sh": []byte("#!/bin/sh\nexit 0\n")})
	e.flag("rdate_first_fails")
	if code := e.run(); code != 0 {
		t.Fatalf("exit %d", code)
	}
	calls, _ := os.ReadFile(e.shims + "/calls")
	c := string(calls)
	first := strings.Index(c, "rdate -n time.cloudflare.com")
	second := strings.Index(c, "rdate -n time.google.com")
	sys := strings.Index(c, "syspatch -c")
	if first < 0 || second < first || sys < second {
		t.Errorf("want rdate (first server, then the next) before syspatch, got:\n%s", c)
	}
}

// No time server answering must not stop the update: syspatch is the judge.
func TestRunnerCarriesOnWhenNoTimeServerAnswers(t *testing.T) {
	e := newRunnerEnv(t)
	e.stageFiles("1.0.1", map[string][]byte{"install.sh": []byte("#!/bin/sh\nexit 0\n")})
	e.flag("rdate_fails")
	if code := e.run(); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if st := e.status(); st["state"] != "success" {
		t.Errorf("state %q, want success", st["state"])
	}
}
