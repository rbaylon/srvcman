// Package database - Database session engine
package database

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func GetEnvVariable(key string) string {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file", err)
	}
	return os.Getenv(key)
}

func ConnectToDB() (*gorm.DB, error) {
	var (
		dbname = GetEnvVariable("DB_NAME")
		dbhost = GetEnvVariable("DB_HOST")
		dbuser = GetEnvVariable("DB_USER")
		dbpass = GetEnvVariable("DB_PASS")
	)
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=5432 TimeZone=Asia/Singapore", dbhost, dbuser, dbpass, dbname)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	return db, nil
}
