// Package billportal - BillPortal specific functions
package billportalmodel

import (
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

// BillPortal is the bill-portal app's runtime configuration - the contents
// of its .env file (see conf.go), which it reads at startup. Single-entry:
// one gateway runs one portal.
//
// ApiAuth is not set by hand. It is the base64 "<user>:<password>" the
// portal presents to this API's /login to obtain a JWT, and it is
// generated alongside a dedicated read-only API user (see provision.go).
// It carries a live credential, so it is tagged json:"-" and never leaves
// the API - the UI cannot display it and cannot post it back. Storage's
// Add/Update are what keep it populated; see the note there about why an
// update has to carry the stored value forward.
type BillPortal struct {
	gorm.Model
	AppIp    string `json:"app_ip"`
	AppPort  int    `json:"app_port"`
	ApiUrl   string `json:"api_url"`
	ApiAuth  string `json:"-"`
	RouterId string `json:"router_id"`
	UnixSock string `json:"unix_sock"`
	Enabled  bool   `json:"enabled"`
}

type BillPortals struct {
	BillPortals []BillPortal `json:"billportals"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&BillPortal{})
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
func (a *BillPortal) Bind(r *http.Request) error {
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
	return validate.SafeToken("unix_sock", a.UnixSock, 253)
}

func (a *BillPortals) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]BillPortal, error)
	GetById(uid uint) (*BillPortal, error)
	Add(billportal *BillPortal) error
	Update(billportal *BillPortal) error
	Delete(billportal *BillPortal) error
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
func (s *Storage) Add(billportal *BillPortal) error {
	auth, err := EnsureApiUser(s.DB, "")
	if err != nil {
		return err
	}
	billportal.ApiAuth = auth
	result := s.DB.Create(billportal)
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
func (s *Storage) Update(billportal *BillPortal) error {
	var old BillPortal
	existing := ""
	if s.DB.First(&old, billportal.ID).Error == nil {
		existing = old.ApiAuth
	}
	auth, err := EnsureApiUser(s.DB, existing)
	if err != nil {
		return err
	}
	billportal.ApiAuth = auth
	result := s.DB.Save(billportal)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]BillPortal, error) {
	var billportals []BillPortal
	result := s.DB.Find(&billportals)
	if result.Error != nil {
		return nil, result.Error
	}
	return billportals, nil
}

func (s *Storage) GetById(id uint) (*BillPortal, error) {
	var billportal BillPortal
	result := s.DB.First(&billportal, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &billportal, nil
}

func (s *Storage) Delete(billportal *BillPortal) error {
	result := s.DB.Delete(billportal)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
