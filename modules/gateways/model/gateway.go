// Package gateways - Gateway specific functions
package gatewaymodel

import (
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

// Gateway represents a routable gateway that firewall rules and other
// modules can point traffic at (pf's "route-to"). Ip and Default mirror the
// fields of the same name on pfifacemodel.Pfiface; MonitorIp is the address
// this gateway's link health is checked against (e.g. a ping target used to
// detect an upstream outage and fail over).
type Gateway struct {
	gorm.Model
	Name       string `json:"name"`
	Ip         string `json:"ip"`
	Default    bool   `json:"default"`
	MonitorIp  string `json:"monitor_ip"`
	PfconfigID uint   `json:"pfconfig_id"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Gateway{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Gateway) Bind(r *http.Request) error {
	if err := validate.Alnum("name", a.Name, 100); err != nil {
		return err
	}
	if err := validate.IP("ip", a.Ip); err != nil {
		return err
	}
	return validate.IP("monitor_ip", a.MonitorIp)
}

type Crud interface {
	GetAll() ([]Gateway, error)
	GetById(uid uint) (*Gateway, error)
	Add(gateway *Gateway) error
	Update(gateway *Gateway) error
	Delete(gateway *Gateway) error
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

func (s *Storage) Add(gateway *Gateway) error {
	result := s.DB.Create(gateway)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Gateway, error) {
	var gateways []Gateway
	result := s.DB.Find(&gateways)
	if result.Error != nil {
		return nil, result.Error
	}
	return gateways, nil
}

func (s *Storage) GetById(id uint) (*Gateway, error) {
	var gateway Gateway
	result := s.DB.First(&gateway, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &gateway, nil
}

func (s *Storage) Update(gateway *Gateway) error {
	result := s.DB.Save(gateway)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(gateway *Gateway) error {
	result := s.DB.Delete(gateway)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
