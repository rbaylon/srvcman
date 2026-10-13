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
//
// Everything here ends up in npppd.conf, and npppd.conf has no tolerance: one
// bad value and npppd refuses to start, taking every PPPoE session with it. So
// the values are checked against what npppd actually accepts, not just for
// characters that are safe to write.
func (a *Pppoe) Bind(r *http.Request) error {
	if err := validate.Required("name", a.Name); err != nil {
		return err
	}
	if err := validate.Alnum("name", a.Name, 100); err != nil {
		return err
	}
	if err := validate.Required("device", a.Device); err != nil {
		return err
	}
	if err := validate.Alnum("device", a.Device, 100); err != nil {
		return err
	}
	if _, ok := ipv4(a.Ip); !ok {
		return fmt.Errorf("ip %q is not a valid IPv4 address", a.Ip)
	}
	if err := checkPool(a.PoolAddress, a.Ip); err != nil {
		return err
	}
	dns, err := normalizeDNS(a.DnsAddress)
	if err != nil {
		return err
	}
	a.DnsAddress = dns
	return validate.IntRange("dev_index", a.DevIndex, 0, maxTunnels-1)
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
	if _, err := GenerateConf(s.DB); err != nil {
		log.Println("pppoes: failed to regenerate npppd.conf:", err)
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
		return refuse("no firewall interface uses device %q", pppoe.Device)
	}
	var pppoes []Pppoe
	res := s.DB.Find(&pppoes)
	if res.Error != nil {
		return res.Error
	}
	used := make([]int, 0, len(pppoes))
	for _, p := range pppoes {
		if p.Device == pppoe.Device {
			return refuse("device %s already has a PPPoE server (%s)", p.Device, p.Name)
		}
		used = append(used, p.DevIndex)
	}
	idx, err := nextDevIndex(used)
	if err != nil {
		return err
	}
	pppoe.DevIndex = idx
	// The firewall interface created below is named after the server and pf.conf
	// has one macro per interface name, so a name already taken would redefine it.
	var clash int64
	if err := s.DB.Model(&pfifacemodel.Pfiface{}).Where("name = ?", pppoe.Name).Count(&clash).Error; err != nil {
		return err
	}
	if clash > 0 {
		return refuse("the name %q is already used by a firewall interface", pppoe.Name)
	}
	pfiface.Name = pppoe.Name
	pfiface.Device = fmt.Sprintf("pppac%d", pppoe.DevIndex)
	pfiface.PfconfigID = pppoe.PfconfigID
	pfiface.Ip = pppoe.Ip
	pfiface.Type = "internal"
	pfiface.Default = false
	pfiface.Speed = parentdevice.Speed
	pfiface.Netmask = pppacNetmask
	pfiface.Gateway = parentdevice.Gateway
	result := s.DB.Create(pppoe)
	if result.Error != nil {
		return result.Error
	}
	ifaceres := s.DB.Create(&pfiface)
	if ifaceres.Error != nil {
		return ifaceres.Error
	}
	if _, err := GenerateConf(s.DB); err != nil {
		log.Println("pppoes: failed to regenerate npppd.conf:", err)
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
	// The pppac number belongs to the record, not to whoever is editing it: the
	// form does not carry it, so what arrives here is 0, and using that found
	// pppac0's firewall interface - renaming it after server 2, giving it server
	// 2's address - and wrote index 0 back, leaving two tunnels called PPPOE0.
	existing, err := s.GetById(pppoe.ID)
	if err != nil {
		return err
	}
	pppoe.DevIndex = existing.DevIndex
	pppoe.CreatedAt = existing.CreatedAt
	var sameDevice int64
	if err := s.DB.Model(&Pppoe{}).Where("device = ? AND id <> ?", pppoe.Device, pppoe.ID).Count(&sameDevice).Error; err != nil {
		return err
	}
	if sameDevice > 0 {
		return refuse("device %s already has a PPPoE server", pppoe.Device)
	}
	if pppoe.PfconfigID == 0 {
		pppoe.PfconfigID = existing.PfconfigID
	}
	pfiface := pfifacemodel.Pfiface{}
	presult := s.DB.Where("Device = ?", fmt.Sprintf("pppac%d", pppoe.DevIndex)).First(&pfiface)
	if presult.Error != nil {
		log.Println("PPP interface update error:", presult.Error)
	} else {
		var clash int64
		if err := s.DB.Model(&pfifacemodel.Pfiface{}).Where("name = ? AND id <> ?", pppoe.Name, pfiface.ID).Count(&clash).Error; err != nil {
			return err
		}
		if clash > 0 {
			return refuse("the name %q is already used by a firewall interface", pppoe.Name)
		}
		pfiface.Ip = pppoe.Ip
		pfiface.Name = pppoe.Name
		pfiface.Netmask = pppacNetmask
		puresult := s.DB.Save(&pfiface)
		if puresult.Error != nil {
			return puresult.Error
		}
	}
	result := s.DB.Save(pppoe)
	if result.Error != nil {
		return result.Error
	}
	if _, err := GenerateConf(s.DB); err != nil {
		log.Println("pppoes: failed to regenerate npppd.conf:", err)
	}
	return nil
}

func (s *Storage) Delete(pppoe *Pppoe) error {
	// Work from the stored record. The caller only knows the ID, and the pppac
	// number is what finds the firewall interface to remove along with it.
	existing, err := s.GetById(pppoe.ID)
	if err != nil {
		return err
	}
	*pppoe = *existing
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
	if _, err := GenerateConf(s.DB); err != nil {
		log.Println("pppoes: failed to regenerate npppd.conf:", err)
	}
	return nil
}
