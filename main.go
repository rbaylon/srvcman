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
	dhcpmodel "github.com/rbaylon/srvcman/modules/dhcps/model"
	dhcproutes "github.com/rbaylon/srvcman/modules/dhcps/routes"
	pfconfigmodel "github.com/rbaylon/srvcman/modules/pfconfig/model"
	pfconfigroutes "github.com/rbaylon/srvcman/modules/pfconfig/routes"
	pfifacemodel "github.com/rbaylon/srvcman/modules/pfifaces/model"
	pfifaceroutes "github.com/rbaylon/srvcman/modules/pfifaces/routes"
	runtimemodel "github.com/rbaylon/srvcman/modules/runtimes/model"
	runtimeroutes "github.com/rbaylon/srvcman/modules/runtimes/routes"
	"github.com/rbaylon/srvcman/modules/security"
	submodel "github.com/rbaylon/srvcman/modules/subs/model"
	subroutes "github.com/rbaylon/srvcman/modules/subs/routes"
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
	pfconfigmodel.MigrateDB(db)
	pfifacemodel.MigrateDB(db)
	dhcpmodel.MigrateDB(db)
	submodel.MigrateDB(db)
	runtimemodel.MigrateDB(db)

	userStore := usermodel.New(db)
	voucherStore := vouchermodel.New(db)
	pfconfigStore := pfconfigmodel.New(db)
	pfifaceStore := pfifacemodel.New(db)
	subsStore := submodel.New(db)
	dhcpStore := dhcpmodel.New(db)
	rtStore := runtimemodel.New(db)

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
	r.Mount("/api/v1/pfconfig", pfconfigroutes.PfconfigRouter(pfconfigStore))
	r.Mount("/api/v1/pfiface", pfifaceroutes.PfifaceRouter(pfifaceStore))
	r.Mount("/api/v1/subs", subroutes.SubRouter(subsStore))
	r.Mount("/api/v1/dhcps", dhcproutes.DhcpRouter(dhcpStore))
	r.Mount("/api/v1/runtime", runtimeroutes.RuntimeRouter(rtStore))

	http.ListenAndServe(fmt.Sprintf("%s:%s", app_ip, app_port), r)
}
