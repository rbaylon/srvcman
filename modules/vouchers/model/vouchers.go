// Package vouchers - Voucher specific functions
package vouchermodel

import (
	"log"
	"net/http"

	"gorm.io/gorm"
)

type Voucher struct {
	gorm.Model
	Value  string `json:"value" bson:"value"`
	Type   string `json:"type" bson:"type"`
	Hours  int    `json:"hours" bson:"hours"`
	Status string `json:"status" bson:"status"`
}

type Vouchers struct {
	Vouchers []Voucher `json:"vouchers"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Voucher{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Voucher) Bind(r *http.Request) error {
	return nil
}

func (a *Vouchers) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Voucher, error)
	GetById(uid uint) (*Voucher, error)
	Add(voucher *Voucher) error
	AddBulk(vouchers []Voucher) error
	Update(voucher *Voucher) error
	Delete(voucher *Voucher) error
	GetByVoucherValue(value string) (*Voucher, error)
}

type Storage struct {
	DB *gorm.DB
}

func New(db *gorm.DB) *Storage {
	return &Storage{
		DB: db,
	}
}

func (s *Storage) AddBulk(vouchers []Voucher) error {
	result := s.DB.Create(vouchers)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Add(voucher *Voucher) error {
	result := s.DB.Create(voucher)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Voucher, error) {
	var vouchers []Voucher
	result := s.DB.Find(&vouchers)
	if result.Error != nil {
		return nil, result.Error
	}
	return vouchers, nil
}

func (s *Storage) GetById(id uint) (*Voucher, error) {
	var voucher Voucher
	result := s.DB.First(&voucher, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &voucher, nil
}

func (s *Storage) GetByVoucherValue(value string) (*Voucher, error) {
	var voucher Voucher
	result := s.DB.Where("Value = ?", value).First(&voucher)
	if result.Error != nil {
		return nil, result.Error
	}
	return &voucher, nil
}

func (s *Storage) Update(voucher *Voucher) error {
	result := s.DB.Save(voucher)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(voucher *Voucher) error {
	result := s.DB.Delete(voucher)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
