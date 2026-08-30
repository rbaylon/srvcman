// Package tunnels - Tunnel specific functions
package tunnelmodel

import (
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

type Tunnel struct {
	gorm.Model
	Router string `json:"router" gorm:"unique;not null"`
	Port   int    `json:"port" gorm:"unique;not null"`
	Active bool   `json:"active"`
	Pid    int    `json:"pid"`
}

type Tunnels struct {
	Tunnels []Tunnel `json:"tunnels"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Tunnel{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Tunnel) Bind(r *http.Request) error {
	if err := validate.Required("router", a.Router); err != nil {
		return err
	}
	if err := validate.Alnum("router", a.Router, 100); err != nil {
		return err
	}
	if err := validate.IntRange("port", a.Port, 1, 65535); err != nil {
		return err
	}
	return validate.Int("pid", a.Pid)
}

func (a *Tunnels) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Tunnel, error)
	GetBySubId(id uint) ([]Tunnel, error)
	GetById(uid uint) (*Tunnel, error)
	Add(tunnel *Tunnel) error
	AddBulk(tunnels []Tunnel) error
	Update(tunnel *Tunnel) error
	Delete(tunnel *Tunnel) error
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

func (s *Storage) AddBulk(tunnels []Tunnel) error {
	result := s.DB.Create(tunnels)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
}

func (s *Storage) Add(tunnel *Tunnel) error {
	result := s.DB.Create(tunnel)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Tunnel, error) {
	var tunnels []Tunnel
	result := s.DB.Find(&tunnels)
	if result.Error != nil {
		return nil, result.Error
	}
	return tunnels, nil
}

func (s *Storage) GetById(id uint) (*Tunnel, error) {
	var tunnel Tunnel
	result := s.DB.First(&tunnel, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &tunnel, nil
}

func (s *Storage) GetBySubId(value uint) ([]Tunnel, error) {
	var tunnels []Tunnel
	result := s.DB.Where("sub_id = ?", value).Find(&tunnels)
	if result.Error != nil {
		return nil, result.Error
	}
	return tunnels, nil
}

func (s *Storage) Update(tunnel *Tunnel) error {
	result := s.DB.Save(tunnel)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(tunnel *Tunnel) error {
	result := s.DB.Delete(tunnel)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
