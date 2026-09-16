// Package rworkers - Rworker specific functions
package rworker

import (
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

type Rworker struct {
	gorm.Model
	Name       string `json:"name"`
	PfconfigID uint   `json:"pfconfig_id"`
}

type Rworkers struct {
	Rworkers []Rworker `json:"rworkers"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Rworker{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Rworker) Bind(r *http.Request) error {
	return validate.Alnum("name", a.Name, 100)
}

func (a *Rworkers) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Rworker, error)
	GetById(uid uint) (*Rworker, error)
	Add(rworker *Rworker) error
	AddBulk(rworkers []Rworker) error
	Update(rworker *Rworker) error
	Delete(rworker *Rworker) error
	GetByRworkerValue(value string) (*Rworker, error)
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

func (s *Storage) AddBulk(rworkers []Rworker) error {
	result := s.DB.Create(rworkers)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
}

func (s *Storage) Add(rworker *Rworker) error {
	result := s.DB.Create(rworker)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Rworker, error) {
	var rworkers []Rworker
	result := s.DB.Find(&rworkers)
	if result.Error != nil {
		return nil, result.Error
	}
	return rworkers, nil
}

func (s *Storage) GetById(id uint) (*Rworker, error) {
	var rworker Rworker
	result := s.DB.First(&rworker, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &rworker, nil
}

func (s *Storage) GetByRworkerValue(value string) (*Rworker, error) {
	var rworker Rworker
	result := s.DB.Where("Value = ?", value).First(&rworker)
	if result.Error != nil {
		return nil, result.Error
	}
	return &rworker, nil
}

func (s *Storage) Update(rworker *Rworker) error {
	result := s.DB.Save(rworker)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(rworker *Rworker) error {
	result := s.DB.Delete(rworker)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
