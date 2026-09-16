// Package subroutes - Arkgate API Sub module
//
//	Module Routes:
//	  /api/v1/subs
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of sub objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/subs/<subId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON sub object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/subs/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON sub object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/subs/migrategateway
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Body: submodel.MigrateGatewayRequest {old_gateway, new_gateway, count}
//	    Return: JSON {"migrated": <n>} - reassigns up to count active subs
//	            off old_gateway onto new_gateway (see
//	            submodel.Storage.MigrateGateway)
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package subroutes

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	collectionmodel "github.com/rbaylon/srvcman/modules/collections/model"
	runtimemodel "github.com/rbaylon/srvcman/modules/runtimes/model"
	"github.com/rbaylon/srvcman/modules/security"
	submodel "github.com/rbaylon/srvcman/modules/subs/model"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func SubRouter(db submodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "subs"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/{subId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "subId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid sub ID %s", chi.URLParam(r, "subId")), http.StatusBadRequest))
			return
		}
		sub, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, sub)
	})
	r.Put("/{subId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "subId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid sub ID %s", chi.URLParam(r, "subId")), http.StatusBadRequest))
			return
		}
		sub := &submodel.Sub{}
		if err = render.Bind(r, sub); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		sub.ID = uint(id)
		errupdate := db.Update(sub)
		if errupdate == nil {
			render.JSON(w, r, sub)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(errupdate, fmt.Sprintf("Error updating record for sub  ID %s", chi.URLParam(r, "subId")), http.StatusBadRequest))
	})
	r.Put("/migrategateway", func(w http.ResponseWriter, r *http.Request) {
		req := &submodel.MigrateGatewayRequest{}
		if err := render.Bind(r, req); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		n, err := db.MigrateGateway(req.OldGateway, req.NewGateway, req.Count)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, map[string]int64{"migrated": n})
	})
	r.Get("/activate/{ip}/{status}", func(w http.ResponseWriter, r *http.Request) {
		ip := chi.URLParam(r, "ip")
		status := chi.URLParam(r, "status")
		sub, err := db.GetByIp(ip)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}

		run := true
		if status == "active" {
			if sub.Status == status {
				render.JSON(w, r, sub)
				return
			}
			sub.Status = status
			sub.LaterCount += 1
			if sub.LaterCount > 14 {
				sub.Status = "inactive"
				sub.LaterCount -= 1
				run = false
			}
		} else {
			sub.Status = status
		}
		err = db.Update(sub)
		if err == nil && run {
			dbconn := db.GetDB()
			rtdb := runtimemodel.New(dbconn)
			rt := &runtimemodel.Runtime{}
			rt.Action = "updatepf"
			rt.Status = "active"
			rt.Workerid = int(sub.PfconfigID)
			err := rtdb.Add(rt)
			if err != nil {
				log.Println("update pf failed")
			}
		} else {
			log.Println("update user status failed")
		}
		render.JSON(w, r, sub)
	})
	r.Get("/getbyip/{ip}", func(w http.ResponseWriter, r *http.Request) {
		ip := chi.URLParam(r, "ip")
		sub, err := db.GetByIp(ip)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, sub)
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		sub := &submodel.Sub{}
		if err := render.Bind(r, sub); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		erradd := db.Add(sub)
		if erradd == nil {
			coldb := collectionmodel.New(db.GetDB())
			if err := coldb.Add(collectionmodel.NewForSub(sub.ID)); err != nil {
				// collections is an optional module - its table may not be
				// migrated, so this is best-effort and shouldn't fail sub
				// creation.
				log.Println("create collection record for sub failed:", err)
			}
			render.JSON(w, r, sub)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(erradd, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{subId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "subId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid sub ID %s", chi.URLParam(r, "subId")), http.StatusBadRequest))
			return
		}
		sub, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		err = db.Delete(sub)
		if err == nil {
			render.JSON(w, r, sub)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for sub  ID %s", chi.URLParam(r, "subId")), http.StatusBadRequest))
	})
	return r
}
