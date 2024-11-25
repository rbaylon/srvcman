// Package pfifaces - Pfiface specific functions
package pfifacemodel

import (
	"log"
	"net/http"

	"gorm.io/gorm"
)

type Pfiface struct {
	gorm.Model
	Name       string `json:"name"`
	Speed      string `json:"speed"`
	Device     string `json:"device"`
	Default    bool   `json:"default"`
	Type       string `json:"type"`
	Gateway    string `json:"gateway"`
	Ip         string `json:"ip"`
	Netmask    string `json:"netmask"`
	PfconfigID uint   `json:"pfconfig_id"`
}

type Pfifaces struct {
	Pfifaces []Pfiface `json:"pfifaces"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Pfiface{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Pfiface) Bind(r *http.Request) error {
	return nil
}

func (a *Pfifaces) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Pfiface, error)
	GetById(uid uint) (*Pfiface, error)
	Add(pfiface *Pfiface) error
	AddBulk(pfifaces []Pfiface) error
	Update(pfiface *Pfiface) error
	Delete(pfiface *Pfiface) error
	GetByPfifaceValue(value string) (*Pfiface, error)
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

func (s *Storage) AddBulk(pfifaces []Pfiface) error {
	result := s.DB.Create(pfifaces)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
}

func (s *Storage) Add(pfiface *Pfiface) error {
	result := s.DB.Create(pfiface)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Pfiface, error) {
	var pfifaces []Pfiface
	result := s.DB.Find(&pfifaces)
	if result.Error != nil {
		return nil, result.Error
	}
	return pfifaces, nil
}

func (s *Storage) GetById(id uint) (*Pfiface, error) {
	var pfiface Pfiface
	result := s.DB.First(&pfiface, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &pfiface, nil
}

func (s *Storage) GetByPfifaceValue(value string) (*Pfiface, error) {
	var pfiface Pfiface
	result := s.DB.Where("Value = ?", value).First(&pfiface)
	if result.Error != nil {
		return nil, result.Error
	}
	return &pfiface, nil
}

func (s *Storage) Update(pfiface *Pfiface) error {
	result := s.DB.Save(pfiface)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(pfiface *Pfiface) error {
	result := s.DB.Delete(pfiface)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
