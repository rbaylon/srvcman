// Package powermodel - restart or shut down the gateway
package powermodel

import (
	"fmt"
	"time"

	Acmd "github.com/rbaylon/arkgatecmd"
	"github.com/rbaylon/srvcman/utils"
)

// Action is something that can be done to the system's power state.
type Action string

const (
	Restart  Action = "restart"
	Shutdown Action = "shutdown"
)

const (
	shBin = "/bin/sh"

	// delaySeconds is how long the command waits before acting. The request
	// that asked for it has to be answered first: arkgated's reply, srvcman's
	// reply and the browser's page all travel over services that are about to
	// stop, and a restart that happens mid-answer leaves the operator staring
	// at a connection error with no idea whether it worked.
	delaySeconds = 3

	// arkTimeout bounds the call to arkgated; the command itself returns at
	// once because the delayed action is started in the background.
	arkTimeout = 20 * time.Second
)

// Valid reports whether a is one of the supported actions.
func Valid(a Action) bool { return a == Restart || a == Shutdown }

// script is the shell text that performs a after a short delay, detached
// from arkgated: its output is discarded and stdin closed so nothing keeps
// arkgated's command runner waiting on the pipes, and it is backgrounded so
// the runner sees the shell exit - and replies - immediately. shutdown(8)
// rather than reboot(8)/halt(8): it takes the system down through init, so
// rc.shutdown runs and services are stopped in order.
func script(a Action) (string, error) {
	var flag string
	switch a {
	case Restart:
		flag = "-r"
	case Shutdown:
		flag = "-p" // halt and power off
	default:
		return "", fmt.Errorf("unsupported power action %q", a)
	}
	return fmt.Sprintf("(sleep %d; /sbin/shutdown %s now) >/dev/null 2>&1 </dev/null &", delaySeconds, flag), nil
}

// Run asks arkgated to perform a. It returns once the command has been
// scheduled, not when the system is down.
func Run(a Action) error {
	s, err := script(a)
	if err != nil {
		return err
	}
	return runScript("Power"+string(a), s)
}

// runScript hands text to arkgated to run as root on the gateway - the
// machine being restarted is arkgated's host, which may not be srvcman's. A
// var so tests can stand in for arkgated.
var runScript = func(name, text string) error {
	conn := utils.GetArkConn()
	if conn == nil {
		return fmt.Errorf("arkgated unreachable")
	}
	conn.SetDeadline(time.Now().Add(arkTimeout))
	return Acmd.PrepareCmd(name, shBin, []string{"-c", text}).SendCmd(conn)
}

// Perform carries out a only if confirm names it. The API repeats the
// confirmation that the UI asks for because it is the one that actually
// restarts the machine: a script, a mistyped URL or a replayed request must
// not be able to do it without saying, in the body, what it means to do. It
// returns the HTTP status the request deserves along with any error.
func Perform(a Action, confirm string, run func(Action) error) (int, error) {
	if !Valid(a) {
		return 400, fmt.Errorf("unsupported power action %q", a)
	}
	if confirm != string(a) {
		return 400, fmt.Errorf("confirmation required: send {\"confirm\": %q} to %s the system", string(a), string(a))
	}
	if err := run(a); err != nil {
		return 500, err
	}
	return 200, nil
}
