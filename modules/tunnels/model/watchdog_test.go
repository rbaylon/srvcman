package tunnelmodel

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// fake stands in for the database and arkgated.
type fake struct {
	mu        sync.Mutex
	tunnels   []Tunnel
	running   map[uint]bool
	statusErr error
	startErr  map[uint]error
	started   []uint
	statusN   int
}

func newFake(tunnels ...Tunnel) *fake {
	return &fake{tunnels: tunnels, running: map[uint]bool{}, startErr: map[uint]error{}}
}

func (f *fake) deps() watchdogDeps {
	return watchdogDeps{
		load: func() ([]Tunnel, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return append([]Tunnel(nil), f.tunnels...), nil
		},
		statuses: func(ts []Tunnel) (map[uint]Proc, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.statusN++
			if f.statusErr != nil {
				return map[uint]Proc{}, f.statusErr
			}
			out := map[uint]Proc{}
			for _, t := range ts {
				if f.running[t.ID] {
					out[t.ID] = Proc{Pid: 100 + int(t.ID)}
				}
			}
			return out, nil
		},
		start: func(t *Tunnel) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.started = append(f.started, t.ID)
			if err := f.startErr[t.ID]; err != nil {
				return err
			}
			f.running[t.ID] = true
			return nil
		},
	}
}

func tun(id uint, name string) Tunnel {
	t := Tunnel{Name: name}
	t.ID = id
	return t
}

func (f *fake) startedIDs() []uint {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint(nil), f.started...)
}

func TestCheckRestartsOnlyInactiveTunnels(t *testing.T) {
	f := newFake(tun(1, "up"), tun(2, "down"), tun(3, "alsodown"))
	f.running[1] = true
	w := newWatchdog(f.deps(), time.Hour)

	w.check(f.tunnels)

	got := f.startedIDs()
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Errorf("started %v, want only the inactive tunnels [2 3]", got)
	}
}

// A tunnel stopped by hand must stay stopped, and checking it costs nothing:
// with every tunnel stopped there is not even a status call.
func TestCheckLeavesManuallyStoppedTunnelsAlone(t *testing.T) {
	stopped := tun(1, "stopped")
	stopped.ManualStop = true
	f := newFake(stopped, tun(2, "down"))
	w := newWatchdog(f.deps(), time.Hour)

	w.check(f.tunnels)
	if got := f.startedIDs(); len(got) != 1 || got[0] != 2 {
		t.Errorf("started %v, want only [2]", got)
	}

	only := newFake(stopped)
	w2 := newWatchdog(only.deps(), time.Hour)
	w2.check(only.tunnels)
	if only.statusN != 0 || len(only.startedIDs()) != 0 {
		t.Errorf("all tunnels stopped by hand: %d status calls, %d starts; want none", only.statusN, len(only.startedIDs()))
	}
}

// When arkgated cannot be asked, every tunnel would look dead. Restarting
// them all would start a second ssh for each healthy one.
func TestCheckStartsNothingWhenStatusIsUnavailable(t *testing.T) {
	f := newFake(tun(1, "a"), tun(2, "b"))
	f.running[1], f.running[2] = true, true
	f.statusErr = errors.New("arkgated unreachable")
	w := newWatchdog(f.deps(), time.Hour)

	w.check(f.tunnels)
	w.check(f.tunnels)

	if got := f.startedIDs(); len(got) != 0 {
		t.Errorf("started %v while status was unavailable, want none", got)
	}
}

// A tunnel that cannot come up is retried on a growing delay instead of every
// minute: each attempt holds arkgated's single command queue.
func TestCheckBacksOffFailingTunnels(t *testing.T) {
	f := newFake(tun(1, "dead"))
	f.startErr[1] = errors.New("connection timed out")
	w := newWatchdog(f.deps(), time.Hour)

	var attemptTicks []int
	for i := 1; i <= 40; i++ {
		before := len(f.startedIDs())
		w.check(f.tunnels)
		if len(f.startedIDs()) > before {
			attemptTicks = append(attemptTicks, i)
		}
	}
	// Attempts on ticks 1, 2, 4, 8, 16 (waits of 1, 2, 4, 8, 15 capped), then
	// every 15.
	want := []int{1, 2, 4, 8, 16, 31}
	if len(attemptTicks) < len(want) {
		t.Fatalf("attempts on ticks %v, want %v", attemptTicks, want)
	}
	for i, tick := range want {
		if attemptTicks[i] != tick {
			t.Errorf("attempts on ticks %v, want %v", attemptTicks, want)
			break
		}
	}

	// Once it is back up the delay is forgotten.
	f.startErr[1] = nil
	w.tick += maxBackoffTicks
	w.check(f.tunnels)
	if !f.running[1] {
		t.Error("tunnel was not restarted once it could be")
	}
	if st := w.state[1]; st == nil || st.failures != 0 {
		t.Errorf("failure count kept after a successful restart: %+v", st)
	}
}

func TestEnsureDoesNotStartWithoutTunnels(t *testing.T) {
	f := newFake()
	w := newWatchdog(f.deps(), 5*time.Millisecond)
	w.Ensure()
	time.Sleep(40 * time.Millisecond)
	w.mu.Lock()
	running := w.running
	w.mu.Unlock()
	if running {
		t.Error("watchdog started with no tunnels on record")
	}
	if f.statusN != 0 {
		t.Errorf("%d status calls with no tunnels, want none", f.statusN)
	}
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// The whole lifecycle: starts when a tunnel exists, restarts it when it
// drops, stops by itself when the last tunnel is deleted, and a tunnel
// created afterwards starts it again.
func TestWatchdogLifecycle(t *testing.T) {
	f := newFake(tun(1, "one"))
	f.running[1] = true
	w := newWatchdog(f.deps(), 5*time.Millisecond)
	isRunning := func() bool { w.mu.Lock(); defer w.mu.Unlock(); return w.running }

	w.Ensure()
	if !isRunning() {
		t.Fatal("watchdog did not start although a tunnel is on record")
	}

	f.mu.Lock()
	f.running[1] = false // the tunnel drops
	f.mu.Unlock()
	waitUntil(t, "the dropped tunnel to be restarted", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.running[1]
	})

	f.mu.Lock()
	f.tunnels = nil // last tunnel deleted
	f.mu.Unlock()
	waitUntil(t, "the watchdog to stop on its own", func() bool { return !isRunning() })

	f.mu.Lock()
	f.tunnels = []Tunnel{tun(2, "two")}
	f.mu.Unlock()
	w.Ensure()
	if !isRunning() {
		t.Error("watchdog did not start again for a newly created tunnel")
	}
	waitUntil(t, "the new tunnel to be started", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.running[2]
	})
	// Leave nothing running behind the test.
	f.mu.Lock()
	f.tunnels = nil
	f.mu.Unlock()
	waitUntil(t, "shutdown", func() bool { return !isRunning() })
}

func TestEnsureIsIdempotent(t *testing.T) {
	f := newFake(tun(1, "one"))
	f.running[1] = true
	w := newWatchdog(f.deps(), 5*time.Millisecond)
	w.Ensure()
	w.Ensure()
	w.Ensure()
	// Several loops would each make a status call per tick; one loop makes
	// about one per interval.
	time.Sleep(60 * time.Millisecond)
	f.mu.Lock()
	n := f.statusN
	f.tunnels = nil
	f.mu.Unlock()
	if n > 14 {
		t.Errorf("%d status calls in 60ms at a 5ms interval - more than one loop is running", n)
	}
	waitUntil(t, "shutdown", func() bool { w.mu.Lock(); defer w.mu.Unlock(); return !w.running })
}

// A tunnel that was stopped by hand, or deleted, after this check began is a
// skip - not a failure to back off from, and not something to start.
func TestCheckSkipsTunnelsThatAreNoLongerWanted(t *testing.T) {
	f := newFake(tun(1, "gone"))
	f.startErr[1] = errNotWanted
	w := newWatchdog(f.deps(), time.Hour)

	for i := 0; i < 3; i++ {
		w.check(f.tunnels)
	}
	if got := len(f.startedIDs()); got != 3 {
		t.Errorf("start attempted %d times over 3 checks, want every check (a skip must not back off)", got)
	}
	if len(w.state) != 0 {
		t.Errorf("state %v recorded for a skipped tunnel, want none", w.state)
	}
}

// The shell that launches a tunnel carries the same forwarding spec in its
// command line for the moment it lives. Matching it reports a pid that is
// gone by the time Stop uses it.
func TestParseProcsIgnoresTheLaunchingShell(t *testing.T) {
	tn := Tunnel{Type: TypeRemote, RemoteIp: "127.0.0.1", RemotePort: 8000, LocalIp: "127.0.0.1", LocalPort: 8000}
	tn.ID = 1
	spec := "-R 127.0.0.1:8000:127.0.0.1:8000"
	out := "67275        00:00 /bin/sh -c /usr/bin/ssh -o ServerAliveInterval=60 " + spec + " -N -f u@h >/dev/null\n" +
		"51383        00:00 /usr/bin/ssh -o ServerAliveInterval=60 " + spec + " -N -f u@h\n"

	got := parseProcs(out, []Tunnel{tn})
	if p, ok := got[1]; !ok || p.Pid != 51383 {
		t.Errorf("matched %+v, want the ssh process 51383 and not the shell 67275", got)
	}
	if got := parseProcs("67275        00:00 /bin/sh -c /usr/bin/ssh "+spec+" -N -f u@h\n", []Tunnel{tn}); len(got) != 0 {
		t.Errorf("shell alone matched as the tunnel: %+v", got)
	}
}

func TestStopTreatsAnAlreadyGoneProcessAsStopped(t *testing.T) {
	oldS, oldK := procStatuses, killProc
	defer func() { procStatuses, killProc = oldS, oldK }()
	calls := 0
	procStatuses = func(ts []Tunnel) (map[uint]Proc, error) {
		calls++
		if calls == 1 {
			return map[uint]Proc{1: {Pid: 42}}, nil
		}
		return map[uint]Proc{}, nil // exited before kill ran
	}
	killProc = func(string, int) error { return errors.New("exit status 1") }

	if err := Stop(&Tunnel{Name: "x", Model: tun(1, "").Model}); err != nil {
		t.Errorf("Stop = %v, want nil when the process exited by itself", err)
	}

	// A kill that fails on a process that is still there is a real failure.
	procStatuses = func([]Tunnel) (map[uint]Proc, error) { return map[uint]Proc{1: {Pid: 42}}, nil }
	if err := Stop(&Tunnel{Name: "x", Model: tun(1, "").Model}); err == nil {
		t.Error("Stop hid a kill failure on a process that is still running")
	}
}

// Start and Stop never overlap, so Stop cannot read the process list while a
// start is half done.
func TestStartAndStopAreSerialized(t *testing.T) {
	oldS, oldK := procStatuses, killProc
	defer func() { procStatuses, killProc = oldS, oldK }()
	var mu sync.Mutex
	inside, overlapped := 0, false
	enter := func() {
		mu.Lock()
		inside++
		if inside > 1 {
			overlapped = true
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		inside--
		mu.Unlock()
	}
	procStatuses = func([]Tunnel) (map[uint]Proc, error) { enter(); return map[uint]Proc{1: {Pid: 1}}, nil }
	killProc = func(string, int) error { return nil }

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := lockOps(); err != nil { // what Start holds while it works
				t.Error(err)
				return
			}
			enter()
			unlockOps()
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			Stop(&Tunnel{Name: "x", Model: tun(1, "").Model})
		}()
	}
	wg.Wait()
	if overlapped {
		t.Error("a Start and a Stop ran at the same time")
	}
}
