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
		res, errdb := db.GetApplied(0)
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		errors := []Changemodel.Change{}
		changes := make(map[string]int)
		for _, data := range res {
			data.Applied = true
			changes[data.Module] += 1
			err := db.Update(&data)
			if err != nil {
				log.Printf("Error updating change: %d", data.ID)
				errors = append(errors, data)
			}
		}
		usocket := database.GetEnvVariable("UNIX_SOCKET")
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
			if foundif {
				ifchange = true
				cmds := []Acmd.Arkcmd{}
				nowunix := time.Time.Unix(time.Time{})
				for _, v := range ifs {
					mvcmd := Acmd.PrepareCmd("Backup Interface: "+v, "mv", []string{"/etc/hostname." + v, "/etc/hostname." +
						v + "." + strconv.FormatInt(nowunix, 10)})
					cmds = append(cmds, *mvcmd)
					etcif := Acmd.PrepareCmd("Move iface to etc: "+v, "mv", []string{"/tmp/hostname." + v, "/etc/"})
					cmds = append(cmds, *etcif)
					ifcmd := Acmd.PrepareCmd("Interface restart: "+v, "sh", []string{"/etc/netstart", v})
					cmds = append(cmds, *ifcmd)
					for _, c := range cmds {
						err = c.SendCmd(utils.GetUnixConn(usocket))
						if err != nil {
							log.Println("Failed" + c.Name)
						}
						log.Println(c.Name)
					}
				}
				_, err := os.Stat("/tmp/mygate")
				if err == nil {
					gwbackup := Acmd.PrepareCmd("Backup mygate", "mv", []string{"/etc/mygate",
						"/etc/mygate." + strconv.FormatInt(nowunix, 10)})
					err = gwbackup.SendCmd(utils.GetUnixConn(usocket))
					if err != nil {
						log.Println("mygate backup failed")
					}
					gwcmd := Acmd.PrepareCmd("Gateway restart", "sh", []string{"/etc/netstart"})
					err = gwcmd.SendCmd(utils.GetUnixConn(usocket))
					if err != nil {
						log.Println("Netstart failed!")
					}
					log.Println("Networking restarted")
				}
			}
		}
		if changes["subs"] > 0 || changes["firewall"] > 0 || ifchange {
			err := pfcmd["check"].SendCmd(utils.GetUnixConn(usocket))
			if err != nil {
				exeerror := Changemodel.Change{}
				exeerror.Module = "subs"
				exeerror.Data = "error in " + rundir + "/pf.conf"
				errors = append(errors, exeerror)
			}
			pfcmd["backup"].SendCmd(utils.GetUnixConn(usocket))
			pfcmd["move"].SendCmd(utils.GetUnixConn(usocket))
			err = pfcmd["apply"].SendCmd(utils.GetUnixConn(usocket))
			if err != nil {
				exeerror := Changemodel.Change{}
				exeerror.Module = "subs"
				exeerror.Data = "error applying " + rundir + "/pf.conf"
				errors = append(errors, exeerror)
				pfcmd["revert"].SendCmd(utils.GetUnixConn(usocket))
			}
		}
		//utils.GetUnixConn(usocket)
		render.JSON(w, r, errors)
	})
	r.Get("/pending", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetApplied(0)
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
