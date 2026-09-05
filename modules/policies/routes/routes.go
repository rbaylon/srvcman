// Package policyroutes - Arkgate API Policy module
//
//	Module Routes:
//	  /api/v1/policys
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of policy objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/policys/defaults
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of the default view policy records
//	            (see policymodel.GetDefaultPolicies), for whichever of them
//	            currently exist in the DB
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/policys/<policyId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON policy object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/policys/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON policy object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package policyroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	policymodel "github.com/rbaylon/srvcman/modules/policies/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func PolicyRouter(db policymodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "policies"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/defaults", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := policymodel.GetDefaultPolicies(dbconn)
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/{policyId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "policyId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid policy ID %s", chi.URLParam(r, "policyId")), http.StatusBadRequest))
			return
		}
		policy, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, policy)
	})
	r.Put("/{policyId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "policyId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid policy ID %s", chi.URLParam(r, "policyId")), http.StatusBadRequest))
			return
		}
		policy := policymodel.Policy{}
		if err = render.Bind(r, &policy); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		oldpolicy, _ := db.GetById(uint(id))
		db.RemoveGroups(oldpolicy)
		policy.ID = uint(id)
		err = db.Update(&policy)
		if err == nil {
			render.JSON(w, r, policy)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for policy  ID %s", chi.URLParam(r, "policyId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		policy := policymodel.Policy{}
		if err := render.Bind(r, &policy); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(&policy)
		if err == nil {
			render.JSON(w, r, policy)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{policyId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "policyId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid policy ID %s", chi.URLParam(r, "policyId")), http.StatusBadRequest))
			return
		}
		policy := policymodel.Policy{}
		if err = render.Bind(r, &policy); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		policy.ID = uint(id)
		err = db.Delete(&policy)
		if err == nil {
			render.JSON(w, r, policy)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for policy  ID %s", chi.URLParam(r, "policyId")), http.StatusBadRequest))
	})
	return r
}
