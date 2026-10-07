// Package tunnelroutes - Arkgate API Tunnel module
//
//	Module Routes:
//	  /api/v1/tunnels
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON list of tunnel objects, each decorated with live
//	            "status" ("active"/"inactive"), "pid" and "age" read from
//	            the gateway's process list
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/tunnels/startinactive
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Return: JSON {"started": <n>, "failed": <n>} - (re)establishes
//	            every tunnel that isn't currently running
//	    Return-Status: 200 on Success
//
//	  /api/v1/tunnels/sshkey
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Return: JSON {"public_key": "<ssh public key>", "generated": <bool>} -
//	            the gateway's tunnel public key, generated first if none
//	            exists yet (generated is true then)
//	    Return-Status: 200 on Success
//	                   500 if the key couldn't be read or created
//
//	  /api/v1/tunnels/<tunnelId>/start
//	  /api/v1/tunnels/<tunnelId>/stop
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Return: JSON tunnel object
//	    Return-Status: 200 on Success
//	                   400 on Bad request
//	                   500 if the ssh/kill command couldn't be dispatched
//
//	  /api/v1/tunnels/<tunnelId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON tunnel object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/tunnels/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON tunnel object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package tunnelroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/rbaylon/srvcman/modules/security"
	tunnelmodel "github.com/rbaylon/srvcman/modules/tunnels/model"
	"github.com/rbaylon/srvcman/utils"
)

func TunnelRouter(db tunnelmodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "tunnels"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, tunnelmodel.Decorate(res))
	})
	r.Put("/startinactive", func(w http.ResponseWriter, r *http.Request) {
		started, failed := tunnelmodel.StartInactive(dbconn)
		render.JSON(w, r, map[string]int{"started": started, "failed": failed})
	})
	r.Put("/sshkey", func(w http.ResponseWriter, r *http.Request) {
		key, err := tunnelmodel.EnsureSshKey()
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Failed to get ssh key", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, key)
	})
	r.Get("/{tunnelId}", func(w http.ResponseWriter, r *http.Request) {
		tunnel, err := lookup(db, r)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		decorated := tunnelmodel.Decorate([]tunnelmodel.Tunnel{*tunnel})
		render.JSON(w, r, decorated[0])
	})
	r.Put("/{tunnelId}/start", func(w http.ResponseWriter, r *http.Request) {
		tunnel, err := lookup(db, r)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		if err := tunnelmodel.Start(tunnel); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Failed to start tunnel", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, tunnel)
	})
	r.Put("/{tunnelId}/stop", func(w http.ResponseWriter, r *http.Request) {
		tunnel, err := lookup(db, r)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		if err := tunnelmodel.Stop(tunnel); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Failed to stop tunnel", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, tunnel)
	})
	r.Put("/{tunnelId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "tunnelId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid tunnel ID %s", chi.URLParam(r, "tunnelId")), http.StatusBadRequest))
			return
		}
		tunnel := tunnelmodel.Tunnel{}
		if err = render.Bind(r, &tunnel); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		tunnel.ID = uint(id)
		err = db.Update(&tunnel)
		if err == nil {
			render.JSON(w, r, tunnel)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for tunnel  ID %s", chi.URLParam(r, "tunnelId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		tunnel := tunnelmodel.Tunnel{}
		if err := render.Bind(r, &tunnel); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(&tunnel)
		if err == nil {
			render.JSON(w, r, tunnel)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{tunnelId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "tunnelId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid tunnel ID %s", chi.URLParam(r, "tunnelId")), http.StatusBadRequest))
			return
		}
		tunnel := tunnelmodel.Tunnel{}
		if err = render.Bind(r, &tunnel); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		tunnel.ID = uint(id)
		err = db.Delete(&tunnel)
		if err == nil {
			render.JSON(w, r, tunnel)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for tunnel  ID %s", chi.URLParam(r, "tunnelId")), http.StatusBadRequest))
	})
	return r
}

// lookup resolves the {tunnelId} URL param to its record.
func lookup(db tunnelmodel.Crud, r *http.Request) (*tunnelmodel.Tunnel, error) {
	id, err := strconv.Atoi(chi.URLParam(r, "tunnelId"))
	if err != nil {
		return nil, fmt.Errorf("invalid tunnel ID %s", chi.URLParam(r, "tunnelId"))
	}
	return db.GetById(uint(id))
}
