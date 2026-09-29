// Package acme - Acme specific functions
package acmemodel

import (
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

// Acme is the gateway's Let's Encrypt certificate configuration: which
// name to get a certificate for, and whether that is currently switched
// on. It is a single-entry config - one box serves one management FQDN -
// so GenerateConf renders one record and the UI only offers to create
// one.
//
// Enabled is what gates the work: applying a disabled record is a no-op,
// not a revert. Turning it off stops srvcman from reissuing or touching
// nginx again; it deliberately does not put the self-signed paths back,
// since that would pull the certificate out from under a running site as
// a side effect of unticking a box.
//
// Fqdn is validated as a strict hostname rather than a free-form string
// because arkgate builds both a command argument (acme-client -v <fqdn>)
// and two file paths (/etc/ssl/<fqdn>.fullchain.pem,
// /etc/ssl/private/<fqdn>.key) out of it.
type Acme struct {
	gorm.Model
	Fqdn    string `json:"fqdn"`
	Enabled bool   `json:"enabled"`
}

type Acmes struct {
	Acmes []Acme `json:"acmes"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Acme{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Acme) Bind(r *http.Request) error {
	if err := validate.Required("fqdn", a.Fqdn); err != nil {
		return err
	}
	return validate.FQDN("fqdn", a.Fqdn, 253)
}

func (a *Acmes) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Acme, error)
	GetById(uid uint) (*Acme, error)
	Add(acme *Acme) error
	Update(acme *Acme) error
	Delete(acme *Acme) error
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

func (s *Storage) Add(acme *Acme) error {
	result := s.DB.Create(acme)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Acme, error) {
	var acmes []Acme
	result := s.DB.Find(&acmes)
	if result.Error != nil {
		return nil, result.Error
	}
	return acmes, nil
}

func (s *Storage) GetById(id uint) (*Acme, error) {
	var acme Acme
	result := s.DB.First(&acme, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &acme, nil
}

// Update saves the record. Save (rather than Updates) matters here:
// Updates skips zero values for a struct argument, and Enabled=false -
// unticking the box - is exactly the value that has to persist.
func (s *Storage) Update(acme *Acme) error {
	result := s.DB.Save(acme)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(acme *Acme) error {
	result := s.DB.Delete(acme)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
