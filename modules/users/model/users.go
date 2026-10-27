// Package users - User specific functions
package usermodel

import (
	"fmt"
	"log"
	"net/http"

	"github.com/rbaylon/srvcman/database"
	groupmodel "github.com/rbaylon/srvcman/modules/groups/model"
	"github.com/rbaylon/srvcman/utils/validate"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Username  string             `json:"username" gorm:"unique;not null"`
	Password  string             `json:"password" gorm:"not null"`
	Firstname string             `json:"firstname" gorm:"not null"`
	Lastname  string             `json:"lastname" gorm:"not null"`
	Groups    []groupmodel.Group `json:"groups" gorm:"many2many:user_groups;"`
}

// MigrateDB - Create the table if not exist in DB
//
// The admin account is not seeded here (or anywhere) as a row in this
// table - its credentials live only in .env (APP_ADMIN/APP_ADMIN_PW) and
// are checked directly by security.Login, and validateToken grants it
// full access purely from the "admin" username claim, independent of any
// DB record. See Bind below for why a DB user can't take that username.
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&User{})
	if err != nil {
		log.Fatal(err)
	}
}

// Bind interface as required by go-chi/render
func (a *User) Bind(r *http.Request) error {
	if err := validate.Required("username", a.Username); err != nil {
		return err
	}
	if err := validate.Alnum("username", a.Username, 100); err != nil {
		return err
	}
	// The admin account's username is reserved for the .env-sourced
	// credential security.Login checks directly - a DB user with the same
	// name would just be an unreachable duplicate, shadowed by that check.
	if adminUsername := database.GetEnvVariable("APP_ADMIN"); adminUsername != "" && a.Username == adminUsername {
		return fmt.Errorf("username %q is reserved for the admin account", a.Username)
	}
	// Password is intentionally not charset-restricted (Alnum) - forcing
	// passwords to be alphanumeric-only would weaken them. It's still
	// length-bounded, and a minimum length is enforced since this field is
	// bcrypt-hashed as-is by usermodel.Storage.Add/Update.
	if err := validate.Required("password", a.Password); err != nil {
		return err
	}
	if err := validate.MaxLen("password", a.Password, 100); err != nil {
		return err
	}
	if len(a.Password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if err := validate.Required("firstname", a.Firstname); err != nil {
		return err
	}
	if err := validate.AlnumSpace("firstname", a.Firstname, 100); err != nil {
		return err
	}
	if err := validate.Required("lastname", a.Lastname); err != nil {
		return err
	}
	if err := validate.AlnumSpace("lastname", a.Lastname, 100); err != nil {
		return err
	}
	return nil
}

type Crud interface {
	GetAll() ([]User, error)
	GetById(uid uint) (*User, error)
	Add(user *User) error
	Update(user *User) error
	Delete(user *User) error
	RemoveGroups(user *User) error
	GetByUsername(username string) (*User, error)
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

// Add creates the user. Username is unique and Delete is a soft delete, so a
// deleted user of the same name would make the insert fail; it is purged first.
func (s *Storage) Add(user *User) error {
	if err := s.DB.Unscoped().Where("username = ? AND deleted_at IS NOT NULL", user.Username).Delete(&User{}).Error; err != nil {
		return err
	}
	safepassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), 14)
	if err != nil {
		return err
	}
	user.Password = string(safepassword)
	result := s.DB.Create(user)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) GetAll() ([]User, error) {
	var users []User
	result := s.DB.Preload("Groups").Find(&users)
	if result.Error != nil {
		return nil, result.Error
	}
	return users, nil
}

func (s *Storage) GetById(id uint) (*User, error) {
	var user User
	result := s.DB.Preload("Groups").First(&user, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &user, nil
}

func (s *Storage) GetByUsername(username string) (*User, error) {
	var user User
	result := s.DB.Preload("Groups").Where("Username = ?", username).First(&user)
	if result.Error != nil {
		return nil, result.Error
	}
	return &user, nil
}

func (s *Storage) Update(user *User) error {
	cost, _ := bcrypt.Cost([]byte(user.Password))
	if cost == 0 {
		safepassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), 14)
		if err != nil {
			return err
		}
		user.Password = string(safepassword)
	}
	result := s.DB.Save(user)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) Delete(user *User) error {
	result := s.DB.Delete(user)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *Storage) RemoveGroups(user *User) error {
	err := s.DB.Model(user).Association("Groups").Delete(user.Groups)
	if err != nil {
		return err
	}
	return nil
}
