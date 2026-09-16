// Package dhcpserverroutes - Arkgate API dhcpserver module
//
//	Module Routes:
//	  /api/v1/dhcpserver
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of dhcpserver objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/dhcpserver/conf
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"conf": "<generated dhcpd.conf text>"}, freshly
//	            rendered from the current DhcpServer rows
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/dhcpserver/<dhcpServerId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON dhcpserver object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/dhcpserver/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON dhcpserver object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package dhcpserverroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	dhcpservermodel "github.com/rbaylon/srvcman/modules/dhcpserver/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

func DhcpServerRouter(db dhcpservermodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "dhcpserver"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/conf", func(w http.ResponseWriter, r *http.Request) {
		conf, err := dhcpservermodel.GenerateConf(dbconn)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]string{"conf": conf})
	})
	r.Get("/{dhcpServerId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "dhcpServerId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid dhcpserver ID %s", chi.URLParam(r, "dhcpServerId")), http.StatusBadRequest))
			return
		}
		dhcpserver, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, dhcpserver)
	})
	r.Put("/{dhcpServerId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "dhcpServerId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid dhcpserver ID %s", chi.URLParam(r, "dhcpServerId")), http.StatusBadRequest))
			return
		}
		dhcpserver := &dhcpservermodel.DhcpServer{}
		if err = render.Bind(r, dhcpserver); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		dhcpserver.ID = uint(id)
		errupdate := db.Update(dhcpserver)
		if errupdate == nil {
			render.JSON(w, r, dhcpserver)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(errupdate, fmt.Sprintf("Error updating record for dhcpserver  ID %s", chi.URLParam(r, "dhcpServerId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		dhcpserver := &dhcpservermodel.DhcpServer{}
		if err := render.Bind(r, dhcpserver); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		erradd := db.Add(dhcpserver)
		if erradd == nil {
			render.JSON(w, r, dhcpserver)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(erradd, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{dhcpServerId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "dhcpServerId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid dhcpserver ID %s", chi.URLParam(r, "dhcpServerId")), http.StatusBadRequest))
			return
		}
		dhcpserver, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		err = db.Delete(dhcpserver)
		if err == nil {
			render.JSON(w, r, dhcpserver)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for dhcpserver  ID %s", chi.URLParam(r, "dhcpServerId")), http.StatusBadRequest))
	})
	return r
}
