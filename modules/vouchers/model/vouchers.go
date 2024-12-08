// Package vouchers - Voucher specific functions
package vouchermodel

import (
	"log"
	"net/http"
	"time"

	runtimemodel "github.com/rbaylon/srvcman/modules/runtimes/model"
	"gorm.io/gorm"
)

type Voucher struct {
	gorm.Model
	Value         string    `json:"value"`
	Type          string    `json:"type"`
	Hours         int       `json:"hours"`
	Status        string    `json:"status"`
	Downspeed     int       `json:"downspeed"`
	Upspeed       int       `json:"upspeed"`
	Burstspeed    int       `json:"burstspeed"`
	Duration      int       `json:"duration"`
	Ip            string    `json:"ip"`
	DateStarted   time.Time `json:"date_started"`
	DateEnd       time.Time `json:"date_end"`
	DateExpires   time.Time `json:"date_expires"`
	HoursConsumed float64   `json:"hours_consumed"`
	Gateway       string    `json:"gateway"`
	PfconfigID    uint      `json:"pfconfig_id"`
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
	GetDB() *gorm.DB
	DisableExpired() error
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

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
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

func (s *Storage) DisableExpired() error {
	var vouchers []Voucher
	result := s.DB.Where("Status = ?", "active").Find(&vouchers)
	if result.Error != nil {
		return result.Error
	}
	update := false
	for _, v := range vouchers {
		overtime := time.Since(v.DateExpires).Minutes()
		if overtime > 0 {
			v.Status = "used"
			s.DB.Save(&v)
			log.Println("Code expired:", v.Value)
			update = true
		}
	}
	if update {
		rtdb := runtimemodel.New(s.DB)
		rt := &runtimemodel.Runtime{}
		rt.Action = "updatepf"
		rt.Status = "active"
		err := rtdb.Add(rt)
		if err != nil {
			log.Println("update pf failed")
		}
	}
	return nil
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
