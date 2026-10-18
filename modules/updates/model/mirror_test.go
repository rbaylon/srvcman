package updatesmodel

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateMirror(t *testing.T) {
	good := map[string]string{
		"https://cdn.openbsd.org/pub/OpenBSD":        "https://cdn.openbsd.org/pub/OpenBSD",
		"https://ftp.jaist.ac.jp/pub/OpenBSD/":       "https://ftp.jaist.ac.jp/pub/OpenBSD",
		"  http://mirror.example.net:8080/OpenBSD  ": "http://mirror.example.net:8080/OpenBSD",
	}
	for in, want := range good {
		got, err := ValidateMirror(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{"", "cdn.openbsd.org", "ftp://x.org/pub", "https://", "https://user:pw@x.org/p", "https://x.org/p?a=1",
		"https://x.org/a b", "https://x.org/$(id)", "https://x.org/a;rm", "https://x.org/../etc", "https://localhost/p",
		"https://x.org:99999999/p", "https://-x.org/p", "https://x.org/'", "https://x.org/a\nb", strings.Repeat("a", 300)}
	for _, in := range bad {
		if got, err := ValidateMirror(in); err == nil {
			t.Errorf("%q accepted as %q", in, got)
		}
	}
}

func TestSetMirrorSendsTheAddressAsAnArgument(t *testing.T) {
	var gotName, gotBin string
	var gotOpts []string
	old := mirrorRun
	defer func() { mirrorRun = old }()
	mirrorRun = func(name, bin string, opts []string) (string, error) {
		gotName, gotBin, gotOpts = name, bin, opts
		return "", nil
	}
	v, err := SetMirror("https://ftp.jaist.ac.jp/pub/OpenBSD/")
	if err != nil || v != "https://ftp.jaist.ac.jp/pub/OpenBSD" {
		t.Fatalf("got %q, %v", v, err)
	}
	if gotName != "MirrorSet" || gotBin != "/bin/sh" || len(gotOpts) != 3 || gotOpts[0] != "-c" || gotOpts[2] != v {
		t.Errorf("command: %s %s %q", gotName, gotBin, gotOpts)
	}
	if strings.Contains(gotOpts[1], "jaist") {
		t.Error("the address must not be part of the script text")
	}
}

func TestSetMirrorRefusedMirrorIsNotInstalledAndSaysWhy(t *testing.T) {
	old := mirrorRun
	defer func() { mirrorRun = old }()
	mirrorRun = func(name, bin string, opts []string) (string, error) {
		return "the mirror did not answer\n", errors.New("exit 3")
	}
	_, err := SetMirror("https://dead.example.org/pub/OpenBSD")
	if err == nil || !strings.Contains(err.Error(), "did not answer") || !strings.Contains(err.Error(), "not changed") {
		t.Fatalf("err = %v", err)
	}
	if _, err := SetMirror("not a url"); err == nil {
		t.Error("an invalid address reached the gateway")
	}
}

func TestGetMirror(t *testing.T) {
	old := mirrorRun
	defer func() { mirrorRun = old }()
	mirrorRun = func(string, string, []string) (string, error) { return "https://cdn.openbsd.org/pub/OpenBSD\n", nil }
	if v, _ := GetMirror(); v != "https://cdn.openbsd.org/pub/OpenBSD" {
		t.Errorf("got %q", v)
	}
	mirrorRun = func(string, string, []string) (string, error) { return "", errors.New("exit 1") }
	if v, err := GetMirror(); v != "" || err != nil {
		t.Errorf("missing file: %q, %v", v, err)
	}
}
