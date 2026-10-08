// Package netblocksroutes - Arkgate API speedtest netblocks module
//
// Network blocks (CIDR) that are kept both in the database and, one line
// each, in the gateway's fast.com.blocks file. Creating, changing or
// deleting a record edits that file through arkgated as well; if the file
// cannot be edited the request fails and the database is left unchanged.
//
//	Module Routes:
//	  /api/v1/netblocks
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON list of netblock objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/netblocks/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Body: JSON {"cidr": "192.168.1.0/24"}
//	    Return: JSON netblock object (cidr in canonical form)
//	    Return-Status: 200 on Success
//	                   400 on a bad or duplicate block
//	                   500 if the database or the blocks file could not be written
//
//	  /api/v1/netblocks/<netblockId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON netblock object ( except for delete method )
//	    Return-Status: 200 on Success
//	                   400 on Bad request
//	                   404 if the record does not exist
//	                   500 on Error
package netblocksroutes

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	netblocksmodel "github.com/rbaylon/srvcman/modules/netblocks/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
	"gorm.io/gorm"
)

// fail renders err with a status that tells a bad request or a missing
// record apart from a real failure.
func fail(w http.ResponseWriter, r *http.Request, err error, msg string) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, netblocksmodel.ErrDuplicate):
		status = http.StatusBadRequest
	case errors.Is(err, gorm.ErrRecordNotFound):
		status = http.StatusNotFound
	}
	render.Render(w, r, utils.ErrInvalidRequest(err, msg, status))
}

func idParam(w http.ResponseWriter, r *http.Request) (uint, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "netblockId"))
	if err != nil || id < 1 {
		render.Render(w, r, utils.ErrInvalidRequest(fmt.Errorf("invalid id"), fmt.Sprintf("Invalid netblock ID %s", chi.URLParam(r, "netblockId")), http.StatusBadRequest))
		return 0, false
	}
	return uint(id), true
}

func NetblocksRouter(db *gorm.DB) chi.Router {
	r := chi.NewRouter()
	r.Use(security.TokenRequired(db, "netblocks"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		all, err := netblocksmodel.GetAll(db)
		if err != nil {
			fail(w, r, err, "DB error")
			return
		}
		render.JSON(w, r, all)
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		n := &netblocksmodel.Netblock{}
		if err := render.Bind(r, n); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		if err := netblocksmodel.Create(db, n); err != nil {
			fail(w, r, err, "Could not add the network block")
			return
		}
		render.JSON(w, r, n)
	})
	r.Get("/{netblockId}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := idParam(w, r)
		if !ok {
			return
		}
		n, err := netblocksmodel.GetById(db, id)
		if err != nil {
			fail(w, r, err, "DB error")
			return
		}
		render.JSON(w, r, n)
	})
	r.Put("/{netblockId}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := idParam(w, r)
		if !ok {
			return
		}
		n := &netblocksmodel.Netblock{}
		if err := render.Bind(r, n); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		n.ID = id
		if err := netblocksmodel.Update(db, n); err != nil {
			fail(w, r, err, "Could not update the network block")
			return
		}
		render.JSON(w, r, n)
	})
	r.Delete("/{netblockId}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := idParam(w, r)
		if !ok {
			return
		}
		if err := netblocksmodel.Delete(db, id); err != nil {
			fail(w, r, err, "Could not delete the network block")
			return
		}
		render.JSON(w, r, map[string]string{"status": "deleted"})
	})
	return r
}
