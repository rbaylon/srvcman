// Package database - Database session engine
package database

import (
	"github.com/joho/godotenv"
  "gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"log"
	"os"
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
		dbname   = GetEnvVariable("DB_NAME")
	)
  db, err := gorm.Open(sqlite.Open(dbname), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	return db, nil
}
