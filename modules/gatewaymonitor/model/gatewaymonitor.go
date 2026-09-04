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
type GatewayMonitor struct {
	gorm.Model
	Name       string `json:"name"`
	Ip         string `json:"ip"`
	MonitorIp  string `json:"monitor_ip"`
	PfconfigID uint   `json:"pfconfig_id"`
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
	return nil
}

func (s *Storage) GetAll() ([]GatewayMonitor, error) {
	var gatewaymonitors []GatewayMonitor
	result := s.DB.Find(&gatewaymonitors)
	if result.Error != nil {
		return nil, result.Error
	}
	return gatewaymonitors, nil
}

func (s *Storage) GetById(id uint) (*GatewayMonitor, error) {
	var gatewaymonitor GatewayMonitor
	result := s.DB.First(&gatewaymonitor, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &gatewaymonitor, nil
}

func (s *Storage) Update(gatewaymonitor *GatewayMonitor) error {
	result := s.DB.Save(gatewaymonitor)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(gatewaymonitor *GatewayMonitor) error {
	result := s.DB.Delete(gatewaymonitor)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
