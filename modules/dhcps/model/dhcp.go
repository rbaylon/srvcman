// Package dhcps - Dhcp specific functions
package dhcpmodel

import (
	"log"
	"net/http"

	"gorm.io/gorm"
)

type Dhcp struct {
	gorm.Model
	Subnet     string `json:"subnet"`
	Netmask    string `json:"netmask"`
	Routers    string `json:"routers"`
	Dnsservers string `json:"dnsservers"`
	Range      string `json:"range"`
	Type       string `json:"type"`
	PfconfigID uint   `json:"pfconfig_id"`
}

type Dhcps struct {
	Dhcps []Dhcp `json:"dhcps"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Dhcp{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Dhcp) Bind(r *http.Request) error {
	return nil
}

func (a *Dhcps) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Dhcp, error)
	GetById(uid uint) (*Dhcp, error)
	Add(dhcp *Dhcp) error
	AddBulk(dhcps []Dhcp) error
	Update(dhcp *Dhcp) error
	Delete(dhcp *Dhcp) error
	GetByDhcpValue(value string) (*Dhcp, error)
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

func (s *Storage) AddBulk(dhcps []Dhcp) error {
	result := s.DB.Create(dhcps)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
}

func (s *Storage) Add(dhcp *Dhcp) error {
	result := s.DB.Create(dhcp)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Dhcp, error) {
	var dhcps []Dhcp
	result := s.DB.Find(&dhcps)
	if result.Error != nil {
		return nil, result.Error
	}
	return dhcps, nil
}

func (s *Storage) GetById(id uint) (*Dhcp, error) {
	var dhcp Dhcp
	result := s.DB.First(&dhcp, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &dhcp, nil
}

func (s *Storage) GetByDhcpValue(value string) (*Dhcp, error) {
	var dhcp Dhcp
	result := s.DB.Where("Value = ?", value).First(&dhcp)
	if result.Error != nil {
		return nil, result.Error
	}
	return &dhcp, nil
}

func (s *Storage) Update(dhcp *Dhcp) error {
	result := s.DB.Save(dhcp)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(dhcp *Dhcp) error {
	result := s.DB.Delete(dhcp)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
