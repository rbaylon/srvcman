// Package groups - Group specific functions
package groupmodel

import (
	"log"
	"net/http"

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
	return nil
}

type Crud interface {
	GetAll() ([]Group, error)
	GetById(uid uint) (*Group, error)
	Add(group *Group) error
	Update(group *Group) error
	Delete(group *Group) error
	GetByGroupname(groupname string) (*Group, error)
}

type Storage struct {
	DB *gorm.DB
}

func New(db *gorm.DB) *Storage {
	return &Storage{
		DB: db,
	}
}

func (s *Storage) Add(group *Group) error {
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
