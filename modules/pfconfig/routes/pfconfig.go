// Package pfconfigroutes - Arkgate API Pfconfig module
//
//	Module Routes:
//	  /api/v1/pfconfigs
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of pfconfig objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/pfconfigs/<pfconfigId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON pfconfig object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/pfconfigs/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON pfconfig object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package pfconfigroutes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	pfconfigmodel "github.com/rbaylon/srvcman/modules/pfconfig/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func PfconfigRouter(db pfconfigmodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "pfconfig"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		w.Header().Set("Content-Type", "application/json")
		if errdb != nil {
			json.NewEncoder(w).Encode(map[string]string{"Error": fmt.Sprintf("%s", errdb)})
			return
		}
		json.NewEncoder(w).Encode(res)
	})
	r.Get("/{pfconfigId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "pfconfigId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid pfconfig ID %s", chi.URLParam(r, "pfconfigId")), http.StatusBadRequest))
			return
		}
		pfconfig, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, pfconfig)
	})
	r.Get("/query/{routerid}", func(w http.ResponseWriter, r *http.Request) {
		val := chi.URLParam(r, "routerid")
		pfconfig, err := db.GetByRouterId(val)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB get error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, pfconfig)
	})
	r.Put("/{pfconfigId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "pfconfigId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid pfconfig ID %s", chi.URLParam(r, "pfconfigId")), http.StatusBadRequest))
			return
		}
		pfconfig := pfconfigmodel.Pfconfig{}
		if err = render.Bind(r, &pfconfig); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		pfconfig.ID = uint(id)
		err = db.Update(&pfconfig)
		if err == nil {
			render.JSON(w, r, pfconfig)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for pfconfig  ID %s", chi.URLParam(r, "pfconfigId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		pfconfig := pfconfigmodel.Pfconfig{}
		err := json.NewDecoder(r.Body).Decode(&pfconfig)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"Error": fmt.Sprintf("%s", err)})
		}

		err = db.Add(&pfconfig)
		if err == nil {
			json.NewEncoder(w).Encode(pfconfig)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{pfconfigId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "pfconfigId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid pfconfig ID %s", chi.URLParam(r, "pfconfigId")), http.StatusBadRequest))
			return
		}
		pfconfig := pfconfigmodel.Pfconfig{}
		if err = render.Bind(r, &pfconfig); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		pfconfig.ID = uint(id)
		err = db.Delete(&pfconfig)
		if err == nil {
			render.JSON(w, r, pfconfig)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for pfconfig  ID %s", chi.URLParam(r, "pfconfigId")), http.StatusBadRequest))
	})
	return r
}
