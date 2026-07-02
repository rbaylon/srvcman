// Package utils - Utilities to support SANG API
package utils

import (
	"log"
	"net"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/render"
	vouchermodel "github.com/rbaylon/srvcman/modules/vouchers/model"
)

type ErrResponse struct {
	Err            error `json:"-"` // low-level runtime error
	HTTPStatusCode int   `json:"-"` // http response status code

	StatusText string `json:"status"`          // user-level status message
	AppCode    int64  `json:"code,omitempty"`  // application-specific error code
	ErrorText  string `json:"error,omitempty"` // application-level error message, for debugging
}

func (e *ErrResponse) Render(w http.ResponseWriter, r *http.Request) error {
	render.Status(r, e.HTTPStatusCode)
	return nil
}

func ErrInvalidRequest(err error, msg string, statuscode int) render.Renderer {
	return &ErrResponse{
		Err:            err,
		HTTPStatusCode: statuscode,
		StatusText:     msg,
		ErrorText:      err.Error(),
	}
}

func CheckExpired(db vouchermodel.Crud) {
	for {
		log.Println("Checking for expired vouchers")
		err := db.DisableExpired()
		if err != nil {
			log.Println("Error during voucher expired check")
		}
		time.Sleep(5 * time.Minute)
	}
}

func GetUnixConn(socketpath string) net.Conn {
	c, err := net.Dial("unix", socketpath)
	if err != nil {
		log.Println("Dial error ", err)
	}
	return c
}

var alnumRegex = regexp.MustCompile(`^[A-Za-z0-9]+$`)

func IsAlnumStrict(s string) bool {
	return alnumRegex.MatchString(s)
}
