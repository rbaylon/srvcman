package tunnelmodel

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	Acmd "github.com/rbaylon/arkgatecmd"
	"github.com/rbaylon/srvcman/utils"
	"github.com/rbaylon/srvcman/utils/validate"
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
	conn := utils.GetArkConn()
	// Without a deadline a stuck arkgated would hold this call - and, in turn,
	// everything waiting behind the tunnel lock - for as long as it stays
	// stuck. Long enough for ssh's own 10s connect limit plus time spent
	// queued behind other commands.
	if conn != nil {
		conn.SetDeadline(time.Now().Add(sendArkTimeout))
	}
	return cmd.SendCmd(conn)
}

// sendArkTimeout bounds one arkgated command that is not expected to return
// output.
const sendArkTimeout = 40 * time.Second

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

// SshTarget is the host ssh connects to: SshHost when set, otherwise
// RemoteIp (tunnels created before the two were split).
func SshTarget(t *Tunnel) string {
	if t.SshHost != "" {
		return t.SshHost
	}
	return t.RemoteIp
}

// validateSshHost accepts an IP address or a hostname. It ends up in a
// shell string, so anything else (including a leading '-') is rejected.
func validateSshHost(host string) error {
	if host == "" || net.ParseIP(host) != nil {
		return nil
	}
	return validate.FQDN("ssh_host", host, 253)
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
	if SshTarget(t) == "" {
		return fmt.Errorf("tunnel %d: ssh_host or remote_ip is required", t.ID)
	}
	if err := validateSshHost(t.SshHost); err != nil {
		return fmt.Errorf("tunnel %d: %w", t.ID, err)
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
//	ssh -o ServerAliveInterval=60 -o ConnectTimeout=10 -o BatchMode=yes -o ExitOnForwardFailure=yes -R|-L <spec> -N -f <user>@<ssh_host|remote_ip>
//
// ConnectTimeout and BatchMode keep a tunnel that cannot come up from
// holding arkgated's single command queue: an unreachable host would
// otherwise wait out the system's TCP timeout, and a password prompt has no
// terminal to be answered on. The tunnel watchdog retries failed tunnels, so
// a start that cannot succeed has to fail fast.
//
// ExitOnForwardFailure makes ssh exit if the forward itself cannot be set up
// - a local port already in use, or (the common one after a network drop) a
// remote port still held by the dead session. Without it ssh stays up with no
// forward, which looks like a running tunnel and is not one, and every retry
// would add another such process.
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
	if err := lockOps(); err != nil {
		return err
	}
	defer unlockOps()
	return startLocked(t)
}

// The tunnel lock serializes starting and stopping tunnels. The watchdog
// starts them in the background while an operator can press Stop at any
// moment: unserialized, a Stop can look at the process list while a start is
// half done (the real ssh not yet forked off) and either miss the tunnel or
// aim at the shell that is launching it.
//
// Waiting for it is bounded. A start can be held up behind arkgated's single
// command queue, and an operator who keeps pressing buttons meanwhile must
// get "busy" back, not a request parked indefinitely for each press.
var (
	opSem      = make(chan struct{}, 1)
	opLockWait = 45 * time.Second
	errBusy    = errors.New("another tunnel operation is in progress, try again shortly")
)

func lockOps() error {
	timer := time.NewTimer(opLockWait)
	defer timer.Stop()
	select {
	case opSem <- struct{}{}:
		return nil
	case <-timer.C:
		return errBusy
	}
}

func unlockOps() { <-opSem }

// startLocked is Start for callers that already hold the tunnel lock.
//
// It does nothing if the tunnel is already running. Every path that starts a
// tunnel funnels through here - the button, Start Inactive, an edit, the
// watchdog - and each decided to start from a status read that may be a
// moment old. Checking again under the lock is what stops two of them (an
// operator's double click racing the watchdog's tick, say) from each starting
// an ssh for the same forward.
func startLocked(t *Tunnel) error {
	if err := checkSafe(t); err != nil {
		return err
	}
	if procs, err := procStatuses([]Tunnel{*t}); err == nil {
		if _, running := procs[t.ID]; running {
			return nil
		}
	}
	if err := launchProc(t.Name, startScript(sshBin, t)); err != nil {
		return fmt.Errorf("ssh %s %s: %w", forwardFlag(t.Type), ForwardSpec(t), err)
	}
	return nil
}

// startScript is the shell text that starts t's ssh.
//
// ssh's standard output goes to /dev/null and its standard error to a
// temporary file, never to arkgated's pipes: with -f, ssh keeps running in
// the background, and anything still holding those pipes would leave
// arkgated's command runner waiting for them to close for the life of the
// tunnel. The file is what lets a failed start say why: ssh's last lines are
// printed back ("remote port forwarding failed for listen port 8000",
// "Permission denied (publickey)", "Connection timed out"...), where before
// the only answer was a bare NOK.
//
// Only a regular file is ever removed: if mktemp itself fails the output is
// discarded to /dev/null, which must not be deleted afterwards.
func startScript(ssh string, t *Tunnel) string {
	return fmt.Sprintf(`e=$(mktemp /tmp/tunnel.XXXXXXXXXX) || e=/dev/null
%s -o ServerAliveInterval=60 -o ConnectTimeout=10 -o BatchMode=yes -o ExitOnForwardFailure=yes %s %s -N -f %s@%s >/dev/null 2>"$e" </dev/null
rc=$?
[ $rc -eq 0 ] || tail -n 3 "$e"
[ -f "$e" ] && rm -f "$e"
exit $rc`, ssh, forwardFlag(t.Type), ForwardSpec(t), t.RemoteUser, SshTarget(t))
}

// launchProc runs script on the gateway and, if it fails, returns what the
// script printed as the error; a var so tests can stand in for arkgated.
var launchProc = func(name, script string) error {
	conn := utils.GetArkConn()
	if conn == nil {
		return fmt.Errorf("arkgated unreachable")
	}
	conn.SetDeadline(time.Now().Add(sendArkTimeout))
	out, err := Acmd.PrepareCmd("StartTunnel: "+name, shBin, []string{"-c", script}).SendCmdOutput(conn)
	if err != nil {
		if msg := strings.TrimSpace(out); msg != "" {
			return errors.New(strings.ReplaceAll(msg, "\n", "; "))
		}
		return err
	}
	return nil
}

// Stop kills t's ssh process, if it's running. A tunnel that isn't
// running is not an error - that's the desired end state either way.
func Stop(t *Tunnel) error {
	if err := lockOps(); err != nil {
		return err
	}
	defer unlockOps()
	procs, err := procStatuses([]Tunnel{*t})
	if err != nil {
		return err
	}
	p, running := procs[t.ID]
	if !running {
		return nil
	}
	if err := killProc(t.Name, p.Pid); err != nil {
		// kill fails if the process is already gone - it exited between the
		// listing and the kill. That is the state Stop was asked for, not a
		// failure, so look again before reporting one.
		if again, e2 := procStatuses([]Tunnel{*t}); e2 == nil {
			if _, still := again[t.ID]; !still {
				return nil
			}
		}
		return fmt.Errorf("kill %d: %w", p.Pid, err)
	}
	return nil
}

// procStatuses and killProc are what Stop uses to reach the gateway; vars so
// tests can stand in for arkgated.
var (
	procStatuses = Statuses
	killProc     = func(name string, pid int) error {
		return sendArkCmd("StopTunnel: "+name, killBin, []string{strconv.Itoa(pid)})
	}
)

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
	if len(tunnels) == 0 {
		return map[uint]Proc{}, nil
	}
	// -a/-x: include other users' processes and ones with no controlling
	// terminal (a backgrounded ssh -f has none). -ww: don't truncate the
	// command line - the forwarding spec we match on sits in the middle of
	// it, and a truncated line would read as "not running".
	out, err := runViaArk("TunnelPs", psBin, []string{"-axww", "-o", "pid=,etime=,command="})
	if err != nil {
		return map[uint]Proc{}, err
	}
	// A real listing always has init in it. An empty or garbled one would
	// read as "no tunnel is running", and the watchdog would act on that by
	// starting a second ssh for every tunnel that is in fact up.
	if !hasProcessList(out) {
		return map[uint]Proc{}, fmt.Errorf("process list from arkgated is empty")
	}
	return parseProcs(out, tunnels), nil
}

// hasProcessList reports whether out looks like ps output: at least one line
// starting with a process id.
func hasProcessList(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) >= 2 {
			if _, err := strconv.Atoi(f[0]); err == nil {
				return true
			}
		}
	}
	return false
}

// isSshProcess reports whether argv0, the first word of a process's command
// line, is the ssh client (by full path or bare name).
func isSshProcess(argv0 string) bool {
	return argv0 == sshBin || argv0 == "ssh"
}

// parseProcs matches `ps -o pid=,etime=,command=` output against tunnels,
// pairing each tunnel with the ssh process carrying its forwarding
// argument. Matching on the rendered "-R host:port:host:port" rather than
// on a recorded pid is what makes this survive a gateway reboot or an ssh
// that was started by hand: the command line is the tunnel's identity.
func parseProcs(out string, tunnels []Tunnel) map[uint]Proc {
	procs := map[uint]Proc{}
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
		// Only ssh itself. Starting a tunnel runs `/bin/sh -c "ssh ... -f"`,
		// and for the moment that shell is alive its command line carries the
		// same forwarding spec - matching it would report a pid that is gone
		// by the time anything acts on it.
		if !isSshProcess(fields[2]) {
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
	return procs
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
		// Pressing "Start Inactive" is the operator asking for these to run,
		// including any they had stopped by hand, so the watchdog may keep
		// them up from here on.
		if tunnels[i].ManualStop {
			if err := SetManualStop(db, tunnels[i].ID, false); err != nil {
				log.Println("tunnels: failed to clear manual stop on", tunnels[i].Name, ":", err)
			}
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

// SshKey is the gateway's tunnel identity: the public half of the key ssh(1)
// offers when it starts a tunnel, which the operator enrolls on a remote
// endpoint (authorized_keys) so the tunnel can log in unattended.
type SshKey struct {
	PublicKey string `json:"public_key"`
	// Generated is true when this call had to create the key pair because
	// none existed yet.
	Generated bool `json:"generated"`
}

// sshKeyScript prints "NEW" or "EXISTING" and then the public key,
// generating an ed25519 pair first when there isn't one. The home directory
// is resolved with ~root rather than $HOME because arkgated is started from
// rc.d, where HOME is not reliably set, and ssh(1) itself looks up the
// identity by the user's passwd entry, not $HOME. The pair is created
// without a passphrase: a tunnel is started unattended, so one could never
// be entered.
const sshKeyScript = `d=$(eval echo ~root)/.ssh; f=$d/id_ed25519
if [ -f "$f.pub" ]; then echo EXISTING; else
  mkdir -p -m 700 "$d" && ssh-keygen -q -t ed25519 -N '' -C "arkgate-tunnel@$(hostname)" -f "$f" </dev/null >/dev/null 2>&1 || exit 1
  echo NEW
fi
cat "$f.pub"`

// EnsureSshKey returns the gateway's tunnel public key, generating the key
// pair on first use. Only the public half ever leaves the gateway.
func EnsureSshKey() (*SshKey, error) {
	out, err := runViaArk("TunnelSshKey", shBin, []string{"-c", sshKeyScript})
	if err != nil {
		return nil, fmt.Errorf("ssh key: %w", err)
	}
	return parseSshKey(out)
}

// parseSshKey reads sshKeyScript's output: a NEW/EXISTING marker line
// followed by the public key line.
func parseSshKey(out string) (*SshKey, error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("ssh key: unexpected output %q", strings.TrimSpace(out))
	}
	key := strings.TrimSpace(lines[len(lines)-1])
	if !strings.HasPrefix(key, "ssh-") {
		return nil, fmt.Errorf("ssh key: output is not a public key")
	}
	return &SshKey{PublicKey: key, Generated: strings.TrimSpace(lines[0]) == "NEW"}, nil
}
