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
	TxPrio       int    `json:"txprio"`
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

func (s *Storage) Add(Vlan *Vlan) error {
	result := s.DB.Create(Vlan)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll(option string) ([]Vlan, error) {
	var Vlans []Vlan
	result := s.DB.Find(&Vlans)
	if result.Error != nil {
		return nil, result.Error
	}
	return Vlans, nil
}

func (s *Storage) GetById(id uint) (*Vlan, error) {
	var Vlan Vlan
	result := s.DB.First(&Vlan, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &Vlan, nil
}

func (s *Storage) Update(Vlan *Vlan) error {
	result := s.DB.Save(Vlan)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(Vlan *Vlan) error {
	result := s.DB.Delete(Vlan)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
