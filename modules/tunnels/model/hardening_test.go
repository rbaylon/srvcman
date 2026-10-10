package tunnelmodel

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// After an outage or a reboot every tunnel is down at once. Starting them all
// in one pass would keep arkgated's single command queue busy for minutes.
func TestCheckStartsAtMostAFewTunnelsPerCheck(t *testing.T) {
	var ts []Tunnel
	for i := uint(1); i <= 10; i++ {
		ts = append(ts, tun(i, "t"))
	}
	f := newFake(ts...)
	w := newWatchdog(f.deps(), time.Hour)

	w.check(f.tunnels)
	if got := len(f.startedIDs()); got != maxStartsPerCheck {
		t.Errorf("first check started %d tunnels, want at most %d", got, maxStartsPerCheck)
	}
	for i := 0; i < 4; i++ {
		w.check(f.tunnels)
	}
	if len(f.startedIDs()) != 10 {
		t.Errorf("after several checks %d of 10 tunnels started, want all - the ones left over must not be forgotten", len(f.startedIDs()))
	}
	// A tunnel that was only deferred is not penalised, and one that was
	// started is not started twice.
	seen := map[uint]int{}
	for _, id := range f.startedIDs() {
		seen[id]++
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("tunnel %d started %d times", id, n)
		}
	}
}

// Dead tunnels burn their attempts on a growing delay but a healthy-but-slow
// one behind them in the list still gets its turn.
func TestFailingTunnelsDoNotStarveOthers(t *testing.T) {
	f := newFake(tun(1, "dead1"), tun(2, "dead2"), tun(3, "dead3"), tun(4, "dead4"), tun(5, "fixable"))
	for _, id := range []uint{1, 2, 3, 4} {
		f.startErr[id] = errors.New("connection timed out")
	}
	w := newWatchdog(f.deps(), time.Hour)
	w.check(f.tunnels) // starts 1,2,3 (all fail)
	w.check(f.tunnels) // 1,2,3 are backing off; 4 and 5 get their turn
	if !f.running[5] {
		t.Error("the tunnel that could start was held back by dead ones ahead of it")
	}
}

// When arkgated cannot be asked, the read itself is retried on a growing
// delay, not every minute.
func TestStatusFailuresBackOff(t *testing.T) {
	f := newFake(tun(1, "a"))
	f.statusErr = errors.New("arkgated unreachable")
	w := newWatchdog(f.deps(), time.Hour)

	for i := 0; i < 30; i++ {
		w.check(f.tunnels)
	}
	// Reads on ticks 1, 2, 4, 8, 13, 18, 23, 28 (waits 1,2,4,5,5,...).
	if f.statusN > 9 || f.statusN < 7 {
		t.Errorf("%d status reads over 30 checks while arkgated is down, want about 8", f.statusN)
	}
	if got := len(f.startedIDs()); got != 0 {
		t.Errorf("started %d tunnels with no status", got)
	}

	f.mu.Lock()
	f.statusErr = nil
	f.mu.Unlock()
	w.tick += maxStatusBackoffTicks
	w.check(f.tunnels)
	if !f.running[1] {
		t.Error("tunnel not restarted once status could be read again")
	}
	if w.statusFails != 0 {
		t.Error("status failure count kept after a good read")
	}
}

// A tunnel that connects and then drops again is slowed down, so a flapping
// link does not become a login attempt on the remote side every minute.
func TestFlappingTunnelIsSlowedDown(t *testing.T) {
	f := newFake(tun(1, "flappy"))
	w := newWatchdog(f.deps(), time.Hour)

	var attempts []int
	for i := 1; i <= 80; i++ {
		f.mu.Lock()
		f.running[1] = false // it drops again before every check
		f.mu.Unlock()
		before := len(f.startedIDs())
		w.check(f.tunnels)
		if len(f.startedIDs()) > before {
			attempts = append(attempts, i)
		}
	}
	if len(attempts) > 12 {
		t.Errorf("a tunnel that drops every minute was restarted %d times in 80 checks (%v), want it slowed to well under that", len(attempts), attempts)
	}
	last := 0
	var gaps []int
	for _, a := range attempts {
		gaps = append(gaps, a-last)
		last = a
	}
	if len(gaps) >= 4 && gaps[len(gaps)-1] < gaps[1] {
		t.Errorf("gaps between restarts %v do not grow", gaps)
	}
}

// ...but a tunnel that then stays up is forgiven.
func TestStableTunnelStopsCountingAsFlapping(t *testing.T) {
	f := newFake(tun(1, "t"))
	w := newWatchdog(f.deps(), time.Hour)
	w.check(f.tunnels)
	f.mu.Lock()
	f.running[1] = false
	f.mu.Unlock()
	w.check(f.tunnels) // drops straight away: flaps
	if w.state[1].flaps == 0 {
		t.Fatal("quick re-drop not counted as a flap")
	}
	for i := 0; i < stableTicks+2; i++ {
		w.check(f.tunnels) // stays up
	}
	if w.state[1].flaps != 0 {
		t.Errorf("flaps = %d after staying up for %d checks, want 0", w.state[1].flaps, stableTicks+2)
	}
}

// Memory stays bounded: what is remembered about a tunnel goes with it.
func TestStateForDeletedAndStoppedTunnelsIsDropped(t *testing.T) {
	a, b := tun(1, "a"), tun(2, "b")
	f := newFake(a, b)
	f.startErr[1], f.startErr[2] = errors.New("x"), errors.New("x")
	w := newWatchdog(f.deps(), time.Hour)
	w.check(f.tunnels)
	if len(w.state) != 2 {
		t.Fatalf("state for %d tunnels, want 2", len(w.state))
	}
	b.ManualStop = true
	w.check([]Tunnel{a, b})
	if _, kept := w.state[2]; kept {
		t.Error("state kept for a tunnel stopped by hand")
	}
	w.check([]Tunnel{})
	if len(w.state) != 0 {
		t.Errorf("state kept for deleted tunnels: %v", w.state)
	}
}

// If the records cannot be read when srvcman starts, tunnels must not be left
// unwatched until someone happens to create another one.
func TestEnsureStartsEvenIfTheFirstReadFails(t *testing.T) {
	var mu sync.Mutex
	failing := true
	f := newFake(tun(1, "one"))
	f.running[1] = true
	d := f.deps()
	realLoad := d.load
	d.load = func() ([]Tunnel, error) {
		mu.Lock()
		defer mu.Unlock()
		if failing {
			return nil, errors.New("database is locked")
		}
		return realLoad()
	}
	w := newWatchdog(d, 5*time.Millisecond)
	w.Ensure()
	w.mu.Lock()
	running := w.running
	w.mu.Unlock()
	if !running {
		t.Fatal("watchdog did not start because the first read failed")
	}
	time.Sleep(30 * time.Millisecond) // reads keep failing: it must survive that
	mu.Lock()
	failing = false
	mu.Unlock()
	f.mu.Lock()
	f.running[1] = false
	f.mu.Unlock()
	waitUntil(t, "the tunnel to be restarted once the database came back", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.running[1]
	})
	f.mu.Lock()
	f.tunnels = nil
	f.mu.Unlock()
	waitUntil(t, "shutdown", func() bool { w.mu.Lock(); defer w.mu.Unlock(); return !w.running })
}

// Starting is idempotent: whoever decided to start from a stale status, a
// second start must not launch a second ssh.
func TestStartDoesNothingIfTheTunnelIsAlreadyRunning(t *testing.T) {
	oldS, oldL := procStatuses, launchProc
	defer func() { procStatuses, launchProc = oldS, oldL }()
	launches := 0
	launchProc = func(string, string) error { launches++; return nil }
	tn := Tunnel{Name: "t", Type: TypeRemote, RemoteUser: "u", SshHost: "h.example.com", RemoteIp: "127.0.0.1", RemotePort: 8000, LocalIp: "127.0.0.1", LocalPort: 8000}
	tn.ID = 1

	procStatuses = func([]Tunnel) (map[uint]Proc, error) { return map[uint]Proc{1: {Pid: 7}}, nil }
	if err := Start(&tn); err != nil || launches != 0 {
		t.Errorf("Start on a running tunnel: err=%v launches=%d, want nil and 0", err, launches)
	}
	procStatuses = func([]Tunnel) (map[uint]Proc, error) { return map[uint]Proc{}, nil }
	if err := Start(&tn); err != nil || launches != 1 {
		t.Errorf("Start on a stopped tunnel: err=%v launches=%d, want nil and 1", err, launches)
	}
}

// The start command must make ssh give up when the forward cannot be set up,
// not carry on as a process that looks like a tunnel and is not one.
func TestStartCommandExitsOnForwardFailureAndNeverHangs(t *testing.T) {
	oldS, oldL := procStatuses, launchProc
	defer func() { procStatuses, launchProc = oldS, oldL }()
	procStatuses = func([]Tunnel) (map[uint]Proc, error) { return map[uint]Proc{}, nil }
	var script string
	launchProc = func(_, s string) error { script = s; return nil }
	tn := Tunnel{Name: "t", Type: TypeLocal, RemoteUser: "u", RemoteIp: "10.0.0.5", RemotePort: 22, LocalIp: "127.0.0.1", LocalPort: 2222}
	tn.ID = 1
	if err := Start(&tn); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ExitOnForwardFailure=yes", "ConnectTimeout=10", "BatchMode=yes", "-L 127.0.0.1:2222:10.0.0.5:22", "</dev/null"} {
		if !strings.Contains(script, want) {
			t.Errorf("start command %q lacks %q", script, want)
		}
	}
}

// An operator who keeps pressing buttons while arkgated is slow must be told
// "busy", not left with a parked request for every press.
func TestOperationsTimeOutInsteadOfPilingUp(t *testing.T) {
	old := opLockWait
	opLockWait = 20 * time.Millisecond
	defer func() { opLockWait = old }()

	if err := lockOps(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- Stop(&Tunnel{Name: "x"}) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, errBusy) {
			t.Errorf("Stop while busy = %v, want errBusy", err)
		}
	}
	unlockOps()
	// Nothing is left holding the lock.
	if err := lockOps(); err != nil {
		t.Errorf("lock not released: %v", err)
	}
	unlockOps()
}

// An empty process listing must not read as "nothing is running".
func TestEmptyProcessListIsAnError(t *testing.T) {
	for _, out := range []string{"", "\n", "garbage\nmore garbage\n", "   \n"} {
		if hasProcessList(out) {
			t.Errorf("hasProcessList(%q) = true", out)
		}
	}
	if !hasProcessList("    1     08:31:53 /sbin/init\n") {
		t.Error("a real listing was rejected")
	}
}

// A problem that lasts is reported once, not once per attempt; a new one is
// reported, and so is the same one coming back after a good read.
func TestRecurringProblemsAreLoggedOnce(t *testing.T) {
	f := newFake(tun(1, "a"))
	f.running[1] = true
	f.statusErr = errors.New("arkgated unreachable")
	w := newWatchdog(f.deps(), time.Hour)

	w.check(f.tunnels)
	first := w.lastErr
	if first == "" {
		t.Fatal("outage not recorded")
	}
	for i := 0; i < 10; i++ {
		w.check(f.tunnels)
	}
	if w.lastErr != first {
		t.Errorf("lastErr changed to %q while the same outage continued", w.lastErr)
	}

	f.mu.Lock()
	f.statusErr = nil
	f.mu.Unlock()
	w.tick += maxStatusBackoffTicks
	w.check(f.tunnels)
	if w.lastErr != "" {
		t.Errorf("lastErr = %q after a good read, want it cleared so a new outage is reported", w.lastErr)
	}
}

// Two records for one forward are one ssh process: they show the same
// status and deleting either stops the other. They are refused.
func TestFindDuplicate(t *testing.T) {
	mk := func(id uint, typ string, rp, lp int) Tunnel {
		x := Tunnel{Name: "t", Type: typ, RemoteIp: "127.0.0.1", RemotePort: rp, LocalIp: "127.0.0.1", LocalPort: lp}
		x.ID = id
		return x
	}
	existing := []Tunnel{mk(1, TypeRemote, 8000, 8000), mk(2, TypeLocal, 8000, 8000), mk(3, TypeRemote, 9000, 9000)}

	// Same direction and forward as 1.
	cand := mk(0, TypeRemote, 8000, 8000)
	if d := findDuplicate(existing, &cand); d == nil || d.ID != 1 {
		t.Errorf("duplicate of tunnel 1 not found: %+v", d)
	}
	// Same numbers but the other direction is a different forward.
	cand = mk(0, TypeLocal, 8000, 8000)
	if d := findDuplicate(existing, &cand); d == nil || d.ID != 2 {
		t.Errorf("-L duplicate should match tunnel 2 only, got %+v", d)
	}
	// A genuinely new forward.
	cand = mk(0, TypeRemote, 8001, 8001)
	if d := findDuplicate(existing, &cand); d != nil {
		t.Errorf("new forward reported as duplicate of %+v", d)
	}
	// Editing a tunnel must not collide with itself.
	cand = mk(1, TypeRemote, 8000, 8000)
	if d := findDuplicate(existing, &cand); d != nil {
		t.Errorf("tunnel reported as a duplicate of itself: %+v", d)
	}
}
