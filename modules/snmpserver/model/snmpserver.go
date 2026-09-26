// Package snmpserver - SnmpServer specific functions
package snmpservermodel

import (
	"fmt"
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

// Security levels accepted by snmpd.conf's "seclevel" directive. Anything
// else is a syntax error to snmpd(8), not a soft fallback.
const (
	SecLevelNone = "none"
	SecLevelAuth = "auth"
	SecLevelEnc  = "enc"
)

// maxSystemServices is snmpd's ceiling on "system services" (the
// sysServices MIB bitmask). snmpd rejects anything outside 0-127 outright
// ("system services too large"/"too small").
const maxSystemServices = 127

// SnmpServer is one OpenBSD snmpd.conf. Unlike dhcpd.conf, which is a
// list of independent subnet blocks, snmpd.conf is a single document -
// "system location", "seclevel" and "filter-routes" are global settings
// with no meaningful repetition - so GenerateConf renders one record (see
// conf.go) and the UI only offers to create one.
//
// SecLevel and the two keys are coupled, and snmpd enforces it: with the
// default seclevel of "enc" a user needs both an authkey and an enckey
// ("user 'x': missing enckey"), with "auth" it needs an authkey, and
// "enc" without "auth" is rejected as "auth is mandatory with enc". Bind
// checks that combination so a bad pairing fails here rather than at
// apply time, when snmpd -nf rejects the staged file.
type SnmpServer struct {
	gorm.Model
	Name              string `json:"name"`
	ListenIp          string `json:"listen_ip"`
	Community         string `json:"community"`
	Username          string `json:"username"`
	AuthKey           string `json:"authkey"`
	EncKey            string `json:"enckey"`
	SystemContact     string `json:"system_contact"`
	SystemDescription string `json:"system_description"`
	SystemLocation    string `json:"system_location"`
	SystemServices    int    `json:"system_services"`
	SecLevel          string `json:"seclevel" gorm:"default:enc"`
	FilterRoutes      bool   `json:"filter_routes" gorm:"default:true"`
	PfconfigID        uint   `json:"pfconfig_id"`
}

type SnmpServers struct {
	SnmpServers []SnmpServer `json:"snmpservers"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&SnmpServer{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render.
//
// Every string here is interpolated into snmpd.conf, most of it inside
// double quotes, so the charset checks are what keep a value from closing
// its quote and appending directives of its own - SafeToken rejects
// quotes, semicolons, newlines, '$' and backticks (see its doc comment,
// which is written for exactly this use).
func (a *SnmpServer) Bind(r *http.Request) error {
	if err := validate.Alnum("name", a.Name, 100); err != nil {
		return err
	}
	if err := validate.IP("listen_ip", a.ListenIp); err != nil {
		return err
	}
	if err := validate.SafeToken("community", a.Community, 100); err != nil {
		return err
	}
	if err := validate.SafeToken("username", a.Username, 100); err != nil {
		return err
	}
	if err := validate.SafeToken("authkey", a.AuthKey, 100); err != nil {
		return err
	}
	if err := validate.SafeToken("enckey", a.EncKey, 100); err != nil {
		return err
	}
	// system contact is usually an email address, so it needs '@'.
	if err := validate.SafeTokenUpstream("system_contact", a.SystemContact, 100); err != nil {
		return err
	}
	if err := validate.SafeToken("system_description", a.SystemDescription, 100); err != nil {
		return err
	}
	if err := validate.SafeToken("system_location", a.SystemLocation, 100); err != nil {
		return err
	}
	if err := validate.IntRange("system_services", a.SystemServices, 0, maxSystemServices); err != nil {
		return err
	}
	switch a.SecLevel {
	case "", SecLevelNone, SecLevelAuth, SecLevelEnc:
	default:
		return fmt.Errorf("seclevel must be %q, %q or %q", SecLevelNone, SecLevelAuth, SecLevelEnc)
	}
	return a.checkKeys()
}

// checkKeys enforces the seclevel/key pairing snmpd itself requires. An
// empty SecLevel counts as "enc", which is snmpd's own default.
func (a *SnmpServer) checkKeys() error {
	if a.Username == "" {
		return nil
	}
	level := a.SecLevel
	if level == "" {
		level = SecLevelEnc
	}
	switch level {
	case SecLevelEnc:
		if a.AuthKey == "" || a.EncKey == "" {
			return fmt.Errorf("seclevel %q needs both authkey and enckey for user %q", SecLevelEnc, a.Username)
		}
	case SecLevelAuth:
		if a.AuthKey == "" {
			return fmt.Errorf("seclevel %q needs an authkey for user %q", SecLevelAuth, a.Username)
		}
	}
	return nil
}

func (a *SnmpServers) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]SnmpServer, error)
	GetById(uid uint) (*SnmpServer, error)
	Add(snmpserver *SnmpServer) error
	Update(snmpserver *SnmpServer) error
	Delete(snmpserver *SnmpServer) error
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

func (s *Storage) Add(snmpserver *SnmpServer) error {
	result := s.DB.Create(snmpserver)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]SnmpServer, error) {
	var snmpservers []SnmpServer
	result := s.DB.Find(&snmpservers)
	if result.Error != nil {
		return nil, result.Error
	}
	return snmpservers, nil
}

func (s *Storage) GetById(id uint) (*SnmpServer, error) {
	var snmpserver SnmpServer
	result := s.DB.First(&snmpserver, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &snmpserver, nil
}

func (s *Storage) Update(snmpserver *SnmpServer) error {
	result := s.DB.Save(snmpserver)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(snmpserver *SnmpServer) error {
	result := s.DB.Delete(snmpserver)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
