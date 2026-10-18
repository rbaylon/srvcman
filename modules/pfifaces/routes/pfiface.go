// Package pfifaceroutes - Arkgate API Pfiface module
//
//	Module Routes:
//	  /api/v1/pfifaces
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of pfiface objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/pfifaces/physical
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of pfiface objects where physical is true
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/pfifaces/devices
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON array of device names detected on this host (physical
//	            NICs plus configured VLAN devices)
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/pfifaces/traffic
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON array of {device, ibytes, obytes} cumulative byte
//	            counters read from this host right now. Callers poll this
//	            repeatedly and diff successive snapshots to get a rate.
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/pfifaces/<pfifaceId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON pfiface object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/pfifaces/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON pfiface object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package pfifaceroutes

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	pfifacemodel "github.com/rbaylon/srvcman/modules/pfifaces/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

//var tokenAuth *jwtauth.JWTAuth

func PfifaceRouter(db pfifacemodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "pfifaces"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/physical", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetPhysical()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/devices", func(w http.ResponseWriter, r *http.Request) {
		res, err := db.ListDevices()
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Device list error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/traffic", func(w http.ResponseWriter, r *http.Request) {
		res, err := db.GetTrafficStats()
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Traffic stats error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/{pfifaceId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "pfifaceId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid pfiface ID %s", chi.URLParam(r, "pfifaceId")), http.StatusBadRequest))
			return
		}
		pfiface, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, pfiface)
	})
	r.Put("/{pfifaceId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "pfifaceId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid pfiface ID %s", chi.URLParam(r, "pfifaceId")), http.StatusBadRequest))
			return
		}
		pfiface := pfifacemodel.Pfiface{}
		if err = render.Bind(r, &pfiface); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		if err = refusePppac(db, uint(id), pfiface.Device); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Not accepted", http.StatusBadRequest))
			return
		}
		pfiface.ID = uint(id)
		err = db.Update(&pfiface)
		if err == nil {
			render.JSON(w, r, pfiface)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for pfiface  ID %s", chi.URLParam(r, "pfifaceId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		pfiface := pfifacemodel.Pfiface{}
		if err := render.Bind(r, &pfiface); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		if err := refusePppac(db, 0, pfiface.Device); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Not accepted", http.StatusBadRequest))
			return
		}
		err := db.Add(&pfiface)
		if err == nil {
			render.JSON(w, r, pfiface)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{pfifaceId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "pfifaceId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid pfiface ID %s", chi.URLParam(r, "pfifaceId")), http.StatusBadRequest))
			return
		}
		pfiface := pfifacemodel.Pfiface{}
		if err = render.Bind(r, &pfiface); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		if err = refusePppac(db, uint(id), ""); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Not accepted", http.StatusBadRequest))
			return
		}
		pfiface.ID = uint(id)
		err = db.Delete(&pfiface)
		if err == nil {
			render.JSON(w, r, pfiface)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for pfiface  ID %s", chi.URLParam(r, "pfifaceId")), http.StatusBadRequest))
	})
	return r
}

// refusePppac keeps the pppacN interfaces out of reach of this module: the PPPoE
// servers create, change and remove them, and npppd creates the devices. An
// edit here would be overwritten or, worse, leave a server pointing at nothing.
// id is the record being changed (0 for a new one), device the one it is given.
func refusePppac(db pfifacemodel.Crud, id uint, device string) error {
	const msg = "pppacN interfaces are managed by the PPPoE servers; change them under PPPoE"
	if strings.HasPrefix(device, "pppac") {
		return errors.New(msg)
	}
	if id > 0 {
		if cur, err := db.GetById(id); err == nil && strings.HasPrefix(cur.Device, "pppac") {
			return errors.New(msg)
		}
	}
	return nil
}
