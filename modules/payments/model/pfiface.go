// Package payments - Payment specific functions
package paymentmodel

import (
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

type Payment struct {
	gorm.Model
	DatePaid   string `json:"date_paid"`
	ScreenShot string `json:"screen_shot"`
	AmountPaid int    `json:"amount_paid"`
	SubID      uint   `json:"sub_id"`
}

type Payments struct {
	Payments []Payment `json:"payments"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Payment{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Payment) Bind(r *http.Request) error {
	if err := validate.Date("date_paid", a.DatePaid, "2006-01-02"); err != nil {
		return err
	}
	// ScreenShot holds a stored filename/reference, not raw image data, so
	// it's treated as a path-like token rather than plain Alnum.
	if err := validate.SafeToken("screen_shot", a.ScreenShot, 100); err != nil {
		return err
	}
	return validate.Int("amount_paid", a.AmountPaid)
}

func (a *Payments) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Payment, error)
	GetBySubId(id uint) ([]Payment, error)
	GetById(uid uint) (*Payment, error)
	Add(payment *Payment) error
	AddBulk(payments []Payment) error
	Update(payment *Payment) error
	Delete(payment *Payment) error
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

func (s *Storage) AddBulk(payments []Payment) error {
	result := s.DB.Create(payments)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
}

func (s *Storage) Add(payment *Payment) error {
	result := s.DB.Create(payment)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Payment, error) {
	var payments []Payment
	result := s.DB.Find(&payments)
	if result.Error != nil {
		return nil, result.Error
	}
	return payments, nil
}

func (s *Storage) GetById(id uint) (*Payment, error) {
	var payment Payment
	result := s.DB.First(&payment, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &payment, nil
}

func (s *Storage) GetBySubId(value uint) ([]Payment, error) {
	var payments []Payment
	result := s.DB.Where("sub_id = ?", value).Find(&payments)
	if result.Error != nil {
		return nil, result.Error
	}
	return payments, nil
}

func (s *Storage) Update(payment *Payment) error {
	result := s.DB.Save(payment)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(payment *Payment) error {
	result := s.DB.Delete(payment)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
