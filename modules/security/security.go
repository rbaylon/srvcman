package security

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt"

	"github.com/go-chi/render"
	"github.com/rbaylon/srvcman/database"
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

func TokenRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqtoken := r.Header.Get("Authorization")
		if reqtoken == "" {
			render.Render(w, r, utils.ErrInvalidRequest(fmt.Errorf("JWT error"), "Authorization Bearer not found.", http.StatusUnauthorized))
			return
		}
		token := reqtoken[len("Bearer "):]
		err := validateToken(token, r)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Invalid token", http.StatusUnauthorized))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func createToken(username string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{
			"username": username,
			"exp":      time.Now().Add(time.Hour * 768).Unix(),
		})

	tokenString, err := token.SignedString(secretKey)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func validateToken(tokenString string, r *http.Request) error {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return secretKey, nil
	})
	if err != nil {
		return err
	}
	if !token.Valid {
		return fmt.Errorf("Invalid token.")
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		user := claims["username"]
		if user != "admin" && r.Method != "GET" {
			log.Println("Access denied for user: ", user)
			return fmt.Errorf("Access denied.")
		}
	}
	return nil
}
