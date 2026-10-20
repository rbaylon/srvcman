// Package rcsvc - rc.d service helpers shared by the modules that configure a
// daemon (npppd, dhcpd, unbound, snmpd, the portals).
package rcsvc

import (
	"errors"
	"net"
	"strings"
	"time"

	Acmd "github.com/rbaylon/arkgatecmd"
)

// cmdTimeout bounds each rcctl call made here.
const cmdTimeout = 15 * time.Second

// Enabled reports whether `rcctl ls on` output lists the service.
func Enabled(listOn, service string) bool {
	if service == "" {
		return false
	}
	for _, line := range strings.Split(listOn, "\n") {
		if strings.TrimSpace(line) == service {
			return true
		}
	}
	return false
}

// ensure is EnsureEnabled with the gateway calls passed in.
func ensure(service string, send func(*Acmd.Arkcmd) error, output func(*Acmd.Arkcmd) (string, error)) (changed bool, err error) {
	list := Acmd.PrepareCmd("RcListOn_"+service, "/usr/sbin/rcctl", []string{"ls", "on"})
	if out, lerr := output(list); lerr == nil && Enabled(out, service) {
		return false, nil
	}
	// Not listed (or the list could not be read): enable it. rcctl enable is
	// harmless on a service that is already enabled.
	if err := send(Acmd.GetRcCmds(service)["enable"]); err != nil {
		return false, errors.New("could not enable " + service + " at boot")
	}
	return true, nil
}

// EnsureEnabled makes sure the rc.d service is enabled before it is started or
// restarted. rcctl refuses to start or restart a service that is not enabled,
// which is exactly the state of a daemon the first time it is configured, and
// enabling it also brings it back after a reboot. conn opens a connection to
// arkgated (utils.GetArkConn). It reports whether it had to enable the service.
func EnsureEnabled(service string, conn func() net.Conn) (changed bool, err error) {
	send := func(c *Acmd.Arkcmd) error {
		cn := conn()
		if cn == nil {
			return errors.New("arkgated unreachable")
		}
		return c.SendCmd(cn)
	}
	output := func(c *Acmd.Arkcmd) (string, error) {
		cn := conn()
		if cn == nil {
			return "", errors.New("arkgated unreachable")
		}
		defer cn.Close()
		cn.SetDeadline(time.Now().Add(cmdTimeout))
		return c.SendCmdOutput(cn)
	}
	return ensure(service, send, output)
}
