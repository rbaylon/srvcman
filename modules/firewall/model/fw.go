// Package Fws - User specific functions
package fwmodel

import (
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"

	pfifacemodel "github.com/rbaylon/srvcman/modules/pfifaces/model"
	planmodel "github.com/rbaylon/srvcman/modules/plans/model"
	"gorm.io/gorm"
)

type Fw struct {
	gorm.Model
	Status        string `json:"status"`
	Action        string `json:"action"`
	Direction     string `json:"direction"`
	Log           string `json:"log"`
	Quick         string `json:"quick"`
	Interface     string `json:"interface"`
	AddressFamily string `json:"address_family"`
	Proto         string `json:"protocol"`
	Source        string `json:"source"`
	SrcPort       string `json:"source_port"`
	Destination   string `json:"destination"`
	DstPort       string `json:"destination_port"`
	Gateway       string `json:"gateway"`
	Priority      int    `json:"priority"`
	PfconfigID    uint   `json:"pfconfig_id"`
	Pipe          string `json:"pipe"`
	AdvanceOpts   string `json:"advance_options"`
	Rule          string
	OutRule       string
	QRule         string
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Fw{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Fw) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll(opt string) ([]Fw, error)
	GetById(uid uint) (*Fw, error)
	Add(Fw *Fw) error
	Update(Fw *Fw) error
	Delete(Fw *Fw) error
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

func (s *Storage) Add(Fw *Fw) error {
	ToRule(s, Fw)
	result := s.DB.Create(Fw)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func GenRandomString() string {
	var letters = []byte("abcdefghijkmnopqrstuvwxyzABCDEFGHIJKLMNPQRSTUVWXYZ123456789")
	code := []byte("00000000")
	maxlen := len(letters) - 1
	for j := 0; j < 8; j++ {
		c := letters[rand.IntN(maxlen)]
		code[j] = c
	}
	return string(code)
}

func ToRule(s *Storage, v *Fw) {
	quick := ""
	if v.Quick == "on" {
		quick = "quick"
	}
	iface := v.Interface
	if v.Interface != "any" {
		iface = "on " + v.Interface
	}
	proto := v.Proto
	if v.Proto != "" {
		proto = "proto " + v.Proto
	}
	src := v.Source
	if v.Source == "" {
		src = "any"
	}
	sport := v.SrcPort
	if v.SrcPort != "" {
		sport = "port " + v.SrcPort
	}
	dst := v.Destination
	if v.Destination == "" {
		dst = "any"
	}
	dport := v.DstPort
	if v.DstPort != "" {
		dport = "port " + v.DstPort
	}
	gw := v.Gateway
	if v.Gateway != "" {
		gw = "route-to " + v.Gateway
	}
	prio := ""
	if v.Priority > 0 {
		prio = fmt.Sprintf("set prio %d", v.Priority)
	}
	plan := planmodel.Plan{}
	result := s.DB.Where("plan = ?", v.Pipe).First(&plan)
	q := true
	if result.Error != nil {
		log.Println("DB errror on Plans: ", result.Error)
		q = false
	}
	uid := GenRandomString()
	pfifaces := []pfifacemodel.Pfiface{}
	result = s.DB.Find(&pfifaces)
	if result.Error != nil {
		log.Println("DB errror on Plans: ", result.Error)
	}
	pipe := ""
	tag := ""
	for _, i := range pfifaces {
		if i.Type == "external" && q {
			if q {
				v.QRule = fmt.Sprintf(
					"%squeue %s%s parent %s bandwidth %dM min 5M max %dM qlimit 1024\n",
					v.QRule, uid, i.Name, i.Name, plan.Upspeed, plan.Upspeed)
				v.OutRule = fmt.Sprintf(
					"%spass out on $%s set queue (%s%s, %slow)  tagged \"%s\"\n",
					v.OutRule, i.Name, uid, i.Name, i.Name, uid)
			}
		}
		if v.Interface == i.Device {
			if q {
				v.QRule = fmt.Sprintf(
					"%squeue %s%s parent %s bandwidth %dM min 5M max %dM qlimit 1024\n",
					v.QRule, uid, i.Name, i.Name, plan.Downspeed, plan.Downspeed)
				pipe = fmt.Sprintf("set queue (%s%s, %slow)", uid, i.Name, i.Name)
				tag = fmt.Sprintf("tag \"%s\"", uid)
			}
			v.Rule = fmt.Sprintf("%s %s %s %s %s %s %s from %s %s to %s %s %s %s %s %s %s\n",
				v.Action, v.Direction, v.Log, quick, iface, v.AddressFamily, proto, src,
				sport, dst, dport, gw, prio, pipe, v.AdvanceOpts, tag)
		}
	}
}

func (s *Storage) GetAll(option string) ([]Fw, error) {
	var Fws []Fw
	result := s.DB.Find(&Fws)
	if result.Error != nil {
		return nil, result.Error
	}
	return Fws, nil
}

func (s *Storage) GetById(id uint) (*Fw, error) {
	var Fw Fw
	result := s.DB.First(&Fw, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &Fw, nil
}

func (s *Storage) Update(Fw *Fw) error {
	ToRule(s, Fw)
	result := s.DB.Save(Fw)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(Fw *Fw) error {
	result := s.DB.Delete(Fw)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
