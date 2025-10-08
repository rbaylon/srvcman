// Package Fws - User specific functions
package fwmodel

import (
	"fmt"
	"log"
	"net/http"

	"gorm.io/gorm"
)

type Fw struct {
	gorm.Model
	Status        string `json:"status"`
	Action        string `json:"action"`
	Direction     string `json:"direction"`
	Log           string `json:"log"`
	Quick         bool   `json:"quick"`
	Interface     string `json:"interface"`
	AddressFamily string `json:"address_family"`
	Proto         string `json:"protocol"`
	Source        string `json:"source"`
	SrcPort       string `json:"source_port"`
	Destination   string `json:"destination"`
	DstPort       string `json:"destination_port"`
	Gateway       string `json:"gateway"`
	Priority      int    `json:"priority"`
	PfconfigID    uint   `json:"pfconfig_id"`
	Pipe          string `json:"pipe"`
	AdvanceOpts   string `json:"advance_options"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Fw{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Fw) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll(opt string) ([]Pfrule, error)
	GetById(uid uint) (*Fw, error)
	Add(Fw *Fw) error
	Update(Fw *Fw) error
	Delete(Fw *Fw) error
	GetDB() *gorm.DB
}

type Storage struct {
	DB *gorm.DB
}

type Pfrule struct {
	ID   uint
	Rule string
}

func New(db *gorm.DB) *Storage {
	return &Storage{
		DB: db,
	}
}

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
}

func (s *Storage) Add(Fw *Fw) error {
	result := s.DB.Create(Fw)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll(option string) ([]Pfrule, error) {
	var Fws []Fw
	result := s.DB.Find(&Fws)
	if result.Error != nil {
		return nil, result.Error
	}
	prs := []Pfrule{}
	if option != "" {
		pr := Pfrule{}
		for _, v := range Fws {
			pr.ID = v.ID
			quick := ""
			if v.Quick {
				quick = "quick"
			}
			pr.Rule = fmt.Sprintf("%s %s %s %s on %s %s %s from %s %s to %s %s %s %d %s\n", v.Action, v.Direction,
				v.Log, quick, v.Interface, v.AddressFamily, v.Proto, v.Source, v.SrcPort, v.Destination, v.DstPort,
				v.Gateway, v.Priority, v.Pipe)
			prs = append(prs, pr)
		}
	}
	return prs, nil
}

func (s *Storage) GetById(id uint) (*Fw, error) {
	var Fw Fw
	result := s.DB.First(&Fw, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &Fw, nil
}

func (s *Storage) Update(Fw *Fw) error {
	result := s.DB.Save(Fw)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(Fw *Fw) error {
	result := s.DB.Delete(Fw)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
