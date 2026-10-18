// Package utils - Utilities to support SANG API
package utils

import (
	"crypto/tls"
	"crypto/x509"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/go-chi/render"
	"github.com/rbaylon/srvcman/database"
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

// GetArkConn connects to arkgated's IPC listener, preferring the local Unix
// domain socket when ARKGATED_SOCKET is set (srvcman and arkgated running on
// the same host - no PKI material needed, trust is filesystem permissions)
// and falling back to mutual TLS over the network otherwise, which is what
// lets arkgated run on a separate host. ARKGATED_TLS_CERT/ARKGATED_TLS_KEY
// are srvcman's own client identity (verified by arkgated's ClientCAs);
// ARKGATED_TLS_CA verifies arkgated's server cert in return. All of these,
// plus ARKGATED_ADDR (host:port), are expected to be provisioned by the
// deployment's own PKI tooling - this only ever reads them.
func GetArkConn() net.Conn {
	if sockPath := database.GetEnvVariable("ARKGATED_SOCKET"); sockPath != "" {
		c, err := net.Dial("unix", sockPath)
		if err == nil {
			return c
		}
		log.Println("ArkConn: unix socket dial error, falling back to mTLS:", err)
	}

	addr := database.GetEnvVariable("ARKGATED_ADDR")
	certfile := database.GetEnvVariable("ARKGATED_TLS_CERT")
	keyfile := database.GetEnvVariable("ARKGATED_TLS_KEY")
	cafile := database.GetEnvVariable("ARKGATED_TLS_CA")

	cert, err := tls.LoadX509KeyPair(certfile, keyfile)
	if err != nil {
		log.Println("ArkConn: loading client cert/key error ", err)
		return nil
	}
	caPEM, err := os.ReadFile(cafile)
	if err != nil {
		log.Println("ArkConn: reading server CA cert error ", err)
		return nil
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		log.Println("ArkConn: no certificates found in ", cafile)
		return nil
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS12,
	}
	c, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		log.Println("ArkConn: dial error ", err)
		return nil
	}
	return c
}

var alnumRegex = regexp.MustCompile(`^[A-Za-z0-9]+$`)

func IsAlnumStrict(s string) bool {
	return alnumRegex.MatchString(s)
}

func MaskToCidr(maskString string) int {
	ip := net.ParseIP(maskString)
	if ip == nil {
		log.Fatalf("Invalid IP address: %s", maskString)
	}

	ip4 := ip.To4()
	if ip4 == nil {
		log.Fatalf("IP address is not IPv4: %s", maskString)
	}
	mask := net.IPMask(ip4)
	prefixSize, _ := mask.Size()
	return prefixSize
}

func BroadcastAddr(n *net.IPNet) net.IP {
	ip := n.IP
	mask := n.Mask
	broadcast := make(net.IP, len(ip))
	for i := range ip {
		// For IPv4, ^mask[i] does the bitwise NOT.
		// For IPv6, the calculation is more complex and usually not done this way.
		broadcast[i] = ip[i] | ^mask[i]
	}
	return broadcast
}
