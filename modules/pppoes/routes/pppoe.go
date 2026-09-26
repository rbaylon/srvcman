// Package pppoeroutes - Arkgate API pppoe module
//
//	Module Routes:
//	  /api/v1/pppoes
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of pppoe objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/pppoes/conf
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"conf": "<generated npppd.conf text>"}, freshly
//	            rendered from the current Pppoe rows
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/pppoes/<pppoeId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON pppoe object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/pppoes/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON pppoe object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/pppoes/status
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"running": bool, "output": "<rcctl check output>"}
//	    Return-Status: 200 on Success (including when the service is
//	                   stopped - that's a normal status, not an error)
//	                   500 if arkgated itself couldn't be reached
//
//	  /api/v1/pppoes/start
//	  /api/v1/pppoes/stop
//	  /api/v1/pppoes/restart
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Return: JSON {"result": "<action> ok"}
//	    Return-Status: 200 on Success
//	                   500 if arkgated itself couldn't be reached, or
//	                   rcctl reported failure
package pppoeroutes

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	Acmd "github.com/rbaylon/arkgatecmd"
	pppoemodel "github.com/rbaylon/srvcman/modules/pppoes/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

// service is the OpenBSD rc.d name of the npppd daemon this module
// configures - srvcman only ever controls it via rcctl, it never runs
// npppd itself.
const service = "npppd"

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
		cmd := Acmd.GetRcCmds(service)[action]
		if err := cmd.SendCmd(utils.GetArkConn()); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("%s failed", action), http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]string{"result": action + " ok"})
	}
}

func PppoeRouter(db pppoemodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "pppoes"))

	r.Get("/status", func(w http.ResponseWriter, r *http.Request) {
		out, err := runViaArk(Acmd.GetRcCmds(service)["status"])
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
		conf, err := pppoemodel.GenerateConf(dbconn)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]string{"conf": conf})
	})
	r.Get("/{pppoeId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "pppoeId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid pppoe ID %s", chi.URLParam(r, "pppoeId")), http.StatusBadRequest))
			return
		}
		pppoe, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, pppoe)
	})
	r.Get("/pfconfig/{pfconfigId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "pfconfigId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid pfconfig ID %s", chi.URLParam(r, "pfconfigId")), http.StatusBadRequest))
			return
		}
		pppoe, err := db.GetByPfconfigId(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, pppoe)
	})
	r.Put("/{pppoeId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "pppoeId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid pppoe ID %s", chi.URLParam(r, "pppoeId")), http.StatusBadRequest))
			return
		}
		pppoe := pppoemodel.Pppoe{}
		if err = render.Bind(r, &pppoe); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		pppoe.ID = uint(id)
		err = db.Update(&pppoe)
		if err == nil {
			render.JSON(w, r, pppoe)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for pppoe  ID %s", chi.URLParam(r, "pppoeId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		pppoe := pppoemodel.Pppoe{}
		if err := render.Bind(r, &pppoe); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(&pppoe)
		if err == nil {
			render.JSON(w, r, pppoe)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{pppoeId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "pppoeId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid pppoe ID %s", chi.URLParam(r, "pppoeId")), http.StatusBadRequest))
			return
		}
		pppoe := pppoemodel.Pppoe{}
		if err = render.Bind(r, &pppoe); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		pppoe.ID = uint(id)
		err = db.Delete(&pppoe)
		if err == nil {
			render.JSON(w, r, pppoe)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for pppoe  ID %s", chi.URLParam(r, "pppoeId")), http.StatusBadRequest))
	})
	return r
}
