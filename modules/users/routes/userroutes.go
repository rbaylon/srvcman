// Package userroutes - Arkgate API User module
//
//	Module Routes:
//	  /api/v1/users
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of user objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/users/<userId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON user object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/users/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON user object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package userroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	groupmodel "github.com/rbaylon/srvcman/modules/groups/model"
	policymodel "github.com/rbaylon/srvcman/modules/policies/model"
	"github.com/rbaylon/srvcman/modules/security"
	usermodel "github.com/rbaylon/srvcman/modules/users/model"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func UserRouter(db usermodel.Crud) chi.Router {
	r := chi.NewRouter()
	dbconn := db.GetDB()
	r.Use(security.TokenRequired(dbconn, "users"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/{userId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "userId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid user ID %s", chi.URLParam(r, "userId")), http.StatusBadRequest))
			return
		}
		user, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, user)
	})
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/username/{uname}", func(w http.ResponseWriter, r *http.Request) {
		u := chi.URLParam(r, "uname")
		user, err := db.GetByUsername(u)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, *user)
	})
	r.Put("/{userId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "userId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid user ID %s", chi.URLParam(r, "userId")), http.StatusBadRequest))
			return
		}
		user := usermodel.User{}
		if err = render.Bind(r, &user); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		olduser, _ := db.GetById(uint(id))
		db.RemoveGroups(olduser)
		user.ID = uint(id)
		err = db.Update(&user)
		if err == nil {
			render.JSON(w, r, user)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for user  ID %s", chi.URLParam(r, "userId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		user := usermodel.User{}
		if err := render.Bind(r, &user); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(&user)
		if err == nil {
			dbconn := db.GetDB()
			grpdb := groupmodel.New(dbconn)
			group := groupmodel.Group{}
			group.Groupname = user.Username
			err = grpdb.Add(&group)
			if err == nil {
				user.Groups = append(user.Groups, group)
				db.Update(&user)
				poldb := policymodel.New(dbconn)
				ropolicy := policymodel.Policy{
					Name:     "DefaultProfile" + user.Username + "RO",
					Type:     "users",
					Resource: "/api/v1/users/username/" + user.Username,
					Action:   "RO",
					Groups:   []groupmodel.Group{group},
				}
				err = poldb.Add(&ropolicy)
				if err != nil {
					fmt.Println("Error adding default user policy for user", user.Username)
				}
				rwpolicy := policymodel.Policy{
					Name:     "DefaultProfile" + user.Username + "RW",
					Type:     "users",
					Resource: "/api/v1/users/" + strconv.FormatUint(uint64(user.ID), 10),
					Action:   "Rw",
					Groups:   []groupmodel.Group{group},
				}
				err = poldb.Add(&rwpolicy)
				if err != nil {
					fmt.Println("Error adding default user policy for user", user.Username)
				}
				defpols, err := policymodel.GetDefaultPolicies(poldb.DB)
				if err != nil {
					fmt.Println("Error getting default policies")
				}
				for _, p := range defpols {
					p.Groups = append(p.Groups, group)
					err = poldb.Update(&p)
					if err != nil {
						fmt.Println("Error adding group", group.Groupname, "to", p.Name)
					}
				}
				render.JSON(w, r, user)
			}
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{userId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "userId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid user ID %s", chi.URLParam(r, "userId")), http.StatusBadRequest))
			return
		}
		user := usermodel.User{}
		if err = render.Bind(r, &user); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		user.ID = uint(id)
		err = db.Delete(&user)
		if err == nil {
			render.JSON(w, r, user)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for user  ID %s", chi.URLParam(r, "userId")), http.StatusBadRequest))
	})
	return r
}
