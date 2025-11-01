// Package pfconfigs - Pfconfig specific functions
package pfconfigmodel

import (
	"log"
	"net/http"

	dhcpmodel "github.com/rbaylon/srvcman/modules/dhcps/model"
	fwmodel "github.com/rbaylon/srvcman/modules/firewall/model"
	pfifacemodel "github.com/rbaylon/srvcman/modules/pfifaces/model"
	pflowmodel "github.com/rbaylon/srvcman/modules/pflows/model"
	planmodel "github.com/rbaylon/srvcman/modules/plans/model"
	submodel "github.com/rbaylon/srvcman/modules/subs/model"
	vouchermodel "github.com/rbaylon/srvcman/modules/vouchers/model"
	"gorm.io/gorm"
)

type Pfconfig struct {
	gorm.Model
	Ifaces            []pfifacemodel.Pfiface `json:"ifaces"`
	WifiIpList        string                 `json:"wifi_ip_list"`
	SubsIpList        string                 `json:"subs_ip_list"`
	SubsPortalPort    int                    `json:"subs_portal_port"`
	CaptivePortalPort int                    `json:"captive_portal_port"`
	Router            string                 `json:"router"`
	LoadBalance       bool                   `json:"load_balance"`
	Vouchers          []vouchermodel.Voucher `json:"vouchers"`
	Dhcps             []dhcpmodel.Dhcp       `json:"dhcps"`
	Subs              []submodel.Sub         `json:"subs"`
	Dns               string                 `json:"dns"`
	Pflows            []pflowmodel.Pflow     `json:"pflows"`
	Workername        string                 `json:"workername"`
	Plans             []planmodel.Plan       `json:"plans"`
	Rules             []fwmodel.Fw           `json:"rules"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Pfconfig{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Pfconfig) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Pfconfig, error)
	GetById(uid uint) (*Pfconfig, error)
	Add(pfconfig *Pfconfig) error
	AddBulk(pfconfigs []Pfconfig) error
	Update(pfconfig *Pfconfig) error
	Delete(pfconfig *Pfconfig) error
	GetByRouterId(value string) (*Pfconfig, error)
	GetDB() *gorm.DB
}

type Storage struct {
	DB *gorm.DB
}

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
}

func New(db *gorm.DB) *Storage {
	return &Storage{
		DB: db,
	}
}

func (s *Storage) AddBulk(pfconfigs []Pfconfig) error {
	result := s.DB.Create(pfconfigs)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Add(pfconfig *Pfconfig) error {
	result := s.DB.Create(pfconfig)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Pfconfig, error) {
	var pfconfigs []Pfconfig
	result := s.DB.Find(&pfconfigs)
	if result.Error != nil {
		return nil, result.Error
	}
	return pfconfigs, nil
}

func (s *Storage) GetById(id uint) (*Pfconfig, error) {
	var pfconfig Pfconfig
	result := s.DB.Preload("Ifaces").Preload("Dhcps").Preload("Pflows").Preload("Plans").First(&pfconfig, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &pfconfig, nil
}

func (s *Storage) GetByRouterId(value string) (*Pfconfig, error) {
	var pfconfig Pfconfig
	result := s.DB.Where("Router = ?", value).Preload(
		"Ifaces").Preload("Vouchers").Preload("Dhcps").Preload(
		"Subs").Preload("Pflows").Preload("Plans").Preload(
		"Rules").First(&pfconfig)
	if result.Error != nil {
		return nil, result.Error
	}
	return &pfconfig, nil
}

func (s *Storage) Update(pfconfig *Pfconfig) error {
	result := s.DB.Save(pfconfig)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(pfconfig *Pfconfig) error {
	result := s.DB.Delete(pfconfig)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
