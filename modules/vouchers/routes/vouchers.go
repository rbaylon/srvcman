// Package voucherroutes - Arkgate API Voucher module
//
//	Module Routes:
//	  /api/v1/vouchers
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of voucher objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/vouchers/<voucherId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON voucher object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/vouchers/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON voucher object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package voucherroutes

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	pfconfigmodel "github.com/rbaylon/srvcman/modules/pfconfig/model"
	"github.com/rbaylon/srvcman/modules/security"
	submodel "github.com/rbaylon/srvcman/modules/subs/model"
	vouchermodel "github.com/rbaylon/srvcman/modules/vouchers/model"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func VoucherRouter(db vouchermodel.Crud) chi.Router {
	r := chi.NewRouter()
	r.Use(security.TokenRequired)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		res, errdb := db.GetAll()
		if errdb != nil {
			render.Render(w, r, utils.ErrInvalidRequest(errdb, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, res)
	})
	r.Get("/{voucherId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "voucherId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid voucher ID %s", chi.URLParam(r, "voucherId")), http.StatusBadRequest))
			return
		}
		voucher, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, voucher)
	})
	r.Get("/value/{voucher}/{ip}/{routerid}", func(w http.ResponseWriter, r *http.Request) {
		val, uerr := url.QueryUnescape(chi.URLParam(r, "voucher"))
		if uerr != nil {
			log.Println("Voucher or sub invalid:", uerr)
			render.Render(w, r, utils.ErrInvalidRequest(uerr, "Invalid voucher", http.StatusNotFound))
		}
		voucher, err := db.GetByVoucherValue(val)
		dbconn := db.GetDB()
		subsdb := submodel.New(dbconn)
		if err != nil {
			log.Println("Voucher not found, checking subs")
			sbinfo, err := subsdb.GetByMacPrefix(val)
			if err != nil {
				log.Println("Voucher or sub not found")
				render.Render(w, r, utils.ErrInvalidRequest(err, "DB get error", http.StatusNotFound))
				return
			}
			sbinfo.Status = "active"
			err = subsdb.Update(sbinfo)
			if err != nil {
				render.Render(w, r, utils.ErrInvalidRequest(err, "DB subs update error", http.StatusNotFound))
				return
			}
			w.WriteHeader(http.StatusAccepted)
			render.JSON(w, r, sbinfo)
			return
		}
		log.Println(val)
		pfdb := pfconfigmodel.New(dbconn)
		pfc, err := pfdb.GetByRouterId(chi.URLParam(r, "routerid"))
		if err != nil {
			log.Println("Router: ", err)
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB get error: pfconfig ", http.StatusInternalServerError))
			return
		}
		subip, ierr := url.QueryUnescape(chi.URLParam(r, "ip"))
		if ierr != nil {
			log.Println("Voucher or sub invalid:", ierr)
			render.Render(w, r, utils.ErrInvalidRequest(ierr, "Invalid ip", http.StatusNotFound))
		}
		if voucher.Status == "available" && pfc != nil {
			voucher.Status = "active"
			voucher.DateStarted = time.Now()
			voucher.DateEnd = voucher.DateStarted.Add(time.Hour * time.Duration(voucher.Hours))
			voucher.DateExpires = voucher.DateEnd.Add(time.Hour * 1)
			voucher.Ip = subip
			voucher.PfconfigID = pfc.ID
			err = db.Update(voucher)
			if err != nil {
				render.Render(w, r, utils.ErrInvalidRequest(err, "DB update error", http.StatusInternalServerError))
				return
			}
			w.WriteHeader(http.StatusCreated)
		}
		if voucher.Status == "active" && voucher.Ip != subip {
			voucher.Ip = subip
			err = db.Update(voucher)
			if err != nil {
				render.Render(w, r, utils.ErrInvalidRequest(err, "DB update error", http.StatusInternalServerError))
				return
			}
			w.WriteHeader(http.StatusAccepted)
		}
		render.JSON(w, r, voucher)
	})
	r.Put("/{voucherId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "voucherId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid voucher ID %s", chi.URLParam(r, "voucherId")), http.StatusBadRequest))
			return
		}
		voucher := vouchermodel.Voucher{}
		if err = render.Bind(r, &voucher); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		voucher.ID = uint(id)
		err = db.Update(&voucher)
		if err == nil {
			render.JSON(w, r, voucher)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for voucher  ID %s", chi.URLParam(r, "voucherId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		voucher := vouchermodel.Voucher{}
		if err := render.Bind(r, &voucher); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(&voucher)
		if err == nil {
			render.JSON(w, r, voucher)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Post("/createBulk", func(w http.ResponseWriter, r *http.Request) {
		voucher := vouchermodel.Vouchers{}
		if err := render.Bind(r, &voucher); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.AddBulk(voucher.Vouchers)
		if err == nil {
			render.JSON(w, r, voucher)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{voucherId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "voucherId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid voucher ID %s", chi.URLParam(r, "voucherId")), http.StatusBadRequest))
			return
		}
		voucher := vouchermodel.Voucher{}
		if err = render.Bind(r, &voucher); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		voucher.ID = uint(id)
		err = db.Delete(&voucher)
		if err == nil {
			render.JSON(w, r, voucher)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for voucher  ID %s", chi.URLParam(r, "voucherId")), http.StatusBadRequest))
	})
	return r
}
