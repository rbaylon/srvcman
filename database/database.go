// Package database - Database session engine
package database

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func GetEnvVariable(key string) string {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file", err)
	}
	return os.Getenv(key)
}

func GetEnvCors(key string) []string {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file", err)
	}
	var cors []string
	if key == "CORS_ALLOWED" {
		cors_allowed := os.Getenv(key)
		// Parse the string into a slice
		cors = strings.Split(cors_allowed, ",")
	}
	return cors
}

func ConnectToDB() (*gorm.DB, error) {
	var (
		dbname = GetEnvVariable("DB_NAME")
	//	dbhost = GetEnvVariable("DB_HOST")
	//	dbuser = GetEnvVariable("DB_USER")
	//	dbpass = GetEnvVariable("DB_PASS")
	)
	//dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=5432 TimeZone=Asia/Singapore",
	//	dbhost, dbuser, dbpass, dbname)
	//db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	db, err := gorm.Open(sqlite.Open(dbname), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	return db, nil
}
