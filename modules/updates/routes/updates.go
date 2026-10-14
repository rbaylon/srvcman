// Package updatesroutes - Arkgate API software update module
//
// Holds the update server setting and reports which OS patches are
// available. Checking is read-only: it runs `syspatch -c`, which lists
// patches without installing any.
//
//	Module Routes:
//	  /api/v1/updates
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"update_server": "<url, or empty>", "product": "<slug>", "token_set": bool}
//	            (the API token itself is never returned)
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/updates
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Body: JSON {"update_server": "<http(s) url, or empty to clear>",
//	                "product": "<slug; omit to keep, empty for the default>",
//	                "api_token": "<token; omit or empty to keep>", "clear_token": true to remove it}
//	    Return: JSON {"update_server": "<url>", "product": "<slug>", "token_set": bool}
//	    Return-Status: 200 on Success
//	                   400 on Bad request
//	                   500 on Error
//
//	  Software updates from the update server (the operating system is patched
//	  automatically as the first step of installing one, so the OS check below
//	  is not part of this flow):
//
//	  /api/v1/updates/software
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Asks the update server what the latest update is for this box and
//	    reports it. A failed check is a 200 with "error" set, not an error status.
//	    Return: JSON {"configured": bool, "product": "<slug>", "release": "<release this box runs, or empty>",
//	                  "installed": "<version of the last update installed, or empty>",
//	                  "tier": "free|pro", "latest": {version, notes, channels, published_at,
//	                  available_from, files[]} or null, "scheduled": {version, available_from},
//	                  "update_available": bool, "error": "<why the check failed>", "checked_at": "<time>"}
//	    Return-Status: 200 on Success
//
//	  /api/v1/updates/software/install
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Starts installing the latest update in the background and returns at
//	    once with the job state; if one is running it is left alone.
//	    Return: JSON job (see GET below)
//	    Return-Status: 200 on Success
//	                   400 on Refusal (no server/token, nothing to install, update server error)
//
//	  /api/v1/updates/software/install
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"state": "idle|running|success|failed", "step": "downloading|preparing|installing",
//	                  "version": "<update version>", "started_at": "<time>", "finished_at": "<time>",
//	                  "message": "<text>", "log": "<tail of the install log when failed>",
//	                  "reboot_pending": bool}
//	    Return-Status: 200 on Success
//
//	  /api/v1/updates/mirror
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"mirror": "<the gateway's /etc/installurl>", "default": "<OpenBSD's default>"}
//	    Return-Status: 200 on Success
//
//	  /api/v1/updates/mirror
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Body: JSON {"mirror": "<http(s) address>"}
//	    The new mirror is tested (it must serve this release's SHA256.sig)
//	    before /etc/installurl is replaced.
//	    Return: JSON as GET
//	    Return-Status: 200 on Success
//	                   400 on a bad address or a mirror that does not answer
//
//	  /api/v1/updates/check
//	    Method: PUT
//	    Headers: Authorization Bearer
//	    Starts a patch check in the background and returns at once; the
//	    check can take minutes. If one is already running it is left alone.
//	    Return: JSON check status (see GET below) - state "running" whether
//	            this call started the check or one was already running
//	    Return-Status: 200 on Success
//
//	  /api/v1/updates/check
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON {"state": "idle|running|done|failed",
//	                  "started_at": "<time>", "finished_at": "<time>",
//	                  "available": ["<patch>", ...], "output": "<syspatch output>",
//	                  "error": "<message, when failed>"}
//	            - state done with an empty "available" list means the
//	              system is up to date
//	    Return-Status: 200 on Success
package updatesroutes

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/rbaylon/srvcman/modules/security"
	updatesmodel "github.com/rbaylon/srvcman/modules/updates/model"
	"github.com/rbaylon/srvcman/utils"
	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

type settings struct {
	UpdateServer string `json:"update_server"`
	Product      string `json:"product"`
	TokenSet     bool   `json:"token_set"`
}

func settingsOf(u *updatesmodel.Updates) settings {
	p := u.Product
	if p == "" {
		p = updatesmodel.DefaultProduct
	}
	return settings{UpdateServer: u.UpdateServer, Product: p, TokenSet: u.ApiToken != ""}
}

// settingsIn is the body of PUT /: the token and product are optional so that
// a client that only knows the update server (older srvcmanui) keeps working.
type settingsIn struct {
	UpdateServer string  `json:"update_server"`
	Product      *string `json:"product"`
	ApiToken     *string `json:"api_token"`
	ClearToken   bool    `json:"clear_token"`
}

func (s *settingsIn) Bind(r *http.Request) error {
	s.UpdateServer = strings.TrimSpace(s.UpdateServer)
	if err := validate.MaxLen("update_server", s.UpdateServer, 2048); err != nil {
		return err
	}
	if err := updatesmodel.ValidateServer(s.UpdateServer); err != nil {
		return err
	}
	if s.Product != nil {
		p := strings.TrimSpace(*s.Product)
		if p != "" && !updatesmodel.ValidProduct(p) {
			return errors.New("product must be a lowercase name with letters, digits and hyphens")
		}
		s.Product = &p
	}
	if s.ApiToken != nil {
		t := strings.TrimSpace(*s.ApiToken)
		if t != "" && !updatesmodel.ValidToken(t) {
			return errors.New("api_token must be 8 to 128 letters, digits, _ or -")
		}
		s.ApiToken = &t
	}
	return nil
}

// mirrorBody is the body of the mirror calls.
type mirrorBody struct {
	Mirror  string `json:"mirror"`
	Default string `json:"default,omitempty"`
}

func (m *mirrorBody) Bind(r *http.Request) error {
	m.Mirror = strings.TrimSpace(m.Mirror)
	return validate.MaxLen("mirror", m.Mirror, 255)
}

func UpdatesRouter(db *gorm.DB) chi.Router {
	r := chi.NewRouter()
	r.Use(security.TokenRequired(db, "updates"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		u, err := updatesmodel.Get(db)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, settingsOf(u))
	})
	r.Put("/", func(w http.ResponseWriter, r *http.Request) {
		in := &settingsIn{}
		if err := render.Bind(r, in); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		u, err := updatesmodel.SaveSettings(db, in.UpdateServer, in.Product, in.ApiToken, in.ClearToken)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, settingsOf(u))
	})
	// Software updates from the update server. The check is quick (one request)
	// so it is answered directly; installing is a background job.
	r.Get("/software", func(w http.ResponseWriter, r *http.Request) {
		sw, err := updatesmodel.Default.CheckSoftware(r.Context(), updatesmodel.GormStore{DB: db})
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, sw)
	})
	r.Put("/software/install", func(w http.ResponseWriter, r *http.Request) {
		job, err := updatesmodel.Default.StartInstall(r.Context(), updatesmodel.GormStore{DB: db})
		if err != nil {
			// A refusal the operator can act on (nothing to install, no token, the
			// update server said no) is a 400 with its reason, which srvcmanui shows.
			render.Render(w, r, utils.ErrInvalidRequest(err, "Update refused", http.StatusBadRequest))
			return
		}
		render.JSON(w, r, job)
	})
	r.Get("/software/install", func(w http.ResponseWriter, r *http.Request) {
		render.JSON(w, r, updatesmodel.Default.JobStatus(updatesmodel.GormStore{DB: db}))
	})
	// The OpenBSD mirror the gateway downloads system updates from
	// (/etc/installurl). Changing it tests the new mirror first.
	r.Get("/mirror", func(w http.ResponseWriter, r *http.Request) {
		m, err := updatesmodel.GetMirror()
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Mirror error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, mirrorBody{Mirror: m, Default: updatesmodel.DefaultMirror})
	})
	r.Put("/mirror", func(w http.ResponseWriter, r *http.Request) {
		in := &mirrorBody{}
		if err := render.Bind(r, in); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		m, err := updatesmodel.SetMirror(in.Mirror)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Mirror not changed", http.StatusBadRequest))
			return
		}
		render.JSON(w, r, mirrorBody{Mirror: m, Default: updatesmodel.DefaultMirror})
	})
	// The check is started in the background and polled, not waited on:
	// syspatch fetches its patch list over the network and can take
	// minutes, far longer than a request should be held open.
	r.Put("/check", func(w http.ResponseWriter, r *http.Request) {
		// Plain 200 either way: srvcmanui's client treats only 200/201 as
		// success, and the state field already says what happened.
		state, _ := updatesmodel.StartCheck()
		render.JSON(w, r, state)
	})
	r.Get("/check", func(w http.ResponseWriter, r *http.Request) {
		render.JSON(w, r, updatesmodel.CheckStatus())
	})
	return r
}
