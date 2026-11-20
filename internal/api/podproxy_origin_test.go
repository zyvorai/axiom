package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

// ppProvider is the demo provider with PodProxyTarget pointed at fake pod
// backends ("ns/pod/port" -> URL).
type ppProvider struct {
	*platform.DemoProvider
	targets map[string]string
}

func (p ppProvider) PodProxyTarget(ctx context.Context, ns, pod string, port int32) (string, error) {
	if t, ok := p.targets[fmt.Sprintf("%s/%s/%d", ns, pod, port)]; ok {
		return t, nil
	}
	return "", platform.NewStatusError(http.StatusNotFound, errors.New("pod not found"))
}

// seen records what a fake pod backend received.
type seen struct {
	mu   sync.Mutex
	reqs []*http.Request
	body []string
}

func (s *seen) add(r *http.Request, body string) {
	s.mu.Lock()
	s.reqs = append(s.reqs, r)
	s.body = append(s.body, body)
	s.mu.Unlock()
}

func (s *seen) last() (*http.Request, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.reqs) == 0 {
		return nil, ""
	}
	return s.reqs[len(s.reqs)-1], s.body[len(s.body)-1]
}

func newPodBackend(t *testing.T, name string) (*httptest.Server, *seen) {
	t.Helper()
	sn := &seen{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		sn.add(r.Clone(context.Background()), string(b))
		switch r.URL.Path {
		case "/stream":
			fl := w.(http.Flusher)
			_, _ = io.WriteString(w, "first\n")
			fl.Flush()
			time.Sleep(150 * time.Millisecond)
			_, _ = io.WriteString(w, "second\n")
			return
		case "/range":
			http.ServeContent(w, r, "x.txt", time.Unix(0, 0), strings.NewReader("0123456789"))
			return
		case "/ws":
			if !strings.EqualFold(r.Header.Get("Upgrade"), "echo") {
				http.Error(w, "no upgrade", 400)
				return
			}
			conn, brw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: echo\r\nConnection: Upgrade\r\n\r\n")
			_ = brw.Flush()
			line, _ := brw.ReadString('\n')
			_, _ = brw.WriteString("echo:" + line)
			_ = brw.Flush()
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "axiom_session", Value: "evil", Path: "/"})
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Content-Security-Policy", "default-src *")
		w.Header().Set("X-Frame-Options", "ALLOWALL")
		w.Header().Set("Service-Worker-Allowed", "/")
		w.Header().Set("Clear-Site-Data", `"cookies"`)
		_, _ = fmt.Fprintf(w, "hello from %s %s %s", name, r.Method, r.URL.RequestURI())
	}))
	t.Cleanup(srv.Close)
	return srv, sn
}

type ppEnv struct {
	console *httptest.Server
	proxy   *httptest.Server
	pp      *PodProxyOrigin
	users   *platform.UserHub
	webSeen *seen
	apiSeen *seen
}

// newPPEnv starts a console and a proxy listener (two httptest servers) and
// two fake pods: team-a/web:8080 and team-a/api:9090, plus team-b/web:8080.
// With on=false the pod proxy origin is not configured.
func newPPEnv(t *testing.T, on bool) *ppEnv {
	t.Helper()
	web, webSeen := newPodBackend(t, "web")
	apiSrv, apiSeen := newPodBackend(t, "api")
	other, _ := newPodBackend(t, "other")
	prov := ppProvider{DemoProvider: platform.NewDemoProvider(), targets: map[string]string{
		"team-a/web/8080": web.URL,
		"team-a/api/9090": apiSrv.URL,
		"team-b/web/8080": other.URL,
	}}
	e := &ppEnv{webSeen: webSeen, apiSeen: apiSeen}
	var ph http.Handler = http.NotFoundHandler()
	var mu sync.RWMutex
	e.proxy = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		h := ph
		mu.RUnlock()
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(e.proxy.Close)
	opts := AuthOptions{SessionSecret: "test-secret", RevocationStore: &memRevStore{}}
	if on {
		pp, err := NewPodProxyOrigin(PodProxyOriginConfig{Addr: "127.0.0.1:0", Origin: e.proxy.URL, ConsoleAddr: ":5090"})
		if err != nil {
			t.Fatal(err)
		}
		e.pp = pp
		opts.PodProxy = pp
		mu.Lock()
		ph = pp.Handler()
		mu.Unlock()
	}
	e.users = platform.NewUserHub(nil)
	e.console = httptest.NewServer(NewWithUsers(prov, "demo", idAdminUser, idAdminPass, "", nil, e.users, opts))
	t.Cleanup(e.console.Close)
	for _, u := range []struct {
		name, role string
		ns         []string
	}{
		{"oper", "operator", nil},
		{"viewer", "viewer", nil},
		{"roper", "operator", []string{"team-a"}},
		{"rviewer", "viewer", []string{"team-a"}},
	} {
		if _, err := e.users.Create(context.Background(), u.name, u.name+"-password", u.role, u.ns); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func (e *ppEnv) consoleDo(t *testing.T, method, path, body, user string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, e.console.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		pass := user + "-password"
		if user == idAdminUser {
			pass = idAdminPass
		}
		req.Header.Set("Authorization", basic(user, pass))
	}
	res, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, string(b)
}

func (e *ppEnv) link(t *testing.T, user, ns, pod string, port int, path string) string {
	t.Helper()
	res, body := e.consoleDo(t, "POST", "/api/v1/pods/"+ns+"/"+pod+"/window-link", fmt.Sprintf(`{"port":%d,"path":%q}`, port, path), user)
	if res.StatusCode != 200 {
		t.Fatalf("window-link as %s: %d %s", user, res.StatusCode, body)
	}
	var out struct {
		URL       string `json:"url"`
		ExpiresIn int    `json:"expiresIn"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.ExpiresIn != 60 || !strings.HasPrefix(out.URL, e.proxy.URL+"/p/") || !strings.HasSuffix(out.URL, "/") {
		t.Fatalf("bad link %+v", out)
	}
	return out.URL
}

// redeem follows a link and returns the session path ("/s/<sid>/…") and cookie.
func (e *ppEnv) redeem(t *testing.T, link string) (string, *http.Cookie) {
	t.Helper()
	res, err := noRedirect.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("redeem: %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.HasPrefix(loc, "/s/") {
		t.Fatalf("redirect to %q", loc)
	}
	var ck *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == podProxyCookieName {
			ck = c
		}
	}
	if ck == nil {
		t.Fatalf("no session cookie: %v", res.Header["Set-Cookie"])
	}
	return loc, ck
}

func sidOf(loc string) string {
	sid, _, _ := strings.Cut(strings.TrimPrefix(loc, "/s/"), "/")
	return sid
}

func (e *ppEnv) proxyDo(t *testing.T, method, path string, ck *http.Cookie, body io.Reader, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, e.proxy.URL+path, body)
	if ck != nil {
		req.AddCookie(ck)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func readAll(t *testing.T, res *http.Response) string {
	t.Helper()
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return string(b)
}

func TestPodProxyOriginFlow(t *testing.T) {
	e := newPPEnv(t, true)
	link := e.link(t, "oper", "team-a", "web", 8080, "/dash?x=1")
	loc, ck := e.redeem(t, link)
	sid := sidOf(loc)
	if loc != "/s/"+sid+"/dash?x=1" {
		t.Fatalf("redirect %q must carry the bound base path", loc)
	}
	if !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/s/"+sid+"/" || ck.Secure || ck.Domain != "" {
		t.Fatalf("cookie attributes: %+v", ck)
	}
	if ck.Value == sid {
		t.Fatal("the cookie secret must not equal the session id in the URL")
	}

	// Ticket reuse is refused.
	if res, _ := noRedirect.Get(link); res.StatusCode != http.StatusForbidden {
		t.Fatalf("reused ticket: %d", res.StatusCode)
	}

	// Proxied content, with the console's credentials sent along by a
	// browser (Basic header, axiom_session cookie, URL token) never reaching
	// the pod.
	res := e.proxyDo(t, "GET", loc+"&access_token=secret&api_key=k", ck, nil, map[string]string{
		"Authorization":               "Basic " + "cm9vdDpyb290LXBhc3N3b3Jk",
		platform.ClusterSessionHeader: "tab-1",
		"X-Api-Key":                   "k",
	})
	body := readAll(t, res)
	if res.StatusCode != 200 || body != "hello from web GET /dash?x=1" {
		t.Fatalf("proxied: %d %q", res.StatusCode, body)
	}
	got, _ := e.webSeen.last()
	if got.Header.Get("Authorization") != "" || got.Header.Get("Cookie") != "" || got.Header.Get("X-Api-Key") != "" || got.Header.Get(platform.ClusterSessionHeader) != "" {
		t.Fatalf("credentials reached the pod: %v", got.Header)
	}
	if q := got.URL.Query(); q.Has("access_token") || q.Has("api_key") {
		t.Fatalf("URL credentials reached the pod: %q", got.URL.RawQuery)
	}
	// Hardening.
	if len(res.Header.Values("Set-Cookie")) != 0 {
		t.Fatal("pod Set-Cookie must be stripped")
	}
	want := map[string]string{
		"Content-Security-Policy":    podProxyCSP,
		"X-Content-Type-Options":     "nosniff",
		"Referrer-Policy":            "no-referrer",
		"Cross-Origin-Opener-Policy": "same-origin",
		"X-Frame-Options":            "DENY",
	}
	for k, v := range want {
		if vals := res.Header.Values(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %q, want exactly %q", k, vals, v)
		}
	}
	if !strings.Contains(podProxyCSP, "frame-ancestors 'none'") || !strings.HasPrefix(podProxyCSP, "sandbox allow-scripts") {
		t.Fatalf("CSP %q", podProxyCSP)
	}
	for k := range res.Header {
		if strings.HasPrefix(k, "Access-Control-") {
			t.Errorf("CORS header %s leaked from the pod", k)
		}
	}
	for _, k := range []string{"Service-Worker-Allowed", "Clear-Site-Data"} {
		if res.Header.Get(k) != "" {
			t.Errorf("%s leaked from the pod", k)
		}
	}

	// Console cookies alongside the proxy cookie are not forwarded either.
	req, _ := http.NewRequest("GET", e.proxy.URL+"/s/"+sid+"/x", nil)
	req.Header.Set("Cookie", "axiom_session=console-token; "+ck.Name+"="+ck.Value)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if b := readAll(t, res); res.StatusCode != 200 || b != "hello from web GET /x" {
		t.Fatalf("with console cookie: %d %q", res.StatusCode, b)
	}
	if got, _ := e.webSeen.last(); got.Header.Get("Cookie") != "" {
		t.Fatalf("cookies reached the pod: %q", got.Header.Get("Cookie"))
	}
}

func TestPodProxyOriginSessionBinding(t *testing.T) {
	e := newPPEnv(t, true)
	locA, ckA := e.redeem(t, e.link(t, "oper", "team-a", "web", 8080, "/"))
	locB, ckB := e.redeem(t, e.link(t, "oper", "team-a", "api", 9090, "/"))
	sidA, sidB := sidOf(locA), sidOf(locB)

	cases := []struct {
		name string
		path string
		ck   *http.Cookie
	}{
		{"no cookie", "/s/" + sidA + "/", nil},
		{"cookie of another pod/port", "/s/" + sidB + "/", ckA},
		{"cookie of the other session", "/s/" + sidA + "/", ckB},
		{"unknown session id", "/s/" + auth.RandomURLToken(32) + "/", ckA},
		{"forged cookie", "/s/" + sidA + "/", &http.Cookie{Name: podProxyCookieName, Value: sidA}},
	}
	for _, c := range cases {
		res := e.proxyDo(t, "GET", c.path, c.ck, nil, nil)
		readAll(t, res)
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("%s: %d, want 403", c.name, res.StatusCode)
		}
	}
	// Each session reaches only its own pod.
	if b := readAll(t, e.proxyDo(t, "GET", "/s/"+sidA+"/", ckA, nil, nil)); b != "hello from web GET /" {
		t.Fatalf("A: %q", b)
	}
	if b := readAll(t, e.proxyDo(t, "GET", "/s/"+sidB+"/", ckB, nil, nil)); b != "hello from api GET /" {
		t.Fatalf("B: %q", b)
	}
	// Console credentials are no credential on the proxy origin.
	res := e.proxyDo(t, "GET", "/s/"+sidA+"/", &http.Cookie{Name: auth.SessionCookie, Value: "x"}, nil, map[string]string{"Authorization": basic(idAdminUser, idAdminPass)})
	readAll(t, res)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("console credentials accepted on proxy origin: %d", res.StatusCode)
	}
	// /s/<sid> without a slash is sent to /s/<sid>/ (only with a valid session).
	res = e.proxyDo(t, "GET", "/s/"+sidA, ckA, nil, nil)
	readAll(t, res)
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/s/"+sidA+"/" {
		t.Fatalf("no-slash: %d %q", res.StatusCode, res.Header.Get("Location"))
	}
}

func TestPodProxyOriginRevokedUserSessionDies(t *testing.T) {
	e := newPPEnv(t, true)
	loc, ck := e.redeem(t, e.link(t, "oper", "team-a", "web", 8080, "/"))
	pending := e.link(t, "oper", "team-a", "web", 8080, "/")
	if res := e.proxyDo(t, "GET", loc, ck, nil, nil); res.StatusCode != 200 {
		t.Fatalf("before revoke: %d", res.StatusCode)
	}
	time.Sleep(2 * time.Millisecond)
	if res, b := e.consoleDo(t, "POST", "/api/v1/auth/logout-all", "", "oper"); res.StatusCode != 200 {
		t.Fatalf("logout-all: %d %s", res.StatusCode, b)
	}
	res := e.proxyDo(t, "GET", loc, ck, nil, nil)
	readAll(t, res)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("revoked user's session still proxies: %d", res.StatusCode)
	}
	// A link minted before the cut-off cannot be redeemed after it.
	if r2, _ := noRedirect.Get(pending); r2.StatusCode != http.StatusForbidden {
		t.Fatalf("pre-revocation ticket redeemed: %d", r2.StatusCode)
	}
	// A fresh link after the cut-off works.
	time.Sleep(2 * time.Millisecond)
	loc2, ck2 := e.redeem(t, e.link(t, "oper", "team-a", "web", 8080, "/"))
	if res := e.proxyDo(t, "GET", loc2, ck2, nil, nil); res.StatusCode != 200 {
		t.Fatalf("fresh session after revoke: %d", res.StatusCode)
	}
	// A role change (to viewer) revokes too, via the user hub's hook.
	time.Sleep(2 * time.Millisecond)
	if _, err := e.users.UpdateRole(context.Background(), "oper", "viewer"); err != nil {
		t.Fatal(err)
	}
	if res := e.proxyDo(t, "GET", loc2, ck2, nil, nil); res.StatusCode != http.StatusForbidden {
		t.Fatalf("session survived a role change: %d", res.StatusCode)
	}
}

func TestPodProxyOriginMethodsRangeStreamingUpgrade(t *testing.T) {
	e := newPPEnv(t, true)
	loc, ck := e.redeem(t, e.link(t, "oper", "team-a", "web", 8080, "/"))
	base := strings.TrimSuffix(loc, "/")

	// HEAD.
	res := e.proxyDo(t, "HEAD", base+"/", ck, nil, nil)
	if b := readAll(t, res); res.StatusCode != 200 || b != "" {
		t.Fatalf("HEAD: %d %q", res.StatusCode, b)
	}
	// POST with a body.
	res = e.proxyDo(t, "POST", base+"/form", ck, strings.NewReader("a=1"), map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if b := readAll(t, res); res.StatusCode != 200 || b != "hello from web POST /form" {
		t.Fatalf("POST: %d %q", res.StatusCode, b)
	}
	if _, body := e.webSeen.last(); body != "a=1" {
		t.Fatalf("POST body reached pod as %q", body)
	}
	// Range.
	res = e.proxyDo(t, "GET", base+"/range", ck, nil, map[string]string{"Range": "bytes=2-4"})
	if b := readAll(t, res); res.StatusCode != http.StatusPartialContent || b != "234" {
		t.Fatalf("Range: %d %q", res.StatusCode, b)
	}
	// Streaming: the first chunk arrives before the pod finishes.
	start := time.Now()
	res = e.proxyDo(t, "GET", base+"/stream", ck, nil, nil)
	br := bufio.NewReader(res.Body)
	line, err := br.ReadString('\n')
	if err != nil || line != "first\n" || time.Since(start) > 120*time.Millisecond {
		t.Fatalf("stream first chunk %q after %v (%v)", line, time.Since(start), err)
	}
	rest, _ := io.ReadAll(br)
	res.Body.Close()
	if string(rest) != "second\n" {
		t.Fatalf("stream rest %q", rest)
	}
	// Methods the proxy does not forward.
	res = e.proxyDo(t, "OPTIONS", base+"/", ck, nil, nil)
	readAll(t, res)
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("OPTIONS: %d", res.StatusCode)
	}
	// Service worker registration is refused.
	res = e.proxyDo(t, "GET", base+"/sw.js", ck, nil, map[string]string{"Service-Worker": "script"})
	readAll(t, res)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("service worker script: %d", res.StatusCode)
	}

	// Upgrade (WebSocket-style) passthrough.
	u, _ := url.Parse(e.proxy.URL)
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprintf(conn, "GET %s/ws HTTP/1.1\r\nHost: %s\r\nUpgrade: echo\r\nConnection: Upgrade\r\nAuthorization: Basic x\r\nCookie: %s=%s\r\n\r\n", base, u.Host, ck.Name, ck.Value)
	cr := bufio.NewReader(conn)
	up, err := http.ReadResponse(cr, nil)
	if err != nil || up.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade: %v %v", up, err)
	}
	fmt.Fprint(conn, "ping\n")
	if got, _ := cr.ReadString('\n'); got != "echo:ping\n" {
		t.Fatalf("upgraded echo %q", got)
	}
	if got, _ := e.webSeen.last(); got.Header.Get("Authorization") != "" || got.Header.Get("Cookie") != "" {
		t.Fatalf("credentials reached the pod on upgrade: %v", got.Header)
	}
}

func TestPodProxyOriginOnlyServesItsRoutes(t *testing.T) {
	e := newPPEnv(t, true)
	for _, p := range []string{"/", "/api/v1/session", "/api/v1/health", "/p", "/s", "/s/", "/favicon.ico", "/index.html", "/api/v1/pods/team-a/web/window/8080/"} {
		res := e.proxyDo(t, "GET", p, nil, nil, map[string]string{"Authorization": basic(idAdminUser, idAdminPass)})
		readAll(t, res)
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", p, res.StatusCode)
		}
		if res.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s: CORS header on the proxy origin", p)
		}
		if res.Header.Get("X-Frame-Options") != "DENY" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: missing policy headers", p)
		}
	}
	// Bad tickets.
	for _, p := range []string{"/p/", "/p/nope/", "/p/" + auth.RandomURLToken(32) + "/"} {
		res := e.proxyDo(t, "GET", p, nil, nil, nil)
		readAll(t, res)
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("%s: %d, want 403", p, res.StatusCode)
		}
	}
	// A ticket cannot be redeemed with POST (and is not consumed by trying).
	link := e.link(t, "oper", "team-a", "web", 8080, "/")
	res, err := noRedirect.Post(link, "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	readAll(t, res)
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /p/: %d", res.StatusCode)
	}
	if res, err := noRedirect.Head(link); err != nil || res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("HEAD /p/ must not redeem: %v %v", res, err)
	}
	e.redeem(t, link)
	// The ticket is not a console credential.
	tok := strings.TrimSuffix(strings.TrimPrefix(link, e.proxy.URL+"/p/"), "/")
	for _, q := range []string{"ticket=", "access_token="} {
		r2, _ := e.consoleDo(t, "GET", "/api/v1/session?"+q+tok, "", "")
		if r2.StatusCode != http.StatusUnauthorized {
			t.Errorf("console accepted a pod proxy ticket as ?%s: %d", q, r2.StatusCode)
		}
	}
}

func TestPodProxyOriginLimits(t *testing.T) {
	e := newPPEnv(t, true)
	srv := e.pp.Server()
	if srv.MaxHeaderBytes != podProxyMaxHeaderBytes || srv.ReadHeaderTimeout <= 0 || srv.IdleTimeout <= 0 {
		t.Fatalf("server limits: %+v", srv)
	}
	loc, ck := e.redeem(t, e.link(t, "oper", "team-a", "web", 8080, "/"))
	// Declared oversize body.
	req, _ := http.NewRequest("POST", e.proxy.URL+loc, strings.NewReader("x"))
	req.ContentLength = podProxyMaxBody + 1
	req.AddCookie(ck)
	req.Body = io.NopCloser(io.LimitReader(neverEnding('x'), podProxyMaxBody+1))
	res, err := http.DefaultClient.Do(req)
	if err == nil {
		readAll(t, res)
		if res.StatusCode != http.StatusRequestEntityTooLarge {
			t.Fatalf("oversize body: %d", res.StatusCode)
		}
	}

	// The real listener (StartPodProxyOrigin) enforces MaxHeaderBytes and
	// stops with its context.
	pp, err := NewPodProxyOrigin(PodProxyOriginConfig{Addr: "127.0.0.1:0", Origin: "http://pods.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	live, err := StartPodProxyOrigin(ctx, pp, "", "")
	if err != nil {
		t.Fatal(err)
	}
	big, _ := http.NewRequest("GET", "http://"+live.Addr+"/s/x/", nil)
	big.Header.Set("X-Big", strings.Repeat("a", 2*podProxyMaxHeaderBytes))
	res, err = http.DefaultClient.Do(big)
	if err != nil {
		t.Fatal(err)
	}
	readAll(t, res)
	if res.StatusCode != http.StatusRequestHeaderFieldsTooLarge {
		t.Fatalf("oversize headers: %d", res.StatusCode)
	}
	res, err = http.Get("http://" + live.Addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	readAll(t, res)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("live listener /: %d", res.StatusCode)
	}
	cancel()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := http.Get("http://" + live.Addr + "/"); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("listener still serving after its context was cancelled")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type neverEnding byte

func (b neverEnding) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(b)
	}
	return len(p), nil
}

func TestPodProxyOriginSecureCookie(t *testing.T) {
	pp, err := NewPodProxyOrigin(PodProxyOriginConfig{Addr: ":5091", Origin: "https://pods.example.com/", ConsoleAddr: ":5090", ConsoleOrigin: "https://axiom.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if pp.Origin() != "https://pods.example.com" {
		t.Fatalf("origin %q", pp.Origin())
	}
	pp.attach(platform.NewDemoProvider(), revocationsChecker{auth.NewRevocations(nil)})
	tok, _, err := pp.tickets.Issue(auth.PodProxyGrant{User: "oper", Role: "operator", Namespace: "ns", Pod: "p", Port: 80, Path: "/"})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	pp.Handler().ServeHTTP(w, httptest.NewRequest("GET", "https://pods.example.com/p/"+tok+"/", nil))
	cks := w.Result().Cookies()
	if w.Code != http.StatusFound || len(cks) != 1 {
		t.Fatalf("redeem: %d %v", w.Code, cks)
	}
	c := cks[0]
	if c.Name != podProxySecureCookieName || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || !strings.HasPrefix(c.Path, "/s/") {
		t.Fatalf("https cookie: %+v", c)
	}
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("https proxy origin should send HSTS")
	}
}

func TestPodProxyOriginConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  PodProxyOriginConfig
		ok   bool
	}{
		{"both unset", PodProxyOriginConfig{ConsoleAddr: ":5090"}, false},
		{"addr only", PodProxyOriginConfig{Addr: ":5091", ConsoleAddr: ":5090"}, false},
		{"origin only", PodProxyOriginConfig{Origin: "http://localhost:5091", ConsoleAddr: ":5090"}, false},
		{"localhost other port", PodProxyOriginConfig{Addr: ":5091", Origin: "http://localhost:5091", ConsoleAddr: ":5090", ConsoleOrigin: "http://localhost:5173"}, true},
		{"separate host", PodProxyOriginConfig{Addr: ":5091", Origin: "https://pods.example.com", ConsoleAddr: ":5090", ConsoleOrigin: "https://axiom.example.com"}, true},
		{"same as allow-origin", PodProxyOriginConfig{Addr: ":5091", Origin: "https://axiom.example.com", ConsoleAddr: ":5090", ConsoleOrigin: "https://axiom.example.com"}, false},
		{"same as allow-origin default port", PodProxyOriginConfig{Addr: ":5091", Origin: "https://AXIOM.example.com:443", ConsoleAddr: ":5090", ConsoleOrigin: "https://axiom.example.com"}, false},
		{"localhost console port", PodProxyOriginConfig{Addr: ":5091", Origin: "http://localhost:5090", ConsoleAddr: ":5090"}, false},
		{"127.0.0.1 console port", PodProxyOriginConfig{Addr: ":5091", Origin: "http://127.0.0.1:5090", ConsoleAddr: "127.0.0.1:5090"}, false},
		{"same listen addr", PodProxyOriginConfig{Addr: ":5090", Origin: "https://pods.example.com", ConsoleAddr: ":5090"}, false},
		{"not an origin (path)", PodProxyOriginConfig{Addr: ":5091", Origin: "https://pods.example.com/x", ConsoleAddr: ":5090"}, false},
		{"not an origin (scheme)", PodProxyOriginConfig{Addr: ":5091", Origin: "ftp://pods.example.com", ConsoleAddr: ":5090"}, false},
		{"not an origin (userinfo)", PodProxyOriginConfig{Addr: ":5091", Origin: "https://u:p@pods.example.com", ConsoleAddr: ":5090"}, false},
		{"bad addr", PodProxyOriginConfig{Addr: "5091", Origin: "https://pods.example.com", ConsoleAddr: ":5090"}, false},
	}
	for _, c := range cases {
		p, err := NewPodProxyOrigin(c.cfg)
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v, want ok=%v", c.name, err, c.ok)
		}
		if c.ok && p == nil {
			t.Errorf("%s: nil origin", c.name)
		}
	}
	if _, err := NewPodProxyOrigin(PodProxyOriginConfig{}); !errors.Is(err, ErrPodProxyDisabled) {
		t.Fatalf("unset config: %v", err)
	}
	// The env wrapper logs and disables instead of failing.
	if p := NewPodProxyOriginFromEnv(PodProxyOriginConfig{Addr: ":5091", ConsoleAddr: ":5090"}); p != nil {
		t.Fatal("misconfiguration must disable the feature")
	}
	if p := NewPodProxyOriginFromEnv(PodProxyOriginConfig{Addr: ":5091", Origin: "https://axiom.example.com", ConsoleAddr: ":5090", ConsoleOrigin: "https://axiom.example.com"}); p != nil {
		t.Fatal("same-origin misconfiguration must disable the feature")
	}
}

func TestPodProxyWindowLinkPolicy(t *testing.T) {
	e := newPPEnv(t, true)
	cases := []struct {
		name, user, ns, body string
		want                 int
	}{
		{"viewer", "viewer", "team-a", `{"port":8080}`, 403},
		{"restricted viewer", "rviewer", "team-a", `{"port":8080}`, 403},
		{"operator", "oper", "team-a", `{"port":8080}`, 200},
		{"operator other ns", "oper", "team-b", `{"port":8080}`, 200},
		{"restricted operator own ns", "roper", "team-a", `{"port":8080}`, 200},
		{"restricted operator other ns", "roper", "team-b", `{"port":8080}`, 403},
		{"admin", idAdminUser, "team-b", `{"port":8080,"path":"/"}`, 200},
		{"anonymous", "", "team-a", `{"port":8080}`, 401},
		{"bad port", "oper", "team-a", `{"port":0}`, 400},
		{"port too big", "oper", "team-a", `{"port":70000}`, 400},
		{"absolute url path", "oper", "team-a", `{"port":8080,"path":"https://evil.example/"}`, 400},
		{"scheme-relative path", "oper", "team-a", `{"port":8080,"path":"//evil.example/"}`, 400},
		{"unknown field", "oper", "team-a", `{"port":8080,"host":"x"}`, 400},
		{"undeclared port", "oper", "team-a", `{"port":22}`, 404},
	}
	for _, c := range cases {
		res, body := e.consoleDo(t, "POST", "/api/v1/pods/"+c.ns+"/web/window-link", c.body, c.user)
		if res.StatusCode != c.want {
			t.Errorf("%s: %d %s, want %d", c.name, res.StatusCode, body, c.want)
		}
		if res.StatusCode == 200 && res.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s: link response must not be cached", c.name)
		}
	}
	// A restricted operator's link reaches the pod it was minted for.
	loc, ck := e.redeem(t, e.link(t, "roper", "team-a", "web", 8080, "/"))
	if b := readAll(t, e.proxyDo(t, "GET", loc, ck, nil, nil)); b != "hello from web GET /" {
		t.Fatalf("restricted operator proxy: %q", b)
	}
	// The per-user ticket cap is enforced through the endpoint.
	var last int
	for i := 0; i < 40; i++ {
		res, _ := e.consoleDo(t, "POST", "/api/v1/pods/team-a/web/window-link", `{"port":8080}`, "oper")
		last = res.StatusCode
		if last != 200 {
			break
		}
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("per-user ticket cap: last status %d", last)
	}
}

func TestPodWindowRouteWithPodProxyOrigin(t *testing.T) {
	// ON: the in-origin route is gone (410) and says where to go instead;
	// the capability flag is set.
	on := newPPEnv(t, true)
	res, body := on.consoleDo(t, "GET", "/api/v1/pods/team-a/web/window/8080/", "", "oper")
	if res.StatusCode != http.StatusGone || !strings.Contains(body, "window-link") {
		t.Fatalf("ON window route: %d %s", res.StatusCode, body)
	}
	if on.webSeen.reqs != nil {
		t.Fatal("ON window route must not reach the pod")
	}
	if _, b := on.consoleDo(t, "GET", "/api/v1/realtime/capabilities", "", "oper"); !strings.Contains(b, `"podProxy":true`) {
		t.Fatalf("capabilities ON: %s", b)
	}
	// Viewers are still refused by the guard first.
	if res, _ := on.consoleDo(t, "GET", "/api/v1/pods/team-a/web/window/8080/", "", "viewer"); res.StatusCode != 403 {
		t.Fatalf("viewer ON: %d", res.StatusCode)
	}

	// OFF: the old route works as before; window-link is 404.
	off := newPPEnv(t, false)
	res, body = off.consoleDo(t, "GET", "/api/v1/pods/team-a/web/window/8080/sub?keep=1", "", "oper")
	if res.StatusCode != 200 || body != "hello from web GET /sub?keep=1" {
		t.Fatalf("OFF window route: %d %q", res.StatusCode, body)
	}
	if csp := res.Header.Get("Content-Security-Policy"); csp != "sandbox allow-scripts allow-forms allow-popups" {
		t.Fatalf("OFF CSP changed: %q", csp)
	}
	if len(res.Header.Values("Set-Cookie")) != 0 {
		t.Fatal("OFF: pod Set-Cookie must be stripped")
	}
	if got, _ := off.webSeen.last(); got.Header.Get("Authorization") != "" {
		t.Fatal("OFF: Authorization reached the pod")
	}
	if res, _ := off.consoleDo(t, "POST", "/api/v1/pods/team-a/web/window-link", `{"port":8080}`, "oper"); res.StatusCode != 404 {
		t.Fatalf("OFF window-link: %d", res.StatusCode)
	}
	if _, b := off.consoleDo(t, "GET", "/api/v1/realtime/capabilities", "", "oper"); !strings.Contains(b, `"podProxy":false`) {
		t.Fatalf("capabilities OFF: %s", b)
	}
}
