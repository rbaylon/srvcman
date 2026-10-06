package security

import (
	"crypto/subtle"
	"errors"
	"fmt"
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

// adminUsername/adminPassword are the sole source of the admin account's
// credentials - it is not a row in the users table (see usermodel.MigrateDB),
// so Login checks a request against these directly instead of going through
// usermodel.Crud. Every admin decision - login, token lifetime, full access
// in validateToken - goes through isAdmin, so the account can be named
// anything APP_ADMIN says.
var (
	adminUsername = database.GetEnvVariable("APP_ADMIN")
	adminPassword = database.GetEnvVariable("APP_ADMIN_PW")
)

// isAdmin reports whether username is the .env-configured admin account.
// An unset APP_ADMIN matches nothing, so an empty username can never be
// mistaken for the admin.
func isAdmin(username string) bool {
	return adminUsername != "" && username == adminUsername
}

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
		fmt.Println(r.RequestURI, r.Method, r.Header.Get("X-Real-IP"))
		username, password, ok := r.BasicAuth()
		if !ok {
			render.Render(w, r, utils.ErrInvalidRequest(fmt.Errorf("Baisc Auth error"), "Credentials not found.", http.StatusUnauthorized))
			return
		}

		if isAdmin(username) {
			if subtle.ConstantTimeCompare([]byte(password), []byte(adminPassword)) != 1 || adminPassword == "" {
				render.Render(w, r, utils.ErrInvalidRequest(fmt.Errorf("Baisc Auth error"), "Password invalid.", http.StatusUnauthorized))
				return
			}
			sendtoken(w, r, username)
			return
		}

		user, err := db.GetByUsername(username)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			render.Render(w, r, utils.ErrInvalidRequest(fmt.Errorf("Baisc Auth error"), "Password invalid.", http.StatusUnauthorized))
			return
		}
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
			fmt.Println(r.Method, r.RequestURI)
			reqtoken := r.Header.Get("Authorization")
			if !strings.Contains(reqtoken, "Bearer ") {
				render.Render(w, r, utils.ErrInvalidRequest(fmt.Errorf("JWT error"), "Authorization Bearer not found.", http.StatusUnauthorized))
				return
			}
			//fmt.Println(reqtoken)
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
	if isAdmin(username) {
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
		// createToken only ever signs with HS256 - reject anything else
		// up front instead of relying on the keyfunc always handing back
		// the same HMAC secret being *incidentally* safe against a
		// crafted "alg" header (e.g. "none", which golang-jwt v3 only
		// blocks because SigningMethodNone.Verify specifically rejects
		// any key that isn't its own sentinel type). Explicitly checking
		// the method here is the standard golang-jwt-recommended pattern
		// and makes this inspectably safe rather than safe by an
		// implementation detail of a specific algorithm's Verify func.
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
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
		// An unchecked user.(string) here would panic on a token whose
		// username claim isn't a JSON string - unreachable with tokens
		// this server itself issues (createToken always sets it to a
		// string), but a crafted token signed with a leaked secret, or a
		// bug in a future caller of createToken, shouldn't be able to
		// crash request handling.
		username, ok = claims["username"].(string)
		if !ok {
			return fmt.Errorf("Invalid token.")
		}
		if isAdmin(username) {
			fmt.Printf("Access granted: %s %s %s from %s\n", username, method, path, remote)
			data := fmt.Sprintf("%s %s %s %s", path, method, username, remote)
			if !strings.Contains(method, "GET") {
				audittrail(data, module, db)
			}
			return nil
		}
		// userdata/poldata errors (a DB hiccup, or a stale JWT for a
		// since-deleted user) used to be silently ignored here, and
		// userdata.Groups below was dereferenced unconditionally -
		// nil-pointer panic on every non-admin request while userdb is
		// erroring. Skip the loop instead so deny stays true and falls
		// through to the ordinary "Access denied" handling below.
		userdb := usermodel.New(db)
		userdata, userErr := userdb.GetByUsername(username)
		poldb := policymodel.New(db)
		poldata, polErr := poldb.GetByType(module)
		if userErr != nil || polErr != nil {
			poldata = nil
		}
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
		fmt.Printf("Access denied: %s %s %s from %s\n", username, method, path, remote)
		return fmt.Errorf("Access denied.")
	}
	fmt.Printf("Access granted: %s %s %s from %s\n", username, method, path, remote)
	data := fmt.Sprintf("%s %s %s %s", path, method, username, remote)
	if !strings.Contains(method, "GET") {
		audittrail(data, module, db)
	}

	return nil
}

func audittrail(data string, module string, db *gorm.DB) {
	chdb := changemodel.New(db)
	ch := changemodel.Change{}
	ch.Module = module
	ch.Data = data
	ch.Applied = "pending"
	err := chdb.Add(&ch)
	if err != nil {
		fmt.Printf("Failed to create %s change: %s", module, data)
	} else {
		fmt.Printf("%s change success: %s", module, data)
	}
}
