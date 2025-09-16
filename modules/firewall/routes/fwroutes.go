// Package Fwroutes - Arkgate API Fw module
//
//	Module Routes:
//	  /api/v1/Fws
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of Fw objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/Fws/<FwId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON Fw object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/Fws/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON Fw object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package fwroutes

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	fwmodel "github.com/rbaylon/srvcman/modules/firewall/model"
	runtimemodel "github.com/rbaylon/srvcman/modules/runtimes/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func FwRouter(db fwmodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "Fws"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/{FwId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "FwId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid Fw ID %s", chi.URLParam(r, "FwId")), http.StatusBadRequest))
			return
		}
		Fw, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, Fw)
	})
	r.Put("/{FwId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "FwId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid Fw ID %s", chi.URLParam(r, "FwId")), http.StatusBadRequest))
			return
		}
		Fw := &fwmodel.Fw{}
		if err = render.Bind(r, Fw); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		Fw.ID = uint(id)
		errupdate := db.Update(Fw)
		if errupdate == nil {
			dbconn := db.GetDB()
			rtdb := runtimemodel.New(dbconn)
			rt := &runtimemodel.Runtime{}
			rt.Action = "updatepf"
			rt.Status = "active"
			err := rtdb.Add(rt)
			if err != nil {
				log.Println("update pf failed")
			}
			render.JSON(w, r, Fw)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(errupdate, fmt.Sprintf("Error updating record for Fw  ID %s", chi.URLParam(r, "FwId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		Fw := &fwmodel.Fw{}
		if err := render.Bind(r, Fw); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		erradd := db.Add(Fw)
		if erradd == nil {
			dbconn := db.GetDB()
			rtdb := runtimemodel.New(dbconn)
			rt := &runtimemodel.Runtime{}
			rt.Action = "updatepf"
			rt.Status = "active"
			err := rtdb.Add(rt)
			if err != nil {
				log.Println("update pf failed")
			}
			render.JSON(w, r, Fw)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(erradd, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{FwId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "FwId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid Fw ID %s", chi.URLParam(r, "FwId")), http.StatusBadRequest))
			return
		}
		Fw, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		err = db.Delete(Fw)
		if err == nil {
			dbconn := db.GetDB()
			rtdb := runtimemodel.New(dbconn)
			rt := &runtimemodel.Runtime{}
			rt.Action = "updatepf"
			rt.Status = "active"
			err := rtdb.Add(rt)
			if err != nil {
				log.Println("update pf failed")
			}
			render.JSON(w, r, Fw)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for Fw  ID %s", chi.URLParam(r, "FwId")), http.StatusBadRequest))
	})
	return r
}
