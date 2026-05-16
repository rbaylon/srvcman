// Package runtimes - Runtime specific functions
package runtimemodel

import (
	"log"
	"net/http"

	"gorm.io/gorm"
)

type Runtime struct {
	gorm.Model
	Action   string `json:"action"`
	Status   string `json:"status"`
	Workerid int    `json:"workerid"`
}

type Runtimes struct {
	Runtimes []Runtime `json:"runtimes"`
}

// Bind interface as required by go-chi/render
func (a *Runtime) Bind(r *http.Request) error {
	return nil
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Runtime{})
	if err != nil {
		log.Fatal(err)
	}
}

type Crud interface {
	GetAll() ([]Runtime, error)
	GetById(uid uint) (*Runtime, error)
	GetByWorkerId(wid uint) (*Runtime, error)
	Add(runtime *Runtime) error
	Update(runtime *Runtime) error
	Delete(runtime *Runtime) error
	GetByAction(value string, wid int) (*Runtime, error)
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

func (s *Storage) Add(runtime *Runtime) error {
	result := s.DB.Create(runtime)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Runtime, error) {
	var runtimes []Runtime
	result := s.DB.Find(&runtimes)
	if result.Error != nil {
		return nil, result.Error
	}
	return runtimes, nil
}

func (s *Storage) GetById(id uint) (*Runtime, error) {
	var runtime Runtime
	result := s.DB.First(&runtime, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &runtime, nil
}

func (s *Storage) GetByWorkerId(wid uint) (*Runtime, error) {
	var runtime Runtime
	result := s.DB.Where("Workerid = ?", wid).First(&runtime)
	if result.Error != nil {
		return nil, result.Error
	}
	return &runtime, nil
}

func (s *Storage) GetByAction(value string, wid int) (*Runtime, error) {
	var runtime Runtime
	result := s.DB.Where("Action = ? AND Workerid = ?", value, wid).First(&runtime)
	if result.Error != nil {
		return nil, result.Error
	}
	return &runtime, nil
}

func (s *Storage) Update(runtime *Runtime) error {
	result := s.DB.Save(runtime)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(runtime *Runtime) error {
	result := s.DB.Delete(runtime)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
