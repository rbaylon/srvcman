// Package captiveportal - CaptivePortal specific functions
package captiveportalmodel

import (
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

// CaptivePortal is the captive-portal app's runtime configuration - the
// contents of its .env file (see conf.go), which it reads at startup.
// Single-entry: one gateway runs one portal.
//
// ApiAuth is not set by hand. It is the base64 "<user>:<password>" the
// portal presents to this API's /login to obtain a JWT, and it is
// generated alongside a dedicated read-only API user (see provision.go).
// It carries a live credential, so it is tagged json:"-" and never leaves
// the API - the UI cannot display it and cannot post it back. Storage's
// Add/Update are what keep it populated; see the note there about why an
// update has to carry the stored value forward.
type CaptivePortal struct {
	gorm.Model
	AppIp         string `json:"app_ip"`
	AppPort       int    `json:"app_port"`
	ApiUrl        string `json:"api_url"`
	ApiAuth       string `json:"-"`
	RouterId      string `json:"router_id"`
	RouterIndex   int    `json:"router_index"`
	UnixSock      string `json:"unix_sock"`
	ArkgateSocket string `json:"arkgate_socket"`
	Enabled       bool   `json:"enabled"`
}

type CaptivePortals struct {
	CaptivePortals []CaptivePortal `json:"captiveportals"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&CaptivePortal{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render.
//
// Every value here is written into a KEY=value .env file, so the charset
// checks are load-bearing rather than cosmetic: a value carrying a newline
// would append env lines of its own, and the one that matters most -
// API_AUTH - is exactly the line an injected override would want to
// replace. SafeToken rejects newlines and quotes (see its doc comment).
func (a *CaptivePortal) Bind(r *http.Request) error {
	if err := validate.IP("app_ip", a.AppIp); err != nil {
		return err
	}
	if err := validate.IntRange("app_port", a.AppPort, 1, 65535); err != nil {
		return err
	}
	if err := validate.SafeToken("api_url", a.ApiUrl, 253); err != nil {
		return err
	}
	if err := validate.SafeToken("router_id", a.RouterId, 100); err != nil {
		return err
	}
	if err := validate.Int("router_index", a.RouterIndex); err != nil {
		return err
	}
	if err := validate.SafeToken("unix_sock", a.UnixSock, 253); err != nil {
		return err
	}
	return validate.SafeToken("arkgate_socket", a.ArkgateSocket, 253)
}

func (a *CaptivePortals) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]CaptivePortal, error)
	GetById(uid uint) (*CaptivePortal, error)
	Add(captiveportal *CaptivePortal) error
	Update(captiveportal *CaptivePortal) error
	Delete(captiveportal *CaptivePortal) error
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

// Add provisions the portal's API user before saving, so the record is
// never stored without the credential its .env needs.
func (s *Storage) Add(captiveportal *CaptivePortal) error {
	auth, err := EnsureApiUser(s.DB, "")
	if err != nil {
		return err
	}
	captiveportal.ApiAuth = auth
	result := s.DB.Create(captiveportal)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// Update carries the stored ApiAuth forward. It has to: ApiAuth is
// json:"-", so a record bound from an API request always arrives with it
// empty, and Save writes every field - without this, saving the form would
// blank the credential and the portal would lose its API access.
//
// EnsureApiUser is still called so that the user, group and policy are
// repaired if any of them were deleted by hand, and it only mints a new
// password when there is no usable credential to keep.
func (s *Storage) Update(captiveportal *CaptivePortal) error {
	var old CaptivePortal
	existing := ""
	if s.DB.First(&old, captiveportal.ID).Error == nil {
		existing = old.ApiAuth
	}
	auth, err := EnsureApiUser(s.DB, existing)
	if err != nil {
		return err
	}
	captiveportal.ApiAuth = auth
	result := s.DB.Save(captiveportal)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]CaptivePortal, error) {
	var captiveportals []CaptivePortal
	result := s.DB.Find(&captiveportals)
	if result.Error != nil {
		return nil, result.Error
	}
	return captiveportals, nil
}

func (s *Storage) GetById(id uint) (*CaptivePortal, error) {
	var captiveportal CaptivePortal
	result := s.DB.First(&captiveportal, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &captiveportal, nil
}

// Delete removes the config but deliberately leaves the provisioned API
// user, group and policy in place: they are ordinary records an admin can
// see and remove in the users/groups/policies UI, and deleting an account
// as a side effect of deleting a config row would be a surprising way to
// revoke access.
func (s *Storage) Delete(captiveportal *CaptivePortal) error {
	result := s.DB.Delete(captiveportal)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
