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
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	"github.com/rbaylon/srvcman/modules/security"
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
	r.Get("/value/{voucher}", func(w http.ResponseWriter, r *http.Request) {
		val := chi.URLParam(r, "voucher")
		voucher, err := db.GetByVoucherValue(val)
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
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
