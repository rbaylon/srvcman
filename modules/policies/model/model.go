// Package policys - Policy specific functions
package policymodel

import (
	"log"
	"net/http"

	groupmodel "github.com/rbaylon/srvcman/modules/groups/model"
	"gorm.io/gorm"
)

type Policy struct {
	gorm.Model
	Name     string             `json:"name" gorm:"unique;not null"`
	Type     string             `json:"type"`
	Resource string             `json:"resource"`
	Action   string             `json:"action"`
	Groups   []groupmodel.Group `json:"groups" gorm:"many2many:policy_groups;"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Policy{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *Policy) Bind(r *http.Request) error {
	return nil
}

type Crud interface {
	GetAll() ([]Policy, error)
	GetById(uid uint) (*Policy, error)
	Add(policy *Policy) error
	Update(policy *Policy) error
	Delete(policy *Policy) error
	RemoveGroups(policy *Policy) error
	GetByPolicyname(policyname string) (*Policy, error)
}

type Storage struct {
	DB *gorm.DB
}

func New(db *gorm.DB) *Storage {
	return &Storage{
		DB: db,
	}
}

func (s *Storage) Add(policy *Policy) error {
	result := s.DB.Create(policy)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]Policy, error) {
	var policys []Policy
	result := s.DB.Find(&policys)
	if result.Error != nil {
		return nil, result.Error
	}
	return policys, nil
}

func (s *Storage) GetById(id uint) (*Policy, error) {
	var policy Policy
	result := s.DB.Preload("Groups").First(&policy, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &policy, nil
}

func (s *Storage) GetByPolicyname(policyname string) (*Policy, error) {
	var policy Policy
	result := s.DB.Preload("Groups").Where("Policyname = ?", policyname).First(&policy)
	if result.Error != nil {
		return nil, result.Error
	}
	return &policy, nil
}

func (s *Storage) Update(policy *Policy) error {
	result := s.DB.Save(policy)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(policy *Policy) error {
	result := s.DB.Delete(policy)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) RemoveGroups(policy *Policy) error {
	err := s.DB.Model(policy).Association("Groups").Delete(policy.Groups)
	if err != nil {
		return err
	}
	admins := groupmodel.Group{}
	res := s.DB.Where("Groupname = ?", "admins").First(&admins)
	if res.Error != nil {
		return res.Error
	}
	groups := []groupmodel.Group{admins}
	policy.Groups = groups
	s.DB.Save(policy)
	return nil
}
