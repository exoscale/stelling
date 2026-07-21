package api

import "net/http"

func RPCMapper(req *http.Request) string {
	return req.Method + " " + req.URL.Path
}
