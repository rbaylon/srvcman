// Package paymentroutes - Arkgate API Payment module
//
//	Module Routes:
//	  /api/v1/payments
//	    Method: GET
//	    Headers: Authorization Bearer
//	    Return: JSON object with list of payment objects
//	    Return-Status: 200 on Success
//	                   500 on Error
//
//	  /api/v1/payments/<paymentId>
//	    Method: GET|PUT|DELETE
//	    Headers: Authorization Bearer
//	    Return: JSON payment object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
//
//	  /api/v1/payments/create
//	    Method: POST
//	    Headers: Authorization Bearer
//	    Return: JSON payment object ( exept for delete method )
//	    Return-Status: 200 on Success
//	                   500 on Error
//	                   400 on Bad request
package paymentroutes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/go-chi/render"
	paymentmodel "github.com/rbaylon/srvcman/modules/payments/model"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
)

var tokenAuth *jwtauth.JWTAuth

func PaymentRouter(db paymentmodel.Crud) chi.Router {
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
	r.Get("/{paymentId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "paymentId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid payment ID %s", chi.URLParam(r, "paymentId")), http.StatusBadRequest))
			return
		}
		payment, err := db.GetById(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, payment)
	})
	r.Get("/sub/{paymentId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "paymentId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid payment ID %s", chi.URLParam(r, "paymentId")), http.StatusBadRequest))
			return
		}
		payments, err := db.GetBySubId(uint(id))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
			return
		}
		render.JSON(w, r, payments)
	})
	r.Put("/{paymentId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "paymentId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid payment ID %s", chi.URLParam(r, "paymentId")), http.StatusBadRequest))
			return
		}
		payment := paymentmodel.Payment{}
		if err = render.Bind(r, &payment); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		payment.ID = uint(id)
		err = db.Update(&payment)
		if err == nil {
			render.JSON(w, r, payment)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error updating record for payment  ID %s", chi.URLParam(r, "paymentId")), http.StatusBadRequest))
	})
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		payment := paymentmodel.Payment{}
		if err := render.Bind(r, &payment); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		err := db.Add(&payment)
		if err == nil {
			render.JSON(w, r, payment)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, "DB error", http.StatusInternalServerError))
	})
	r.Delete("/{paymentId}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "paymentId"))
		if err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Invalid payment ID %s", chi.URLParam(r, "paymentId")), http.StatusBadRequest))
			return
		}
		payment := paymentmodel.Payment{}
		if err = render.Bind(r, &payment); err != nil {
			render.Render(w, r, utils.ErrInvalidRequest(err, "Bind error", http.StatusBadRequest))
			return
		}
		payment.ID = uint(id)
		err = db.Delete(&payment)
		if err == nil {
			render.JSON(w, r, payment)
			return
		}
		render.Render(w, r, utils.ErrInvalidRequest(err, fmt.Sprintf("Error deleting record for payment  ID %s", chi.URLParam(r, "paymentId")), http.StatusBadRequest))
	})
	return r
}
