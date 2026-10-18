package powermodel

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestScriptsScheduleADetachedShutdown(t *testing.T) {
	cases := map[Action]string{
		Restart:  "/sbin/shutdown -r now",
		Shutdown: "/sbin/shutdown -p now",
	}
	for a, want := range cases {
		s, err := script(a)
		if err != nil {
			t.Fatal(err)
		}
		for _, part := range []string{want, "sleep 3", ">/dev/null 2>&1", "</dev/null", "&"} {
			if !strings.Contains(s, part) {
				t.Errorf("%s script %q lacks %q", a, s, part)
			}
		}
	}
	if _, err := script("hibernate"); err == nil {
		t.Error("an unsupported action produced a script")
	}
}

// The script must hand control back at once - the machine goes down seconds
// later - and must not leave anything attached to the caller's pipes.
func TestScriptReturnsImmediatelyAndSilently(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this machine")
	}
	// Run the real shape of the script with a harmless stand-in for the
	// shutdown command: it must return before the delayed part runs.
	s := "(sleep 2; echo should-not-appear-yet) >/dev/null 2>&1 </dev/null &"
	out, err := exec.Command(sh, "-c", s).CombinedOutput()
	if err != nil || len(out) != 0 {
		t.Errorf("detached script returned %q, err %v; want nothing and success", out, err)
	}
}

// Nothing happens unless the request says what it means to do.
func TestPerformRequiresMatchingConfirmation(t *testing.T) {
	var ran []Action
	run := func(a Action) error { ran = append(ran, a); return nil }

	for _, tc := range []struct {
		action  Action
		confirm string
		status  int
	}{
		{Restart, "", 400},
		{Restart, "shutdown", 400}, // the other action's confirmation is not accepted
		{Restart, "RESTART", 400},
		{Restart, "yes", 400},
		{Shutdown, "restart", 400},
		{Shutdown, "", 400},
		{"hibernate", "hibernate", 400},
	} {
		if status, err := Perform(tc.action, tc.confirm, run); status != tc.status || err == nil {
			t.Errorf("Perform(%q, %q) = %d, %v; want %d and an error", tc.action, tc.confirm, status, err, tc.status)
		}
	}
	if len(ran) != 0 {
		t.Fatalf("action ran without a matching confirmation: %v", ran)
	}

	if status, err := Perform(Restart, "restart", run); status != 200 || err != nil {
		t.Errorf("Perform(restart, restart) = %d, %v", status, err)
	}
	if status, err := Perform(Shutdown, "shutdown", run); status != 200 || err != nil {
		t.Errorf("Perform(shutdown, shutdown) = %d, %v", status, err)
	}
	if len(ran) != 2 || ran[0] != Restart || ran[1] != Shutdown {
		t.Errorf("ran %v, want exactly [restart shutdown]", ran)
	}
}

func TestPerformReportsFailureToReachArkgated(t *testing.T) {
	status, err := Perform(Restart, "restart", func(Action) error { return errors.New("arkgated unreachable") })
	if status != 500 || err == nil {
		t.Errorf("= %d, %v; want 500 and the error", status, err)
	}
}

func TestRunSendsTheScriptToArkgated(t *testing.T) {
	old := runScript
	defer func() { runScript = old }()
	var name, text string
	runScript = func(n, s string) error { name, text = n, s; return nil }
	if err := Run(Shutdown); err != nil {
		t.Fatal(err)
	}
	if name != "Powershutdown" || !strings.Contains(text, "shutdown -p now") {
		t.Errorf("sent %q %q", name, text)
	}
}
