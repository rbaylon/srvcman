// Package pppoes - pppoe specific functions
package pppoemodel

import (
	"log"
	"net/http"

	pfifacemodel "github.com/rbaylon/srvcman/modules/pfifaces/model"
	"gorm.io/gorm"
)

type Pppoe struct {
	gorm.Model
	Name        string `json:"name"`
	Device      string `json:"device"`
	Ip          string `json:"ip"`
	PoolAddress string `json:"pool"`
	DnsAddress  string `json:"dns"`
	PfconfigID  uint   `json:"pfconfig_id"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Pppoe{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Pppoe) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Pppoe, error)
	GetById(uid uint) (*Pppoe, error)
	Add(pppoe *Pppoe) error
	AddBulk(pppoes []Pppoe) error
	Update(pppoe *Pppoe) error
	Delete(pppoe *Pppoe) error
	GetBypppoeValue(value string) (*Pppoe, error)
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

func (s *Storage) AddBulk(pppoes []Pppoe) error {
	result := s.DB.Create(pppoes)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
}

func (s *Storage) Add(pppoe *Pppoe) error {
	pfiface := pfifacemodel.Pfiface{}
	pfiface.Name = pppoe.Name
	pfiface.Device = "pppac0"
	pfiface.PfconfigID = pppoe.PfconfigID
	pfiface.Ip = pppoe.Ip
	pfiface.Type = "internal"
	pfiface.Default = false
	result := s.DB.Create(pppoe)
	if result.Error != nil {
		return result.Error
	}
	ifaceres := s.DB.Create(pfiface)
	if ifaceres.Error != nil {
		return ifaceres.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Pppoe, error) {
	var pppoes []Pppoe
	result := s.DB.Find(&pppoes)
	if result.Error != nil {
		return nil, result.Error
	}
	return pppoes, nil
}

func (s *Storage) GetById(id uint) (*Pppoe, error) {
	var pppoe Pppoe
	result := s.DB.First(&pppoe, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &pppoe, nil
}

func (s *Storage) GetBypppoeValue(value string) (*Pppoe, error) {
	var pppoe Pppoe
	result := s.DB.Where("Value = ?", value).First(&pppoe)
	if result.Error != nil {
		return nil, result.Error
	}
	return &pppoe, nil
}

func (s *Storage) Update(pppoe *Pppoe) error {
	pfiface := pfifacemodel.Pfiface{}
	presult := s.DB.Where("Device = ?", "pppac0").First(&pfiface)
	if presult.Error != nil {
		log.Println("PPP interface update error:", presult.Error)
	} else {
		puresult := s.DB.Save(pfiface)
		if puresult.Error != nil {
			return puresult.Error
		}
	}
	pfiface.Ip = pppoe.Ip
	result := s.DB.Save(pppoe)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(pppoe *Pppoe) error {
	pfiface := pfifacemodel.Pfiface{}
	presult := s.DB.Where("Device = ?", "pppac0").First(&pfiface)
	if presult.Error != nil {
		log.Println("PPP interface delete error:", presult.Error)
	} else {
		pdresult := s.DB.Delete(pfiface)
		if pdresult.Error != nil {
			return pdresult.Error
		}
	}
	result := s.DB.Delete(pppoe)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
