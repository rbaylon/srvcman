// Package database - Database session engine
package database

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var loadEnvOnce sync.Once

// loadEnv loads .env into the process environment exactly once. Every
// call to GetEnvVariable/GetEnvCors used to re-read and re-parse .env from
// disk on every single call and log.Fatal (os.Exit) on any read error -
// including from utils.GetArkConn, which reads 4 env vars (so 4 reloads)
// on every srvcman<->arkgated call: every gatewaymonitor health-check ping
// (per monitored gateway, every 10s, from a background goroutine with no
// HTTP request wrapping it), every pfifaces traffic poll, every diagnostics
// request, every modules/changes apply. Since godotenv.Load only needs to
// run once - it populates the process environment via os.Setenv, so later
// lookups just need os.Getenv - a single transient disk hiccup at any
// point after startup (the file briefly unavailable during a deploy, an
// NFS blip, anything) could take the entire server down, not just fail
// the one call that hit it. Load once at first use; only that first load
// can still be fatal (there's genuinely no way to run without config, so
// failing fast at startup is correct), every call after that just reads
// the already-populated environment and can't fail this way again.
func loadEnv() {
	loadEnvOnce.Do(func() {
		if err := godotenv.Load(".env"); err != nil {
			log.Fatal("Error loading .env file", err)
		}
	})
}

func GetEnvVariable(key string) string {
	loadEnv()
	return os.Getenv(key)
}

func GetEnvCors(key string) []string {
	loadEnv()
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
