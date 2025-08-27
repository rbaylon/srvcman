// Package rworkerroutes - Arkgate API Rworker module
//
//	Module Routes:
//	  /api/v1/rworkers
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of rworker objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/rworkers/<rworkerId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON rworker object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/rworkers/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON rworker object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package rworkerroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	rworkermodel "github.com/rbaylon/srvcman/modules/rworkers/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func RworkerRouter(db rworkermodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "rworkers"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/{rworkerId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "rworkerId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid rworker ID %s", chi.URLParam(r, "rworkerId")), http.StatusBadRequest))
			return
		}
		rworker, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, rworker)
	})
	r.Put("/{rworkerId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "rworkerId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid rworker ID %s", chi.URLParam(r, "rworkerId")), http.StatusBadRequest))
			return
		}
		rworker := rworkermodel.Rworker{}
		if err = render.Bind(r, &rworker); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		rworker.ID = uint(id)
		err = db.Update(&rworker)
		if err == nil {
			render.JSON(w, r, rworker)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for rworker  ID %s", chi.URLParam(r, "rworkerId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		rworker := rworkermodel.Rworker{}
		if err := render.Bind(r, &rworker); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(&rworker)
		if err == nil {
			render.JSON(w, r, rworker)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{rworkerId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "rworkerId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid rworker ID %s", chi.URLParam(r, "rworkerId")), http.StatusBadRequest))
			return
		}
		rworker := rworkermodel.Rworker{}
		if err = render.Bind(r, &rworker); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		rworker.ID = uint(id)
		err = db.Delete(&rworker)
		if err == nil {
			render.JSON(w, r, rworker)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for rworker  ID %s", chi.URLParam(r, "rworkerId")), http.StatusBadRequest))
	})
	return r
}
