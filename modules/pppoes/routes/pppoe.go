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
package pppoeroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	pppoemodel "github.com/rbaylon/srvcman/modules/pppoes/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func PppoeRouter(db pppoemodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "pppoes"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
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
