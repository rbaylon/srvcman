// Package dhcproutes - Arkgate API Dhcp module
//
//	Module Routes:
//	  /api/v1/dhcps
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of dhcp objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/dhcps/<dhcpId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON dhcp object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/dhcps/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON dhcp object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package dhcproutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	dhcpmodel "github.com/rbaylon/srvcman/modules/dhcps/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func DhcpRouter(db dhcpmodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "dhcps"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/{dhcpId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "dhcpId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid dhcp ID %s", chi.URLParam(r, "dhcpId")), http.StatusBadRequest))
			return
		}
		dhcp, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, dhcp)
	})
	r.Put("/{dhcpId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "dhcpId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid dhcp ID %s", chi.URLParam(r, "dhcpId")), http.StatusBadRequest))
			return
		}
		dhcp := dhcpmodel.Dhcp{}
		if err = render.Bind(r, &dhcp); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		dhcp.ID = uint(id)
		err = db.Update(&dhcp)
		if err == nil {
			render.JSON(w, r, dhcp)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for dhcp  ID %s", chi.URLParam(r, "dhcpId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		dhcp := dhcpmodel.Dhcp{}
		if err := render.Bind(r, &dhcp); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(&dhcp)
		if err == nil {
			render.JSON(w, r, dhcp)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{dhcpId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "dhcpId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid dhcp ID %s", chi.URLParam(r, "dhcpId")), http.StatusBadRequest))
			return
		}
		dhcp := dhcpmodel.Dhcp{}
		if err = render.Bind(r, &dhcp); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		dhcp.ID = uint(id)
		err = db.Delete(&dhcp)
		if err == nil {
			render.JSON(w, r, dhcp)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for dhcp  ID %s", chi.URLParam(r, "dhcpId")), http.StatusBadRequest))
	})
	return r
}
