// Package main - Arkgate REST API.
//
//		Core Routes:
//			/
//	     Method: GET
//	     Headers: NONE
//	     Return: Basic page for health check only
//	     Return-Status: 200
//		  /api/v1/login
//		    Method: GET
//		    Headers: Authorization Basic
//		    Return: JSON object with JWT
//		    Return-Status: 200
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/go-chi/render"
	"github.com/rbaylon/srvcman/database"
	gatewaymonitormodel "github.com/rbaylon/srvcman/modules/gatewaymonitor/model"
	"github.com/rbaylon/srvcman/modules/security"
	usermodel "github.com/rbaylon/srvcman/modules/users/model"
	vouchermodel "github.com/rbaylon/srvcman/modules/vouchers/model"
	"github.com/rbaylon/srvcman/utils"
)

func main() {

	var (
		app_ip       = database.GetEnvVariable("APP_IP")
		app_port     = database.GetEnvVariable("APP_PORT")
		cors_allowed = database.GetEnvCors("CORS_ALLOWED")
	)
	wd, wderr := os.Getwd()
	if wderr != nil {
		log.Fatal("Failed to get current working directory: Possible filesystem issue.")
	}
	// moduleConfigPath is the JSON file listing which optional modules to load.
	// See moduleregistry.go for the core/optional module split.
	moduleConfigPath := wd + "/modules.json"
	log.Printf("Arkgate Socket: %s:%s\n", app_ip, app_port)
	log.Printf("Allowed for CORS %v", cors_allowed)

	db, err := database.ConnectToDB()
	if err != nil {
		log.Fatal(err)
	}

	enabledModules := LoadModuleNames(moduleConfigPath)

	r := chi.NewRouter()
	r.Use(render.SetContentType(render.ContentTypeJSON))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cors_allowed, // Use this to allow specific origin hosts
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	}))
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ARKGATE API"))
	})

	// API login route - BasicAuth required, returns JWT access token. Not a
	// pluggable module: security is core infra required by every other module.
	r.Get("/api/v1/login", security.Login(usermodel.New(db)))

	// Core modules: always migrated and mounted.
	for _, m := range coreModules {
		log.Printf("Loading core module: %s", m.Name)
		m.Load(r, db)
	}

	// Optional modules: migrated and mounted only if listed in the module config file.
	var voucherStore vouchermodel.Crud
	for _, name := range enabledModules {
		m, ok := optionalModules[name]
		if !ok {
			log.Printf("Unknown module %q in %s, skipping", name, moduleConfigPath)
			continue
		}
		log.Printf("Loading optional module: %s", m.Name)
		m.Load(r, db)
		if name == "vouchers" {
			voucherStore = vouchermodel.New(db)
		}
	}

	if voucherStore != nil {
		go utils.CheckExpired(voucherStore)
	}

	srv := &http.Server{Addr: fmt.Sprintf("%s:%s", app_ip, app_port), Handler: r}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Block until asked to stop (rc.d sends SIGTERM; Ctrl-C sends SIGINT),
	// then wind background work down before the process exits, rather than
	// leaving goroutines - and whatever subprocess a ping check has
	// in-flight - to be killed mid-stride.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down...")

	gatewaymonitormodel.StopAllMonitors()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Println("Server forced to shutdown:", err)
	}
	log.Println("Server exited")
}
