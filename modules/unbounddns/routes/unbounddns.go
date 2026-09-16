// Package unbounddnsroutes - Arkgate API unbounddns module
//
//	Module Routes:
//	  /api/v1/unbounddns
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of unbounddns objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/unbounddns/conf
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"conf": "<generated unbound.conf text>"}, freshly
//	            rendered from the current UnboundDns rows
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/unbounddns/<unboundDnsId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON unbounddns object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/unbounddns/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON unbounddns object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package unbounddnsroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/rbaylon/srvcman/modules/security"
	unbounddnsmodel "github.com/rbaylon/srvcman/modules/unbounddns/model"
	"github.com/rbaylon/srvcman/utils"
)

func UnboundDnsRouter(db unbounddnsmodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "unbounddns"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/conf", func(w http.ResponseWriter, r *http.Request) {
		conf, err := unbounddnsmodel.GenerateConf(dbconn)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]string{"conf": conf})
	})
	r.Get("/{unboundDnsId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "unboundDnsId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid unbounddns ID %s", chi.URLParam(r, "unboundDnsId")), http.StatusBadRequest))
			return
		}
		unbounddns, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, unbounddns)
	})
	r.Put("/{unboundDnsId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "unboundDnsId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid unbounddns ID %s", chi.URLParam(r, "unboundDnsId")), http.StatusBadRequest))
			return
		}
		unbounddns := &unbounddnsmodel.UnboundDns{}
		if err = render.Bind(r, unbounddns); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		unbounddns.ID = uint(id)
		errupdate := db.Update(unbounddns)
		if errupdate == nil {
			render.JSON(w, r, unbounddns)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(errupdate, fmt.Sprintf("Error updating record for unbounddns  ID %s", chi.URLParam(r, "unboundDnsId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		unbounddns := &unbounddnsmodel.UnboundDns{}
		if err := render.Bind(r, unbounddns); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		erradd := db.Add(unbounddns)
		if erradd == nil {
			render.JSON(w, r, unbounddns)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(erradd, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{unboundDnsId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "unboundDnsId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid unbounddns ID %s", chi.URLParam(r, "unboundDnsId")), http.StatusBadRequest))
			return
		}
		unbounddns, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		err = db.Delete(unbounddns)
		if err == nil {
			render.JSON(w, r, unbounddns)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for unbounddns  ID %s", chi.URLParam(r, "unboundDnsId")), http.StatusBadRequest))
	})
	return r
}
