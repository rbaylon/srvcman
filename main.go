// Package main - Arkgate REST API.
//
//	Core Routes:
//		/
//      Method: GET
//      Headers: NONE
//      Return: Basic page for health check only
//      Return-Status: 200
//	  /api/v1/login
//	    Method: GET
//	    Headers: Authorization Basic
//	    Return: JSON object with JWT
//	    Return-Status: 200

package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/rbaylon/srvcman/database"
	"github.com/rbaylon/srvcman/modules/security"
	usermodel "github.com/rbaylon/srvcman/modules/users/model"
	userroutes "github.com/rbaylon/srvcman/modules/users/routes"
	vouchermodel "github.com/rbaylon/srvcman/modules/vouchers/model"
	voucherroutes "github.com/rbaylon/srvcman/modules/vouchers/routes"
)

func main() {
	var (
		app_ip   = database.GetEnvVariable("APP_IP")
		app_port = database.GetEnvVariable("APP_PORT")
	)

	log.Printf("Arkgate Socket: %s:%s\n", app_ip, app_port)

	db, err := database.ConnectToDB()
	if err != nil {
		log.Fatal(err)
	}

	// Migrate tables
	usermodel.MigrateDB(db)
	vouchermodel.MigrateDB(db)

	userStore := usermodel.New(db)
	voucherStore := vouchermodel.New(db)

	r := chi.NewRouter()
	r.Use(render.SetContentType(render.ContentTypeJSON))
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ARKGATE API"))
	})

	// API login route - BasicAuth required, returns JWT access token
	r.Get("/api/v1/login", security.Login(userStore))

	// Mount the user sub-router:
	r.Mount("/api/v1/users", userroutes.UserRouter(userStore))
	r.Mount("/api/v1/vouchers", voucherroutes.VoucherRouter(voucherStore))

	http.ListenAndServe(fmt.Sprintf("%s:%s", app_ip, app_port), r)
}
