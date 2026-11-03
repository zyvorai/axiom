package api

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// proxyPodWindow serves the in-origin pod window route
// (/api/v1/pods/{ns}/{name}/window/{port}/…). It is only reachable when the
// separate pod-proxy origin is not configured; see podproxy_origin.go.
func proxyPodWindow(w http.ResponseWriter, r *http.Request, target, ns, name string, port int64) {
	prefix := fmt.Sprintf("/api/v1/pods/%s/%s/window/%d/", ns, name, port)
	// The pod's content is served from Axiom's own origin, so treat it as
	// hostile: sandbox it into an opaque origin (it cannot reach the console's
	// cookies or same-origin API), and never let it set cookies on our origin.
	servePodProxy(w, r, target, strings.TrimPrefix(r.URL.Path, prefix), func(h http.Header) {
		h.Set("Content-Security-Policy", "sandbox allow-scripts allow-forms allow-popups")
		h.Set("X-Content-Type-Options", "nosniff")
	})
}

// servePodProxy is the proxy core shared by the in-origin window route and the
// separate pod-proxy origin: it forwards r to target with upstreamPath as the
// path, after removing the caller's Axiom credentials, drops every Set-Cookie
// from the pod's response and then lets harden set the response policy
// headers. WebSocket upgrades are passed through by httputil.ReverseProxy.
func servePodProxy(w http.ResponseWriter, r *http.Request, target, upstreamPath string, harden func(http.Header)) {
	u, err := url.Parse(target)
	if err != nil {
		fail(w, 500, err)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, e error) {
		fail(rw, 502, e)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Set-Cookie")
		harden(resp.Header)
		return nil
	}
	r.URL.Path = upstreamPath
	r.URL.RawPath = ""
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}
	stripWindowCredentials(r)
	proxy.ServeHTTP(w, r)
}

// stripWindowCredentials removes the operator's Axiom credentials from a
// request before it is forwarded to a workload, so a pod listening on the
// proxied port never receives the console session or Basic credentials.
func stripWindowCredentials(r *http.Request) {
	r.Header.Del("Authorization")
	r.Header.Del("Cookie")
	q := r.URL.Query()
	if q.Has("access_token") || q.Has("axiom_session") || q.Has("api_key") {
		q.Del("access_token")
		q.Del("axiom_session")
		q.Del("api_key")
		r.URL.RawQuery = q.Encode()
	}
}
