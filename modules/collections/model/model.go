// Package collections - Collection specific functions
package collectionmodel

import (
	"log"
	"net/http"

	submodel "github.com/rbaylon/srvcman/modules/subs/model"
	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

type Collection struct {
	gorm.Model
	Year    string `json:"year"`
	Jan     int    `json:"jan"`
	Feb     int    `json:"feb"`
	Mar     int    `json:"mar"`
	Apr     int    `json:"apr"`
	May     int    `json:"may"`
	Jun     int    `json:"jun"`
	Jul     int    `json:"jul"`
	Aug     int    `json:"aug"`
	Sep     int    `json:"sep"`
	Oct     int    `json:"oct"`
	Nov     int    `json:"nov"`
	Dec     int    `json:"dec"`
	SubID   uint   `json:"sub_id"`
	SubName string `json:"sub_name"`
	SubDue  int    `json:"sub_due"`
	SubPlan string `json:"sub_plan"`
}

type Collections struct {
	Collections []Collection `json:"collections"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Collection{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Collection) Bind(r *http.Request) error {
	if err := validate.Alnum("year", a.Year, 100); err != nil {
		return err
	}
	for field, v := range map[string]int{
		"jan": a.Jan, "feb": a.Feb, "mar": a.Mar, "apr": a.Apr,
		"may": a.May, "jun": a.Jun, "jul": a.Jul, "aug": a.Aug,
		"sep": a.Sep, "oct": a.Oct, "nov": a.Nov, "dec": a.Dec,
		"sub_due": a.SubDue,
	} {
		if err := validate.Int(field, v); err != nil {
			return err
		}
	}
	if err := validate.AlnumSpace("sub_name", a.SubName, 100); err != nil {
		return err
	}
	return validate.Alnum("sub_plan", a.SubPlan, 100)
}

func (a *Collections) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Collection, error)
	GetBySubId(id uint) ([]Collection, error)
	GetById(uid uint) (*Collection, error)
	GetByYear(id string) ([]Collection, error)
	Add(collection *Collection) error
	AddBulk(collections []Collection) error
	Update(collection *Collection) error
	Delete(collection *Collection) error
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

func (s *Storage) AddBulk(collections []Collection) error {
	result := s.DB.Create(collections)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetDB() *gorm.DB {
	return s.DB
}

func (s *Storage) Add(collection *Collection) error {
	result := s.DB.Create(collection)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Collection, error) {
	var collections []Collection
	result := s.DB.Find(&collections)
	if result.Error != nil {
		return nil, result.Error
	}
	return collections, nil
}

func (s *Storage) GetByYear(y string) ([]Collection, error) {
	var collections []Collection
	result := s.DB.Where("year = ?", y).Find(&collections)
	if result.Error != nil {
		return nil, result.Error
	}
	dbconn := s.GetDB()
	subdb := submodel.New(dbconn)
	for k, v := range collections {
		sub, err := subdb.GetById(v.SubID)
		if err != nil {
			log.Println("Error getting sub name from collections", err)
			continue
		}
		collections[k].SubName = sub.FirstName + " " + sub.LastName
		collections[k].SubDue = sub.DueDay
		collections[k].SubPlan = sub.Plan
	}
	return collections, nil
}

func (s *Storage) GetById(id uint) (*Collection, error) {
	var collection Collection
	result := s.DB.First(&collection, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &collection, nil
}

func (s *Storage) GetBySubId(value uint) ([]Collection, error) {
	var collections []Collection
	result := s.DB.Where("sub_id = ?", value).Find(&collections)
	if result.Error != nil {
		return nil, result.Error
	}
	return collections, nil
}

func (s *Storage) Update(collection *Collection) error {
	result := s.DB.Save(collection)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(collection *Collection) error {
	result := s.DB.Delete(collection)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
