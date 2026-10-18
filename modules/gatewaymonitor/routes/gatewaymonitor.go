// Package gatewaymonitorroutes - Arkgate API gatewaymonitor module
//
//	Module Routes:
//	  /api/v1/gatewaymonitor
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of gatewaymonitor objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/gatewaymonitor/applyroutes
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Return: JSON {"applied": <n>, "failed": <n>} - re-installs the
//	            route for every gatewaymonitor record (see
//	            gatewaymonitormodel.ApplyAllRoutes); normally only run at
//	            startup, exposed here for manually re-triggering it
//	    Return-Status: 200 on Success
//
//	  /api/v1/gatewaymonitor/activeroutes
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"output": <text>, "error": <text, omitted if none>} -
//	            the output of `netstat -rn | grep UGHS` on arkgated (see
//	            gatewaymonitormodel.GetActiveRoutes). Always 200; a failure
//	            reaching arkgated is reported in "error" rather than as an
//	            HTTP error status, since this is supplementary info for the
//	            gatewaymonitor list page.
//	    Return-Status: 200 always
//
//	  /api/v1/gatewaymonitor/activehealthchecks
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"count": <n>, "total_count": <n>, "commands": [...],
//	            "error": <text, omitted if none>} - how many HealthPing
//	            connections are currently in flight on arkgated (see
//	            gatewaymonitormodel.GetActiveHealthChecks). Always 200; a
//	            failure reaching arkgated is reported in "error", same
//	            convention as /activeroutes.
//	    Return-Status: 200 always
//
//	  /api/v1/gatewaymonitor/clearhealthchecks
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Return: JSON {"cleared": <n>, "error": <text, omitted if none>} -
//	            terminates every in-flight HealthPing on arkgated (see
//	            gatewaymonitormodel.ClearActiveHealthChecks) and reports how
//	            many. The administrative "clear stuck health checks" action;
//	            does not change monitoring configuration, the next
//	            scheduled tick pings normally.
//	    Return-Status: 200 always
//
//	  /api/v1/gatewaymonitor/<gatewayMonitorId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON gatewaymonitor object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/gatewaymonitor/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON gatewaymonitor object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package gatewaymonitorroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/rbaylon/arkgatecmd"
	gatewaymonitormodel "github.com/rbaylon/srvcman/modules/gatewaymonitor/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

func GatewayMonitorRouter(db gatewaymonitormodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "gatewaymonitor"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Put("/applyroutes", func(w http.ResponseWriter, r *http.Request) {
		applied, failed := gatewaymonitormodel.ApplyAllRoutes(dbconn)
		render.JSON(w, r, map[string]int{"applied": applied, "failed": failed})
	})
	r.Get("/activeroutes", func(w http.ResponseWriter, r *http.Request) {
		out, err := gatewaymonitormodel.GetActiveRoutes()
		resp := struct {
			Output string `json:"output"`
			Error  string `json:"error,omitempty"`
		}{Output: out}
		if err != nil {
			resp.Error = err.Error()
		}
		render.JSON(w, r, resp)
	})
	r.Get("/activehealthchecks", func(w http.ResponseWriter, r *http.Request) {
		snap, err := gatewaymonitormodel.GetActiveHealthChecks()
		resp := struct {
			arkgatecmd.ActiveHealthChecksSnapshot
			Error string `json:"error,omitempty"`
		}{ActiveHealthChecksSnapshot: snap}
		if err != nil {
			resp.Error = err.Error()
		}
		render.JSON(w, r, resp)
	})
	r.Put("/clearhealthchecks", func(w http.ResponseWriter, r *http.Request) {
		n, err := gatewaymonitormodel.ClearActiveHealthChecks()
		resp := struct {
			Cleared int    `json:"cleared"`
			Error   string `json:"error,omitempty"`
		}{Cleared: n}
		if err != nil {
			resp.Error = err.Error()
		}
		render.JSON(w, r, resp)
	})
	r.Get("/{gatewayMonitorId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "gatewayMonitorId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid gatewaymonitor ID %s", chi.URLParam(r, "gatewayMonitorId")), http.StatusBadRequest))
			return
		}
		gatewaymonitor, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, gatewaymonitor)
	})
	r.Put("/{gatewayMonitorId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "gatewayMonitorId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid gatewaymonitor ID %s", chi.URLParam(r, "gatewayMonitorId")), http.StatusBadRequest))
			return
		}
		gatewaymonitor := &gatewaymonitormodel.GatewayMonitor{}
		if err = render.Bind(r, gatewaymonitor); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		gatewaymonitor.ID = uint(id)
		errupdate := db.Update(gatewaymonitor)
		if errupdate == nil {
			render.JSON(w, r, gatewaymonitor)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(errupdate, fmt.Sprintf("Error updating record for gatewaymonitor  ID %s", chi.URLParam(r, "gatewayMonitorId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		gatewaymonitor := &gatewaymonitormodel.GatewayMonitor{}
		if err := render.Bind(r, gatewaymonitor); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		erradd := db.Add(gatewaymonitor)
		if erradd == nil {
			render.JSON(w, r, gatewaymonitor)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(erradd, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{gatewayMonitorId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "gatewayMonitorId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid gatewaymonitor ID %s", chi.URLParam(r, "gatewayMonitorId")), http.StatusBadRequest))
			return
		}
		gatewaymonitor, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		err = db.Delete(gatewaymonitor)
		if err == nil {
			render.JSON(w, r, gatewaymonitor)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for gatewaymonitor  ID %s", chi.URLParam(r, "gatewayMonitorId")), http.StatusBadRequest))
	})
	return r
}
