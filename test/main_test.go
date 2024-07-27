package main

import (
	"fmt"
	"net/http"
	"testing"
)

func doHttp(url string, method string, payload string, headers map[string]string) *http.Response {
	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		fmt.Println(err)
	}
	_, exist := headers["Authorization"]
	if exist {
		req.Header.Add("Authorization", headers["Authorization"])
	}
	resp, err := client.Do(req)
	return resp
}

func TestGeneric(t *testing.T) {
	// Defining the columns of the table
	var tests = []struct {
		name    string
		path    string
		method  string
		payload string
		headers map[string]string
		want    int
	}{
		// the table itself
		{"Base path", "/", "GET", "", map[string]string{}, 200},
		{"Non-Existent path", "/none", "GET", "", map[string]string{}, 404},
		{"Login empty creds", "/api/v1/login", "GET", "", map[string]string{}, 401},
		{"Login bad credentials", "/api/v1/login", "GET", "", map[string]string{}, 401},
		{"Login good credentials", "/api/v1/login", "GET", "", map[string]string{"Authorization": "Basic YWRtaW46bG9uZ2xvbmc="}, 200},
	}
	// The execution loop
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var baseUrl string = "http://127.0.0.1:3333"
			res := doHttp(baseUrl+tt.path, tt.method, tt.payload, tt.headers)
			if res.StatusCode != tt.want {
				t.Errorf("got %d, want %d", res.StatusCode, tt.want)
			}
		})
	}
}
