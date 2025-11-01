// Package planroutes - Arkgate API Plan module
//
//	Module Routes:
//	  /api/v1/plans
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of plan objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/plans/<planId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON plan object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/plans/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON plan object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package planroutes

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	planmodel "github.com/rbaylon/srvcman/modules/plans/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func PlanRouter(db planmodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "plans"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		w.Header().Set("Content-Type", "application/json")
		if errdb != nil {
			json.NewEncoder(w).Encode(map[string]string{"Error": fmt.Sprintf("%s", errdb)})
			return
		}
		json.NewEncoder(w).Encode(res)
	})
	r.Get("/{planId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "planId"))
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"Error": fmt.Sprintf("%s", err)})
			return
		}
		plan, err := db.GetById(uint(id))
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"Error": fmt.Sprintf("%s", err)})
			return
		}
		json.NewEncoder(w).Encode(plan)
	})
	r.Put("/{planId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "planId"))
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"Error": fmt.Sprintf("%s", err)})
			return
		}
		plan := planmodel.Plan{}
		err = json.NewDecoder(r.Body).Decode(&plan)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"Error": fmt.Sprintf("%s", err)})
			return
		}
		plan.ID = uint(id)
		err = db.Update(&plan)
		if err == nil {
			json.NewEncoder(w).Encode(&plan)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"Error": "Error updating record for plan  ID"})
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		plan := planmodel.Plan{}
		err := json.NewDecoder(r.Body).Decode(&plan)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"Error": fmt.Sprintf("%s", err)})
		}

		err = db.Add(&plan)
		if err == nil {
			json.NewEncoder(w).Encode(&plan)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{planId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "planId"))
		log.Println("About to delete Plan:", id)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"Error": fmt.Sprintf("Atoi error %s", err)})
			return
		}
		plan, err := db.GetById(uint(id))
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"Error": fmt.Sprintf("Error getting plan from DB %s", err)})
			return
		}
		err = db.Delete(plan)
		if err == nil {
			log.Println("Deleted Plan:", id)
			json.NewEncoder(w).Encode(&plan)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"Error": "Error deleting record for plan ID"})
	})
	return r
}
