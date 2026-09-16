// Package billportalroutes - Arkgate API billportal module
//
// Like modules/diagnostics and modules/captiveportal, there's no
// modules/billportal/model: this module manages nothing in srvcman's own
// DB. billportal (rc.d service name "billportal" - see that repo's
// CLAUDE.md) is a separate, unprivileged process that runs on the gateway
// box; it talks to the upstream subscriber API directly and reads its own
// .env, and isn't configured through srvcman at all. All this module does
// is ask arkgated to run rcctl against that service, the same mechanism
// used to control dhcpd/unbound/npppd/captiveportal.
//
//	Module Routes:
//	  /api/v1/billportal/status
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"running": bool, "output": "<rcctl check output>"}
//	    Return-Status: 200 on Success (including when the service is
//	                   stopped - that's a normal status, not an error)
//	                   500 if arkgated itself couldn't be reached
//
//	  /api/v1/billportal/start
//	  /api/v1/billportal/stop
//	  /api/v1/billportal/restart
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Return: JSON {"result": "<action> ok"}
//	    Return-Status: 200 on Success
//	                   500 if arkgated itself couldn't be reached, or
//	                   rcctl reported failure
package billportalroutes

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

// service is the OpenBSD rc.d name of the billportal app - srvcman only
// ever controls it via rcctl, it never runs or configures the app itself.
const service = "billportal"

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

func BillPortalRouter(db *gorm.DB) chi.Router {
	r := chi.NewRouter()
	r.Use(security.TokenRequired(db, "billportal"))

	r.Get("/status", func(w http.ResponseWriter, r *http.Request) {
		out, err := runViaArk(Acmd.GetRcCmds(service)["status"])
		// A non-nil err here can mean either "rcctl check ran and reported
		// not running" (out still has the "billportal(failed)" text - a
		// normal result, not a request failure) or "couldn't even reach
		// arkgated" (out is empty). Only the latter is a 500.
		if out == "" && err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "arkgated unreachable", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]any{"running": err == nil, "output": out})
	})

	r.Put("/start", billAction("start"))
	r.Put("/stop", billAction("stop"))
	r.Put("/restart", billAction("restart"))

	return r
}

func billAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cmd := Acmd.GetRcCmds(service)[action]
		if err := cmd.SendCmd(utils.GetArkConn()); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("%s failed", action), http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]string{"result": action + " ok"})
	}
}
