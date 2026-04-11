// Package changes - change specific functions
package changemodel

import (
	"log"
	"net/http"

	"gorm.io/gorm"
)

type Change struct {
	gorm.Model
	Module  string `json:"module"`
	Data    string `json:"data"`
	Applied bool   `json:"applied"`
}

type Changes struct {
	Changes []Change `json:"changes"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Change{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Change) Bind(r *http.Request) error {
	return nil
}

func (a *Changes) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Change, error)
	GetBySubId(id uint) ([]Change, error)
	GetById(uid uint) (*Change, error)
	Add(change *Change) error
	AddBulk(changes []Change) error
	Update(change *Change) error
	Delete(change *Change) error
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

func (s *Storage) AddBulk(changes []Change) error {
	result := s.DB.Create(changes)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
}

func (s *Storage) Add(change *Change) error {
	result := s.DB.Create(change)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Change, error) {
	var changes []Change
	result := s.DB.Find(&changes)
	if result.Error != nil {
		return nil, result.Error
	}
	return changes, nil
}

func (s *Storage) GetById(id uint) (*Change, error) {
	var change Change
	result := s.DB.First(&change, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &change, nil
}

func (s *Storage) GetBySubId(value uint) ([]Change, error) {
	var changes []Change
	result := s.DB.Where("sub_id = ?", value).Find(&changes)
	if result.Error != nil {
		return nil, result.Error
	}
	return changes, nil
}

func (s *Storage) Update(change *Change) error {
	result := s.DB.Save(change)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(change *Change) error {
	result := s.DB.Delete(change)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
