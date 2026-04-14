package security

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
	"gorm.io/gorm"

	"github.com/go-chi/render"
	"github.com/rbaylon/srvcman/database"
	changemodel "github.com/rbaylon/srvcman/modules/changes/model"
	policymodel "github.com/rbaylon/srvcman/modules/policies/model"
	usermodel "github.com/rbaylon/srvcman/modules/users/model"
	"github.com/rbaylon/srvcman/utils"
	"golang.org/x/crypto/bcrypt"
)

var secretKey = []byte(database.GetEnvVariable("APP_SECRET"))

type Token struct {
	Name string
	Jwt  string
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func Login(db usermodel.Crud) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.RequestURI, r.Method, r.Header.Get("X-Real-IP"))
		username, password, ok := r.BasicAuth()
		if ok {
			user, err := db.GetByUsername(username)
			if err != nil {
				render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
				return
			}

			// If password is plaintext only, use password encryption and store it back to db
			if user.Password == password {
				encryptedpassword, err := HashPassword(password)
				if err != nil {
					render.Render(w, r, utils.ErrInvalidRequest(err, "Bcrypt Error encrypting password.", http.StatusInternalServerError))
				}
				user.Password = encryptedpassword
				err = db.Update(user)
				if err != nil {
					render.Render(w, r, utils.ErrInvalidRequest(err, "DB update error", http.StatusInternalServerError))
					return
				}
				sendtoken(w, r, user.Username)
				return
			}
			// Check already hashed password
			if CheckPasswordHash(password, user.Password) {
				sendtoken(w, r, user.Username)
				return
			}
			render.Render(w, r, utils.ErrInvalidRequest(fmt.Errorf("Baisc Auth error"), "Password invalid.", http.StatusUnauthorized))
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(fmt.Errorf("Baisc Auth error"), "Credentials not found.", http.StatusUnauthorized))
	})
}

func sendtoken(w http.ResponseWriter, r *http.Request, username string) {
	tokenString, err := createToken(username)
	if err != nil {
		render.Render(w, r, utils.ErrInvalidRequest(err, "JWT error", http.StatusInternalServerError))
		return
	}
	accesstoken := &Token{Name: "AccessToken", Jwt: tokenString}
	render.JSON(w, r, accesstoken)
}

func TokenRequired(db *gorm.DB, module string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Println(r.Method, r.RequestURI)
			reqtoken := r.Header.Get("Authorization")
			if !strings.Contains(reqtoken, "Bearer ") {
				render.Render(w, r, utils.ErrInvalidRequest(fmt.Errorf("JWT error"), "Authorization Bearer not found.", http.StatusUnauthorized))
				return
			}
			//log.Println(reqtoken)
			token := reqtoken[len("Bearer "):]
			err := validateToken(token, r, db, module)
			if err != nil {
				render.Render(w, r, utils.ErrInvalidRequest(err, "Invalid token", http.StatusUnauthorized))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func createToken(username string) (string, error) {
	var expiry time.Duration = 1
	if username == "admin" {
		expiry = 8760
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{
			"username": username,
			"exp":      time.Now().Add(time.Hour * expiry).Unix(),
		})

	tokenString, err := token.SignedString(secretKey)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func validateToken(tokenString string, r *http.Request, db *gorm.DB, module string) error {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return secretKey, nil
	})
	if err != nil {
		return err
	}
	if !token.Valid {
		return fmt.Errorf("Invalid token.")
	}
	method := r.Method
	path := r.RequestURI
	remote := r.Header.Get("X-Real-IP")
	deny := true
	username := ""
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		user := claims["username"]
		username = user.(string)
		if username == "admin" {
			log.Printf("Access granted: %s %s %s from %s\n", username, method, path, remote)
			return nil
		}
		userdb := usermodel.New(db)
		userdata, _ := userdb.GetByUsername(username)
		poldb := policymodel.New(db)
		poldata, _ := poldb.GetByType(module)
		for _, p := range poldata {
			for _, pg := range p.Groups {
				for _, ug := range userdata.Groups {
					if ug.Groupname == pg.Groupname {
						switch p.Action {
						case "RW":
							//deny = false
							if p.Resource != "any" {
								if path == p.Resource {
									deny = false
								}
							} else {
								deny = false
							}
						case "RO":
							if r.Method == "GET" {
								//deny = false
								if p.Resource != "any" {
									if path == p.Resource {
										deny = false
									}
								} else {
									deny = false
								}
							}
						}
					}
				}
			}
		}
	}
	if deny {
		log.Printf("Access denied: %s %s %s from %s\n", username, method, path, remote)
		return fmt.Errorf("Access denied.")
	}
	log.Printf("Access granted: %s %s %s from %s\n", username, method, path, remote)
	if strings.TrimSpace(method) == "PUT" || strings.TrimSpace(method) == "POST" || strings.TrimSpace(method) == "DELETE" {
		data := fmt.Sprintf("%s %s %s %s", path, method, username, remote)
		chdb := changemodel.New(db)
		ch := changemodel.Change{}
		ch.Module = module
		ch.Data = data
		ch.Applied = false
		err = chdb.Add(&ch)
		if err != nil {
			log.Printf("Failed to create %s change: %s", module, data)
		} else {
			log.Printf("%s change success: %s", module, data)
		}
	}

	return nil
}
