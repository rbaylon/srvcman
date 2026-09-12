package staticroutesmodel

import (
	"fmt"

	Acmd "github.com/rbaylon/arkgatecmd"
	"github.com/rbaylon/srvcman/utils"
)

// routeBin is the OpenBSD route(8) binary, invoked via arkgated (see
// sendArkCmd) since modifying the routing table needs privileges srvcman's
// own process doesn't have.
const routeBin = "/sbin/route"

// sendArkCmd hands a command to arkgated over its mTLS IPC connection for
// privileged execution - the same mechanism gatewaymonitormodel and
// modules/changes use to run route(8)/pfctl/mv as root. srvcman itself
// never execs route(8) directly.
func sendArkCmd(name, binpath string, opts []string) error {
	cmd := Acmd.PrepareCmd(name, binpath, opts)
	return cmd.SendCmd(utils.GetArkConn())
}

// destFlag returns the route(8) destination-type flag for routeType
// ("net" -> "-net", anything else -> "-host").
func destFlag(routeType string) string {
	if routeType == "net" {
		return "-net"
	}
	return "-host"
}

// applyRoute installs a labeled OpenBSD route: `route add -host|-net
// <destination> <gateway> -label <name>`.
func applyRoute(name, routeType, destination, gateway string) error {
	if destination == "" || gateway == "" {
		return nil
	}
	flag := destFlag(routeType)
	opts := []string{"add", flag, destination, gateway, "-label", name}
	if err := sendArkCmd("AddRoute: "+name, routeBin, opts); err != nil {
		return fmt.Errorf("route add %s %s %s -label %s: %w", flag, destination, gateway, name, err)
	}
	return nil
}

// removeRoute removes the route to destination, if any.
func removeRoute(routeType, destination string) error {
	if destination == "" {
		return nil
	}
	flag := destFlag(routeType)
	opts := []string{"delete", flag, destination}
	if err := sendArkCmd("DeleteRoute: "+destination, routeBin, opts); err != nil {
		return fmt.Errorf("route delete %s %s: %w", flag, destination, err)
	}
	return nil
}
