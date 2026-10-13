// Package powerroutes - Arkgate API power module
//
// Restarts or shuts down the gateway through arkgated. Both actions are
// disruptive - every service on the box stops, and a shutdown stays down
// until someone with physical (or hypervisor) access starts it - so each one
// must carry its own confirmation in the request body, and the request is
// recorded in the audit trail like any other change.
//
//	Module Routes:
//	  /api/v1/power/restart
//	  /api/v1/power/shutdown
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Body: JSON {"confirm": "restart"} or {"confirm": "shutdown"} - it
//	          must name the action being asked for
//	    Return: JSON {"status": "restarting"} or {"status": "shutting down"};
//	            the action happens a few seconds after the reply
//	    Return-Status: 200 on Success (the action has been scheduled)
//	                   400 if the confirmation is missing or does not match
//	                   500 if arkgated could not be reached
package powerroutes

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	powermodel "github.com/rbaylon/srvcman/modules/power/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
	"gorm.io/gorm"
)

type request struct {
	Confirm string `json:"confirm"`
}

func (request) Bind(r *http.Request) error { return nil }

func PowerRouter(db *gorm.DB) chi.Router {
	r := chi.NewRouter()
	r.Use(security.TokenRequired(db, "power"))

	handler := func(action powermodel.Action, doing string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var in request
			// An absent or unreadable body is just an absent confirmation,
			// which Perform refuses.
			_ = render.Bind(r, &in)
			status, err := powermodel.Perform(action, in.Confirm, powermodel.Run)
			if err != nil {
				render.Render(w, r, utils.ErrInvalidRequest(err, "Power action refused", status))
				return
			}
			log.Printf("power: %s requested (%s)", action, r.RemoteAddr)
			render.JSON(w, r, map[string]string{"status": doing})
		}
	}
	r.Put("/restart", handler(powermodel.Restart, "restarting"))
	r.Put("/shutdown", handler(powermodel.Shutdown, "shutting down"))
	return r
}
