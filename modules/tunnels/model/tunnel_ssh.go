package tunnelmodel

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	Acmd "github.com/rbaylon/arkgatecmd"
	"github.com/rbaylon/srvcman/utils"
	"gorm.io/gorm"
)

const (
	sshBin  = "/usr/bin/ssh"
	shBin   = "/bin/sh"
	psBin   = "/bin/ps"
	killBin = "/bin/kill"
)

// arkCmdTimeout bounds how long a single tunnel command (ssh/ps/kill) may
// wait on arkgated.
const arkCmdTimeout = 15 * time.Second

// sendArkCmd hands a command to arkgated for privileged execution - the
// same mechanism staticroutesmodel and modules/changes use. srvcman never
// execs ssh(1) itself; the tunnel has to originate from the gateway box,
// which is arkgated's host, not srvcman's.
func sendArkCmd(name, binpath string, opts []string) error {
	cmd := Acmd.PrepareCmd(name, binpath, opts)
	return cmd.SendCmd(utils.GetArkConn())
}

// runViaArk dispatches a command to arkgated and returns its output.
func runViaArk(name, binpath string, opts []string) (string, error) {
	conn := utils.GetArkConn()
	if conn == nil {
		return "", fmt.Errorf("arkgated unreachable")
	}
	conn.SetDeadline(time.Now().Add(arkCmdTimeout))
	cmd := Acmd.PrepareCmd(name, binpath, opts)
	return cmd.SendCmdOutput(conn)
}

// forwardFlag is ssh(1)'s forwarding flag for t's type.
func forwardFlag(tunnelType string) string {
	if tunnelType == TypeLocal {
		return "-L"
	}
	return "-R"
}

// ForwardSpec builds ssh(1)'s bind:port:host:port forwarding argument.
// The two sides are ordered by type: -R listens on the remote and forwards
// back to the local side, -L listens locally and forwards out to the
// remote, so the pairs swap.
func ForwardSpec(t *Tunnel) string {
	if t.Type == TypeLocal {
		return fmt.Sprintf("%s:%d:%s:%d", t.LocalIp, t.LocalPort, t.RemoteIp, t.RemotePort)
	}
	return fmt.Sprintf("%s:%d:%s:%d", t.RemoteIp, t.RemotePort, t.LocalIp, t.LocalPort)
}

// checkSafe re-validates everything that gets interpolated into the ssh
// command line. Bind already covers the API path, but this also runs for
// rows written straight to the DB, and the command is assembled as a
// shell string (see Start) - a RemoteUser of "-oProxyCommand=..." or a
// value carrying a quote would otherwise turn into something other than
// the intended ssh invocation.
func checkSafe(t *Tunnel) error {
	if t.Type != TypeRemote && t.Type != TypeLocal {
		return fmt.Errorf("tunnel %d: type must be %q or %q", t.ID, TypeRemote, TypeLocal)
	}
	if !utils.IsAlnumStrict(t.RemoteUser) {
		return fmt.Errorf("tunnel %d: remote_user must be letters and numbers only", t.ID)
	}
	for field, ip := range map[string]string{"remote_ip": t.RemoteIp, "local_ip": t.LocalIp} {
		if net.ParseIP(ip) == nil {
			return fmt.Errorf("tunnel %d: %s %q is not a valid IP address", t.ID, field, ip)
		}
	}
	for field, port := range map[string]int{"remote_port": t.RemotePort, "local_port": t.LocalPort} {
		if port < 1 || port > 65535 {
			return fmt.Errorf("tunnel %d: %s %d out of range", t.ID, field, port)
		}
	}
	return nil
}

// Start establishes t's forward on the gateway:
//
//	ssh -o ServerAliveInterval=60 -R|-L <spec> -N -f <user>@<remote_ip>
//
// It's run through sh(1) with the standard streams redirected because
// ssh's -f backgrounds a child that inherits them: left attached to
// arkgated's output pipes, that child would hold them open for the life
// of the tunnel, and arkgated's command runner - which waits for the
// pipes to close, not just for the process to exit - would block on this
// one command forever, stalling every other privileged command behind it.
// Redirecting in the shell hands ssh /dev/null instead, so arkgated sees
// the parent exit immediately with ssh's real success/failure status.
func Start(t *Tunnel) error {
	if err := checkSafe(t); err != nil {
		return err
	}
	script := fmt.Sprintf("%s -o ServerAliveInterval=60 %s %s -N -f %s@%s >/dev/null 2>&1 </dev/null",
		sshBin, forwardFlag(t.Type), ForwardSpec(t), t.RemoteUser, t.RemoteIp)
	if err := sendArkCmd("StartTunnel: "+t.Name, shBin, []string{"-c", script}); err != nil {
		return fmt.Errorf("ssh %s %s: %w", forwardFlag(t.Type), ForwardSpec(t), err)
	}
	return nil
}

// Stop kills t's ssh process, if it's running. A tunnel that isn't
// running is not an error - that's the desired end state either way.
func Stop(t *Tunnel) error {
	procs, err := Statuses([]Tunnel{*t})
	if err != nil {
		return err
	}
	p, running := procs[t.ID]
	if !running {
		return nil
	}
	if err := sendArkCmd("StopTunnel: "+t.Name, killBin, []string{strconv.Itoa(p.Pid)}); err != nil {
		return fmt.Errorf("kill %d: %w", p.Pid, err)
	}
	return nil
}

// Proc is a running tunnel's process id and how long it's been up, as
// reported by ps(1) on the gateway.
type Proc struct {
	Pid int
	Age string
}

// Statuses reports which of tunnels are currently running, keyed by
// tunnel ID. This is one ps(1) call covering every tunnel rather than one
// per tunnel, so listing N tunnels costs a single arkgated round trip
// regardless of N.
func Statuses(tunnels []Tunnel) (map[uint]Proc, error) {
	procs := map[uint]Proc{}
	if len(tunnels) == 0 {
		return procs, nil
	}
	// -a/-x: include other users' processes and ones with no controlling
	// terminal (a backgrounded ssh -f has none). -ww: don't truncate the
	// command line - the forwarding spec we match on sits in the middle of
	// it, and a truncated line would read as "not running".
	out, err := runViaArk("TunnelPs", psBin, []string{"-axww", "-o", "pid=,etime=,command="})
	if err != nil {
		return procs, err
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		command := strings.Join(fields[2:], " ")
		if !strings.Contains(command, "ssh") {
			continue
		}
		for i := range tunnels {
			t := &tunnels[i]
			if _, seen := procs[t.ID]; seen {
				continue
			}
			if strings.Contains(command, forwardFlag(t.Type)+" "+ForwardSpec(t)) {
				procs[t.ID] = Proc{Pid: pid, Age: fields[1]}
			}
		}
	}
	return procs, nil
}

// Decorate fills in the Status/Pid/Age fields the API reports for each
// tunnel. A failure to reach arkgated leaves every tunnel reported as
// inactive (and is logged) rather than failing the whole listing - the
// records themselves are still worth showing.
func Decorate(tunnels []Tunnel) []Tunnel {
	procs, err := Statuses(tunnels)
	if err != nil {
		log.Println("tunnels: failed to read tunnel status:", err)
	}
	for i := range tunnels {
		if p, running := procs[tunnels[i].ID]; running {
			tunnels[i].Status = StatusActive
			tunnels[i].Pid = p.Pid
			tunnels[i].Age = p.Age
			continue
		}
		tunnels[i].Status = StatusInactive
	}
	return tunnels
}

// StartInactive establishes every tunnel that isn't currently running and
// reports how many succeeded/failed. This is what the list page's
// re-establish button calls: ssh forwards don't survive a gateway reboot
// (or an upstream dropping the connection), so the set of records and the
// set of live processes drift apart on their own.
func StartInactive(db *gorm.DB) (started int, failed int) {
	var tunnels []Tunnel
	if err := db.Find(&tunnels).Error; err != nil {
		log.Println("tunnels: failed to load records to start:", err)
		return 0, 0
	}
	procs, err := Statuses(tunnels)
	if err != nil {
		log.Println("tunnels: failed to read tunnel status:", err)
	}
	for i := range tunnels {
		if _, running := procs[tunnels[i].ID]; running {
			continue
		}
		if err := Start(&tunnels[i]); err != nil {
			log.Println("tunnels: failed to start tunnel", tunnels[i].Name, ":", err)
			failed++
			continue
		}
		started++
	}
	return started, failed
}
