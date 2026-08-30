// Package plans - Plan specific functions
package planmodel

import (
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

type Plan struct {
	gorm.Model
	Plan          string `json:"plan"`
	Downspeed     int    `json:"downspeed"`
	Upspeed       int    `json:"upspeed"`
	SpeedTestDown int    `json:"stdown"`
	SpeedTestUp   int    `json:"stup"`
	PfconfigID    uint   `json:"pfconfig_id"`
}

// MigrateDB - Create the plan if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Plan{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Plan) Bind(r *http.Request) error {
	if err := validate.Alnum("plan", a.Plan, 100); err != nil {
		return err
	}
	if err := validate.Int("downspeed", a.Downspeed); err != nil {
		return err
	}
	if err := validate.Int("upspeed", a.Upspeed); err != nil {
		return err
	}
	if err := validate.Int("stdown", a.SpeedTestDown); err != nil {
		return err
	}
	return validate.Int("stup", a.SpeedTestUp)
}

type Crud interface {
	GetAll() ([]Plan, error)
	GetById(uid uint) (*Plan, error)
	Add(plan *Plan) error
	Update(plan *Plan) error
	Delete(plan *Plan) error
	GetDB() *gorm.DB
}

type Storage struct {
	DB *gorm.DB
}

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
}

func New(db *gorm.DB) *Storage {
	return &Storage{
		DB: db,
	}
}

func (s *Storage) Add(plan *Plan) error {
	result := s.DB.Create(plan)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Plan, error) {
	var plans []Plan
	result := s.DB.Find(&plans)
	if result.Error != nil {
		return nil, result.Error
	}
	return plans, nil
}

func (s *Storage) GetById(id uint) (*Plan, error) {
	var plan Plan
	result := s.DB.First(&plan, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &plan, nil
}

func (s *Storage) Update(plan *Plan) error {
	result := s.DB.Save(plan)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(plan *Plan) error {
	result := s.DB.Delete(plan)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
