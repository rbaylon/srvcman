// Package dhcpserver - DhcpServer specific functions
package dhcpservermodel

import (
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

// DhcpServer represents one OpenBSD dhcpd.conf subnet declaration: the
// interface it's served on (an internal pfifacemodel.Pfiface's Device),
// the subnet/range dhcpd hands out addresses from, and the options
// (routers, DNS, domain, broadcast, NTP, lease times, PXE boot) that go
// inside that subnet's "{ ... }" block.
type DhcpServer struct {
	gorm.Model
	Name               string `json:"name"`
	Interface          string `json:"interface"`
	Subnet             string `json:"subnet"`
	Netmask            string `json:"netmask"`
	RangeStart         string `json:"range_start"`
	RangeEnd           string `json:"range_end"`
	Routers            string `json:"routers"`
	DomainName         string `json:"domain_name"`
	DomainNameServers  string `json:"domain_name_servers"`
	BroadcastAddress   string `json:"broadcast_address"`
	NtpServers         string `json:"ntp_servers"`
	DefaultLeaseTime   int    `json:"default_lease_time"`
	MaxLeaseTime       int    `json:"max_lease_time"`
	NextServer         string `json:"next_server"`
	Filename           string `json:"filename"`
	PfconfigID         uint   `json:"pfconfig_id"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&DhcpServer{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *DhcpServer) Bind(r *http.Request) error {
	if err := validate.Alnum("name", a.Name, 100); err != nil {
		return err
	}
	if err := validate.Alnum("interface", a.Interface, 100); err != nil {
		return err
	}
	if err := validate.IP("subnet", a.Subnet); err != nil {
		return err
	}
	if err := validate.IP("netmask", a.Netmask); err != nil {
		return err
	}
	if err := validate.IP("range_start", a.RangeStart); err != nil {
		return err
	}
	if err := validate.IP("range_end", a.RangeEnd); err != nil {
		return err
	}
	if err := validate.IPList("routers", a.Routers); err != nil {
		return err
	}
	// DomainName is a hostname-like value (letters, digits, '.', '-'), not
	// an IP, so it needs SafeToken rather than Alnum.
	if err := validate.SafeToken("domain_name", a.DomainName, 100); err != nil {
		return err
	}
	if err := validate.IPList("domain_name_servers", a.DomainNameServers); err != nil {
		return err
	}
	if err := validate.IP("broadcast_address", a.BroadcastAddress); err != nil {
		return err
	}
	if err := validate.IPList("ntp_servers", a.NtpServers); err != nil {
		return err
	}
	if err := validate.Int("default_lease_time", a.DefaultLeaseTime); err != nil {
		return err
	}
	if err := validate.Int("max_lease_time", a.MaxLeaseTime); err != nil {
		return err
	}
	if err := validate.IP("next_server", a.NextServer); err != nil {
		return err
	}
	// Filename is a PXE boot path (e.g. "boot/pxeboot"), so it needs
	// SafeToken rather than Alnum.
	return validate.SafeToken("filename", a.Filename, 100)
}

type Crud interface {
	GetAll() ([]DhcpServer, error)
	GetById(uid uint) (*DhcpServer, error)
	Add(dhcpserver *DhcpServer) error
	Update(dhcpserver *DhcpServer) error
	Delete(dhcpserver *DhcpServer) error
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

func (s *Storage) Add(dhcpserver *DhcpServer) error {
	result := s.DB.Create(dhcpserver)
	if result.Error != nil {
		return result.Error
	}
	if _, err := GenerateConf(s.DB); err != nil {
		log.Println("dhcpserver: failed to regenerate dhcpd.conf:", err)
	}
	return nil
}

func (s *Storage) GetAll() ([]DhcpServer, error) {
	var dhcpservers []DhcpServer
	result := s.DB.Find(&dhcpservers)
	if result.Error != nil {
		return nil, result.Error
	}
	return dhcpservers, nil
}

func (s *Storage) GetById(id uint) (*DhcpServer, error) {
	var dhcpserver DhcpServer
	result := s.DB.First(&dhcpserver, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &dhcpserver, nil
}

func (s *Storage) Update(dhcpserver *DhcpServer) error {
	result := s.DB.Save(dhcpserver)
	if result.Error != nil {
		return result.Error
	}
	if _, err := GenerateConf(s.DB); err != nil {
		log.Println("dhcpserver: failed to regenerate dhcpd.conf:", err)
	}
	return nil
}

func (s *Storage) Delete(dhcpserver *DhcpServer) error {
	result := s.DB.Delete(dhcpserver)
	if result.Error != nil {
		return result.Error
	}
	if _, err := GenerateConf(s.DB); err != nil {
		log.Println("dhcpserver: failed to regenerate dhcpd.conf:", err)
	}
	return nil
}
