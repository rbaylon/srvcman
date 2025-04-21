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
	"github.com/go-chi/cors"
	"github.com/go-chi/render"
	"github.com/rbaylon/srvcman/database"
	collectionmodel "github.com/rbaylon/srvcman/modules/collections/model"
	collectionroutes "github.com/rbaylon/srvcman/modules/collections/routes"
	dhcpmodel "github.com/rbaylon/srvcman/modules/dhcps/model"
	dhcproutes "github.com/rbaylon/srvcman/modules/dhcps/routes"
	groupmodel "github.com/rbaylon/srvcman/modules/groups/model"
	grouproutes "github.com/rbaylon/srvcman/modules/groups/routes"
	paymentmodel "github.com/rbaylon/srvcman/modules/payments/model"
	paymentroutes "github.com/rbaylon/srvcman/modules/payments/routes"
	pfconfigmodel "github.com/rbaylon/srvcman/modules/pfconfig/model"
	pfconfigroutes "github.com/rbaylon/srvcman/modules/pfconfig/routes"
	pfifacemodel "github.com/rbaylon/srvcman/modules/pfifaces/model"
	pfifaceroutes "github.com/rbaylon/srvcman/modules/pfifaces/routes"
	pflowmodel "github.com/rbaylon/srvcman/modules/pflows/model"
	pflowroutes "github.com/rbaylon/srvcman/modules/pflows/routes"
	planmodel "github.com/rbaylon/srvcman/modules/plans/model"
	planroutes "github.com/rbaylon/srvcman/modules/plans/routes"
	policymodel "github.com/rbaylon/srvcman/modules/policies/model"
	policyroutes "github.com/rbaylon/srvcman/modules/policies/routes"
	runtimemodel "github.com/rbaylon/srvcman/modules/runtimes/model"
	runtimeroutes "github.com/rbaylon/srvcman/modules/runtimes/routes"
	"github.com/rbaylon/srvcman/modules/security"
	submodel "github.com/rbaylon/srvcman/modules/subs/model"
	subroutes "github.com/rbaylon/srvcman/modules/subs/routes"
	tunnelmodel "github.com/rbaylon/srvcman/modules/tunnels/model"
	tunnelroutes "github.com/rbaylon/srvcman/modules/tunnels/routes"
	usermodel "github.com/rbaylon/srvcman/modules/users/model"
	userroutes "github.com/rbaylon/srvcman/modules/users/routes"
	vouchermodel "github.com/rbaylon/srvcman/modules/vouchers/model"
	voucherroutes "github.com/rbaylon/srvcman/modules/vouchers/routes"
	"github.com/rbaylon/srvcman/utils"
)

func main() {
	var (
		app_ip       = database.GetEnvVariable("APP_IP")
		app_port     = database.GetEnvVariable("APP_PORT")
		cors_allowed = database.GetEnvCors("CORS_ALLOWED")
	)

	log.Printf("Arkgate Socket: %s:%s\n", app_ip, app_port)
	log.Printf("Allowed for CORS %v", cors_allowed)

	db, err := database.ConnectToDB()
	if err != nil {
		log.Fatal(err)
	}

	// Migrate tables
	usermodel.MigrateDB(db)
	groupmodel.MigrateDB(db)
	vouchermodel.MigrateDB(db)
	pfconfigmodel.MigrateDB(db)
	pfifacemodel.MigrateDB(db)
	pflowmodel.MigrateDB(db)
	dhcpmodel.MigrateDB(db)
	submodel.MigrateDB(db)
	runtimemodel.MigrateDB(db)
	paymentmodel.MigrateDB(db)
	tunnelmodel.MigrateDB(db)
	collectionmodel.MigrateDB(db)
	planmodel.MigrateDB(db)
	policymodel.MigrateDB(db)

	userStore := usermodel.New(db)
	groupStore := groupmodel.New(db)
	policyStore := policymodel.New(db)
	voucherStore := vouchermodel.New(db)
	pfconfigStore := pfconfigmodel.New(db)
	pfifaceStore := pfifacemodel.New(db)
	pflowStore := pflowmodel.New(db)
	subsStore := submodel.New(db)
	dhcpStore := dhcpmodel.New(db)
	rtStore := runtimemodel.New(db)
	paymentStore := paymentmodel.New(db)
	tunnelStore := tunnelmodel.New(db)
	collectionStore := collectionmodel.New(db)
	planStore := planmodel.New(db)

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

	// API login route - BasicAuth required, returns JWT access token
	r.Get("/api/v1/login", security.Login(userStore))

	// Mount the user sub-router:
	r.Mount("/api/v1/users", userroutes.UserRouter(userStore))
	r.Mount("/api/v1/groups", grouproutes.GroupRouter(groupStore))
	r.Mount("/api/v1/policies", policyroutes.PolicyRouter(policyStore))
	r.Mount("/api/v1/vouchers", voucherroutes.VoucherRouter(voucherStore))
	r.Mount("/api/v1/pfconfig", pfconfigroutes.PfconfigRouter(pfconfigStore))
	r.Mount("/api/v1/pfiface", pfifaceroutes.PfifaceRouter(pfifaceStore))
	r.Mount("/api/v1/pflow", pflowroutes.PflowRouter(pflowStore))
	r.Mount("/api/v1/subs", subroutes.SubRouter(subsStore))
	r.Mount("/api/v1/dhcps", dhcproutes.DhcpRouter(dhcpStore))
	r.Mount("/api/v1/runtime", runtimeroutes.RuntimeRouter(rtStore))
	r.Mount("/api/v1/payments", paymentroutes.PaymentRouter(paymentStore))
	r.Mount("/api/v1/tunnels", tunnelroutes.TunnelRouter(tunnelStore))
	r.Mount("/api/v1/collections", collectionroutes.CollectionRouter(collectionStore))
	r.Mount("/api/v1/plans", planroutes.PlanRouter(planStore))
	go utils.CheckExpired(voucherStore)
	http.ListenAndServe(fmt.Sprintf("%s:%s", app_ip, app_port), r)
}
