// Package Vlans - User specific functions
package Vlanmodel

import (
	"log"
	"net/http"

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
	return nil
}

type Crud interface {
	GetAll(opt string) ([]Vlan, error)
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

func (s *Storage) GetAll(option string) ([]Vlan, error) {
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
