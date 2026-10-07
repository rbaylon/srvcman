// Package updatesroutes - Arkgate API software update module
//
// Holds the update server setting and reports which OS patches are
// available. Checking is read-only: it runs `syspatch -c`, which lists
// patches without installing any.
//
//	Module Routes:
//	  /api/v1/updates
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"update_server": "<url, or empty>"}
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/updates
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Body: JSON {"update_server": "<http(s) url, or empty to clear>"}
//	    Return: JSON {"update_server": "<url>"}
//	    Return-Status: 200 on Success
//	                   400 on Bad request
//	                   500 on Error
//
//	  /api/v1/updates/check
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Starts a patch check in the background and returns at once; the
//	    check can take minutes. If one is already running it is left alone.
//	    Return: JSON check status (see GET below) - state "running" whether
//	            this call started the check or one was already running
//	    Return-Status: 200 on Success
//
//	  /api/v1/updates/check
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"state": "idle|running|done|failed",
//	                  "started_at": "<time>", "finished_at": "<time>",
//	                  "available": ["<patch>", ...], "output": "<syspatch output>",
//	                  "error": "<message, when failed>"}
//	            - state done with an empty "available" list means the
//	              system is up to date
//	    Return-Status: 200 on Success
package updatesroutes

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/rbaylon/srvcman/modules/security"
	updatesmodel "github.com/rbaylon/srvcman/modules/updates/model"
	"github.com/rbaylon/srvcman/utils"
	"gorm.io/gorm"
)

type settings struct {
	UpdateServer string `json:"update_server"`
}

func UpdatesRouter(db *gorm.DB) chi.Router {
	r := chi.NewRouter()
	r.Use(security.TokenRequired(db, "updates"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		u, err := updatesmodel.Get(db)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, settings{UpdateServer: u.UpdateServer})
	})
	r.Put("/", func(w http.ResponseWriter, r *http.Request) {
		in := &updatesmodel.Updates{}
		if err := render.Bind(r, in); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		u, err := updatesmodel.Save(db, in.UpdateServer)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, settings{UpdateServer: u.UpdateServer})
	})
	// The check is started in the background and polled, not waited on:
	// syspatch fetches its patch list over the network and can take
	// minutes, far longer than a request should be held open.
	r.Put("/check", func(w http.ResponseWriter, r *http.Request) {
		// Plain 200 either way: srvcmanui's client treats only 200/201 as
		// success, and the state field already says what happened.
		state, _ := updatesmodel.StartCheck()
		render.JSON(w, r, state)
	})
	r.Get("/check", func(w http.ResponseWriter, r *http.Request) {
		render.JSON(w, r, updatesmodel.CheckStatus())
	})
	return r
}
