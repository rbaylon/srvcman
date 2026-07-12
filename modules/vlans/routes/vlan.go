// Package vlanroutes - Arkgate API vlan module
//
//	Module Routes:
//	  /api/v1/vlans
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of vlan objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/vlans/<vlanId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON vlan object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/vlans/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON vlan object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package Vlanroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/rbaylon/srvcman/modules/security"
	vlanmodel "github.com/rbaylon/srvcman/modules/vlans/model"
	"github.com/rbaylon/srvcman/utils"
)

//var tokenAuth *jwtauth.JWTAuth

func VlanRouter(db vlanmodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "vlans"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/{vlanId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "vlanId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid vlan ID %s", chi.URLParam(r, "vlanId")), http.StatusBadRequest))
			return
		}
		vlan, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, vlan)
	})
	r.Get("/name/{vlandevice}", func(w http.ResponseWriter, r *http.Request) {
		vlandev := chi.URLParam(r, "vlandevice")
		vlan, err := db.GetByName(vlandev)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, vlan)
	})
	r.Put("/{vlanId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "vlanId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid vlan ID %s", chi.URLParam(r, "vlanId")), http.StatusBadRequest))
			return
		}
		vlan := &vlanmodel.Vlan{}
		if err = render.Bind(r, vlan); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		vlan.ID = uint(id)
		errupdate := db.Update(vlan)
		if errupdate == nil {
			render.JSON(w, r, vlan)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(errupdate, fmt.Sprintf("Error updating record for vlan  ID %s", chi.URLParam(r, "vlanId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		vlan := &vlanmodel.Vlan{}
		if err := render.Bind(r, vlan); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		erradd := db.Add(vlan)
		if erradd == nil {
			render.JSON(w, r, vlan)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(erradd, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{vlanId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "vlanId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid vlan ID %s", chi.URLParam(r, "vlanId")), http.StatusBadRequest))
			return
		}
		vlan, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		err = db.Delete(vlan)
		if err == nil {
			render.JSON(w, r, vlan)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for vlan  ID %s", chi.URLParam(r, "vlanId")), http.StatusBadRequest))
	})
	return r
}
