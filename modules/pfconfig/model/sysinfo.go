package pfconfigmodel

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	Acmd "github.com/rbaylon/arkgatecmd"
	"github.com/rbaylon/srvcman/utils"
)

// sysInfoCmdTimeout bounds how long GetSystemInfo waits for arkgated to run
// its system-info probe and reply.
const sysInfoCmdTimeout = 15 * time.Second

// sysInfoScript queries a handful of read-only OpenBSD host facts in one
// arkgated round trip, each on its own KEY=value line so the reply can be
// parsed without depending on any single command's exact output format.
const sysInfoScript = `echo "HOST=$(hostname)"
echo "OS=$(uname -sr)"
echo "UPTIME=$(uptime)"
echo "CPUMODEL=$(sysctl -n hw.model)"
echo "CPUCORES=$(sysctl -n hw.ncpu)"
echo "MEMBYTES=$(sysctl -n hw.physmem64)"
echo "DISKROOT=$(df -h / | tail -1)"`

// GetSystemInfo returns a human-readable snapshot of the OpenBSD host that
// pfconfig's arkgated runs on: hostname, OS version, uptime, CPU, memory,
// root disk usage. Like ListDevices/GetTrafficStats in the pfifaces
// package, this has to be dispatched to arkgated rather than gathered
// in-process - srvcmanui previously gathered this itself via gopsutil,
// which only ever reported on whatever host srvcmanui happened to be
// running on, not the actual router the pfconfig record represents.
func GetSystemInfo() (map[string]string, error) {
	conn := utils.GetArkConn()
	if conn == nil {
		return nil, fmt.Errorf("arkgated unreachable")
	}
	conn.SetDeadline(time.Now().Add(sysInfoCmdTimeout))
	cmd := Acmd.PrepareCmd("SystemInfo", "/bin/sh", []string{"-c", sysInfoScript})
	out, err := cmd.SendCmdOutput(conn)
	if err != nil {
		return nil, err
	}

	raw := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		raw[key] = strings.TrimSpace(value)
	}

	info := map[string]string{}
	if v, ok := raw["HOST"]; ok && v != "" {
		info["Host"] = v
	}
	if v, ok := raw["OS"]; ok && v != "" {
		info["OS"] = v
	}
	if v, ok := raw["UPTIME"]; ok && v != "" {
		info["Uptime"] = v
	}
	if model, ok := raw["CPUMODEL"]; ok && model != "" {
		if cores := raw["CPUCORES"]; cores != "" {
			info["CPU Model"] = model + " (" + cores + " cores)"
		} else {
			info["CPU Model"] = model
		}
	}
	if v, ok := raw["MEMBYTES"]; ok {
		if membytes, perr := strconv.ParseUint(v, 10, 64); perr == nil {
			info["Memory"] = strconv.FormatUint(membytes/1024/1024/1024, 10) + " GB"
		}
	}
	if v, ok := raw["DISKROOT"]; ok && v != "" {
		info["Disk /"] = v
	}
	return info, nil
}
