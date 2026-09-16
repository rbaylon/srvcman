// Package gatewaymonitor - GatewayMonitor specific functions
package gatewaymonitormodel

import (
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

// GatewayMonitor represents a routable gateway that firewall rules and
// other modules can point traffic at (pf's "route-to"), tracked alongside a
// MonitorIp - the address this gateway's link health is checked against
// (e.g. a ping target used to detect an upstream outage and fail over).
//
// Creating a record installs an OpenBSD host route ("route add -host
// MonitorIp Ip -label Name") so traffic to MonitorIp actually goes out via
// this gateway, and starts a background goroutine that pings MonitorIp on
// an interval and records the result; Update/Delete keep the route and
// goroutine in sync. The route(8) calls are dispatched to arkgated over its
// Unix socket rather than exec'd directly, since modifying the routing
// table needs privileges srvcman's own process doesn't have (see
// sendArkCmd in monitor.go). Status is the ping goroutine's last result;
// SuccessCount/FailedCount are how many of its health checks in the last
// 5 minutes (see historyWindow) succeeded vs. failed. None of the three
// are stored in the DB, only held in memory.
type GatewayMonitor struct {
	gorm.Model
	Name         string `json:"name"`
	Ip           string `json:"ip"`
	MonitorIp    string `json:"monitor_ip"`
	PfconfigID   uint   `json:"pfconfig_id"`
	Status       string `json:"status" gorm:"-"`
	SuccessCount int    `json:"success_count" gorm:"-"`
	FailedCount  int    `json:"failed_count" gorm:"-"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&GatewayMonitor{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *GatewayMonitor) Bind(r *http.Request) error {
	if err := validate.Alnum("name", a.Name, 100); err != nil {
		return err
	}
	if err := validate.IP("ip", a.Ip); err != nil {
		return err
	}
	return validate.IP("monitor_ip", a.MonitorIp)
}

type Crud interface {
	GetAll() ([]GatewayMonitor, error)
	GetById(uid uint) (*GatewayMonitor, error)
	Add(gatewaymonitor *GatewayMonitor) error
	Update(gatewaymonitor *GatewayMonitor) error
	Delete(gatewaymonitor *GatewayMonitor) error
	GetDB() *gorm.DB
}

type Storage struct {
	DB *gorm.DB
}

func New(db *gorm.DB) *Storage {
	return &Storage{
		DB: db,
	}
}

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
}

func (s *Storage) Add(gatewaymonitor *GatewayMonitor) error {
	result := s.DB.Create(gatewaymonitor)
	if result.Error != nil {
		return result.Error
	}
	if err := applyRoute(gatewaymonitor.Name, gatewaymonitor.MonitorIp, gatewaymonitor.Ip); err != nil {
		log.Println("gatewaymonitor: failed to add route:", err)
	}
	StartMonitor(s.DB, gatewaymonitor.ID)
	return nil
}

// populateMonitorStats fills g's in-memory Status/SuccessCount/FailedCount
// from the current monitor state.
func populateMonitorStats(g *GatewayMonitor) {
	g.Status = getStatus(g.ID)
	g.SuccessCount, g.FailedCount = pingCounts(g.ID)
}

func (s *Storage) GetAll() ([]GatewayMonitor, error) {
	var gatewaymonitors []GatewayMonitor
	result := s.DB.Find(&gatewaymonitors)
	if result.Error != nil {
		return nil, result.Error
	}
	for i := range gatewaymonitors {
		populateMonitorStats(&gatewaymonitors[i])
	}
	return gatewaymonitors, nil
}

func (s *Storage) GetById(id uint) (*GatewayMonitor, error) {
	var gatewaymonitor GatewayMonitor
	result := s.DB.First(&gatewaymonitor, id)
	if result.Error != nil {
		return nil, result.Error
	}
	populateMonitorStats(&gatewaymonitor)
	return &gatewaymonitor, nil
}

func (s *Storage) Update(gatewaymonitor *GatewayMonitor) error {
	var old GatewayMonitor
	hasOld := s.DB.First(&old, gatewaymonitor.ID).Error == nil

	result := s.DB.Save(gatewaymonitor)
	if result.Error != nil {
		return result.Error
	}

	if hasOld && old.MonitorIp != "" && (old.MonitorIp != gatewaymonitor.MonitorIp || old.Ip != gatewaymonitor.Ip) {
		if err := removeRoute(old.MonitorIp); err != nil {
			log.Println("gatewaymonitor: failed to remove old route:", err)
		}
	}
	if err := applyRoute(gatewaymonitor.Name, gatewaymonitor.MonitorIp, gatewaymonitor.Ip); err != nil {
		log.Println("gatewaymonitor: failed to update route:", err)
	}
	return nil
}

func (s *Storage) Delete(gatewaymonitor *GatewayMonitor) error {
	result := s.DB.Delete(gatewaymonitor)
	if result.Error != nil {
		return result.Error
	}
	StopMonitor(gatewaymonitor.ID)
	if err := removeRoute(gatewaymonitor.MonitorIp); err != nil {
		log.Println("gatewaymonitor: failed to remove route:", err)
	}
	return nil
}
