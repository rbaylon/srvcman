// Package tunnels - Tunnel specific functions
package tunnelmodel

import (
	"fmt"
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

// Tunnel types, selecting which ssh(1) forwarding flag is used.
const (
	// TypeRemote is `ssh -R remote_ip:remote_port:local_ip:local_port` -
	// the remote side listens and forwards back here.
	TypeRemote = "remote"
	// TypeLocal is `ssh -L local_ip:local_port:remote_ip:remote_port` -
	// this side listens and forwards out to the remote.
	TypeLocal = "local"
)

// Status values reported on Tunnel.Status.
const (
	StatusActive   = "active"
	StatusInactive = "inactive"
)

// Tunnel is one ssh(1) port-forward maintained on the gateway box. The
// ssh process itself runs on arkgated's host (see tunnel_ssh.go), not in
// srvcman - Add establishes it, Delete tears it down, and Update restarts
// it, the same apply-on-write pattern staticroutesmodel uses for route(8).
//
// Status/Pid/Age are not stored: they're filled in per request from the
// live process list on the gateway (one `ps` call covering every tunnel,
// see Statuses), so they stay correct across both srvcman restarts and
// gateway reboots - an ssh forward doesn't survive either, and a Pid
// recorded in the DB would just go stale.
type Tunnel struct {
	gorm.Model
	Name       string `json:"name"`
	Type       string `json:"type"`
	RemoteUser string `json:"remote_user"`
	RemoteIp   string `json:"remote_ip"`
	RemotePort int    `json:"remote_port"`
	LocalIp    string `json:"local_ip"`
	LocalPort  int    `json:"local_port"`
	Status     string `json:"status" gorm:"-"`
	Pid        int    `json:"pid" gorm:"-"`
	Age        string `json:"age" gorm:"-"`
}

type Tunnels struct {
	Tunnels []Tunnel `json:"tunnels"`
}

// legacyColumns are from the pre-rewrite schema, when a Tunnel was a
// router name plus a single port. AutoMigrate only ever adds columns, so
// these would survive the rewrite - and "router"/"port" were declared
// NOT NULL UNIQUE, which makes every insert against the new schema fail.
// They have to be dropped, not just ignored.
var legacyColumns = []string{"router", "port", "active", "pid"}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Tunnel{})
	if err != nil {
		log.Fatal(err)
	}
	for _, col := range legacyColumns {
		if !db.Migrator().HasColumn(&Tunnel{}, col) {
			continue
		}
		if err := db.Migrator().DropColumn(&Tunnel{}, col); err != nil {
			log.Println("tunnels: failed to drop legacy column", col, ":", err)
		}
	}
}

// Bind interface as required by go-chi/render.
//
// These values end up interpolated into the ssh command line built in
// tunnel_ssh.go, so validation here is load-bearing, not just a nicety:
// RemoteUser in particular must not be able to look like an ssh option
// (e.g. "-oProxyCommand=...").
func (a *Tunnel) Bind(r *http.Request) error {
	if err := validate.Required("name", a.Name); err != nil {
		return err
	}
	if err := validate.Alnum("name", a.Name, 100); err != nil {
		return err
	}
	if a.Type != TypeRemote && a.Type != TypeLocal {
		return fmt.Errorf("type must be %q or %q", TypeRemote, TypeLocal)
	}
	if err := validate.Required("remote_user", a.RemoteUser); err != nil {
		return err
	}
	if err := validate.Alnum("remote_user", a.RemoteUser, 32); err != nil {
		return err
	}
	if err := validate.IP("remote_ip", a.RemoteIp); err != nil {
		return err
	}
	if err := validate.IntRange("remote_port", a.RemotePort, 1, 65535); err != nil {
		return err
	}
	if err := validate.IP("local_ip", a.LocalIp); err != nil {
		return err
	}
	return validate.IntRange("local_port", a.LocalPort, 1, 65535)
}

func (a *Tunnels) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Tunnel, error)
	GetById(uid uint) (*Tunnel, error)
	Add(tunnel *Tunnel) error
	AddBulk(tunnels []Tunnel) error
	Update(tunnel *Tunnel) error
	Delete(tunnel *Tunnel) error
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

func (s *Storage) AddBulk(tunnels []Tunnel) error {
	result := s.DB.Create(tunnels)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
}

func (s *Storage) Add(tunnel *Tunnel) error {
	result := s.DB.Create(tunnel)
	if result.Error != nil {
		return result.Error
	}
	if err := Start(tunnel); err != nil {
		log.Println("tunnels: failed to start tunnel", tunnel.Name, ":", err)
	}
	return nil
}

func (s *Storage) GetAll() ([]Tunnel, error) {
	var tunnels []Tunnel
	result := s.DB.Find(&tunnels)
	if result.Error != nil {
		return nil, result.Error
	}
	return tunnels, nil
}

func (s *Storage) GetById(id uint) (*Tunnel, error) {
	var tunnel Tunnel
	result := s.DB.First(&tunnel, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &tunnel, nil
}

// Update restarts the tunnel when anything the ssh command line is built
// from changed - the running ssh process was started with the old
// forwarding spec and won't pick up the new one by itself.
func (s *Storage) Update(tunnel *Tunnel) error {
	var old Tunnel
	hasOld := s.DB.First(&old, tunnel.ID).Error == nil

	result := s.DB.Save(tunnel)
	if result.Error != nil {
		return result.Error
	}

	if hasOld && ForwardSpec(&old) == ForwardSpec(tunnel) && old.Type == tunnel.Type && old.RemoteUser == tunnel.RemoteUser {
		return nil
	}
	if hasOld {
		if err := Stop(&old); err != nil {
			log.Println("tunnels: failed to stop old tunnel", old.Name, ":", err)
		}
	}
	if err := Start(tunnel); err != nil {
		log.Println("tunnels: failed to restart tunnel", tunnel.Name, ":", err)
	}
	return nil
}

// Delete tears the ssh process down as well as removing the record -
// otherwise the forward would keep running on the gateway with nothing
// left in srvcman pointing at it.
func (s *Storage) Delete(tunnel *Tunnel) error {
	full, err := s.GetById(tunnel.ID)
	if err != nil {
		full = tunnel
	}
	result := s.DB.Delete(tunnel)
	if result.Error != nil {
		return result.Error
	}
	if err := Stop(full); err != nil {
		log.Println("tunnels: failed to stop tunnel", full.Name, ":", err)
	}
	return nil
}
