// Package database - Database session engine
package database

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
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
	dbtype := GetEnvVariable("DB_TYPE")
	if dbtype == "postgres" {
		var (
			dbname = GetEnvVariable("DB_NAME")
			dbhost = GetEnvVariable("DB_HOST")
			dbuser = GetEnvVariable("DB_USER")
			dbpass = GetEnvVariable("DB_PASS")
		)
		dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=5432 TimeZone=Asia/Singapore",
			dbhost, dbuser, dbpass, dbname)
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			return nil, err
		}
		return db, nil
	} else {
		var dbname = GetEnvVariable("DB_NAME")
		// _busy_timeout lets a connection wait for a lock instead of failing
		// immediately, and WAL lets readers proceed while a write is in
		// flight - both needed because SQLite only ever allows one writer at
		// a time. SetMaxOpenConns(1) below is the other half of this: it
		// keeps database/sql from opening additional concurrent connections
		// under load, which under the previous unbounded-pool config caused
		// a crash in mattn/go-sqlite3's cgo layer during concurrent
		// connection Open() (see incident 2026-09-05).
		db, err := gorm.Open(sqlite.Open(dbname+".db?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{})
		if err != nil {
			return nil, err
		}
		sqlDB, err := db.DB()
		if err != nil {
			return nil, err
		}
		sqlDB.SetMaxOpenConns(1)
		return db, nil
	}
}
