// Package pflowroutes - Arkgate API Pflow module
//
//	Module Routes:
//	  /api/v1/pflows
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of pflow objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/pflows/<pflowId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON pflow object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/pflows/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON pflow object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package pflowroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	pfconfig "github.com/rbaylon/srvcman/modules/pfconfig/model"
	pflowmodel "github.com/rbaylon/srvcman/modules/pflows/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func PflowRouter(db pflowmodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "pflows"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/{pflowId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "pflowId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid pflow ID %s", chi.URLParam(r, "pflowId")), http.StatusBadRequest))
			return
		}
		pflow, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, pflow)
	})
	r.Put("/{pflowId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "pflowId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid pflow ID %s", chi.URLParam(r, "pflowId")), http.StatusBadRequest))
			return
		}
		pflow := pflowmodel.Pflow{}
		if err = render.Bind(r, &pflow); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		pflow.ID = uint(id)
		err = db.Update(&pflow)
		if err == nil {
			render.JSON(w, r, pflow)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for pflow  ID %s", chi.URLParam(r, "pflowId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		pflow := pflowmodel.Pflow{}
		if err := render.Bind(r, &pflow); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(&pflow)
		if err == nil {
			cfgdb := pfconfig.New(db.GetDB())
			pfcfg, err := cfgdb.GetById(pflow.PfconfigID)
			if err != nil {
				fmt.Println("Failed to load pfconfig")
				return
			}
			if !pfcfg.PflowEnable {
				pfcfg.PflowEnable = true
				err := cfgdb.Update(pfcfg)
				if err != nil {
					fmt.Println("Failed to set Pflow Enable!")
				}
			}
			render.JSON(w, r, pflow)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{pflowId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "pflowId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid pflow ID %s", chi.URLParam(r, "pflowId")), http.StatusBadRequest))
			return
		}
		pflow := pflowmodel.Pflow{}
		if err = render.Bind(r, &pflow); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		pflow.ID = uint(id)
		err = db.Delete(&pflow)
		if err == nil {
			cfgdb := pfconfig.New(db.GetDB())
			pfcfg, err := cfgdb.GetById(pflow.PfconfigID)
			if err != nil {
				fmt.Println("Failed to load pfconfig")
				return
			}
			if pfcfg.PflowEnable {
				pfcfg.PflowEnable = false
				err := cfgdb.Update(pfcfg)
				if err != nil {
					fmt.Println("Failed to set Pflow Enable!")
				}
			}
			render.JSON(w, r, pflow)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for pflow  ID %s", chi.URLParam(r, "pflowId")), http.StatusBadRequest))
	})
	return r
}
