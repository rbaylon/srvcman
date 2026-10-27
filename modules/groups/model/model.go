// Package groups - Group specific functions
package groupmodel

import (
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

type Group struct {
	gorm.Model
	Groupname string `json:"groupname" gorm:"unique;not null"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Group{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Group) Bind(r *http.Request) error {
	if err := validate.Required("groupname", a.Groupname); err != nil {
		return err
	}
	return validate.Alnum("groupname", a.Groupname, 100)
}

type Crud interface {
	GetAll() ([]Group, error)
	GetById(uid uint) (*Group, error)
	Add(group *Group) error
	Update(group *Group) error
	Delete(group *Group) error
	GetByGroupname(groupname string) (*Group, error)
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

// Add creates the group. Groupname is unique and Delete is a soft delete, so a
// deleted group of the same name would make the insert fail; it is purged first.
func (s *Storage) Add(group *Group) error {
	if err := s.DB.Unscoped().Where("groupname = ? AND deleted_at IS NOT NULL", group.Groupname).Delete(&Group{}).Error; err != nil {
		return err
	}
	result := s.DB.Create(group)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Group, error) {
	var groups []Group
	result := s.DB.Find(&groups)
	if result.Error != nil {
		return nil, result.Error
	}
	return groups, nil
}

func (s *Storage) GetById(id uint) (*Group, error) {
	var group Group
	result := s.DB.First(&group, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &group, nil
}

func (s *Storage) GetByGroupname(groupname string) (*Group, error) {
	var group Group
	result := s.DB.Where("Groupname = ?", groupname).First(&group)
	if result.Error != nil {
		return nil, result.Error
	}
	return &group, nil
}

func (s *Storage) Update(group *Group) error {
	result := s.DB.Save(group)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(group *Group) error {
	result := s.DB.Delete(group)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
