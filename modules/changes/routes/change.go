// Package Changeroutes - Arkgate API Change module
//
//	Module Routes:
//	  /api/v1/changes
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of Change objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/changes/<ChangeId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON Change object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/changes/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON Change object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package changeroutes

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	Acmd "github.com/rbaylon/arkgatecmd"
	changeapply "github.com/rbaylon/srvcman/modules/changes/apply"
	Changemodel "github.com/rbaylon/srvcman/modules/changes/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

//var tokenAuth *jwtauth.JWTAuth

func ChangeRouter(db Changemodel.Crud, pppoeRecords func() (int, error)) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "changes"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Put("/pending", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetApplied("pending")
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		errors := []Changemodel.Change{}
		changes := make(map[string]int)
		for _, data := range res {
			changes[data.Module] += 1
		}
		pfcmd := Acmd.GetPFcmds("/tmp/")
		ifchange := false
		if changes["pfifaces"] > 0 || changes["pflows"] > 0 {
			// arkgated generates its own hostname.<if>/mygate files (from
			// live data it fetches from this API, see
			// config/pf.ConfigCreate) and does the actual backup/cp/mv into
			// /etc plus the netstart itself - it no longer depends on
			// srvcman having written those files locally first, since
			// srvcman and arkgated may run on separate hosts. Only files
			// whose content actually changed get promoted/restarted.
			//
			// pflows go through the same path: ConfigCreate renders a
			// hostname.<if> for each pflow device (flowsrc/flowdst/
			// pflowproto) exactly as it does for a regular interface, so a
			// pflow edit needs this apply step or it never reaches /etc.
			ifchange = true
			applycmd := Acmd.PrepareCmd("ApplyIfaces", "true", []string{})
			if err := applycmd.SendCmd(utils.GetArkConn()); err != nil {
				log.Println("Failed to apply interface changes:", err)
				ifchange = false
			}
		}
		if changes["subs"] > 0 || changes["firewall"] > 0 || ifchange {
			err := pfcmd["check"].SendCmd(utils.GetArkConn())
			if err != nil {
				// a rule set that fails its syntax check is never
				// installed; the running /etc/pf.conf stays as it was
				exeerror := Changemodel.Change{}
				exeerror.Module = "subs"
				exeerror.Data = "error in /tmp/pf.conf"
				errors = append(errors, exeerror)
			} else {
				pfcmd["backup"].SendCmd(utils.GetArkConn())
				pfcmd["move"].SendCmd(utils.GetArkConn())
				err = pfcmd["apply"].SendCmd(utils.GetArkConn())
				if err != nil {
					exeerror := Changemodel.Change{}
					exeerror.Module = "subs"
					exeerror.Data = "error applying /tmp/pf.conf"
					errors = append(errors, exeerror)
					pfcmd["revert"].SendCmd(utils.GetArkConn())
				}
			}
		}
		if changes["dhcpserver"] > 0 {
			dhcpconf := "/tmp/dhcpd.conf"
			dhcpcmd := Acmd.GetServiceCmds("dhcpd", "/usr/sbin/dhcpd", "/etc/", dhcpconf)
			dhcprc := Acmd.GetRcCmds("dhcpd")
			err := dhcpcmd["check"].SendCmd(utils.GetArkConn())
			if err != nil {
				exeerror := Changemodel.Change{}
				exeerror.Module = "dhcpserver"
				exeerror.Data = "error in " + dhcpconf
				errors = append(errors, exeerror)
			}
			dhcpcmd["backup"].SendCmd(utils.GetArkConn())
			dhcpcmd["stage"].SendCmd(utils.GetArkConn())
			err = dhcprc["restart"].SendCmd(utils.GetArkConn())
			if err != nil {
				exeerror := Changemodel.Change{}
				exeerror.Module = "dhcpserver"
				exeerror.Data = "error applying " + dhcpconf
				errors = append(errors, exeerror)
				dhcpcmd["revert"].SendCmd(utils.GetArkConn())
			}
		}
		if changes["snmpserver"] > 0 {
			// snmpd takes -nf for a config check, the same shape as dhcpd,
			// so GetServiceCmds works unmodified. Its "check" Arkcmd is
			// named CheckConf_snmpd, which is what arkgated intercepts to
			// render /tmp/snmpd.conf before validating it.
			snmpconf := "/tmp/snmpd.conf"
			snmpcmd := Acmd.GetServiceCmds("snmpd", "/usr/sbin/snmpd", "/etc/", snmpconf)
			snmprc := Acmd.GetRcCmds("snmpd")
			err := snmpcmd["check"].SendCmd(utils.GetArkConn())
			if err != nil {
				exeerror := Changemodel.Change{}
				exeerror.Module = "snmpserver"
				exeerror.Data = "error in " + snmpconf
				errors = append(errors, exeerror)
			}
			snmpcmd["backup"].SendCmd(utils.GetArkConn())
			snmpcmd["stage"].SendCmd(utils.GetArkConn())
			err = snmprc["restart"].SendCmd(utils.GetArkConn())
			if err != nil {
				exeerror := Changemodel.Change{}
				exeerror.Module = "snmpserver"
				exeerror.Data = "error applying " + snmpconf
				errors = append(errors, exeerror)
				snmpcmd["revert"].SendCmd(utils.GetArkConn())
			}
		}
		if changes["unbounddns"] > 0 {
			unboundconf := "/tmp/unbound.conf"
			unboundcmd := Acmd.GetServiceCmds("unbound", "/usr/sbin/unbound-checkconf", "/var/unbound/etc/", unboundconf)
			unboundrc := Acmd.GetRcCmds("unbound")
			err := unboundcmd["check_unbound"].SendCmd(utils.GetArkConn())
			if err != nil {
				exeerror := Changemodel.Change{}
				exeerror.Module = "unbounddns"
				exeerror.Data = "error in " + unboundconf
				errors = append(errors, exeerror)
			}
			unboundcmd["backup"].SendCmd(utils.GetArkConn())
			unboundcmd["stage"].SendCmd(utils.GetArkConn())
			err = unboundrc["restart"].SendCmd(utils.GetArkConn())
			if err != nil {
				exeerror := Changemodel.Change{}
				exeerror.Module = "unbounddns"
				exeerror.Data = "error applying " + unboundconf
				errors = append(errors, exeerror)
				unboundcmd["revert"].SendCmd(utils.GetArkConn())
			}
		}
		if changes["pppoes"] > 0 {
			errors = append(errors, changeapply.Npppd(changeapply.Calls{
				Send: func(c *Acmd.Arkcmd) error { return c.SendCmd(utils.GetArkConn()) },
				Output: func(c *Acmd.Arkcmd) (string, error) {
					return c.SendCmdOutput(utils.GetArkConn())
				},
				Records: pppoeRecords,
			})...)
		}
		// A change is marked applied only once its step succeeded. One whose step
		// failed stays pending, so the operator still sees it and a later apply
		// (after the cause is fixed) retries it.
		failed := changeapply.Failed(errors)
		for _, data := range res {
			if changeapply.Held(data.Module, failed) {
				continue
			}
			data.Applied = "yes"
			if err := db.Update(&data); err != nil {
				log.Printf("Error updating change: %d", data.ID)
				errors = append(errors, data)
			}
		}
		render.JSON(w, r, errors)
	})
	r.Get("/pending", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetApplied("pending")
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/{ChangeId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "ChangeId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid Change ID %s", chi.URLParam(r, "ChangeId")), http.StatusBadRequest))
			return
		}
		Change, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, Change)
	})
	r.Get("/sub/{ChangeId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "ChangeId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid Change ID %s", chi.URLParam(r, "ChangeId")), http.StatusBadRequest))
			return
		}
		Changes, err := db.GetBySubId(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, Changes)
	})
	r.Put("/{ChangeId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "ChangeId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid Change ID %s", chi.URLParam(r, "ChangeId")), http.StatusBadRequest))
			return
		}
		Change := Changemodel.Change{}
		if err = render.Bind(r, &Change); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		Change.ID = uint(id)
		err = db.Update(&Change)
		if err == nil {
			render.JSON(w, r, Change)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for Change  ID %s", chi.URLParam(r, "ChangeId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		Change := Changemodel.Change{}
		if err := render.Bind(r, &Change); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(&Change)
		if err == nil {
			render.JSON(w, r, Change)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{ChangeId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "ChangeId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid Change ID %s", chi.URLParam(r, "ChangeId")), http.StatusBadRequest))
			return
		}
		Change := Changemodel.Change{}
		if err = render.Bind(r, &Change); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		Change.ID = uint(id)
		err = db.Delete(&Change)
		if err == nil {
			render.JSON(w, r, Change)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for Change  ID %s", chi.URLParam(r, "ChangeId")), http.StatusBadRequest))
	})
	return r
}
