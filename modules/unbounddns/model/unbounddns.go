// Package unbounddns - UnboundDns specific functions
package unbounddnsmodel

import (
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

// UnboundDns represents one OpenBSD unbound.conf, covering the directives
// in a single "server:" block and its default "forward-zone:" block.
// Interface is a pfifacemodel.Pfiface's Ip (or "0.0.0.0" for all
// interfaces) to bind on. AccessControl, LocalData and ForwardAddr are
// comma-separated lists, each rendered as one repeated directive line
// (access-control:, local-data:, forward-addr:) per entry.
type UnboundDns struct {
	gorm.Model
	Name                string `json:"name"`
	Interface           string `json:"interface"`
	TlsCertBundle       string `json:"tls_cert_bundle"`
	AccessControl       string `json:"access_control"`
	DoNotQueryLocalhost bool   `json:"do_not_query_localhost"`
	HideIdentity        bool   `json:"hide_identity"`
	HideVersion         bool   `json:"hide_version"`
	Prefetch            bool   `json:"prefetch"`
	LocalData           string `json:"local_data"`
	ForwardZoneName     string `json:"forward_zone_name"`
	ForwardTlsUpstream  bool   `json:"forward_tls_upstream"`
	ForwardAddr         string `json:"forward_addr"`
	PfconfigID          uint   `json:"pfconfig_id"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&UnboundDns{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *UnboundDns) Bind(r *http.Request) error {
	if err := validate.Alnum("name", a.Name, 100); err != nil {
		return err
	}
	if err := validate.IP("interface", a.Interface); err != nil {
		return err
	}
	// TlsCertBundle is a filesystem path (e.g. "/etc/ssl/cert.pem"), so it
	// needs SafeToken rather than Alnum.
	if err := validate.SafeToken("tls_cert_bundle", a.TlsCertBundle, 100); err != nil {
		return err
	}
	// AccessControl entries pair a CIDR with an action word (e.g.
	// "172.16.0.0/12 allow"), so they need SafeToken rather than IPList.
	if err := validate.SafeToken("access_control", a.AccessControl, 300); err != nil {
		return err
	}
	// LocalData entries are "hostname. IN A ip" triples, so they need
	// SafeToken rather than IPList or Alnum.
	if err := validate.SafeToken("local_data", a.LocalData, 300); err != nil {
		return err
	}
	if err := validate.SafeToken("forward_zone_name", a.ForwardZoneName, 100); err != nil {
		return err
	}
	// ForwardAddr entries may be "ip@port" for DNS-over-TLS upstreams.
	return validate.SafeTokenUpstream("forward_addr", a.ForwardAddr, 300)
}

type Crud interface {
	GetAll() ([]UnboundDns, error)
	GetById(uid uint) (*UnboundDns, error)
	Add(unbounddns *UnboundDns) error
	Update(unbounddns *UnboundDns) error
	Delete(unbounddns *UnboundDns) error
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

func (s *Storage) Add(unbounddns *UnboundDns) error {
	result := s.DB.Create(unbounddns)
	if result.Error != nil {
		return result.Error
	}
	if _, err := GenerateConf(s.DB); err != nil {
		log.Println("unbounddns: failed to regenerate unbound.conf:", err)
	}
	return nil
}

func (s *Storage) GetAll() ([]UnboundDns, error) {
	var unbounddnss []UnboundDns
	result := s.DB.Find(&unbounddnss)
	if result.Error != nil {
		return nil, result.Error
	}
	return unbounddnss, nil
}

func (s *Storage) GetById(id uint) (*UnboundDns, error) {
	var unbounddns UnboundDns
	result := s.DB.First(&unbounddns, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &unbounddns, nil
}

func (s *Storage) Update(unbounddns *UnboundDns) error {
	result := s.DB.Save(unbounddns)
	if result.Error != nil {
		return result.Error
	}
	if _, err := GenerateConf(s.DB); err != nil {
		log.Println("unbounddns: failed to regenerate unbound.conf:", err)
	}
	return nil
}

func (s *Storage) Delete(unbounddns *UnboundDns) error {
	result := s.DB.Delete(unbounddns)
	if result.Error != nil {
		return result.Error
	}
	if _, err := GenerateConf(s.DB); err != nil {
		log.Println("unbounddns: failed to regenerate unbound.conf:", err)
	}
	return nil
}
