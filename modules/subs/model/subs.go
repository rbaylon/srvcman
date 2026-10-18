// Package subs - User specific functions
package submodel

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

type Sub struct {
	gorm.Model
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	ContactNumber string `json:"contact_number"`
	Email         string `json:"email"`
	FramedIp      string `json:"framed_ip"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	Mac           string `json:"mac"`
	Loc           string `json:"loc"`
	Downspeed     int    `json:"downspeed"`
	Upspeed       int    `json:"upspeed"`
	Burstspeed    int    `json:"burstspeed"`
	Duration      int    `json:"duration"`
	Gateway       string `json:"gateway"`
	Priority      int    `json:"priority"`
	PfconfigID    uint   `json:"pfconfig_id"`
	DueDay        int    `json:"due_day"`
	OverDue       bool   `json:"over_due"`
	LaterCount    int    `json:"later_count"`
	Plan          string `json:"plan"`
	Pppusername   string `json:"pppusername"`
	Ppppassword   string `json:"ppppassword"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Sub{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Sub) Bind(r *http.Request) error {
	if err := validate.AlnumSpace("first_name", a.FirstName, 100); err != nil {
		return err
	}
	if err := validate.AlnumSpace("last_name", a.LastName, 100); err != nil {
		return err
	}
	if err := validate.Phone("contact_number", a.ContactNumber, 32); err != nil {
		return err
	}
	if err := validate.Email("email", a.Email); err != nil {
		return err
	}
	if err := validate.IP("framed_ip", a.FramedIp); err != nil {
		return err
	}
	if err := validate.Alnum("type", a.Type, 100); err != nil {
		return err
	}
	if err := validate.Alnum("status", a.Status, 100); err != nil {
		return err
	}
	if err := validate.Alnum("mac", a.Mac, 100); err != nil {
		return err
	}
	if err := validate.AlnumSpace("loc", a.Loc, 100); err != nil {
		return err
	}
	if err := validate.Int("downspeed", a.Downspeed); err != nil {
		return err
	}
	if err := validate.Int("upspeed", a.Upspeed); err != nil {
		return err
	}
	if err := validate.Int("burstspeed", a.Burstspeed); err != nil {
		return err
	}
	if err := validate.Int("duration", a.Duration); err != nil {
		return err
	}
	if err := validate.IP("gateway", a.Gateway); err != nil {
		return err
	}
	if err := validate.IntRange("priority", a.Priority, 0, 7); err != nil {
		return err
	}
	if err := validate.Int("due_day", a.DueDay); err != nil {
		return err
	}
	if err := validate.Int("later_count", a.LaterCount); err != nil {
		return err
	}
	if err := validate.Alnum("plan", a.Plan, 100); err != nil {
		return err
	}
	if err := validate.Alnum("pppusername", a.Pppusername, 100); err != nil {
		return err
	}
	// Ppppassword is intentionally not charset-restricted, same reasoning as
	// usermodel.User.Password.
	return validate.MaxLen("ppppassword", a.Ppppassword, 100)
}

// RequireContactDetails is the extra check a new subscriber has to pass on
// top of Bind: the fields needed to bill and reach them must all be filled
// in. It is separate from Bind because Bind also runs on updates, and
// subscribers created before email existed have none - requiring it there
// would stop their records being saved at all.
func (a *Sub) RequireContactDetails() error {
	for _, f := range []struct{ name, value string }{
		{"first_name", a.FirstName},
		{"last_name", a.LastName},
		{"framed_ip", a.FramedIp},
		{"contact_number", a.ContactNumber},
		{"email", a.Email},
	} {
		if err := validate.Required(f.name, f.value); err != nil {
			return err
		}
	}
	return nil
}

type Crud interface {
	GetAll() ([]Sub, error)
	GetById(uid uint) (*Sub, error)
	GetByIp(ip string) (*Sub, error)
	Add(sub *Sub) error
	Update(sub *Sub) error
	Delete(sub *Sub) error
	GetByMac(mac string) (*Sub, error)
	GetByMacPrefix(macprefix string) (*Sub, error)
	MigrateGateway(oldGateway, newGateway string, count int) (int64, error)
	GetDB() *gorm.DB
}

// MigrateGatewayRequest is the request body for Storage.MigrateGateway -
// bulk-reassigns up to Count active subs off OldGateway onto NewGateway,
// e.g. to rebalance load off a WAN link before maintenance. OldGateway and
// NewGateway may both be "" - an empty Gateway is the existing "Default
// gateway" convention (see the dashboard's gateway-distribution chart,
// which already buckets Gateway == "" subs as "Default"), so this lets
// callers migrate subs onto or off of the default route, not just between
// two named gateways.
type MigrateGatewayRequest struct {
	OldGateway string `json:"old_gateway"`
	NewGateway string `json:"new_gateway"`
	Count      int    `json:"count"`
}

// Bind interface as required by go-chi/render
func (a *MigrateGatewayRequest) Bind(r *http.Request) error {
	// validate.IP accepts "" (see its doc comment) - that's relied on here
	// deliberately, not incidentally, to allow the "Default gateway" case
	// described above.
	if err := validate.IP("old_gateway", a.OldGateway); err != nil {
		return err
	}
	if err := validate.IP("new_gateway", a.NewGateway); err != nil {
		return err
	}
	return validate.IntRange("count", a.Count, 1, 100000)
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

func (s *Storage) Add(sub *Sub) error {
	result := s.DB.Create(sub)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Sub, error) {
	var subs []Sub
	result := s.DB.Find(&subs)
	if result.Error != nil {
		return nil, result.Error
	}
	return subs, nil
}

func (s *Storage) GetById(id uint) (*Sub, error) {
	var sub Sub
	result := s.DB.First(&sub, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &sub, nil
}

func (s *Storage) GetByMac(mac string) (*Sub, error) {
	var sub Sub
	result := s.DB.Where("Mac = ?", mac).First(&sub)
	if result.Error != nil {
		return nil, result.Error
	}
	return &sub, nil
}

func (s *Storage) GetByIp(ip string) (*Sub, error) {
	var sub Sub
	result := s.DB.Where("framed_ip = ?", ip).First(&sub)
	if result.Error != nil {
		return nil, result.Error
	}
	return &sub, nil
}

func (s *Storage) GetByMacPrefix(macprefix string) (*Sub, error) {
	var sub Sub
	if macprefix != "" {
		mac := strings.Split(macprefix, "")
		newmac := fmt.Sprintf("%s:%s:%s:%s", strings.Join(mac[0:2], ""), strings.Join(mac[2:4], ""), strings.Join(mac[4:6], ""), strings.Join(mac[6:8], ""))
		result := s.DB.Where("Mac LIKE ?", "%"+newmac+"%").First(&sub)
		if result.Error != nil {
			return nil, result.Error
		}
		return &sub, nil
	}
	return nil, fmt.Errorf("Prefix empty")
}

// MigrateGateway reassigns up to count active subs whose Gateway is
// oldGateway to newGateway - the gorm equivalent of:
//
//	UPDATE subs SET gateway = ?
//	WHERE id IN (SELECT id FROM subs WHERE gateway = ? AND status = 'active' LIMIT ?)
//
// Done as a select-then-update rather than a single nested query so the
// same code works identically on SQLite and Postgres (DB_TYPE). Returns
// how many rows were actually updated.
func (s *Storage) MigrateGateway(oldGateway, newGateway string, count int) (int64, error) {
	var ids []uint
	if err := s.DB.Model(&Sub{}).
		Where("gateway = ? AND status = ?", oldGateway, "active").
		Limit(count).
		Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := s.DB.Model(&Sub{}).Where("id IN ?", ids).Update("gateway", newGateway)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func (s *Storage) Update(sub *Sub) error {
	result := s.DB.Save(sub)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(sub *Sub) error {
	result := s.DB.Delete(sub)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// CheckPlan refuses a plan name that is not a configured plan. The plan name is
// what pf.conf's queue for the subscriber is built from; an unknown one yields a
// queue with no bandwidth, which makes pfctl reject the whole rule set and
// blocks every apply until it is fixed. A blank plan is left alone: subscribers
// on interfaces without shaping have none.
func CheckPlan(db *gorm.DB, plan string) error {
	if plan == "" {
		return nil
	}
	var n int64
	if err := db.Table("plans").Where("plan = ? AND deleted_at IS NULL", plan).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("plan %q does not exist; choose one of the configured plans", plan)
	}
	return nil
}
