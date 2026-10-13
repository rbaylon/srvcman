package changeapply

import (
	"fmt"
	"log"
	"strings"

	Acmd "github.com/rbaylon/arkgatecmd"
	Changemodel "github.com/rbaylon/srvcman/modules/changes/model"
)

const (
	npppdBin      = "/usr/sbin/npppd"
	npppdStaged   = "/tmp/npppd.conf"
	npppdDir      = "/etc/npppd/"
	npppdLiveConf = "/etc/npppd/npppd.conf"
)

// Calls is how Npppd reaches the gateway: Send runs a command and
// reports success, Output also returns what the command printed. A struct so
// tests can stand in for arkgated.
type Calls struct {
	Send   func(*Acmd.Arkcmd) error
	Output func(*Acmd.Arkcmd) (string, error)
}

// Npppd installs the PPPoE configuration srvcman has rendered and
// restarts npppd on it, returning the problems it met (none on success).
//
// It never restarts npppd on a configuration npppd has not accepted. Before,
// the new file was moved into place and npppd restarted blindly: a bad file
// stopped the daemon and took every PPPoE session with it, and nothing started
// it again after the old file was put back. Each step now has to succeed before
// the next, and whenever one fails the previous configuration is restored and,
// if npppd was already stopped by a failed restart, started again on it.
//
// The ordering is: set the old file aside; install the new one; have npppd
// check the installed file (npppd -n); enable the service; restart. Enabling
// comes first because rcctl refuses to start or restart a disabled service -
// enabling only after a successful restart meant that a disabled npppd could
// never get there, and a restart of a running one left it stopped. It also
// means the service comes back after a reboot. The new file is fetched from
// srvcman by arkgated as part of the first step ("BackupConf_npppd"), so if
// that fetch fails there is nothing to install and the install step fails.
func Npppd(c Calls) []Changemodel.Change {
	var errs []Changemodel.Change
	fail := func(data string) {
		errs = append(errs, Changemodel.Change{Module: "pppoes", Data: data})
	}
	cmds := Acmd.GetServiceCmds("npppd", npppdBin, npppdDir, npppdStaged)
	rc := Acmd.GetRcCmds("npppd")

	// There is no previous file the first time; failing here is expected then.
	_ = c.Send(cmds["backup"])

	if err := c.Send(cmds["stage"]); err != nil {
		fail("could not install the new npppd.conf - srvcman's configuration could not be fetched or moved into " + npppdDir)
		_ = c.Send(cmds["revert"])
		return errs
	}

	check := Acmd.PrepareCmd("CheckInstalled_npppd", npppdBin, []string{"-n", "-f", npppdLiveConf})
	if out, err := c.Output(check); err != nil {
		fail("npppd rejected the new configuration: " + oneLine(out, err) + " (the previous one was kept and npppd was not restarted)")
		_ = c.Send(cmds["revert"])
		return errs
	}

	if err := c.Send(rc["enable"]); err != nil {
		log.Println("changes: could not enable npppd at boot:", err)
	}
	if err := c.Send(rc["restart"]); err != nil {
		fail("error applying " + npppdLiveConf + " - npppd did not restart on it")
		_ = c.Send(cmds["revert"])
		// A failed restart leaves npppd stopped. Put it back on the old file.
		if err := c.Send(rc["restart"]); err != nil {
			fail("npppd is not running: it could not be restarted on the previous configuration either")
		}
		return errs
	}

	return errs
}

// oneLine reduces command output to a single line for an error message.
func oneLine(out string, err error) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return fmt.Sprint(err)
	}
	lines := strings.Split(out, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.Join(lines, "; ")
}
