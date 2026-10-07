package updatesmodel

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateServer(t *testing.T) {
	good := []string{"", "https://updates.example.com", "http://10.0.0.5:8080/base", "https://u.example.com/a/b?c=d"}
	for _, v := range good {
		if err := ValidateServer(v); err != nil {
			t.Errorf("ValidateServer(%q) = %v, want nil", v, err)
		}
	}
	bad := map[string]string{
		"ftp://x.example.com":                           "scheme",
		"updates.example.com":                           "no scheme",
		"https://":                                      "no host",
		"https://u:p@x.example.com":                     "credentials",
		"https://x.example.com/a b":                     "space",
		"https://x.example.com/\x00":                    "control char",
		"https://x.example.com/\r\nX":                   "newline",
		"https://" + strings.Repeat("a", 2048) + ".com": "too long",
	}
	for v, why := range bad {
		if err := ValidateServer(v); err == nil {
			t.Errorf("ValidateServer(%q) = nil, want an error (%s)", v, why)
		}
	}
}

func TestParsePatches(t *testing.T) {
	p := parsePatches("001_sndio\n\n  002_vmm  \n")
	if len(p.Available) != 2 || p.Available[0] != "001_sndio" || p.Available[1] != "002_vmm" {
		t.Errorf("Available = %v", p.Available)
	}
	if p := parsePatches(""); p.Available == nil || len(p.Available) != 0 {
		t.Errorf("empty output: Available = %#v, want a non-nil empty list (JSON [] not null)", p.Available)
	}
}

// resetCheck puts the package-level check back to idle and restores
// runCheck when the test ends.
func resetCheck(t *testing.T) {
	t.Helper()
	old := runCheck
	t.Cleanup(func() {
		runCheck = old
		check.Lock()
		check.state = CheckState{State: StateIdle, Available: []string{}}
		check.Unlock()
	})
	check.Lock()
	check.state = CheckState{State: StateIdle, Available: []string{}}
	check.Unlock()
}

func waitFor(t *testing.T, state string) CheckState {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s := CheckStatus(); s.State == state {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("check never reached state %q, now %q", state, CheckStatus().State)
	return CheckState{}
}

// The caller gets its answer straight away while the check keeps running,
// and a second start does not launch a second check.
func TestStartCheckRunsInBackgroundAndIsSingleFlight(t *testing.T) {
	resetCheck(t)
	release := make(chan struct{})
	calls := 0
	runCheck = func() (*Patches, error) {
		calls++
		<-release
		return &Patches{Available: []string{"001_x"}, Output: "001_x"}, nil
	}

	s, started := StartCheck()
	if !started || s.State != StateRunning || s.StartedAt == nil {
		t.Fatalf("first StartCheck = %+v started=%v, want running and started", s, started)
	}
	if s2, started2 := StartCheck(); started2 || s2.State != StateRunning {
		t.Errorf("second StartCheck = %+v started=%v, want it to leave the running check alone", s2, started2)
	}

	close(release)
	done := waitFor(t, StateDone)
	if calls != 1 {
		t.Errorf("runCheck called %d times, want 1", calls)
	}
	if len(done.Available) != 1 || done.Available[0] != "001_x" || done.FinishedAt == nil {
		t.Errorf("done state = %+v", done)
	}
}

func TestStartCheckRecordsFailureWithSyspatchOutput(t *testing.T) {
	resetCheck(t)
	runCheck = func() (*Patches, error) {
		return &Patches{Available: []string{}, Output: "syspatch: Error retrieving mirror"}, errors.New("exit status 1")
	}
	StartCheck()
	s := waitFor(t, StateFailed)
	if s.Error != "syspatch: Error retrieving mirror" {
		t.Errorf("Error = %q, want syspatch's own output", s.Error)
	}
}

// A crashing job must not leave the state stuck on running, or no later
// check could ever start.
func TestStartCheckSurvivesAPanic(t *testing.T) {
	resetCheck(t)
	runCheck = func() (*Patches, error) { panic("boom") }
	StartCheck()
	s := waitFor(t, StateFailed)
	if !strings.Contains(s.Error, "boom") {
		t.Errorf("Error = %q, want the panic reported", s.Error)
	}
	runCheck = func() (*Patches, error) { return &Patches{Available: []string{}}, nil }
	if _, started := StartCheck(); !started {
		t.Error("a new check could not start after a failed one")
	}
}
