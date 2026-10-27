// Package grouproutes - Arkgate API Group module
//
//	Module Routes:
//	  /api/v1/groups
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of group objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/groups/<groupId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON group object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/groups/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON group object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package grouproutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	groupmodel "github.com/rbaylon/srvcman/modules/groups/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func GroupRouter(db groupmodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "groups"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/{groupId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "groupId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid group ID %s", chi.URLParam(r, "groupId")), http.StatusBadRequest))
			return
		}
		group, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, group)
	})

	r.Get("/getbyname/{name}", func(w http.ResponseWriter, r *http.Request) {
		gname := chi.URLParam(r, "name")
		group, err := db.GetByGroupname(gname)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, group)
	})

	r.Put("/{groupId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "groupId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid group ID %s", chi.URLParam(r, "groupId")), http.StatusBadRequest))
			return
		}
		group := groupmodel.Group{}
		if err = render.Bind(r, &group); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		group.ID = uint(id)
		err = db.Update(&group)
		if err == nil {
			render.JSON(w, r, group)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for group  ID %s", chi.URLParam(r, "groupId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		group := groupmodel.Group{}
		if err := render.Bind(r, &group); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(&group)
		if err == nil {
			render.JSON(w, r, group)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{groupId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "groupId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid group ID %s", chi.URLParam(r, "groupId")), http.StatusBadRequest))
			return
		}
		// No render.Bind here, unlike the other modules': a DELETE carries no
		// body worth validating, and Bind requires a groupname - which the empty body
		// does not have, so binding it would reject every delete.
		group := groupmodel.Group{}
		group.ID = uint(id)
		err = db.Delete(&group)
		if err == nil {
			render.JSON(w, r, group)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for group  ID %s", chi.URLParam(r, "groupId")), http.StatusBadRequest))
	})
	return r
}
