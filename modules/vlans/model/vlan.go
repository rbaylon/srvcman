// Package Vlans - User specific functions
package Vlanmodel

import (
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

type Vlan struct {
	gorm.Model
	Name         string `json:"name"`
	ParentDevice string `json:"parent_device"`
	VlanTag      int    `json:"vlan_tag"`
	PfconfigID   uint   `json:"pfconfig_id"`
	RxPrio       int    `json:"rx_prio"`
	TxPrio       int    `json:"tx_prio"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Vlan{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Vlan) Bind(r *http.Request) error {
	if err := validate.Alnum("name", a.Name, 100); err != nil {
		return err
	}
	if err := validate.Alnum("parent_device", a.ParentDevice, 100); err != nil {
		return err
	}
	// 802.1Q reserves VLAN IDs 0 and 4095, so the usable range is 1-4094.
	if err := validate.IntRange("vlan_tag", a.VlanTag, 1, 4094); err != nil {
		return err
	}
	// pf's "set prio" only accepts 0-7.
	if err := validate.IntRange("rx_prio", a.RxPrio, 0, 7); err != nil {
		return err
	}
	return validate.IntRange("tx_prio", a.TxPrio, 0, 7)
}

type Crud interface {
	GetAll() ([]Vlan, error)
	GetById(uid uint) (*Vlan, error)
	GetByName(name string) (*Vlan, error)
	Add(Vlan *Vlan) error
	Update(Vlan *Vlan) error
	Delete(Vlan *Vlan) error
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

func (s *Storage) Add(vlan *Vlan) error {
	result := s.DB.Create(vlan)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetByName(name string) (*Vlan, error) {
	var vlan Vlan
	result := s.DB.Where("Name = ?", name).First(&vlan)
	if result.Error != nil {
		return nil, result.Error
	}
	return &vlan, nil
}

func (s *Storage) GetAll() ([]Vlan, error) {
	var vlans []Vlan
	result := s.DB.Find(&vlans)
	if result.Error != nil {
		return nil, result.Error
	}
	return vlans, nil
}

func (s *Storage) GetById(id uint) (*Vlan, error) {
	var vlan Vlan
	result := s.DB.First(&vlan, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &vlan, nil
}

func (s *Storage) Update(vlan *Vlan) error {
	result := s.DB.Save(vlan)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(vlan *Vlan) error {
	result := s.DB.Delete(vlan)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
