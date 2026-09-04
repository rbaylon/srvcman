// Package gatewayroutes - Arkgate API gateway module
//
//	Module Routes:
//	  /api/v1/gateways
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of gateway objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/gateways/<gatewayId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON gateway object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/gateways/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON gateway object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package gatewayroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	gatewaymodel "github.com/rbaylon/srvcman/modules/gateways/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

func GatewayRouter(db gatewaymodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "gateways"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/{gatewayId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "gatewayId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid gateway ID %s", chi.URLParam(r, "gatewayId")), http.StatusBadRequest))
			return
		}
		gateway, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, gateway)
	})
	r.Put("/{gatewayId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "gatewayId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid gateway ID %s", chi.URLParam(r, "gatewayId")), http.StatusBadRequest))
			return
		}
		gateway := &gatewaymodel.Gateway{}
		if err = render.Bind(r, gateway); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		gateway.ID = uint(id)
		errupdate := db.Update(gateway)
		if errupdate == nil {
			render.JSON(w, r, gateway)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(errupdate, fmt.Sprintf("Error updating record for gateway  ID %s", chi.URLParam(r, "gatewayId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		gateway := &gatewaymodel.Gateway{}
		if err := render.Bind(r, gateway); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		erradd := db.Add(gateway)
		if erradd == nil {
			render.JSON(w, r, gateway)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(erradd, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{gatewayId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "gatewayId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid gateway ID %s", chi.URLParam(r, "gatewayId")), http.StatusBadRequest))
			return
		}
		gateway, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		err = db.Delete(gateway)
		if err == nil {
			render.JSON(w, r, gateway)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for gateway  ID %s", chi.URLParam(r, "gatewayId")), http.StatusBadRequest))
	})
	return r
}
