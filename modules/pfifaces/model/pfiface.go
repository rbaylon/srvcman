// Package pfifaces - Pfiface specific functions
package pfifacemodel

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	Acmd "github.com/rbaylon/arkgatecmd"
	vlanmodel "github.com/rbaylon/srvcman/modules/vlans/model"
	"github.com/rbaylon/srvcman/utils"
	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

// arkCmdTimeout bounds how long ListDevices/GetTrafficStats wait for
// arkgated to run ifconfig/netstat and reply.
const arkCmdTimeout = 15 * time.Second

// runViaArk dispatches a read-only introspection command (ifconfig,
// netstat) to arkgated and returns its output. srvcman may not run on the
// same host as the gateway being inspected anymore, so these can no longer
// be exec'd locally - only arkgated (on the actual OpenBSD box) can report
// on its own interfaces/traffic.
func runViaArk(name, binpath string, opts []string) (string, error) {
	conn := utils.GetArkConn()
	if conn == nil {
		return "", fmt.Errorf("arkgated unreachable")
	}
	conn.SetDeadline(time.Now().Add(arkCmdTimeout))
	cmd := Acmd.PrepareCmd(name, binpath, opts)
	return cmd.SendCmdOutput(conn)
}

type Pfiface struct {
	gorm.Model
	Name         string `json:"name"`
	Speed        string `json:"speed"`
	Device       string `json:"device"`
	Default      bool   `json:"default"`
	Physical     bool   `json:"physical" gorm:"default:true"`
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
	if err := validate.Alnum("name", a.Name, 100); err != nil {
		return err
	}
	if err := validate.Alnum("speed", a.Speed, 100); err != nil {
		return err
	}
	if err := validate.Alnum("device", a.Device, 100); err != nil {
		return err
	}
	if err := validate.Alnum("type", a.Type, 100); err != nil {
		return err
	}
	if err := validate.IP("gateway", a.Gateway); err != nil {
		return err
	}
	if err := validate.IP("ip", a.Ip); err != nil {
		return err
	}
	if err := validate.IP("netmask", a.Netmask); err != nil {
		return err
	}
	return validate.IntRange("lb_percentage", a.LbPercentage, 0, 100)
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
	GetByDevice(d string) (*Pfiface, error)
	GetPhysical() ([]Pfiface, error)
	ListDevices() ([]string, error)
	GetTrafficStats() ([]TrafficStat, error)
	GetDB() *gorm.DB
}

// TrafficStat is a point-in-time snapshot of an interface's cumulative byte
// counters, as reported by the OS. Callers poll this repeatedly and diff
// successive snapshots to derive a transfer rate.
type TrafficStat struct {
	Device string `json:"device"`
	Ibytes uint64 `json:"ibytes"`
	Obytes uint64 `json:"obytes"`
}

type Storage struct {
	DB *gorm.DB
}

func New(db *gorm.DB) *Storage {
	return &Storage{
		DB: db,
	}
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

func (s *Storage) GetByDevice(value string) (*Pfiface, error) {
	var pfiface Pfiface
	result := s.DB.Where("Device = ?", value).First(&pfiface)
	if result.Error != nil {
		return nil, result.Error
	}
	return &pfiface, nil
}

func (s *Storage) GetPhysical() ([]Pfiface, error) {
	var pfifaces []Pfiface
	result := s.DB.Where("Physical = ?", true).Find(&pfifaces)
	if result.Error != nil {
		return nil, result.Error
	}
	return pfifaces, nil
}

// pseudoDevicePrefixes are OpenBSD software/cloning interfaces to exclude
// from ListDevices so only physical NICs are reported (VLAN devices are
// reported separately, from the vlans table).
var pseudoDevicePrefixes = []string{
	"lo", "enc", "pflog", "pfsync", "vlan", "trunk", "carp", "wg", "tun", "tap",
	"bridge", "gif", "gre", "mpe", "mpw", "ppp", "pppoe", "sec", "veb", "vport",
	"svlan", "vether", "aggr",
}

func isPseudoDevice(name string) bool {
	for _, prefix := range pseudoDevicePrefixes {
		rest := strings.TrimPrefix(name, prefix)
		if rest == name {
			continue
		}
		if _, err := strconv.Atoi(rest); err == nil {
			return true
		}
	}
	return false
}

// listInterfaceNames runs ifconfig on this host and extracts the interface
// names present, e.g. "em0: flags=8843<UP,..." -> "em0".
func listInterfaceNames() ([]string, error) {
	out, err := runViaArk("ListInterfaces", "/bin/sh", []string{"-c", "ifconfig -a | grep -Eo '^[a-zA-Z0-9]+:' | tr -d ':'"})
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

// ListDevices returns the physical network interfaces present on this host,
// plus any VLAN interface names configured in the vlans table.
func (s *Storage) ListDevices() ([]string, error) {
	devices := []string{}
	seen := map[string]bool{}
	names, err := listInterfaceNames()
	if err != nil {
		log.Println("ListDevices: failed to list interfaces:", err)
	}
	for _, name := range names {
		if isPseudoDevice(name) || seen[name] {
			continue
		}
		seen[name] = true
		devices = append(devices, name)
	}
	vlandb := vlanmodel.New(s.GetDB())
	vlans, err := vlandb.GetAll()
	if err != nil {
		log.Println("ListDevices: failed to get vlans:", err)
	}
	for _, v := range vlans {
		for _, name := range []string{v.Name, v.ParentDevice} {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			devices = append(devices, name)
		}
	}
	return devices, nil
}

// GetTrafficStats reads current per-interface byte counters from this host
// via `netstat -ibn`. OpenBSD's -b flag inserts Ibytes/Obytes columns into
// the default -i layout: Name Mtu Network Address Ipkts Ierrs Ibytes Opkts
// Oerrs Obytes Coll. Each interface can appear on multiple rows (one per
// address family); the counters are identical on every row for that
// interface, so only the first row per device is kept.
//
// Network/Address are frequently blank (down interfaces, ones with no
// configured link address, pseudo-devices), and strings.Fields drops blank
// columns entirely, so those leading columns can't be indexed by a fixed
// position from the left. The trailing 7 numeric columns (Ipkts Ierrs
// Ibytes Opkts Oerrs Obytes Coll) are always present, so they're indexed
// from the end of the row instead.
func (s *Storage) GetTrafficStats() ([]TrafficStat, error) {
	out, err := runViaArk("Netstat", "/usr/bin/netstat", []string{"-ibn"})
	if err != nil {
		return nil, err
	}
	stats := []TrafficStat{}
	seen := map[string]bool{}
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		//fmt.Println(line)
		if i == 0 {
			continue
		}
		fields := strings.Fields(line)
		fieldslength := len(fields)
		if fieldslength < 6 {
			continue
		}
		name := fields[0]
		if seen[name] {
			continue
		}
		ibytes, errI := strconv.ParseUint(fields[4], 10, 64)
		obytes, errO := strconv.ParseUint(fields[5], 10, 64)
		if errI != nil || errO != nil {
			continue
		}
		seen[name] = true
		stats = append(stats, TrafficStat{Device: name, Ibytes: ibytes, Obytes: obytes})
	}
	return stats, nil
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
