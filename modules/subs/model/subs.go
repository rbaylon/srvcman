// Package subs - User specific functions
package submodel

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Sub struct {
	gorm.Model
	FirstName   string    `json:"first_name"`
	LastName    string    `json:"last_name"`
	FramedIp    string    `json:"framed_ip"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	Mac         string    `json:"mac"`
	Loc         string    `json:"loc"`
	Downspeed   int       `json:"downspeed"`
	Upspeed     int       `json:"upspeed"`
	Burstspeed  int       `json:"burstspeed"`
	Duration    int       `json:"duration"`
	Gateway     string    `json:"gateway"`
	Priority    int       `json:"priority"`
	DateEnd     time.Time `json:"date_end"`
	DateExpires time.Time `json:"date_expires"`
	PfconfigID  uint      `json:"pfconfig_id"`
	DueDay      int       `json:"due_day"`
	LaterCount  int       `json:"later_count"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Sub{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Sub) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Sub, error)
	GetById(uid uint) (*Sub, error)
	GetByIp(ip string) (*Sub, error)
	Add(sub *Sub) error
	Update(sub *Sub) error
	Delete(sub *Sub) error
	GetByMac(mac string) (*Sub, error)
	GetByMacPrefix(macprefix string) (*Sub, error)
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

func (s *Storage) Add(sub *Sub) error {
	result := s.DB.Create(sub)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Sub, error) {
	var subs []Sub
	result := s.DB.Find(&subs)
	if result.Error != nil {
		return nil, result.Error
	}
	return subs, nil
}

func (s *Storage) GetById(id uint) (*Sub, error) {
	var sub Sub
	result := s.DB.First(&sub, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &sub, nil
}

func (s *Storage) GetByMac(mac string) (*Sub, error) {
	var sub Sub
	result := s.DB.Where("Mac = ?", mac).First(&sub)
	if result.Error != nil {
		return nil, result.Error
	}
	return &sub, nil
}

func (s *Storage) GetByIp(ip string) (*Sub, error) {
	var sub Sub
	result := s.DB.Where("framed_ip = ?", ip).First(&sub)
	if result.Error != nil {
		return nil, result.Error
	}
	return &sub, nil
}

func (s *Storage) GetByMacPrefix(macprefix string) (*Sub, error) {
	var sub Sub
	if macprefix != "" {
		mac := strings.Split(macprefix, "")
		newmac := fmt.Sprintf("%s:%s:%s:%s", strings.Join(mac[0:2], ""), strings.Join(mac[2:4], ""), strings.Join(mac[4:6], ""), strings.Join(mac[6:8], ""))
		result := s.DB.Where("Mac LIKE ?", "%"+newmac+"%").First(&sub)
		if result.Error != nil {
			return nil, result.Error
		}
		return &sub, nil
	}
	return nil, fmt.Errorf("Prefix empty")
}

func (s *Storage) Update(sub *Sub) error {
	result := s.DB.Save(sub)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(sub *Sub) error {
	result := s.DB.Delete(sub)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
