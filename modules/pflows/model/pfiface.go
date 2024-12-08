// Package pflows - Pflow specific functions
package pflowmodel

import (
	"log"
	"net/http"

	"gorm.io/gorm"
)

type Pflow struct {
	gorm.Model
	Device     string `json:"device"`
	Src        string `json:"src"`
	Dst        string `json:"dst"`
	Proto      int    `json:"proto"`
	PfconfigID uint   `json:"pfconfig_id"`
}

type Pflows struct {
	Pflows []Pflow `json:"pflows"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Pflow{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Pflow) Bind(r *http.Request) error {
	return nil
}

func (a *Pflows) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Pflow, error)
	GetById(uid uint) (*Pflow, error)
	Add(pflow *Pflow) error
	AddBulk(pflows []Pflow) error
	Update(pflow *Pflow) error
	Delete(pflow *Pflow) error
	GetByPflowValue(value string) (*Pflow, error)
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

func (s *Storage) AddBulk(pflows []Pflow) error {
	result := s.DB.Create(pflows)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
}

func (s *Storage) Add(pflow *Pflow) error {
	result := s.DB.Create(pflow)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Pflow, error) {
	var pflows []Pflow
	result := s.DB.Find(&pflows)
	if result.Error != nil {
		return nil, result.Error
	}
	return pflows, nil
}

func (s *Storage) GetById(id uint) (*Pflow, error) {
	var pflow Pflow
	result := s.DB.First(&pflow, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &pflow, nil
}

func (s *Storage) GetByPflowValue(value string) (*Pflow, error) {
	var pflow Pflow
	result := s.DB.Where("Value = ?", value).First(&pflow)
	if result.Error != nil {
		return nil, result.Error
	}
	return &pflow, nil
}

func (s *Storage) Update(pflow *Pflow) error {
	result := s.DB.Save(pflow)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(pflow *Pflow) error {
	result := s.DB.Delete(pflow)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
