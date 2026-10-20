// Package snmpserverroutes - Arkgate API snmpserver module
//
//	Module Routes:
//	  /api/v1/snmpserver
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of snmpserver objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/snmpserver/conf
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"conf": "<generated snmpd.conf text>"}, freshly
//	            rendered from the current SnmpServer row
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/snmpserver/status
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"running": bool, "output": "<rcctl check output>"}
//	    Return-Status: 200 on Success (a stopped service is a normal
//	                   status, not an error)
//	                   500 if arkgated itself couldn't be reached
//
//	  /api/v1/snmpserver/start
//	  /api/v1/snmpserver/stop
//	  /api/v1/snmpserver/restart
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Return: JSON {"result": "<action> ok"}
//	    Return-Status: 200 on Success
//	                   500 if the rcctl call failed
//
//	  /api/v1/snmpserver/<snmpServerId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON snmpserver object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/snmpserver/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON snmpserver object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package snmpserverroutes

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	Acmd "github.com/rbaylon/arkgatecmd"
	"github.com/rbaylon/srvcman/modules/security"
	snmpservermodel "github.com/rbaylon/srvcman/modules/snmpserver/model"
	"github.com/rbaylon/srvcman/utils"
	"github.com/rbaylon/srvcman/utils/rcsvc"
)

// service is the OpenBSD rc.d name of the snmpd daemon this module
// configures - srvcman only ever controls it via rcctl, it never runs
// snmpd itself.
const service = "snmpd"

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

func SnmpServerRouter(db snmpservermodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "snmpserver"))

	r.Get("/status", func(w http.ResponseWriter, r *http.Request) {
		out, err := runViaArk(Acmd.GetRcCmds(service)["status"])
		// A non-nil err here can mean either "rcctl check ran and reported
		// not running" (out still has the "snmpd(failed)" text - a normal
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
		conf, err := snmpservermodel.GenerateConf(dbconn)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]string{"conf": conf})
	})
	r.Get("/{snmpServerId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "snmpServerId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid snmpserver ID %s", chi.URLParam(r, "snmpServerId")), http.StatusBadRequest))
			return
		}
		snmpserver, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, snmpserver)
	})
	r.Put("/{snmpServerId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "snmpServerId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid snmpserver ID %s", chi.URLParam(r, "snmpServerId")), http.StatusBadRequest))
			return
		}
		snmpserver := &snmpservermodel.SnmpServer{}
		if err = render.Bind(r, snmpserver); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		snmpserver.ID = uint(id)
		errupdate := db.Update(snmpserver)
		if errupdate == nil {
			render.JSON(w, r, snmpserver)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(errupdate, fmt.Sprintf("Error updating record for snmpserver  ID %s", chi.URLParam(r, "snmpServerId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		snmpserver := &snmpservermodel.SnmpServer{}
		if err := render.Bind(r, snmpserver); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(snmpserver)
		if err == nil {
			render.JSON(w, r, snmpserver)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{snmpServerId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "snmpServerId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid snmpserver ID %s", chi.URLParam(r, "snmpServerId")), http.StatusBadRequest))
			return
		}
		snmpserver := &snmpservermodel.SnmpServer{}
		if err = render.Bind(r, snmpserver); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		snmpserver.ID = uint(id)
		err = db.Delete(snmpserver)
		if err == nil {
			render.JSON(w, r, snmpserver)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for snmpserver  ID %s", chi.URLParam(r, "snmpServerId")), http.StatusBadRequest))
	})
	return r
}
