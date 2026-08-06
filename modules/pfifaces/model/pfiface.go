// Package pfifaces - Pfiface specific functions
package pfifacemodel

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	vlanmodel "github.com/rbaylon/srvcman/modules/vlans/model"
	"github.com/rbaylon/srvcman/utils"
	"gorm.io/gorm"
)

type Pfiface struct {
	gorm.Model
	Name         string `json:"name"`
	Speed        string `json:"speed"`
	Device       string `json:"device"`
	Default      bool   `json:"default"`
	Type         string `json:"type"`
	Gateway      string `json:"gateway"`
	Ip           string `json:"ip"`
	Netmask      string `json:"netmask"`
	LbPercentage int    `json:"lb_percentage"`
	PfconfigID   uint   `json:"pfconfig_id"`
}

type Pfifaces struct {
	Pfifaces []Pfiface `json:"pfifaces"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Pfiface{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Pfiface) Bind(r *http.Request) error {
	return nil
}

func (a *Pfifaces) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Pfiface, error)
	GetById(uid uint) (*Pfiface, error)
	Add(pfiface *Pfiface) error
	AddBulk(pfifaces []Pfiface) error
	Update(pfiface *Pfiface) error
	Delete(pfiface *Pfiface) error
	GetByPfifaceValue(value string) (*Pfiface, error)
	GetDB() *gorm.DB
	GenConF(pfiface *Pfiface) error
}

type Storage struct {
	DB *gorm.DB
}

func New(db *gorm.DB) *Storage {
	return &Storage{
		DB: db,
	}
}

type vlanmap struct {
	Vmap map[string]vlanmodel.Vlan
}

func getVlans(vlans []vlanmodel.Vlan) vlanmap {
	vm := vlanmap{}
	vm.Vmap = make(map[string]vlanmodel.Vlan)
	for _, v := range vlans {
		vm.Vmap[v.Name] = v
	}
	return vm
}

func (s *Storage) GenConF(d *Pfiface) error {
	vif := ""
	vlandb := vlanmodel.New(s.GetDB())
	vlanall, err := vlandb.GetAll()
	if err != nil {
		log.Println("Get vlans from db failed")
	}
	vlans := getVlans(vlanall)
	if strings.Contains(d.Device, "vlan") {
		cidr := utils.MaskToCidr(d.Netmask)
		_, ipNet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", d.Ip, cidr))
		if err != nil {
			log.Println("Error parsing CIDR:", err)
			return err
		}
		broadcast := utils.BroadcastAddr(ipNet)
		vlan := vlans.Vmap[d.Device]
		prio := ""
		if vlan.RxPrio > 0 || vlan.TxPrio > 0 {
			prio = fmt.Sprintf("rxprio %d txprio %d", vlan.RxPrio, vlan.TxPrio)
		}
		vif = fmt.Sprintf(" %s parent %s %s vnetid %d",
			broadcast, vlan.ParentDevice, prio, vlan.VlanTag)
	}
	iface := fmt.Sprintf("inet %s %s%s\n", d.Ip, d.Netmask, vif)
	err = os.WriteFile("/tmp/hostname."+d.Device, []byte(iface), 0640)
	if err != nil {
		return err
	}
	return nil
}

func (s *Storage) AddBulk(pfifaces []Pfiface) error {
	result := s.DB.Create(pfifaces)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
}

func (s *Storage) Add(pfiface *Pfiface) error {
	result := s.DB.Create(pfiface)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Pfiface, error) {
	var pfifaces []Pfiface
	result := s.DB.Find(&pfifaces)
	if result.Error != nil {
		return nil, result.Error
	}
	return pfifaces, nil
}

func (s *Storage) GetById(id uint) (*Pfiface, error) {
	var pfiface Pfiface
	result := s.DB.First(&pfiface, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &pfiface, nil
}

func (s *Storage) GetByPfifaceValue(value string) (*Pfiface, error) {
	var pfiface Pfiface
	result := s.DB.Where("Value = ?", value).First(&pfiface)
	if result.Error != nil {
		return nil, result.Error
	}
	return &pfiface, nil
}

func (s *Storage) Update(pfiface *Pfiface) error {
	result := s.DB.Save(pfiface)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(pfiface *Pfiface) error {
	result := s.DB.Delete(pfiface)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
