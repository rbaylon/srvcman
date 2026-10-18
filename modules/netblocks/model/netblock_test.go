package netblocksmodel

import (
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	good := map[string]string{
		"192.168.1.0/24":   "192.168.1.0/24",
		" 192.168.1.0/24 ": "192.168.1.0/24",
		"192.168.1.77/24":  "192.168.1.0/24", // host bits cleared
		"10.0.0.1/32":      "10.0.0.1/32",
		"2001:db8::/32":    "2001:db8::/32",
		"2001:DB8::1/64":   "2001:db8::/64",
	}
	for in, want := range good {
		got, err := Normalize(in)
		if err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "  ", "192.168.1.0", "192.168.1.0/33", "192.168.1/24", "abc",
		"0.0.0.0/0", "::/0", "192.168.1.0/24; rm -rf /", "192.168.1.0/24\n10.0.0.0/8", "$(id)/24", "1.2.3.4/24 5.6.7.8/24"} {
		if got, err := Normalize(in); err == nil {
			t.Errorf("Normalize(%q) = %q, want an error", in, got)
		}
	}
}

func TestBindNormalizes(t *testing.T) {
	n := &Netblock{Cidr: "172.16.5.9/16"}
	if err := n.Bind(&http.Request{}); err != nil || n.Cidr != "172.16.0.0/16" {
		t.Errorf("Bind = %v, Cidr = %q; want nil and the canonical block", err, n.Cidr)
	}
}

// The scripts are shell text with the block pasted in, so only a plain
// CIDR may ever be accepted.
func TestScriptsRefuseAnythingButACidr(t *testing.T) {
	for _, bad := range []string{"", "1.2.3.4", "1.2.3.4/24; reboot", "1.2.3.4/24' ; id ; '", "$(id)", "1.2.3.4/24\n", "a b/24"} {
		if _, err := appendScript("/tmp/x", bad); err == nil {
			t.Errorf("appendScript accepted %q", bad)
		}
		if _, err := removeScript("/tmp/x", bad); err == nil {
			t.Errorf("removeScript accepted %q", bad)
		}
	}
}

// runSh executes a generated script with the system shell against a file in
// a temp dir, so the scripts' real behaviour is tested, not just their text.
func runSh(t *testing.T, script string) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this machine")
	}
	if out, err := exec.Command(sh, "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("script failed: %v\n%s\n--- script:\n%s", err, out, script)
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n"), "\n")
}

func add(t *testing.T, path, cidr string) {
	t.Helper()
	s, err := appendScript(path, cidr)
	if err != nil {
		t.Fatal(err)
	}
	runSh(t, s)
}

func remove(t *testing.T, path, cidr string) {
	t.Helper()
	s, err := removeScript(path, cidr)
	if err != nil {
		t.Fatal(err)
	}
	runSh(t, s)
}

func TestAppendKeepsExistingLinesAndFixesMissingNewline(t *testing.T) {
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "fast.com.blocks"))
	// Like the shipped file: entries, and no newline after the last one.
	if err := os.WriteFile(path, []byte("202.138.160.0/19\n1.37.0.0/16"), 0o644); err != nil {
		t.Fatal(err)
	}
	add(t, path, "192.168.1.0/24")
	got := readLines(t, path)
	want := []string{"202.138.160.0/19", "1.37.0.0/16", "192.168.1.0/24"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("file = %v, want %v (new block must not be glued onto the last line)", got, want)
	}
}

func TestAppendIsIdempotentAndCreatesTheFile(t *testing.T) {
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "new.blocks"))
	add(t, path, "10.0.0.0/8")
	add(t, path, "10.0.0.0/8")
	add(t, path, "10.1.0.0/16")
	got := strings.Join(readLines(t, path), "|")
	if got != "10.0.0.0/8|10.1.0.0/16" {
		t.Errorf("file = %q, want each block once", got)
	}
}

// 10.0.0.0/8 must not be mistaken for a longer line that contains it.
func TestRemoveMatchesWholeLinesOnly(t *testing.T) {
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "f.blocks"))
	if err := os.WriteFile(path, []byte("110.0.0.0/8\n10.0.0.0/8\n10.0.0.0/80\n192.0.2.0/24"), 0o644); err != nil {
		t.Fatal(err)
	}
	remove(t, path, "10.0.0.0/8")
	got := strings.Join(readLines(t, path), "|")
	if got != "110.0.0.0/8|10.0.0.0/80|192.0.2.0/24" {
		t.Errorf("file = %q, want only the exact line removed", got)
	}
}

func TestRemoveLastRemainingLineAndMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "one.blocks"))
	if err := os.WriteFile(path, []byte("10.0.0.0/8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	remove(t, path, "10.0.0.0/8") // grep prints nothing and exits 1: not an error
	if b, _ := os.ReadFile(path); len(b) != 0 {
		t.Errorf("file = %q, want empty", b)
	}
	remove(t, filepath.ToSlash(filepath.Join(dir, "absent.blocks")), "10.0.0.0/8") // nothing to do
}

func TestFileFailureSurfaces(t *testing.T) {
	old := runScript
	defer func() { runScript = old }()
	runScript = func(name, script string) error { return errors.New("arkgated unreachable") }
	if err := AddToFile("10.0.0.0/8"); err == nil {
		t.Error("AddToFile hid an arkgated failure")
	}
	if err := RemoveFromFile("10.0.0.0/8"); err == nil {
		t.Error("RemoveFromFile hid an arkgated failure")
	}
}
