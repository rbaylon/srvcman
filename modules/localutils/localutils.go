package localutils

import (
	b64 "encoding/base64"
	"errors"
	"log"
	"net"
	"os"

	"github.com/rbaylon/srvcman/database"
)

func B64StdEncode(s string) string {
	return b64.StdEncoding.EncodeToString([]byte(s))
}
func B64StdDecode(s string) (string, error) {
	res, err := b64.StdEncoding.DecodeString(s)
	return string(res), err
}
func B64UrlEncode(s string) string {
	return b64.URLEncoding.EncodeToString([]byte(s))
}
func B64UrlDecode(s string) (string, error) {
	res, err := b64.URLEncoding.DecodeString(s)
	return string(res), err
}

func SendCmd(s string) error {
	servsocket := database.GetEnvVariable("SRV_SOCKET")
	c, err := net.Dial("unix", servsocket)
	if err != nil {
		log.Fatal("Dial error", err)
	}
	defer c.Close()

	_, err = c.Write([]byte(s))
	if err != nil {
		return err
	}
	buf := make([]byte, 8)
	n, err := c.Read(buf[:])
	if err != nil {
		return err
	}
	ret := string(buf[0:n])
	if ret != "OK" {
		return errors.New(ret)
	}
	return nil
}

func GenerateConfigFile(path string, content []string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, s := range content {
		_, err = file.WriteString(s)
		if err != nil {
			return err
		}
	}
	return nil
}
