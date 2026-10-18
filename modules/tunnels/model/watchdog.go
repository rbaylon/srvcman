package tunnelmodel

import (
	"errors"
	"log"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"
)

const (
	// watchdogInterval is how often tunnel status is checked.
	watchdogInterval = time.Minute

	// maxBackoffTicks caps how many checks a tunnel that keeps failing or
	// flapping is skipped for between restart attempts.
	maxBackoffTicks = 15

	// maxStatusBackoffTicks caps how many checks are skipped after the
	// status read itself fails (arkgated down or wedged).
	maxStatusBackoffTicks = 5

	// maxStartsPerCheck bounds how many tunnels one check will try to
	// start. Each start goes through arkgated's single command queue and, if
	// the remote is unreachable, holds it for ssh's whole connect timeout.
	// After an outage or a reboot every tunnel is down at once; starting
	// them all in one pass would keep the queue busy for minutes and delay
	// every firewall and DHCP change behind it. The rest are picked up on
	// the following checks.
	maxStartsPerCheck = 3

	// stableTicks is how long a restarted tunnel must stay up before it
	// stops counting as flapping. It is longer than maxBackoffTicks so that a
	// tunnel being retried at the slowest pace is not mistaken for a stable
	// one between attempts.
	stableTicks = 2 * maxBackoffTicks
)

// The watchdog keeps ssh forwards up. An ssh forward does not survive the
// remote end dropping it, a network blip or a gateway reboot, and nothing
// else brings it back - the list page's "Start Inactive" button was the only
// way. Every watchdogInterval it asks the gateway which tunnels are running
// (one ps for all of them) and restarts the ones that are not.
//
// It runs only while there are tunnels on record. It is started at module
// load if there are any and whenever one is created, and it ends itself the
// first time it finds none, so a gateway that does not use tunnels never has
// anything wake up for them.
//
// A tunnel the operator stopped on purpose (Tunnel.ManualStop) is left
// alone; otherwise the Stop button would be undone within a minute.
//
// What it must never do is make a bad day worse, so it is deliberately slow
// and bounded wherever the network or an operator can hand it a problem:
//   - a tunnel that fails to start is retried after 1, 2, 4, 8, then 15
//     checks, not every minute;
//   - a tunnel that comes up and drops again is slowed the same way;
//   - at most maxStartsPerCheck tunnels are started per check;
//   - if the status of the gateway cannot be read, nothing is started (every
//     tunnel would look dead) and the read itself is retried on a growing
//     delay;
//   - a start first checks the tunnel is not already running, so two starts
//     that raced cannot leave two ssh processes for one forward.
//
// This lives in srvcman rather than arkgated because the records do: the
// loop needs "which tunnels should exist", and arkgated only knows how to
// run the commands that start and inspect them.

// watchdogDeps is everything the loop touches outside itself, so tests can
// replace the database and arkgated.
type watchdogDeps struct {
	load     func() ([]Tunnel, error)
	statuses func([]Tunnel) (map[uint]Proc, error)
	start    func(*Tunnel) error
}

// errNotWanted is what the start step returns for a tunnel that should not be
// brought up after all: it was stopped by hand, or deleted, after this check
// began. It is a skip, not a failure.
var errNotWanted = errors.New("tunnel is no longer wanted")

// tunnelState is what the watchdog remembers about one tunnel between
// checks. All of it is in memory: after a restart of srvcman it starts again
// from a clean slate, which only means a failing tunnel is retried promptly
// once before it backs off again.
type tunnelState struct {
	failures    int // consecutive failed start attempts
	notBefore   int // first check on which another attempt is allowed
	lastRestart int // check on which it was last restarted successfully
	lastAttempt int // check on which a start was last tried, for fair ordering
	flaps       int // consecutive restarts that did not last
}

type watchdog struct {
	mu       sync.Mutex // guards running, and makes "no tunnels -> stop" atomic with Ensure
	running  bool
	interval time.Duration
	deps     watchdogDeps

	// Only the loop goroutine touches these.
	tick            int
	state           map[uint]*tunnelState
	statusFails     int
	statusNotBefore int
	lastErr         string
}

func newWatchdog(deps watchdogDeps, interval time.Duration) *watchdog {
	return &watchdog{deps: deps, interval: interval, state: map[uint]*tunnelState{}}
}

// backoff is how many checks to wait after n consecutive failures: 1, 2, 4,
// 8, then maxBackoffTicks.
func backoff(n int) int {
	if n < 1 {
		return 0
	}
	if n > 5 {
		return maxBackoffTicks
	}
	if wait := 1 << (n - 1); wait < maxBackoffTicks {
		return wait
	}
	return maxBackoffTicks
}

// Ensure starts the loop if tunnels exist and it is not already running. It
// is safe to call as often as is convenient.
func (w *watchdog) Ensure() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return
	}
	tunnels, err := w.deps.load()
	// If the records cannot be read right now - the database busy at boot,
	// say - start anyway: the loop retries the read every check, and stops
	// itself once it can read them and finds none. Giving up here would leave
	// tunnels unwatched until the next one happened to be created.
	if err == nil && len(tunnels) == 0 {
		return
	}
	w.running = true
	w.state = map[uint]*tunnelState{}
	go w.loop()
	log.Println("tunnels: watchdog started, checking every", w.interval)
}

func (w *watchdog) loop() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for range ticker.C {
		if !w.step() {
			return
		}
	}
}

// step is one tick. It reports whether the loop should keep going.
func (w *watchdog) step() (keepGoing bool) {
	// A panic here must not take srvcman down with it, and must not leave
	// the watchdog marked running with no loop behind it.
	defer func() {
		if r := recover(); r != nil {
			log.Println("tunnels: watchdog check crashed:", r)
			keepGoing = true
		}
	}()

	// Deciding to stop happens under the same lock Ensure takes, so a
	// tunnel created at this moment either is seen here or finds the loop
	// already marked stopped and starts a new one.
	w.mu.Lock()
	tunnels, err := w.deps.load()
	if err == nil && len(tunnels) == 0 {
		w.running = false
		w.mu.Unlock()
		log.Println("tunnels: no tunnels on record, watchdog stopped")
		return false
	}
	w.mu.Unlock()
	if err != nil {
		w.logOnce("could not read the tunnel records", err)
		return true
	}
	w.check(tunnels)
	return true
}

// logOnce logs a recurring problem the first time it is seen, and again only
// if it changes, so a condition that lasts for hours is one line, not one
// per check.
func (w *watchdog) logOnce(what string, err error) {
	if msg := what + ": " + err.Error(); msg != w.lastErr {
		log.Println("tunnels: watchdog", msg)
		w.lastErr = msg
	}
}

func (w *watchdog) stateOf(id uint) *tunnelState {
	st := w.state[id]
	if st == nil {
		st = &tunnelState{}
		w.state[id] = st
	}
	return st
}

// check restarts watched tunnels that are not running.
func (w *watchdog) check(tunnels []Tunnel) {
	w.tick++

	var watched []Tunnel
	for _, t := range tunnels {
		if !t.ManualStop {
			watched = append(watched, t)
		}
	}
	w.forgetUnwatched(watched)
	if len(watched) == 0 {
		return
	}

	// While the status read keeps failing, try it on a growing delay rather
	// than dialling a dead arkgated (and logging about it) every minute.
	if w.tick < w.statusNotBefore {
		return
	}
	procs, err := w.deps.statuses(watched)
	if err != nil {
		// Without a status every tunnel would look dead, and restarting
		// them all would start a second ssh for each one that is in fact
		// fine. Wait for arkgated instead.
		w.statusFails++
		wait := 1 << (w.statusFails - 1)
		if w.statusFails > 4 || wait > maxStatusBackoffTicks {
			wait = maxStatusBackoffTicks
		}
		w.statusNotBefore = w.tick + wait
		w.logOnce("cannot read tunnel status, skipping checks until it can", err)
		return
	}
	// A good read ends the problem, so the next one is reported afresh.
	w.statusFails, w.statusNotBefore, w.lastErr = 0, 0, ""

	var due []*Tunnel
	for i := range watched {
		t := &watched[i]
		st := w.stateOf(t.ID)
		if _, running := procs[t.ID]; running {
			st.failures = 0
			if st.flaps > 0 && w.tick-st.lastRestart > stableTicks {
				st.flaps = 0
			}
			continue
		}
		if w.tick >= st.notBefore {
			due = append(due, t)
		}
	}

	// Least recently tried first. Without this, tunnels that fail early in
	// the list would use up every start slot on each check while one further
	// down - perhaps the only one that could actually come up - never got a
	// turn. A tunnel never tried has lastAttempt 0 and so goes first.
	sort.SliceStable(due, func(i, j int) bool {
		return w.state[due[i].ID].lastAttempt < w.state[due[j].ID].lastAttempt
	})

	for n, t := range due {
		if n >= maxStartsPerCheck {
			// Not a failure: these just wait for the next check, and will
			// be first in line then.
			break
		}
		st := w.state[t.ID]
		st.lastAttempt = w.tick

		err := w.deps.start(t)
		switch {
		case errors.Is(err, errNotWanted):
			delete(w.state, t.ID)
		case err != nil:
			st.failures++
			wait := backoff(st.failures)
			st.notBefore = w.tick + wait
			log.Printf("tunnels: watchdog could not restart %q (attempt %d, next try in %d min): %v", t.Name, st.failures, wait, err)
		default:
			// Back up. If it was last restarted only recently it is
			// flapping: slow the next restart down, a little more each
			// time, so a link that connects and drops again does not turn
			// into a connection attempt (and an auth log line on the remote
			// side) every minute.
			if st.lastRestart != 0 && w.tick-st.lastRestart <= stableTicks {
				st.flaps++
			} else {
				st.flaps = 0
			}
			st.lastRestart = w.tick
			st.failures = 0
			st.notBefore = 0
			if st.flaps > 0 {
				st.notBefore = w.tick + backoff(st.flaps+1)
			}
			log.Printf("tunnels: watchdog restarted inactive tunnel %q", t.Name)
		}
	}
}

// forgetUnwatched drops what is remembered about tunnels that were deleted
// or are now stopped by hand, so that state cannot accumulate.
func (w *watchdog) forgetUnwatched(watched []Tunnel) {
	for id := range w.state {
		found := false
		for i := range watched {
			if watched[i].ID == id {
				found = true
				break
			}
		}
		if !found {
			delete(w.state, id)
		}
	}
}

var (
	defaultWatchdog     *watchdog
	defaultWatchdogOnce sync.Once
)

// StartWatchdog starts the tunnel watchdog if there are tunnels on record
// and it is not already running. Call it once at startup, after MigrateDB,
// and again whenever a tunnel is created.
func StartWatchdog(db *gorm.DB) {
	defaultWatchdogOnce.Do(func() {
		defaultWatchdog = newWatchdog(watchdogDeps{
			load: func() ([]Tunnel, error) {
				var tunnels []Tunnel
				err := db.Find(&tunnels).Error
				return tunnels, err
			},
			statuses: Statuses,
			start: func(t *Tunnel) error {
				// Held across the check and the start so that a Stop cannot
				// slip in between them: either it runs first and the flag it
				// set is seen here, or it runs after and finds the tunnel
				// up to kill. If the lock cannot be had in time something
				// else is working on tunnels; the next check tries again.
				if err := lockOps(); err != nil {
					return err
				}
				defer unlockOps()
				var current Tunnel
				if err := db.First(&current, t.ID).Error; err != nil || current.ManualStop {
					return errNotWanted
				}
				return startLocked(&current)
			},
		}, watchdogInterval)
	})
	defaultWatchdog.Ensure()
}

// SetManualStop records whether the operator has deliberately stopped the
// tunnel. The watchdog never restarts a tunnel for which this is true.
func SetManualStop(db *gorm.DB, id uint, stopped bool) error {
	return db.Model(&Tunnel{}).Where("id = ?", id).Update("manual_stop", stopped).Error
}
