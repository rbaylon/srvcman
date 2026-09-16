// Package staticroutes - StaticRoute specific functions
package staticroutesmodel

import (
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

// StaticRoute represents a manually managed OpenBSD route: same
// route(8)-via-arkgated mechanism as gatewaymonitormodel.GatewayMonitor
// (see route.go's applyRoute/removeRoute), but for a route the user
// defines directly rather than one derived from a health check. Type
// selects "route add -host" vs "route add -net" - Destination is a plain
// IP for "host", or a CIDR block for "net".
type StaticRoute struct {
	gorm.Model
	Name        string `json:"name"`
	Type        string `json:"type"`
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
	PfconfigID  uint   `json:"pfconfig_id"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&StaticRoute{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *StaticRoute) Bind(r *http.Request) error {
	if err := validate.Alnum("name", a.Name, 100); err != nil {
		return err
	}
	if err := validate.Alnum("type", a.Type, 100); err != nil {
		return err
	}
	if a.Type == "net" {
		if err := validate.CIDR("destination", a.Destination); err != nil {
			return err
		}
	} else if err := validate.IP("destination", a.Destination); err != nil {
		return err
	}
	return validate.IP("gateway", a.Gateway)
}

type Crud interface {
	GetAll() ([]StaticRoute, error)
	GetById(uid uint) (*StaticRoute, error)
	Add(staticroute *StaticRoute) error
	Update(staticroute *StaticRoute) error
	Delete(staticroute *StaticRoute) error
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

func (s *Storage) Add(staticroute *StaticRoute) error {
	result := s.DB.Create(staticroute)
	if result.Error != nil {
		return result.Error
	}
	if err := applyRoute(staticroute.Name, staticroute.Type, staticroute.Destination, staticroute.Gateway); err != nil {
		log.Println("staticroutes: failed to add route:", err)
	}
	return nil
}

func (s *Storage) GetAll() ([]StaticRoute, error) {
	var staticroutes []StaticRoute
	result := s.DB.Find(&staticroutes)
	if result.Error != nil {
		return nil, result.Error
	}
	return staticroutes, nil
}

func (s *Storage) GetById(id uint) (*StaticRoute, error) {
	var staticroute StaticRoute
	result := s.DB.First(&staticroute, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &staticroute, nil
}

func (s *Storage) Update(staticroute *StaticRoute) error {
	var old StaticRoute
	hasOld := s.DB.First(&old, staticroute.ID).Error == nil

	result := s.DB.Save(staticroute)
	if result.Error != nil {
		return result.Error
	}

	if hasOld && old.Destination != "" && (old.Type != staticroute.Type || old.Destination != staticroute.Destination || old.Gateway != staticroute.Gateway) {
		if err := removeRoute(old.Type, old.Destination); err != nil {
			log.Println("staticroutes: failed to remove old route:", err)
		}
	}
	if err := applyRoute(staticroute.Name, staticroute.Type, staticroute.Destination, staticroute.Gateway); err != nil {
		log.Println("staticroutes: failed to update route:", err)
	}
	return nil
}

func (s *Storage) Delete(staticroute *StaticRoute) error {
	result := s.DB.Delete(staticroute)
	if result.Error != nil {
		return result.Error
	}
	if err := removeRoute(staticroute.Type, staticroute.Destination); err != nil {
		log.Println("staticroutes: failed to remove route:", err)
	}
	return nil
}
