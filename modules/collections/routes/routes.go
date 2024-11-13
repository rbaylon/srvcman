// Package collectionroutes - Arkgate API Collection module
//
//	Module Routes:
//	  /api/v1/collections
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of collection objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/collections/<collectionId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON collection object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/collections/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON collection object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package collectionroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	collectionmodel "github.com/rbaylon/srvcman/modules/collections/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func CollectionRouter(db collectionmodel.Crud) chi.Router {
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
	r.Get("/{collectionId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "collectionId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid collection ID %s", chi.URLParam(r, "collectionId")), http.StatusBadRequest))
			return
		}
		collection, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, collection)
	})
	r.Get("/sub/{collectionId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "collectionId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid collection ID %s", chi.URLParam(r, "collectionId")), http.StatusBadRequest))
			return
		}
		collections, err := db.GetBySubId(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, collections)
	})
	r.Put("/{collectionId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "collectionId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid collection ID %s", chi.URLParam(r, "collectionId")), http.StatusBadRequest))
			return
		}
		collection := collectionmodel.Collection{}
		if err = render.Bind(r, &collection); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		collection.ID = uint(id)
		err = db.Update(&collection)
		if err == nil {
			render.JSON(w, r, collection)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for collection  ID %s", chi.URLParam(r, "collectionId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		collection := collectionmodel.Collection{}
		if err := render.Bind(r, &collection); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(&collection)
		if err == nil {
			render.JSON(w, r, collection)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{collectionId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "collectionId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid collection ID %s", chi.URLParam(r, "collectionId")), http.StatusBadRequest))
			return
		}
		collection, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		err = db.Delete(collection)
		if err == nil {
			render.JSON(w, r, collection)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for collection  ID %s", chi.URLParam(r, "collectionId")), http.StatusBadRequest))
	})
	return r
}
