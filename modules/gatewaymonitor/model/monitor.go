package gatewaymonitormodel

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"

	Acmd "github.com/rbaylon/arkgatecmd"
	"github.com/rbaylon/srvcman/database"
	"github.com/rbaylon/srvcman/utils"
	"gorm.io/gorm"
)

// pingInterval is how often a gateway's MonitorIp is health-checked.
const pingInterval = 10 * time.Second

// pingTimeout bounds how long a single health-check ping is allowed to run.
const pingTimeout = 2 * time.Second

// Status values reported on GatewayMonitor.Status.
const (
	StatusUnknown = "unknown"
	StatusUp      = "up"
	StatusDown    = "down"
)

var (
	statusMu sync.RWMutex
	statuses = map[uint]string{}

	// monitorMu guards activeIDs, loopCancel and loopRunning below. A single
	// shared goroutine (monitorLoop) checks every id in activeIDs on one
	// ticker via one batched query per tick, rather than the previous
	// one-goroutine-and-one-query-per-monitor design - under N monitors that
	// meant N concurrent goroutines hitting the shared SQLite connection
	// every tick, which under load caused ever-slower queries and eventually
	// crashed the process inside go-sqlite3's cgo layer (incident
	// 2026-09-05). See database.ConnectToDB, which now also caps the pool
	// to a single connection and adds a busy-timeout - this batching change
	// is the other half of that fix, cutting concurrent query count from N
	// to 1 regardless of how many gateways are monitored.
	monitorMu   sync.Mutex
	activeIDs   = map[uint]bool{}
	loopCancel  context.CancelFunc
	loopRunning bool
)

func getStatus(id uint) string {
	statusMu.RLock()
	defer statusMu.RUnlock()
	if s, ok := statuses[id]; ok {
		return s
	}
	return StatusUnknown
}

func setStatus(id uint, up bool) {
	statusMu.Lock()
	defer statusMu.Unlock()
	if up {
		statuses[id] = StatusUp
	} else {
		statuses[id] = StatusDown
	}
}

func clearStatus(id uint) {
	statusMu.Lock()
	defer statusMu.Unlock()
	delete(statuses, id)
}

// pingOnce reports whether ip answered a single ping within pingTimeout.
// It's derived from the monitor's own ctx so cancelling that ctx (StopMonitor
// / StopAllMonitors) kills an in-flight ping process immediately instead of
// leaving it to run out its own timeout.
func pingOnce(ctx context.Context, ip string) bool {
	if ip == "" {
		return false
	}
	pctx, cancel := context.WithTimeout(ctx, pingTimeout+time.Second)
	defer cancel()
	cmd := exec.CommandContext(pctx, "ping", "-c", "1", "-w", fmt.Sprintf("%d", int(pingTimeout.Seconds())), ip)
	return cmd.Run() == nil
}

// StartMonitor registers id for health-checking (a no-op if id is already
// registered) and starts the shared monitor loop if it isn't already
// running. The loop re-reads every registered id from the DB on every tick
// in one batched query, so an Update to id's MonitorIp takes effect without
// a restart, same as before.
func StartMonitor(db *gorm.DB, id uint) {
	monitorMu.Lock()
	defer monitorMu.Unlock()
	activeIDs[id] = true
	if !loopRunning {
		ctx, cancel := context.WithCancel(context.Background())
		loopCancel = cancel
		loopRunning = true
		go monitorLoop(ctx, db)
	}
}

// StopMonitor stops health-checking id and clears its last-known status.
// The shared loop keeps running (it's a no-op per tick once activeIDs is
// empty) so a later StartMonitor doesn't need to restart anything.
func StopMonitor(id uint) {
	monitorMu.Lock()
	delete(activeIDs, id)
	monitorMu.Unlock()
	clearStatus(id)
}

// StopAllMonitors stops the shared monitor loop, killing any in-flight ping
// process, and clears every status. Call this during graceful shutdown
// (e.g. on SIGTERM/SIGINT) so the background goroutine doesn't get
// abandoned mid-request when the process exits.
func StopAllMonitors() {
	monitorMu.Lock()
	if loopCancel != nil {
		loopCancel()
		loopCancel = nil
	}
	loopRunning = false
	activeIDs = map[uint]bool{}
	monitorMu.Unlock()

	statusMu.Lock()
	statuses = map[uint]string{}
	statusMu.Unlock()
}

// StartAllMonitors registers every existing GatewayMonitor row and starts
// the shared monitor loop. Call this once at startup, after MigrateDB.
func StartAllMonitors(db *gorm.DB) {
	var gateways []GatewayMonitor
	if err := db.Find(&gateways).Error; err != nil {
		log.Println("gatewaymonitor: failed to load records for monitoring:", err)
		return
	}
	for _, g := range gateways {
		StartMonitor(db, g.ID)
	}
}

// monitorLoop checks every registered monitor immediately, then on every
// tick of pingInterval, until ctx is cancelled (StopAllMonitors). Each tick
// does exactly one batched query for every currently-registered id (instead
// of one query per monitor per tick), then pings them concurrently; an id
// whose row no longer exists (deleted through some path other than
// Storage.Delete) is dropped from activeIDs.
func monitorLoop(ctx context.Context, db *gorm.DB) {
	check := func() {
		monitorMu.Lock()
		ids := make([]uint, 0, len(activeIDs))
		for id := range activeIDs {
			ids = append(ids, id)
		}
		monitorMu.Unlock()
		if len(ids) == 0 {
			return
		}

		var gateways []GatewayMonitor
		if err := db.Find(&gateways, ids).Error; err != nil {
			log.Println("gatewaymonitor: batch load failed:", err)
			return
		}

		found := make(map[uint]bool, len(gateways))
		var wg sync.WaitGroup
		for _, gw := range gateways {
			found[gw.ID] = true
			wg.Add(1)
			go func(id uint, ip string) {
				defer wg.Done()
				setStatus(id, pingOnce(ctx, ip))
			}(gw.ID, gw.MonitorIp)
		}
		wg.Wait()

		monitorMu.Lock()
		for id := range activeIDs {
			if !found[id] {
				delete(activeIDs, id)
				clearStatus(id)
			}
		}
		monitorMu.Unlock()
	}

	check()
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}

// routeBin is the OpenBSD route(8) binary, invoked via arkgated (see
// sendArkCmd) since modifying the routing table needs privileges srvcman's
// own process doesn't have.
const routeBin = "/sbin/route"

// sendArkCmd hands a command to arkgated over its Unix socket for
// privileged execution - the same mechanism modules/changes uses to run
// pfctl/mv as root. srvcman itself never execs route(8) directly.
func sendArkCmd(name, binpath string, opts []string) error {
	usocket := database.GetEnvVariable("UNIX_SOCKET")
	cmd := Acmd.PrepareCmd(name, binpath, opts)
	return cmd.SendCmd(utils.GetUnixConn(usocket))
}

// applyRoute installs a labeled OpenBSD host route so traffic to monitorIP
// goes out via gatewayIP: `route add -host <monitorIP> <gatewayIP> -label <name>`.
func applyRoute(name, monitorIP, gatewayIP string) error {
	if monitorIP == "" || gatewayIP == "" {
		return nil
	}
	opts := []string{"add", "-host", monitorIP, gatewayIP, "-label", name}
	if err := sendArkCmd("AddRoute: "+name, routeBin, opts); err != nil {
		return fmt.Errorf("route add -host %s %s -label %s: %w", monitorIP, gatewayIP, name, err)
	}
	return nil
}

// removeRoute removes the host route to monitorIP, if any.
func removeRoute(monitorIP string) error {
	if monitorIP == "" {
		return nil
	}
	opts := []string{"delete", "-host", monitorIP}
	if err := sendArkCmd("DeleteRoute: "+monitorIP, routeBin, opts); err != nil {
		return fmt.Errorf("route delete -host %s: %w", monitorIP, err)
	}
	return nil
}
