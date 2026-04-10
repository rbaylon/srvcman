// Package utils - Utilities to support SANG API
package utils

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
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

type Arkcmd struct {
	Name string   `json:"name"`
	Cmd  string   `json:"cmd"`
	Opts []string `json:"opts"`
}

type Arkcmds struct {
	Cmds []Arkcmd `json:"cmds"`
}

type Cmd interface {
	Run() (int, error)
	RunWithOutput() (int, []byte)
}

func (ac *Arkcmd) Run() (int, error) {
	cmd := exec.Command(ac.Cmd, ac.Opts...)
	out, err := cmd.Output()
	if err != nil {
		log.Println(string(out))
		return 1, err
	}
	log.Println(string(out))
	return 0, nil
}

func (ac *Arkcmd) RunWithOutput() (int, []byte) {
	log.Println("running:", ac.Cmd)
	cmd := exec.Command(ac.Cmd, ac.Opts...)
	out, err := cmd.Output()
	if err != nil {
		log.Println(string(out))
		return 1, nil
	}
	log.Println(string(out))
	return 0, out
}

func Init(cmdfile string) map[string]Cmd {
	cmds := map[string]Cmd{}
	log.Println("Arkcmd file loaded: ", cmdfile)
	jsoncmdFile, err := os.Open(cmdfile)
	if err != nil {
		log.Println("Error during json open file: ", err)
	}
	defer jsoncmdFile.Close()
	byteValue, err := io.ReadAll(jsoncmdFile)
	if err != nil {
		log.Println("Error during reading json content: ", err)
	}
	var acmds Arkcmds
	err = json.Unmarshal(byteValue, &acmds)
	if err != nil {
		log.Println("Error during unmarshal: ", err)
	}
	for i := 0; i < len(acmds.Cmds); i++ {
		log.Println("json acmd:", acmds.Cmds[i].Name)
		cmds[acmds.Cmds[i].Name] = &acmds.Cmds[i]
	}
	return cmds
}

func GetUnixConn(socketpath string) net.Conn {
	c, err := net.Dial("unix", socketpath)
	if err != nil {
		log.Println("Dial error ", err)
	}
	return c
}
