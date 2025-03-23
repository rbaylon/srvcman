// Package runtimeroutes - Arkgate API Runtime module
//
//	Module Routes:
//	  /api/v1/runtimes
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of runtime objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/runtimes/<runtimeId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON runtime object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/runtimes/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON runtime object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package runtimeroutes

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	runtimemodel "github.com/rbaylon/srvcman/modules/runtimes/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func RuntimeRouter(db runtimemodel.Crud) chi.Router {
	r := chi.NewRouter()
	r.Use(security.TokenRequired)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/query/{action}", func(w http.ResponseWriter, r *http.Request) {
		val := chi.URLParam(r, "action")
		rt, err := db.GetByAction(val)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB get error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, rt)
	})
	r.Get("/delete/{action}", func(w http.ResponseWriter, r *http.Request) {
		val := chi.URLParam(r, "action")
		rt, err := db.GetByAction(val)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB get error", http.StatusInternalServerError))
			return
		}
		err = db.Delete(rt)
		if err == nil {
			render.JSON(w, r, rt)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB delete error", http.StatusInternalServerError))
		return
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		rt := &runtimemodel.Runtime{}
		if err := render.Bind(r, rt); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		erradd := db.Add(rt)
		if erradd == nil {
			render.JSON(w, r, rt)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(erradd, "DB error", http.StatusInternalServerError))
	})
	return r
}
