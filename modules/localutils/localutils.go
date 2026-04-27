package localutils

import (
	b64 "encoding/base64"
	"os"
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
