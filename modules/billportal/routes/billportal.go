// Package billportalroutes - Arkgate API billportal module
//
// The bill-portal web app (rc.d service name "billportal" - see that
// repo's CLAUDE.md) is a separate, unprivileged process on the gateway.
// This module both configures it - rendering its .env and having arkgated
// install it - and controls its service via rcctl, the same mechanism
// modules/changes uses for dhcpd/unbound/npppd.
//
// Saving the config also provisions the read-only API account the portal
// logs in with, and writes the resulting credential into API_AUTH (see the
// model's provision.go). That credential never leaves this API: the field
// is json:"-", so it appears in the rendered .env and nowhere else.
//
//	Module Routes:
//	  /api/v1/billportal
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON list of billportal objects (at most one), with
//	            API_AUTH omitted
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/billportal/conf
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"conf": "<generated .env text>"} - contains
//	            API_AUTH in clear, so this is for arkgated, not for
//	            display
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/billportal/<billPortalId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON billportal object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/billportal/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON billportal object
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
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
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	Acmd "github.com/rbaylon/arkgatecmd"
	billportalmodel "github.com/rbaylon/srvcman/modules/billportal/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
	"github.com/rbaylon/srvcman/utils/rcsvc"
)

// service is the OpenBSD rc.d name of the billportal app - srvcman only
// ever controls it via rcctl, it never runs or configures the app itself.
const service = "billportal"

// cmdTimeout bounds how long a single rcctl call is allowed to take.
const cmdTimeout = 15 * time.Second

// InstallEnvCmd is the command name arkgated special-cases to fetch the
// rendered .env and write it to the portal's config path. Cmd/Opts go
// unused: arkgated pulls the content from this API rather than taking it
// off the wire, so the credential in it never travels as a command
// argument.
const InstallEnvCmd = "BillPortalEnv"

// runViaArk dispatches an rcctl Arkcmd to arkgated and returns its output.
func runViaArk(cmd *Acmd.Arkcmd) (string, error) {
	conn := utils.GetArkConn()
	if conn == nil {
		return "", fmt.Errorf("arkgated unreachable")
	}
	conn.SetDeadline(time.Now().Add(cmdTimeout))
	return cmd.SendCmdOutput(conn)
}

// installEnv asks arkgated to (re)write the portal's .env from the
// rendered config. Failures are logged and reported rather than aborting
// the save: the record is already stored, and the file can be installed
// again by saving once arkgated is reachable.
func installEnv() error {
	cmd := Acmd.PrepareCmd(InstallEnvCmd, "true", []string{})
	if _, err := runViaArk(cmd); err != nil {
		return err
	}
	return nil
}

func BillPortalRouter(bdb billportalmodel.Crud) chi.Router {
	r := chi.NewRouter()
	db := bdb.GetDB()
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

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := bdb.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/conf", func(w http.ResponseWriter, r *http.Request) {
		conf, err := billportalmodel.GenerateConf(db)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]string{"conf": conf})
	})
	r.Get("/{billPortalId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "billPortalId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid billportal ID %s", chi.URLParam(r, "billPortalId")), http.StatusBadRequest))
			return
		}
		record, err := bdb.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, record)
	})
	r.Put("/{billPortalId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "billPortalId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid billportal ID %s", chi.URLParam(r, "billPortalId")), http.StatusBadRequest))
			return
		}
		record := &billportalmodel.BillPortal{}
		if err = render.Bind(r, record); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		record.ID = uint(id)
		if err := bdb.Update(record); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for billportal  ID %s", chi.URLParam(r, "billPortalId")), http.StatusBadRequest))
			return
		}
		if err := installEnv(); err != nil {
			log.Println("billportal: failed to install .env:", err)
			render.Render(w, r, utils.ErrInvalidRequest(err, "Saved, but writing "+billportalmodel.EnvPath+" failed", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, record)
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		record := &billportalmodel.BillPortal{}
		if err := render.Bind(r, record); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		if err := bdb.Add(record); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		if err := installEnv(); err != nil {
			log.Println("billportal: failed to install .env:", err)
			render.Render(w, r, utils.ErrInvalidRequest(err, "Saved, but writing "+billportalmodel.EnvPath+" failed", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, record)
	})
	r.Delete("/{billPortalId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "billPortalId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid billportal ID %s", chi.URLParam(r, "billPortalId")), http.StatusBadRequest))
			return
		}
		record := &billportalmodel.BillPortal{}
		record.ID = uint(id)
		if err := bdb.Delete(record); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for billportal  ID %s", chi.URLParam(r, "billPortalId")), http.StatusBadRequest))
			return
		}
		render.JSON(w, r, record)
	})

	return r
}

func billAction(action string) http.HandlerFunc {
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
