package api

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func proxyPodWindow(w http.ResponseWriter, r *http.Request, target, ns, name string, port int64) {
	u, err := url.Parse(target)
	if err != nil {
		fail(w, 500, err)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, e error) {
		fail(rw, 502, e)
	}
	prefix := fmt.Sprintf("/api/v1/pods/%s/%s/window/%d/", ns, name, port)
	r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}
	proxy.ServeHTTP(w, r)
}
