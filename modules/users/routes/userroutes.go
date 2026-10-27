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
		olduser, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error finding record for user ID %s", chi.URLParam(r, "userId")), http.StatusBadRequest))
			return
		}
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
		// No render.Bind here, unlike the other modules': a DELETE carries no
		// body worth validating, and Bind requires a username - which the empty body
		// does not have, so binding it would reject every delete.
		user := usermodel.User{}
		user.ID = uint(id)
		// Create adds a group of the user's own name and two default profile
		// policies; they go with the user instead of lingering unowned.
		var username string
		if old, gerr := db.GetById(uint(id)); gerr == nil {
			username = old.Username
		}
		err = db.Delete(&user)
		if err == nil {
			if username != "" {
				dbconn := db.GetDB()
				poldb := policymodel.New(dbconn)
				pols := []policymodel.Policy{}
				dbconn.Where("name IN ?", []string{"DefaultProfile" + username + "RO", "DefaultProfile" + username + "RW"}).Find(&pols)
				for i := range pols {
					poldb.RemoveGroups(&pols[i])
					poldb.Delete(&pols[i])
				}
				grp := groupmodel.Group{}
				if dbconn.Where("groupname = ?", username).First(&grp).Error == nil {
					dbconn.Exec("DELETE FROM policy_groups WHERE group_id = ?", grp.ID)
					groupmodel.New(dbconn).Delete(&grp)
				}
			}
			render.JSON(w, r, user)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for user  ID %s", chi.URLParam(r, "userId")), http.StatusBadRequest))
	})
	return r
}
