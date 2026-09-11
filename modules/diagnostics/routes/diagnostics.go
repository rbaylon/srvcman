// Package diagnosticsroutes - Arkgate API diagnostics module
//
// Unlike every other module, diagnostics has no modules/diagnostics/model
// package: there's nothing to persist, it just triggers ping/traceroute
// on demand and returns their output, so DiagnosticsRouter takes a plain
// *gorm.DB (only needed by security.TokenRequired for policy checks)
// rather than a model.Crud.
//
//	Module Routes:
//	  /api/v1/diagnostics/ping/<ip>
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Query:   ?count=<n> (optional, 1-20, default 4)
//	    Return: JSON {"output": "<ping output>"}
//	    Return-Status: 200 on Success
//	                   400 if <ip> isn't a valid IP address
//
//	  /api/v1/diagnostics/traceroute/<ip>
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"output": "<traceroute output>"}
//	    Return-Status: 200 on Success
//	                   400 if <ip> isn't a valid IP address
package diagnosticsroutes

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
	"gorm.io/gorm"
)

// cmdTimeout bounds how long a single ping/traceroute run is allowed to take.
const cmdTimeout = 15 * time.Second

// defaultPingCount is how many echo requests ping sends when the caller
// doesn't override it with ?count=.
const defaultPingCount = 4

// maxPingCount caps the ?count= query param so a caller can't turn this
// into an indefinite ping flood against cmdTimeout.
const maxPingCount = 20

func DiagnosticsRouter(db *gorm.DB) chi.Router {
	r := chi.NewRouter()
	r.Use(security.TokenRequired(db, "diagnostics"))

	r.Get("/ping/{ip}", func(w http.ResponseWriter, r *http.Request) {
		ip := chi.URLParam(r, "ip")
		if net.ParseIP(ip) == nil {
			render.Render(w, r, utils.ErrInvalidRequest(fmt.Errorf("invalid IP"), fmt.Sprintf("%q is not a valid IP address", ip), http.StatusBadRequest))
			return
		}
		count := defaultPingCount
		if c, err := strconv.Atoi(r.URL.Query().Get("count")); err == nil && c > 0 && c <= maxPingCount {
			count = c
		}
		ctx, cancel := context.WithTimeout(r.Context(), cmdTimeout)
		defer cancel()
		// exec.CommandContext passes ip as a single argv element, never
		// through a shell, so there's no injection risk from it - but the
		// net.ParseIP check above still matters, since a value starting
		// with '-' would otherwise be parsed as a ping/traceroute flag
		// rather than a target.
		out, _ := exec.CommandContext(ctx, "ping", "-c", strconv.Itoa(count), ip).CombinedOutput()
		render.JSON(w, r, map[string]string{"output": string(out)})
	})

	r.Get("/traceroute/{ip}", func(w http.ResponseWriter, r *http.Request) {
		ip := chi.URLParam(r, "ip")
		if net.ParseIP(ip) == nil {
			render.Render(w, r, utils.ErrInvalidRequest(fmt.Errorf("invalid IP"), fmt.Sprintf("%q is not a valid IP address", ip), http.StatusBadRequest))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), cmdTimeout)
		defer cancel()
		// -n: don't resolve hop addresses to hostnames. -I: use ICMP echo
		// instead of UDP, since UDP traceroute needs firewall holes ping
		// doesn't.
		out, _ := exec.CommandContext(ctx, "traceroute", "-nI", ip).CombinedOutput()
		render.JSON(w, r, map[string]string{"output": string(out)})
	})

	return r
}
