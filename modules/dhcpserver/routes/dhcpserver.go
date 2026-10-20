// Package dhcpserverroutes - Arkgate API dhcpserver module
//
//	Module Routes:
//	  /api/v1/dhcpserver
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of dhcpserver objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/dhcpserver/conf
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"conf": "<generated dhcpd.conf text>"}, freshly
//	            rendered from the current DhcpServer rows
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/dhcpserver/<dhcpServerId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON dhcpserver object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/dhcpserver/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON dhcpserver object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/dhcpserver/status
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"running": bool, "output": "<rcctl check output>"}
//	    Return-Status: 200 on Success (including when the service is
//	                   stopped - that's a normal status, not an error)
//	                   500 if arkgated itself couldn't be reached
//
//	  /api/v1/dhcpserver/start
//	  /api/v1/dhcpserver/stop
//	  /api/v1/dhcpserver/restart
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Return: JSON {"result": "<action> ok"}
//	    Return-Status: 200 on Success
//	                   500 if arkgated itself couldn't be reached, or
//	                   rcctl reported failure
package dhcpserverroutes

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	Acmd "github.com/rbaylon/arkgatecmd"
	dhcpservermodel "github.com/rbaylon/srvcman/modules/dhcpserver/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
	"github.com/rbaylon/srvcman/utils/rcsvc"
)

// service is the OpenBSD rc.d name of the dhcpd daemon this module
// configures - srvcman only ever controls it via rcctl, it never runs
// dhcpd itself.
const service = "dhcpd"

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

func serviceAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if action == "start" || action == "restart" {
			// rcctl refuses to start a service that is not enabled, which is the
			// state of a daemon the first time it is configured.
			if _, err := rcsvc.EnsureEnabled(service, utils.GetArkConn); err != nil {
				render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("%s failed", action), http.StatusInternalServerError))
				return
			}
		}
		cmd := Acmd.GetRcCmds(service)[action]
		if err := cmd.SendCmd(utils.GetArkConn()); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("%s failed", action), http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]string{"result": action + " ok"})
	}
}

func DhcpServerRouter(db dhcpservermodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "dhcpserver"))

	r.Get("/status", func(w http.ResponseWriter, r *http.Request) {
		out, err := runViaArk(Acmd.GetRcCmds(service)["status"])
		// A non-nil err here can mean either "rcctl check ran and reported
		// not running" (out still has the "dhcpd(failed)" text - a normal
		// result, not a request failure) or "couldn't even reach arkgated"
		// (out is empty). Only the latter is a 500.
		if out == "" && err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "arkgated unreachable", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]any{"running": err == nil, "output": out})
	})
	r.Put("/start", serviceAction("start"))
	r.Put("/stop", serviceAction("stop"))
	r.Put("/restart", serviceAction("restart"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/conf", func(w http.ResponseWriter, r *http.Request) {
		conf, err := dhcpservermodel.GenerateConf(dbconn)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]string{"conf": conf})
	})
	r.Get("/{dhcpServerId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "dhcpServerId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid dhcpserver ID %s", chi.URLParam(r, "dhcpServerId")), http.StatusBadRequest))
			return
		}
		dhcpserver, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, dhcpserver)
	})
	r.Put("/{dhcpServerId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "dhcpServerId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid dhcpserver ID %s", chi.URLParam(r, "dhcpServerId")), http.StatusBadRequest))
			return
		}
		dhcpserver := &dhcpservermodel.DhcpServer{}
		if err = render.Bind(r, dhcpserver); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		dhcpserver.ID = uint(id)
		errupdate := db.Update(dhcpserver)
		if errupdate == nil {
			render.JSON(w, r, dhcpserver)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(errupdate, fmt.Sprintf("Error updating record for dhcpserver  ID %s", chi.URLParam(r, "dhcpServerId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		dhcpserver := &dhcpservermodel.DhcpServer{}
		if err := render.Bind(r, dhcpserver); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		erradd := db.Add(dhcpserver)
		if erradd == nil {
			render.JSON(w, r, dhcpserver)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(erradd, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{dhcpServerId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "dhcpServerId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid dhcpserver ID %s", chi.URLParam(r, "dhcpServerId")), http.StatusBadRequest))
			return
		}
		dhcpserver, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		err = db.Delete(dhcpserver)
		if err == nil {
			render.JSON(w, r, dhcpserver)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for dhcpserver  ID %s", chi.URLParam(r, "dhcpServerId")), http.StatusBadRequest))
	})
	return r
}
