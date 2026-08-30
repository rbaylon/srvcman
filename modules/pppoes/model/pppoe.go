// Package pppoes - pppoe specific functions
package pppoemodel

import (
	"fmt"
	"log"
	"net/http"

	pfifacemodel "github.com/rbaylon/srvcman/modules/pfifaces/model"
	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

type Pppoe struct {
	gorm.Model
	Name        string `json:"name"`
	Device      string `json:"device"`
	Ip          string `json:"ip"`
	PoolAddress string `json:"pool"`
	DnsAddress  string `json:"dns"`
	DevIndex    int    `json:"dev_index"`
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
	if err := validate.Alnum("name", a.Name, 100); err != nil {
		return err
	}
	if err := validate.Alnum("device", a.Device, 100); err != nil {
		return err
	}
	if err := validate.IP("ip", a.Ip); err != nil {
		return err
	}
	// PoolAddress may be a CIDR block or an address range, so it needs '.',
	// '/' and '-'; SafeToken still blocks characters that could inject
	// extra config directives.
	if err := validate.SafeToken("pool", a.PoolAddress, 100); err != nil {
		return err
	}
	if err := validate.IPList("dns", a.DnsAddress); err != nil {
		return err
	}
	return validate.Int("dev_index", a.DevIndex)
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
	GetByPfconfigId(id uint) ([]Pppoe, error)
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
	parentdevice := pfifacemodel.Pfiface{}
	presult := s.DB.Where("Device = ?", pppoe.Device).First(&parentdevice)
	if presult.Error != nil {
		return presult.Error
	}
	var pppoes []Pppoe
	res := s.DB.Find(&pppoes)
	if res.Error != nil {
		return res.Error
	}
	pppoe.DevIndex = len(pppoes)
	if pppoe.DevIndex > 7 {
		return fmt.Errorf("max pppac index limit reached.")
	}
	pfiface.Name = pppoe.Name
	pfiface.Device = fmt.Sprintf("pppac%d", pppoe.DevIndex)
	pfiface.PfconfigID = pppoe.PfconfigID
	pfiface.Ip = pppoe.Ip
	pfiface.Type = "internal"
	pfiface.Default = false
	pfiface.Speed = parentdevice.Speed
	pfiface.Netmask = parentdevice.Netmask
	pfiface.Gateway = parentdevice.Gateway
	result := s.DB.Create(pppoe)
	if result.Error != nil {
		return result.Error
	}
	ifaceres := s.DB.Create(&pfiface)
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

func (s *Storage) GetByPfconfigId(id uint) ([]Pppoe, error) {
	var pppoes []Pppoe
	result := s.DB.Where("pfconfig_id = ?", id).Find(&pppoes)
	if result.Error != nil {
		return nil, result.Error
	}
	return pppoes, nil
}

func (s *Storage) Update(pppoe *Pppoe) error {
	pfiface := pfifacemodel.Pfiface{}
	presult := s.DB.Where("Device = ?", fmt.Sprintf("pppac%d", pppoe.DevIndex)).First(&pfiface)
	if presult.Error != nil {
		log.Println("PPP interface update error:", presult.Error)
	} else {
		pfiface.Ip = pppoe.Ip
		pfiface.Name = pppoe.Name
		puresult := s.DB.Save(&pfiface)
		if puresult.Error != nil {
			return puresult.Error
		}
	}
	result := s.DB.Save(pppoe)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(pppoe *Pppoe) error {
	pfiface := pfifacemodel.Pfiface{}
	presult := s.DB.Where("Device = ?", fmt.Sprintf("pppac%d", pppoe.DevIndex)).First(&pfiface)
	if presult.Error != nil {
		log.Println("PPP interface delete error:", presult.Error)
	} else {
		pdresult := s.DB.Delete(&pfiface)
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
