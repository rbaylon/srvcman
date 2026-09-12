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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	Acmd "github.com/rbaylon/arkgatecmd"
	"github.com/rbaylon/srvcman/database"
	Changemodel "github.com/rbaylon/srvcman/modules/changes/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

//var tokenAuth *jwtauth.JWTAuth

func ChangeRouter(db Changemodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "Changes"))

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
			data.Applied = "yes"
			changes[data.Module] += 1
			err := db.Update(&data)
			if err != nil {
				log.Printf("Error updating change: %d", data.ID)
				errors = append(errors, data)
			}
		}
		rundir := database.GetEnvVariable("RUN_DIR")
		pfcmd := Acmd.GetPFcmds(rundir)
		ifchange := false
		if changes["pfifaces"] > 0 {
			files, err := os.ReadDir("/tmp")
			if err != nil {
				log.Fatal(err)
			}
			foundif := false
			ifs := []string{}
			for _, file := range files {
				if strings.Contains(file.Name(), "hostname.") {
					foundif = true
					iface := strings.Split(file.Name(), ".")
					ifs = append(ifs, iface[1])
				}
			}
			nowunix := time.Time.Unix(time.Time{})
			if foundif {
				ifchange = true
				cmds := []Acmd.Arkcmd{}
				for _, v := range ifs {
					mvcmd := Acmd.PrepareCmd("Backup Interface: "+v, "cp", []string{"/etc/hostname." + v, "/etc/hostname." +
						v + "." + strconv.FormatInt(nowunix, 10)})
					cmds = append(cmds, *mvcmd)
					etcif := Acmd.PrepareCmd("Move iface to etc: "+v, "mv", []string{"/tmp/hostname." + v, "/etc/"})
					cmds = append(cmds, *etcif)
					ifcmd := Acmd.PrepareCmd("Interface restart: "+v, "sh", []string{"/etc/netstart", v})
					cmds = append(cmds, *ifcmd)
					for _, c := range cmds {
						err = c.SendCmd(utils.GetArkConn())
						if err != nil {
							log.Println("Failed:" + c.Name)
						}
						log.Println("Success: ", c.Name)
						time.Sleep(2 * time.Second)
					}
					clear(cmds)
					cmds = cmds[:0]
				}
				_, err := os.Stat("/tmp/mygate")
				if err == nil {
					gwbackup := Acmd.PrepareCmd("Backup mygate", "cp", []string{"/etc/mygate",
						"/etc/mygate." + strconv.FormatInt(nowunix, 10)})
					err = gwbackup.SendCmd(utils.GetArkConn())
					if err != nil {
						log.Println("mygate backup failed")
					}
					time.Sleep(1 * time.Second)
					tmptoetc := Acmd.PrepareCmd("/tmp/mygate to etc", "mv", []string{"/tmp/mygate", "/etc/"})
					err = tmptoetc.SendCmd(utils.GetArkConn())
					if err != nil {
						log.Println("mygate tmp to etc failed")
					} else {
						time.Sleep(1 * time.Second)
						gwcmd := Acmd.PrepareCmd("Gateway restart", "sh", []string{"/etc/netstart"})
						err = gwcmd.SendCmd(utils.GetArkConn())
						if err != nil {
							log.Println("Netstart failed!")
						}
						log.Println("Networking restarted")
						time.Sleep(3 * time.Second)
					}
				}
			}
		}
		if changes["subs"] > 0 || changes["firewall"] > 0 || ifchange {
			err := pfcmd["check"].SendCmd(utils.GetArkConn())
			if err != nil {
				exeerror := Changemodel.Change{}
				exeerror.Module = "subs"
				exeerror.Data = "error in " + rundir + "/pf.conf"
				errors = append(errors, exeerror)
			}
			pfcmd["backup"].SendCmd(utils.GetArkConn())
			pfcmd["move"].SendCmd(utils.GetArkConn())
			err = pfcmd["apply"].SendCmd(utils.GetArkConn())
			if err != nil {
				exeerror := Changemodel.Change{}
				exeerror.Module = "subs"
				exeerror.Data = "error applying " + rundir + "/pf.conf"
				errors = append(errors, exeerror)
				pfcmd["revert"].SendCmd(utils.GetArkConn())
			}
		}
		if changes["dhcpserver"] > 0 {
			dhcpconf := rundir + "dhcpd.conf"
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
		if changes["unbounddns"] > 0 {
			unboundconf := rundir + "unbound.conf"
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
			npppdconf := rundir + "npppd.conf"
			// npppd has no built-in config-validate mode (unlike pfctl -nf
			// or unbound-checkconf), so there's no "check" step here.
			npppdcmd := Acmd.GetServiceCmds("npppd", "", "/etc/npppd/", npppdconf)
			npppdrc := Acmd.GetRcCmds("npppd")
			npppdcmd["backup"].SendCmd(utils.GetArkConn())
			npppdcmd["stage"].SendCmd(utils.GetArkConn())
			err := npppdrc["restart"].SendCmd(utils.GetArkConn())
			if err != nil {
				exeerror := Changemodel.Change{}
				exeerror.Module = "pppoes"
				exeerror.Data = "error applying " + npppdconf
				errors = append(errors, exeerror)
				npppdcmd["revert"].SendCmd(utils.GetArkConn())
			}
		}
		//utils.GetArkConn()
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
