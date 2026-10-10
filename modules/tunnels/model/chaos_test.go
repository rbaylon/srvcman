package tunnelmodel

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"sync"
	"testing"
	"time"
)

// gateway is a stand-in for the box arkgated runs on: a table of ssh
// processes, with the behaviour of the real ssh options the start command
// uses (a connect failure or an unusable forward makes ssh exit, so no
// process is left behind).
type gateway struct {
	mu        sync.Mutex
	procs     map[int]string // pid -> "-R spec"
	next      int
	launches  int
	deadHosts map[string]bool
	psOutage  bool // arkgated cannot be reached
	psEmpty   bool // arkgated answers with an empty listing
}

var startRe = regexp.MustCompile(`-(R|L) (\S+) -N -f (\S+)@(\S+) `)

func newGateway() *gateway {
	return &gateway{procs: map[int]string{}, next: 1000, deadHosts: map[string]bool{}}
}

func (g *gateway) launch(_, script string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.launches++
	m := startRe.FindStringSubmatch(script)
	if m == nil {
		return fmt.Errorf("unrecognised start command %q", script)
	}
	flagSpec := "-" + m[1] + " " + m[2]
	if g.deadHosts[m[4]] {
		return errors.New("ssh: connect timed out") // ConnectTimeout: exits, no process left
	}
	for _, spec := range g.procs {
		if spec == flagSpec {
			return errors.New("remote port forwarding failed") // ExitOnForwardFailure
		}
	}
	g.next++
	g.procs[g.next] = flagSpec
	return nil
}

func (g *gateway) psText() string {
	out := "    1     08:00:00 /sbin/init\n"
	for pid, spec := range g.procs {
		out += fmt.Sprintf("%d     00:10 /usr/bin/ssh -o ServerAliveInterval=60 -o ConnectTimeout=10 -o BatchMode=yes -o ExitOnForwardFailure=yes %s -N -f u@h\n", pid, spec)
	}
	return out
}

// statuses is Statuses without the real arkgated: the same sanity check and
// the same matching of processes to tunnels.
func (g *gateway) statuses(ts []Tunnel) (map[uint]Proc, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.psOutage {
		return map[uint]Proc{}, errors.New("arkgated unreachable")
	}
	out := g.psText()
	if g.psEmpty {
		out = ""
	}
	if !hasProcessList(out) {
		return map[uint]Proc{}, errors.New("process list from arkgated is empty")
	}
	return parseProcs(out, ts), nil
}

func (g *gateway) maxPerSpec() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	count := map[string]int{}
	most := 0
	for _, spec := range g.procs {
		count[spec]++
		if count[spec] > most {
			most = count[spec]
		}
	}
	return most
}

func remoteTunnel(id uint, port int, host string) Tunnel {
	t := Tunnel{Name: fmt.Sprintf("t%d", id), Type: TypeRemote, RemoteUser: "u", SshHost: host,
		RemoteIp: "127.0.0.1", RemotePort: port, LocalIp: "127.0.0.1", LocalPort: port}
	t.ID = id
	return t
}

// A long run of everything that can go wrong at once. Whatever the order,
// there must never be two ssh processes for one forward, no check may start
// more than its share, and the number of connection attempts must stay small
// - this is what keeps the watchdog from becoming the problem.
func runBadDay(t *testing.T, seed int64) {
	oldS, oldL := procStatuses, launchProc
	defer func() { procStatuses, launchProc = oldS, oldL }()
	g := newGateway()
	procStatuses, launchProc = g.statuses, g.launch

	tunnels := []Tunnel{
		remoteTunnel(1, 9001, "good.example.com"), // healthy
		remoteTunnel(2, 9002, "dead.example.com"), // host never reachable
		remoteTunnel(3, 9003, "good.example.com"), // duplicated by...
		remoteTunnel(4, 9003, "good.example.com"), // ...a second record, same forward
		remoteTunnel(5, 9005, "good.example.com"), // flaps: dies every other minute
		remoteTunnel(6, 9006, "dead.example.com"), // more dead ones, to press on the
		remoteTunnel(7, 9007, "dead.example.com"), // per-check limit
		remoteTunnel(8, 9008, "dead.example.com"),
		remoteTunnel(9, 9009, "dead.example.com"),
		remoteTunnel(10, 9010, "good.example.com"),
	}
	g.deadHosts["dead.example.com"] = true

	w := newWatchdog(watchdogDeps{
		load:     func() ([]Tunnel, error) { return append([]Tunnel(nil), tunnels...), nil },
		statuses: g.statuses,
		start: func(t *Tunnel) error {
			if err := lockOps(); err != nil {
				return err
			}
			defer unlockOps()
			return startLocked(t)
		},
	}, time.Hour)

	rng := rand.New(rand.NewSource(seed))
	const minutes = 400
	worstPerCheck := 0
	for tick := 1; tick <= minutes; tick++ {
		g.mu.Lock()
		g.psOutage = rng.Intn(100) < 15 // arkgated unreachable some minutes
		g.psEmpty = rng.Intn(100) < 5   // ...or answering with nothing
		if tick%2 == 0 {                // the flapper's link drops
			for pid, spec := range g.procs {
				if spec == "-R 127.0.0.1:9005:127.0.0.1:9005" {
					delete(g.procs, pid)
				}
			}
		}
		if tick%97 == 0 { // the gateway's links all drop at once
			g.procs = map[int]string{}
		}
		before := g.launches
		g.mu.Unlock()

		// An operator, now and then, pressing Start on a tunnel at the same
		// moment as the watchdog's check.
		if rng.Intn(100) < 20 {
			tn := tunnels[rng.Intn(len(tunnels))]
			Start(&tn)
		}
		manual := 0
		g.mu.Lock()
		manual = g.launches - before
		g.mu.Unlock()

		w.check(tunnels)

		g.mu.Lock()
		perCheck := g.launches - before - manual
		g.mu.Unlock()
		if perCheck > worstPerCheck {
			worstPerCheck = perCheck
		}
		if perCheck > maxStartsPerCheck {
			t.Fatalf("check %d made %d connection attempts, want at most %d", tick, perCheck, maxStartsPerCheck)
		}
		if n := g.maxPerSpec(); n > 1 {
			t.Fatalf("check %d left %d ssh processes for one forward", tick, n)
		}
	}

	g.mu.Lock()
	launches := g.launches
	g.mu.Unlock()
	// 10 tunnels over 400 minutes. Started every minute whenever one looked
	// down, that would be thousands of attempts; with backing off, a few
	// hundred at the very most.
	if launches > 400 {
		t.Errorf("%d connection attempts over %d minutes, want the backoff to keep this far lower", launches, minutes)
	}
	t.Logf("%d connection attempts over %d minutes for %d tunnels; busiest check made %d", launches, minutes, len(tunnels), worstPerCheck)
}

func TestSimulatedBadDaysNeverLeakProcessesOrFloodArkgated(t *testing.T) {
	for _, seed := range []int64{1, 2, 3, 7, 11, 42, 99, 2026} {
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) { runBadDay(t, seed) })
	}
}
