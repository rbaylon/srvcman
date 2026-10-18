// Package acmeroutes - Arkgate API acme module
//
// Certificate issuance is a single self-contained operation on arkgated's
// side (see its ApplyAcme): install /etc/acme-client.conf, run
// acme-client, rewrite nginx.conf's certificate paths, restart nginx.
// srvcman only triggers it and reports the outcome - it never runs
// acme-client or edits nginx.conf itself, since both need root on the
// gateway rather than on whatever host srvcman happens to run on.
//
//	Module Routes:
//	  /api/v1/acme
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of acme objects (at most one)
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/acme/conf
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"conf": "<generated acme-client.conf text>"}
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/acme/active
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"fqdn": "<enabled fqdn, or empty>"} - what arkgated
//	            reads to know which name to issue for
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/acme/apply
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Return: JSON {"result": "<arkgated output>"}
//	    Return-Status: 200 on Success
//	                   400 if no enabled record exists
//	                   500 if arkgated couldn't be reached, or the
//	                   issuance/nginx step failed
//
//	  /api/v1/acme/<acmeId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON acme object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/acme/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON acme object
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package acmeroutes

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	Acmd "github.com/rbaylon/arkgatecmd"
	acmemodel "github.com/rbaylon/srvcman/modules/acme/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

// applyTimeout is generous because acme-client has to complete an ACME
// challenge against Let's Encrypt over the network before it returns -
// that is a round trip to a third party, not a local command, and the
// nginx restart follows it.
const applyTimeout = 120 * time.Second

// ApplyAcmeCmd is the command name arkgated special-cases to run the
// whole issuance. Cmd/Opts are unused: arkgated fetches the FQDN from
// this API rather than taking it from the wire, so a client cannot steer
// which name gets issued or which paths get written.
const ApplyAcmeCmd = "ApplyAcme"

// applyViaArk asks arkgated to run the issuance and returns its output.
func applyViaArk() (string, error) {
	conn := utils.GetArkConn()
	if conn == nil {
		return "", fmt.Errorf("arkgated unreachable")
	}
	conn.SetDeadline(time.Now().Add(applyTimeout))
	cmd := Acmd.PrepareCmd(ApplyAcmeCmd, "true", []string{})
	return cmd.SendCmdOutput(conn)
}

func AcmeRouter(db acmemodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "acme"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/conf", func(w http.ResponseWriter, r *http.Request) {
		conf, err := acmemodel.GenerateConf(dbconn)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]string{"conf": conf})
	})
	r.Get("/active", func(w http.ResponseWriter, r *http.Request) {
		fqdn, err := acmemodel.Active(dbconn)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]string{"fqdn": fqdn})
	})
	r.Put("/apply", func(w http.ResponseWriter, r *http.Request) {
		fqdn, err := acmemodel.Active(dbconn)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		if fqdn == "" {
			render.Render(w, r, utils.ErrInvalidRequest(fmt.Errorf("no enabled acme record"), "Enable a certificate record before applying", http.StatusBadRequest))
			return
		}
		out, err := applyViaArk()
		if err != nil {
			// acme-client's own output is the useful part of a failure
			// (an unresolvable name, a failed challenge), so it is passed
			// through rather than replaced with a generic message.
			msg := err.Error()
			if out != "" {
				msg = out
			}
			render.Render(w, r, utils.ErrInvalidRequest(fmt.Errorf("%s", msg), "Certificate request failed", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]string{"result": out})
	})
	r.Get("/{acmeId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "acmeId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid acme ID %s", chi.URLParam(r, "acmeId")), http.StatusBadRequest))
			return
		}
		acme, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, acme)
	})
	r.Put("/{acmeId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "acmeId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid acme ID %s", chi.URLParam(r, "acmeId")), http.StatusBadRequest))
			return
		}
		acme := &acmemodel.Acme{}
		if err = render.Bind(r, acme); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		acme.ID = uint(id)
		errupdate := db.Update(acme)
		if errupdate == nil {
			render.JSON(w, r, acme)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(errupdate, fmt.Sprintf("Error updating record for acme  ID %s", chi.URLParam(r, "acmeId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		acme := &acmemodel.Acme{}
		if err := render.Bind(r, acme); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(acme)
		if err == nil {
			render.JSON(w, r, acme)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{acmeId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "acmeId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid acme ID %s", chi.URLParam(r, "acmeId")), http.StatusBadRequest))
			return
		}
		// No render.Bind here, unlike the other modules': a DELETE carries
		// no body worth validating, and Bind requires a valid fqdn - which
		// an empty body does not have, so binding it would reject every
		// delete.
		acme := &acmemodel.Acme{}
		acme.ID = uint(id)
		err = db.Delete(acme)
		if err == nil {
			render.JSON(w, r, acme)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for acme  ID %s", chi.URLParam(r, "acmeId")), http.StatusBadRequest))
	})
	return r
}
