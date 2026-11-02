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
	// The pod's content is served from Axiom's own origin, so treat it as
	// hostile: sandbox it into an opaque origin (it cannot reach the console's
	// cookies or same-origin API), and never let it set cookies on our origin.
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Set-Cookie")
		resp.Header.Set("Content-Security-Policy", "sandbox allow-scripts allow-forms allow-popups")
		resp.Header.Set("X-Content-Type-Options", "nosniff")
		return nil
	}
	prefix := fmt.Sprintf("/api/v1/pods/%s/%s/window/%d/", ns, name, port)
	r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
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
