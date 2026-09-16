// Package captiveportalroutes - Arkgate API captiveportal module
//
// Like modules/diagnostics, there's no modules/captiveportal/model: this
// module manages nothing in srvcman's own DB. The captive-portal web app
// ("subsportal", rc.d service name "captiveportal" - see that repo's
// CLAUDE.md) is a separate, unprivileged process that runs on the gateway
// box alongside arkgated; it reads its own .env and isn't configured
// through srvcman at all. All this module does is ask arkgated to run
// rcctl against that service, the same mechanism modules/changes uses to
// start/stop/restart dhcpd/unbound/npppd.
//
//	Module Routes:
//	  /api/v1/captiveportal/status
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"running": bool, "output": "<rcctl check output>"}
//	    Return-Status: 200 on Success (including when the service is
//	                   stopped - that's a normal status, not an error)
//	                   500 if arkgated itself couldn't be reached
//
//	  /api/v1/captiveportal/start
//	  /api/v1/captiveportal/stop
//	  /api/v1/captiveportal/restart
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Return: JSON {"result": "<action> ok"}
//	    Return-Status: 200 on Success
//	                   500 if arkgated itself couldn't be reached, or
//	                   rcctl reported failure
package captiveportalroutes

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	Acmd "github.com/rbaylon/arkgatecmd"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
	"gorm.io/gorm"
)

// service is the OpenBSD rc.d name of the captive-portal app - srvcman
// only ever controls it via rcctl, it never runs or configures the app
// itself.
const service = "captiveportal"

// cmdTimeout bounds how long a single rcctl call is allowed to take.
const cmdTimeout = 15 * time.Second

// runViaArk dispatches an rcctl Arkcmd to arkgated and returns its output.
func runViaArk(cmd *Acmd.Arkcmd) (string, error) {
	conn := utils.GetArkConn()
	if conn == nil {
		return "", fmt.Errorf("arkgated unreachable")
	}
	conn.SetDeadline(time.Now().Add(cmdTimeout))
	return cmd.SendCmdOutput(conn)
}

func CaptivePortalRouter(db *gorm.DB) chi.Router {
	r := chi.NewRouter()
	r.Use(security.TokenRequired(db, "captiveportal"))

	r.Get("/status", func(w http.ResponseWriter, r *http.Request) {
		out, err := runViaArk(Acmd.GetRcCmds(service)["status"])
		// A non-nil err here can mean either "rcctl check ran and reported
		// not running" (out still has the "captiveportal(failed)" text -
		// a normal result, not a request failure) or "couldn't even reach
		// arkgated" (out is empty). Only the latter is a 500.
		if out == "" && err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "arkgated unreachable", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]any{"running": err == nil, "output": out})
	})

	r.Put("/start", captiveAction("start"))
	r.Put("/stop", captiveAction("stop"))
	r.Put("/restart", captiveAction("restart"))

	return r
}

func captiveAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cmd := Acmd.GetRcCmds(service)[action]
		if err := cmd.SendCmd(utils.GetArkConn()); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("%s failed", action), http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]string{"result": action + " ok"})
	}
}
