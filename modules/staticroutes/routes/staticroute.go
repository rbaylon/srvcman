// Package staticroutesroutes - Arkgate API staticroutes module
//
//	Module Routes:
//	  /api/v1/staticroutes
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of staticroute objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/staticroutes/<staticRouteId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON staticroute object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/staticroutes/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON staticroute object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package staticroutesroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/rbaylon/srvcman/modules/security"
	staticroutesmodel "github.com/rbaylon/srvcman/modules/staticroutes/model"
	"github.com/rbaylon/srvcman/utils"
)

func StaticRouteRouter(db staticroutesmodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "staticroutes"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/{staticRouteId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "staticRouteId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid staticroute ID %s", chi.URLParam(r, "staticRouteId")), http.StatusBadRequest))
			return
		}
		staticroute, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, staticroute)
	})
	r.Put("/{staticRouteId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "staticRouteId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid staticroute ID %s", chi.URLParam(r, "staticRouteId")), http.StatusBadRequest))
			return
		}
		staticroute := &staticroutesmodel.StaticRoute{}
		if err = render.Bind(r, staticroute); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		staticroute.ID = uint(id)
		errupdate := db.Update(staticroute)
		if errupdate == nil {
			render.JSON(w, r, staticroute)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(errupdate, fmt.Sprintf("Error updating record for staticroute  ID %s", chi.URLParam(r, "staticRouteId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		staticroute := &staticroutesmodel.StaticRoute{}
		if err := render.Bind(r, staticroute); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		erradd := db.Add(staticroute)
		if erradd == nil {
			render.JSON(w, r, staticroute)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(erradd, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{staticRouteId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "staticRouteId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid staticroute ID %s", chi.URLParam(r, "staticRouteId")), http.StatusBadRequest))
			return
		}
		staticroute, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		err = db.Delete(staticroute)
		if err == nil {
			render.JSON(w, r, staticroute)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for staticroute  ID %s", chi.URLParam(r, "staticRouteId")), http.StatusBadRequest))
	})
	return r
}
