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

	monitorMu sync.Mutex
	monitors  = map[uint]context.CancelFunc{}
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
func pingOnce(ip string) bool {
	if ip == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout+time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-w", fmt.Sprintf("%d", int(pingTimeout.Seconds())), ip)
	return cmd.Run() == nil
}

// StartMonitor spawns a goroutine that periodically pings id's MonitorIp,
// re-reading it from the DB on every tick so an Update takes effect without
// a restart, and records the result for Status to report. It's a no-op if a
// monitor for id is already running.
func StartMonitor(db *gorm.DB, id uint) {
	monitorMu.Lock()
	defer monitorMu.Unlock()
	if _, ok := monitors[id]; ok {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	monitors[id] = cancel
	go monitorLoop(ctx, db, id)
}

// StopMonitor stops id's monitor goroutine, if running, and clears its
// last-known status.
func StopMonitor(id uint) {
	monitorMu.Lock()
	if cancel, ok := monitors[id]; ok {
		cancel()
		delete(monitors, id)
	}
	monitorMu.Unlock()
	clearStatus(id)
}

// StartAllMonitors starts a monitor goroutine for every existing
// GatewayMonitor row. Call this once at startup, after MigrateDB.
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

// monitorLoop checks id's MonitorIp immediately, then on every tick of
// pingInterval, until ctx is cancelled (StopMonitor) or the record no
// longer exists (deleted through some path other than Storage.Delete).
func monitorLoop(ctx context.Context, db *gorm.DB, id uint) {
	check := func() bool {
		var gw GatewayMonitor
		if err := db.First(&gw, id).Error; err != nil {
			return false
		}
		setStatus(id, pingOnce(gw.MonitorIp))
		return true
	}
	if !check() {
		StopMonitor(id)
		return
	}
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !check() {
				StopMonitor(id)
				return
			}
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
