// Package snmpserverroutes - Arkgate API snmpserver module
//
//	Module Routes:
//	  /api/v1/snmpserver
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of snmpserver objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/snmpserver/conf
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"conf": "<generated snmpd.conf text>"}, freshly
//	            rendered from the current SnmpServer row
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/snmpserver/<snmpServerId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON snmpserver object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/snmpserver/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON snmpserver object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package snmpserverroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/rbaylon/srvcman/modules/security"
	snmpservermodel "github.com/rbaylon/srvcman/modules/snmpserver/model"
	"github.com/rbaylon/srvcman/utils"
)

func SnmpServerRouter(db snmpservermodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "snmpserver"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/conf", func(w http.ResponseWriter, r *http.Request) {
		conf, err := snmpservermodel.GenerateConf(dbconn)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]string{"conf": conf})
	})
	r.Get("/{snmpServerId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "snmpServerId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid snmpserver ID %s", chi.URLParam(r, "snmpServerId")), http.StatusBadRequest))
			return
		}
		snmpserver, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, snmpserver)
	})
	r.Put("/{snmpServerId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "snmpServerId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid snmpserver ID %s", chi.URLParam(r, "snmpServerId")), http.StatusBadRequest))
			return
		}
		snmpserver := &snmpservermodel.SnmpServer{}
		if err = render.Bind(r, snmpserver); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		snmpserver.ID = uint(id)
		errupdate := db.Update(snmpserver)
		if errupdate == nil {
			render.JSON(w, r, snmpserver)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(errupdate, fmt.Sprintf("Error updating record for snmpserver  ID %s", chi.URLParam(r, "snmpServerId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		snmpserver := &snmpservermodel.SnmpServer{}
		if err := render.Bind(r, snmpserver); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(snmpserver)
		if err == nil {
			render.JSON(w, r, snmpserver)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{snmpServerId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "snmpServerId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid snmpserver ID %s", chi.URLParam(r, "snmpServerId")), http.StatusBadRequest))
			return
		}
		snmpserver := &snmpservermodel.SnmpServer{}
		if err = render.Bind(r, snmpserver); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		snmpserver.ID = uint(id)
		err = db.Delete(snmpserver)
		if err == nil {
			render.JSON(w, r, snmpserver)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for snmpserver  ID %s", chi.URLParam(r, "snmpServerId")), http.StatusBadRequest))
	})
	return r
}
