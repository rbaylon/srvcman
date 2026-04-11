// Package Changeroutes - Arkgate API Change module
//
//	Module Routes:
//	  /api/v1/changes
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of Change objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/changes/<ChangeId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON Change object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/changes/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON Change object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package changeroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	Changemodel "github.com/rbaylon/srvcman/modules/changes/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func ChangeRouter(db Changemodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "Changes"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/{ChangeId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "ChangeId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid Change ID %s", chi.URLParam(r, "ChangeId")), http.StatusBadRequest))
			return
		}
		Change, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, Change)
	})
	r.Get("/sub/{ChangeId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "ChangeId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid Change ID %s", chi.URLParam(r, "ChangeId")), http.StatusBadRequest))
			return
		}
		Changes, err := db.GetBySubId(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, Changes)
	})
	r.Put("/{ChangeId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "ChangeId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid Change ID %s", chi.URLParam(r, "ChangeId")), http.StatusBadRequest))
			return
		}
		Change := Changemodel.Change{}
		if err = render.Bind(r, &Change); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		Change.ID = uint(id)
		err = db.Update(&Change)
		if err == nil {
			render.JSON(w, r, Change)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for Change  ID %s", chi.URLParam(r, "ChangeId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		Change := Changemodel.Change{}
		if err := render.Bind(r, &Change); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(&Change)
		if err == nil {
			render.JSON(w, r, Change)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{ChangeId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "ChangeId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid Change ID %s", chi.URLParam(r, "ChangeId")), http.StatusBadRequest))
			return
		}
		Change := Changemodel.Change{}
		if err = render.Bind(r, &Change); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		Change.ID = uint(id)
		err = db.Delete(&Change)
		if err == nil {
			render.JSON(w, r, Change)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for Change  ID %s", chi.URLParam(r, "ChangeId")), http.StatusBadRequest))
	})
	return r
}
