package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

// ppHostEnv is a console and a pod proxy listener in per-session-host mode
// (origin http://{id}.pods.test:<proxy port>). Requests reach the listener on
// 127.0.0.1 with the session's name in the Host header.
type ppHostEnv struct {
	*ppEnv
	port string
}

func newPPHostEnv(t *testing.T, tweak func(*PodProxyOrigin)) *ppHostEnv {
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
	pu, _ := url.Parse(e.proxy.URL)
	port := pu.Port()
	pp, err := NewPodProxyOrigin(PodProxyOriginConfig{Addr: "127.0.0.1:0", Origin: "http://{id}.pods.test:" + port, ConsoleAddr: ":5090", ConsoleOrigin: "http://axiom.test:5090"})
	if err != nil {
		t.Fatal(err)
	}
	if tweak != nil {
		tweak(pp)
	}
	e.pp = pp
	mu.Lock()
	ph = pp.Handler()
	mu.Unlock()
	e.users = platform.NewUserHub(nil)
	e.console = httptest.NewServer(NewWithUsers(prov, "demo", idAdminUser, idAdminPass, "", nil, e.users,
		AuthOptions{SessionSecret: "test-secret", RevocationStore: &memRevStore{}, PodProxy: pp}))
	t.Cleanup(e.console.Close)
	for _, u := range []struct {
		name, role string
		ns         []string
	}{
		{"oper", "operator", nil},
		{"oper2", "operator", nil},
		{"viewer", "viewer", nil},
		{"roper", "operator", []string{"team-a"}},
		{"rviewer", "viewer", []string{"team-a"}},
	} {
		if _, err := e.users.Create(context.Background(), u.name, u.name+"-password", u.role, u.ns); err != nil {
			t.Fatal(err)
		}
	}
	return &ppHostEnv{ppEnv: e, port: port}
}

// hostLink is a parsed host-mode window link.
type hostLink struct {
	sid, host, ticketPath string
}

func (e *ppHostEnv) linkHost(t *testing.T, user, ns, pod string, port int, path string) hostLink {
	t.Helper()
	res, body := e.consoleDo(t, "POST", "/api/v1/pods/"+ns+"/"+pod+"/window-link", fmt.Sprintf(`{"port":%d,"path":%q}`, port, path), user)
	if res.StatusCode != 200 {
		t.Fatalf("window-link as %s: %d %s", user, res.StatusCode, body)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil || len(raw) != 2 || raw["expiresIn"] != float64(60) {
		t.Fatalf("link response shape: %s (%v)", body, err)
	}
	link, _ := raw["url"].(string)
	m := regexp.MustCompile(`^http://([0-9a-f]{32})\.pods\.test:` + e.port + `(/p/[A-Za-z0-9_-]+/)$`).FindStringSubmatch(link)
	if m == nil {
		t.Fatalf("bad host-mode link %q", link)
	}
	return hostLink{sid: m[1], host: m[1] + ".pods.test:" + e.port, ticketPath: m[2]}
}

func (e *ppHostEnv) hostDo(t *testing.T, host, method, path string, ck *http.Cookie, body io.Reader, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, e.proxy.URL+path, body)
	req.Host = host
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

// redeemHost opens l on its own host and returns the redirect target and the
// session cookie.
func (e *ppHostEnv) redeemHost(t *testing.T, l hostLink) (string, *http.Cookie) {
	t.Helper()
	res := e.hostDo(t, l.host, "GET", l.ticketPath, nil, nil, nil)
	b := readAll(t, res)
	if res.StatusCode != http.StatusFound {
		t.Fatalf("redeem on %s: %d %s", l.host, res.StatusCode, b)
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
	return res.Header.Get("Location"), ck
}

func TestPodProxyHostConfigValidation(t *testing.T) {
	cases := []struct {
		name, origin, consoleAddr, consoleOrigin, addr string
		ok                                             bool
	}{
		{"wildcard https", "https://{id}.pods.example.com", ":5090", "https://axiom.example.com", "", true},
		{"wildcard https explicit port", "https://{id}.pods.example.com:8443", ":5090", "https://axiom.example.com", "", true},
		{"localhost lab", "http://{id}.localhost:5091", ":5090", "", "", true},
		{"localhost lab, console on 127.0.0.1", "http://{id}.localhost:5091", ":5090", "http://127.0.0.1:5090", "", true},
		{"upper-case base", "https://{id}.PODS.example.com", ":5090", "https://axiom.example.com", "", true},
		{"placeholder twice", "https://{id}.{id}.example.com", ":5090", "", "", false},
		{"placeholder not leftmost", "https://pods.{id}.example.com", ":5090", "", "", false},
		{"placeholder part of a label (prefix)", "https://x{id}.pods.example.com", ":5090", "", "", false},
		{"placeholder part of a label (suffix)", "https://{id}x.pods.example.com", ":5090", "", "", false},
		{"placeholder is the whole host", "https://{id}", ":5090", "", "", false},
		{"placeholder is the whole host with port", "http://{id}:5091", ":5090", "", "", false},
		{"placeholder in the path", "https://pods.example.com/{id}", ":5090", "", "", false},
		{"placeholder in the port", "https://pods.example.com:{id}", ":5090", "", "", false},
		{"no scheme", "{id}.pods.example.com", ":5090", "", "", false},
		{"bad scheme", "ftp://{id}.pods.example.com", ":5090", "", "", false},
		{"path", "https://{id}.pods.example.com/x", ":5090", "", "", false},
		{"query", "https://{id}.pods.example.com?a=1", ":5090", "", "", false},
		{"userinfo", "https://u:p@{id}.pods.example.com", ":5090", "", "", false},
		{"IP base", "http://{id}.127.0.0.1:5091", ":5090", "", "", false},
		{"bad base label", "https://{id}.-pods.example.com", ":5090", "", "", false},
		{"console host is the base domain", "https://{id}.example.com", ":5090", "https://example.com", "", false},
		{"console host is the base domain (localhost)", "http://{id}.localhost:5091", ":5090", "http://localhost:5090", "", false},
		{"console is a sibling inside the base domain", "https://{id}.pods.example.com", ":5090", "https://axiom.pods.example.com", "", false},
		{"base domain inside the console host", "https://{id}.pods.axiom.example.com", ":5090", "https://axiom.example.com", "", false},
		{"base domain inside the console host (localhost)", "http://{id}.pods.localhost:5091", ":5090", "http://localhost:5173", "", false},
		{"localhost on the console port", "http://{id}.localhost:5090", ":5090", "", "", false},
		{"localhost on the console port (127.0.0.1 listener)", "http://{id}.localhost:5090", "127.0.0.1:5090", "", "", false},
		{"same listen address", "https://{id}.pods.example.com", ":5090", "", ":5090", false},
	}
	for _, c := range cases {
		addr := c.addr
		if addr == "" {
			addr = ":5091"
		}
		p, err := NewPodProxyOrigin(PodProxyOriginConfig{Addr: addr, Origin: c.origin, ConsoleAddr: c.consoleAddr, ConsoleOrigin: c.consoleOrigin})
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v, want ok=%v", c.name, err, c.ok)
			continue
		}
		if c.ok && (p == nil || p.Mode() != "host") {
			t.Errorf("%s: mode %v", c.name, p)
		}
		if !c.ok && NewPodProxyOriginFromEnv(PodProxyOriginConfig{Addr: addr, Origin: c.origin, ConsoleAddr: c.consoleAddr, ConsoleOrigin: c.consoleOrigin}) != nil {
			t.Errorf("%s: the env wrapper must disable the feature", c.name)
		}
	}
	p, err := NewPodProxyOrigin(PodProxyOriginConfig{Addr: ":5091", Origin: "https://{id}.PODS.example.com:8443/", ConsoleAddr: ":5090"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Origin() != "https://{id}.pods.example.com:8443" || p.host.originFor("ab") != "https://ab.pods.example.com:8443" {
		t.Fatalf("origin %q / %q", p.Origin(), p.host.originFor("ab"))
	}
	// The fixed-origin form is unchanged and reports "path".
	if p, err := NewPodProxyOrigin(PodProxyOriginConfig{Addr: ":5091", Origin: "https://pods.example.com", ConsoleAddr: ":5090"}); err != nil || p.Mode() != "path" || p.host != nil {
		t.Fatalf("fixed origin: %v %v", p, err)
	}
}

func TestPodProxyHostSessionIDFromHost(t *testing.T) {
	sid := auth.NewPodProxySessionID()
	cases := []struct {
		origin, host string
		ok           bool
	}{
		{"https://{id}.pods.example.com", sid + ".pods.example.com", true},
		{"https://{id}.pods.example.com", sid + ".pods.example.com:443", true},
		{"https://{id}.pods.example.com", strings.ToUpper(sid) + ".PODS.example.com", true},
		{"https://{id}.pods.example.com", sid + ".pods.example.com:8443", false},
		{"https://{id}.pods.example.com", sid + ".pods.example.com.", false},
		{"https://{id}.pods.example.com", "pods.example.com", false},
		{"https://{id}.pods.example.com", sid + ".evil.example.com", false},
		{"https://{id}.pods.example.com", "x." + sid + ".pods.example.com", false},
		{"https://{id}.pods.example.com", sid[:31] + ".pods.example.com", false},
		{"https://{id}.pods.example.com", sid + "0.pods.example.com", false},
		{"https://{id}.pods.example.com", strings.Repeat("g", 32) + ".pods.example.com", false},
		{"https://{id}.pods.example.com", "", false},
		{"https://{id}.pods.example.com", "[::1]:443", false},
		{"http://{id}.localhost:5091", sid + ".localhost:5091", true},
		{"http://{id}.localhost:5091", sid + ".localhost", false},
		{"http://{id}.localhost:5091", sid + ".localhost:5090", false},
		{"http://{id}.localhost:5091", "localhost:5091", false},
		{"http://{id}.localhost:5091", "127.0.0.1:5091", false},
	}
	for _, c := range cases {
		p, err := NewPodProxyOrigin(PodProxyOriginConfig{Addr: ":5099", Origin: c.origin, ConsoleAddr: ":5090"})
		if err != nil {
			t.Fatal(err)
		}
		got, ok := p.host.sessionID(c.host)
		if ok != c.ok || (ok && got != sid) {
			t.Errorf("%s on %s: %q %v, want ok=%v", c.host, c.origin, got, ok, c.ok)
		}
	}
}

func TestPodProxyHostFlow(t *testing.T) {
	e := newPPHostEnv(t, nil)
	l := e.linkHost(t, "oper", "team-a", "web", 8080, "/dash?x=1")
	loc, ck := e.redeemHost(t, l)
	if loc != "/dash?x=1" {
		t.Fatalf("redirect %q must be the bound base path at the host root", loc)
	}
	if !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/" || ck.Secure || ck.Domain != "" || ck.Value == l.sid {
		t.Fatalf("cookie attributes: %+v", ck)
	}

	// Proxied at the root, with the console's credentials stripped.
	res := e.hostDo(t, l.host, "GET", loc+"&access_token=secret&api_key=k", ck, nil, map[string]string{
		"Authorization":               "Basic cm9vdDpyb290LXBhc3N3b3Jk",
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
	if len(res.Header.Values("Set-Cookie")) != 0 {
		t.Fatal("pod Set-Cookie must be stripped")
	}
	for k, v := range map[string]string{
		"Content-Security-Policy":    podProxyCSP,
		"X-Content-Type-Options":     "nosniff",
		"Referrer-Policy":            "no-referrer",
		"Cross-Origin-Opener-Policy": "same-origin",
		"X-Frame-Options":            "DENY",
	} {
		if vals := res.Header.Values(k); len(vals) != 1 || vals[0] != v {
			t.Errorf("%s = %q, want exactly %q", k, vals, v)
		}
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

	// Absolute-path resources load from the root of the session's host.
	for _, p := range []string{"/static/app.js", "/api/data?x=1", "/", "/s/whatever/"} {
		res := e.hostDo(t, l.host, "GET", p, ck, nil, nil)
		if b := readAll(t, res); res.StatusCode != 200 || b != "hello from web GET "+p {
			t.Errorf("%s: %d %q", p, res.StatusCode, b)
		}
	}

	// After the first use /p/ is a pod path like any other: the spent ticket
	// path is proxied, not redeemed again.
	res = e.hostDo(t, l.host, "GET", l.ticketPath, ck, nil, nil)
	if b := readAll(t, res); res.StatusCode != 200 || b != "hello from web GET "+l.ticketPath {
		t.Fatalf("/p/ after first use: %d %q", res.StatusCode, b)
	}
	if len(res.Header.Values("Set-Cookie")) != 0 {
		t.Fatal("no new session cookie after first use")
	}
	// ... and without the cookie it is refused like every other path.
	res = e.hostDo(t, l.host, "GET", l.ticketPath, nil, nil, nil)
	readAll(t, res)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("/p/ without cookie after first use: %d", res.StatusCode)
	}

	// Console cookies alongside the proxy cookie are not forwarded.
	res = e.hostDo(t, l.host, "GET", "/x", nil, nil, map[string]string{"Cookie": "axiom_session=console-token; " + ck.Name + "=" + ck.Value})
	if b := readAll(t, res); res.StatusCode != 200 || b != "hello from web GET /x" {
		t.Fatalf("with console cookie: %d %q", res.StatusCode, b)
	}
	if got, _ := e.webSeen.last(); got.Header.Get("Cookie") != "" {
		t.Fatalf("cookies reached the pod: %q", got.Header.Get("Cookie"))
	}
	// Console credentials are no credential here.
	res = e.hostDo(t, l.host, "GET", "/", &http.Cookie{Name: auth.SessionCookie, Value: "x"}, nil, map[string]string{"Authorization": basic(idAdminUser, idAdminPass)})
	readAll(t, res)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("console credentials accepted: %d", res.StatusCode)
	}
}

func TestPodProxyHostIsolation(t *testing.T) {
	e := newPPHostEnv(t, nil)
	a := e.linkHost(t, "oper", "team-a", "web", 8080, "/")
	b := e.linkHost(t, "oper", "team-a", "api", 9090, "/")
	c := e.linkHost(t, "oper", "team-a", "web", 8080, "/")
	if a.sid == b.sid || a.sid == c.sid || b.sid == c.sid {
		t.Fatal("every link must get its own session id")
	}

	// A's ticket presented on B's host is refused and burnt.
	res := e.hostDo(t, b.host, "GET", a.ticketPath, nil, nil, nil)
	readAll(t, res)
	if res.StatusCode != http.StatusForbidden || len(res.Cookies()) != 0 {
		t.Fatalf("ticket for A on B: %d %v", res.StatusCode, res.Cookies())
	}
	res = e.hostDo(t, a.host, "GET", a.ticketPath, nil, nil, nil)
	readAll(t, res)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("burnt ticket redeemed on its own host: %d", res.StatusCode)
	}
	if res := e.hostDo(t, a.host, "GET", "/", nil, nil, nil); readAll(t, res) != "" || res.StatusCode != http.StatusNotFound {
		t.Fatalf("A must not exist: %d", res.StatusCode)
	}
	// B's own ticket still works after the failed attempt on its host.
	_, ckB := e.redeemHost(t, b)
	_, ckC := e.redeemHost(t, c)

	// Each session reaches only its own pod.
	if body := readAll(t, e.hostDo(t, b.host, "GET", "/", ckB, nil, nil)); body != "hello from api GET /" {
		t.Fatalf("B: %q", body)
	}
	if body := readAll(t, e.hostDo(t, c.host, "GET", "/", ckC, nil, nil)); body != "hello from web GET /" {
		t.Fatalf("C: %q", body)
	}
	// A cookie from one session never authorizes another, even replayed by
	// hand against the other session's host (same pod or not).
	for _, x := range []struct {
		name, host string
		ck         *http.Cookie
	}{
		{"C's cookie on B", b.host, ckC},
		{"B's cookie on C", c.host, ckB},
		{"no cookie on B", b.host, nil},
		{"forged cookie (the session id)", b.host, &http.Cookie{Name: podProxyCookieName, Value: b.sid}},
		{"secure-name cookie", b.host, &http.Cookie{Name: podProxyHostCookieName, Value: ckB.Value}},
	} {
		res := e.hostDo(t, x.host, "GET", "/", x.ck, nil, nil)
		readAll(t, res)
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("%s: %d, want 403", x.name, res.StatusCode)
		}
	}

	// Hosts that are not a live session: 404 with an empty body, no CORS,
	// policy headers on.
	unknown := auth.NewPodProxySessionID()
	for _, h := range []string{
		"pods.test:" + e.port,
		"127.0.0.1:" + e.port,
		"localhost:" + e.port,
		unknown + ".pods.test:" + e.port,
		b.sid + ".pods.test:1",
		b.sid + ".pods.test",
		b.sid + ".other.test:" + e.port,
		"x." + b.sid + ".pods.test:" + e.port,
		b.sid[:31] + ".pods.test:" + e.port,
		"axiom.test:5090",
	} {
		res := e.hostDo(t, h, "GET", "/", ckB, nil, map[string]string{"Authorization": basic(idAdminUser, idAdminPass)})
		body := readAll(t, res)
		if res.StatusCode != http.StatusNotFound || body != "" {
			t.Errorf("Host %s: %d %q, want an empty 404", h, res.StatusCode, body)
		}
		if res.Header.Get("Access-Control-Allow-Origin") != "" || res.Header.Get("X-Frame-Options") != "DENY" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("Host %s: headers %v", h, res.Header)
		}
	}
	// The path-mode routes do not exist in host mode.
	for _, p := range []string{"/s/" + b.sid + "/", "/api/v1/session"} {
		res := e.hostDo(t, "pods.test:"+e.port, "GET", p, ckB, nil, nil)
		readAll(t, res)
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("%s on the bare base host: %d", p, res.StatusCode)
		}
	}
	// A redeem attempt with a made-up ticket on an unused host is refused.
	res = e.hostDo(t, unknown+".pods.test:"+e.port, "GET", "/p/"+auth.RandomURLToken(32)+"/", nil, nil, nil)
	readAll(t, res)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("made-up ticket: %d", res.StatusCode)
	}
}

func TestPodProxyHostTicketRules(t *testing.T) {
	e := newPPHostEnv(t, nil)
	l := e.linkHost(t, "oper", "team-a", "web", 8080, "/")
	// POST and HEAD do not redeem (nor consume) the ticket.
	for _, m := range []string{"POST", "HEAD"} {
		res := e.hostDo(t, l.host, m, l.ticketPath, nil, nil, nil)
		readAll(t, res)
		if res.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s /p/: %d", m, res.StatusCode)
		}
	}
	_, ck := e.redeemHost(t, l)
	// A ticket minted without a session id (fixed-origin form) is refused.
	sid := auth.NewPodProxySessionID()
	tok, _, err := e.pp.tickets.Issue(auth.PodProxyGrant{User: "oper", Role: "operator", Namespace: "team-a", Pod: "web", Port: 8080, Path: "/"})
	if err != nil {
		t.Fatal(err)
	}
	res := e.hostDo(t, sid+".pods.test:"+e.port, "GET", "/p/"+tok+"/", nil, nil, nil)
	readAll(t, res)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("unbound ticket: %d", res.StatusCode)
	}
	// Methods and service workers.
	res = e.hostDo(t, l.host, "OPTIONS", "/", ck, nil, nil)
	readAll(t, res)
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("OPTIONS: %d", res.StatusCode)
	}
	res = e.hostDo(t, l.host, "GET", "/sw.js", ck, nil, map[string]string{"Service-Worker": "script"})
	readAll(t, res)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("service worker script: %d", res.StatusCode)
	}
	res = e.hostDo(t, l.host, "POST", "/form", ck, strings.NewReader("a=1"), map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if b := readAll(t, res); res.StatusCode != 200 || b != "hello from web POST /form" {
		t.Fatalf("POST: %d %q", res.StatusCode, b)
	}
	// Body cap.
	req, _ := http.NewRequest("POST", e.proxy.URL+"/upload", io.NopCloser(io.LimitReader(neverEnding('x'), podProxyMaxBody+1)))
	req.Host = l.host
	req.ContentLength = podProxyMaxBody + 1
	req.AddCookie(ck)
	if res, err := http.DefaultClient.Do(req); err == nil {
		readAll(t, res)
		if res.StatusCode != http.StatusRequestEntityTooLarge {
			t.Fatalf("oversize body: %d", res.StatusCode)
		}
	}
}

func TestPodProxyHostRevokedUserSessionDies(t *testing.T) {
	e := newPPHostEnv(t, nil)
	l := e.linkHost(t, "oper", "team-a", "web", 8080, "/")
	_, ck := e.redeemHost(t, l)
	pending := e.linkHost(t, "oper", "team-a", "web", 8080, "/")
	other := e.linkHost(t, "oper2", "team-a", "web", 8080, "/")
	_, ckOther := e.redeemHost(t, other)
	if res := e.hostDo(t, l.host, "GET", "/", ck, nil, nil); readAll(t, res) == "" || res.StatusCode != 200 {
		t.Fatalf("before revoke: %d", res.StatusCode)
	}
	time.Sleep(2 * time.Millisecond)
	if res, b := e.consoleDo(t, "POST", "/api/v1/auth/logout-all", "", "oper"); res.StatusCode != 200 {
		t.Fatalf("logout-all: %d %s", res.StatusCode, b)
	}
	res := e.hostDo(t, l.host, "GET", "/", ck, nil, nil)
	readAll(t, res)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("revoked user's session still proxies: %d", res.StatusCode)
	}
	// The session is gone: its host is now unknown.
	res = e.hostDo(t, l.host, "GET", "/", ck, nil, nil)
	readAll(t, res)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("revoked session's host: %d", res.StatusCode)
	}
	res = e.hostDo(t, pending.host, "GET", pending.ticketPath, nil, nil, nil)
	readAll(t, res)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("pre-revocation ticket redeemed: %d", res.StatusCode)
	}
	// Another user's session is unaffected.
	if res := e.hostDo(t, other.host, "GET", "/", ckOther, nil, nil); readAll(t, res) == "" || res.StatusCode != 200 {
		t.Fatalf("other user's session: %d", res.StatusCode)
	}
	// A role change revokes too.
	time.Sleep(2 * time.Millisecond)
	if _, err := e.users.UpdateRole(context.Background(), "oper2", "viewer"); err != nil {
		t.Fatal(err)
	}
	res = e.hostDo(t, other.host, "GET", "/", ckOther, nil, nil)
	readAll(t, res)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("session survived a role change: %d", res.StatusCode)
	}
}

type ppClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *ppClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *ppClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func TestPodProxyHostExpiryAndCaps(t *testing.T) {
	clk := &ppClock{t: time.Now()}
	e := newPPHostEnv(t, func(p *PodProxyOrigin) {
		p.sess = auth.NewPodProxySessionStoreWithClock(clk.now, auth.PodProxySessionIdle, auth.PodProxySessionMax)
	})
	get := func(l hostLink, ck *http.Cookie) int {
		res := e.hostDo(t, l.host, "GET", "/", ck, nil, nil)
		readAll(t, res)
		return res.StatusCode
	}

	// Sliding idle expiry.
	l := e.linkHost(t, "oper", "team-a", "web", 8080, "/")
	_, ck := e.redeemHost(t, l)
	clk.advance(auth.PodProxySessionIdle - time.Minute)
	if s := get(l, ck); s != 200 {
		t.Fatalf("within idle: %d", s)
	}
	clk.advance(auth.PodProxySessionIdle - time.Minute)
	if s := get(l, ck); s != 200 {
		t.Fatalf("idle expiry must slide: %d", s)
	}
	clk.advance(auth.PodProxySessionIdle + time.Second)
	if s := get(l, ck); s != http.StatusNotFound {
		t.Fatalf("idle-expired session: %d, want 404", s)
	}

	// Absolute maximum, however active.
	l = e.linkHost(t, "oper", "team-a", "web", 8080, "/")
	_, ck = e.redeemHost(t, l)
	for elapsed := time.Duration(0); elapsed+9*time.Minute < auth.PodProxySessionMax; elapsed += 9 * time.Minute {
		clk.advance(9 * time.Minute)
		if s := get(l, ck); s != 200 {
			t.Fatalf("active session at %v: %d", elapsed+9*time.Minute, s)
		}
	}
	clk.advance(9 * time.Minute)
	if s := get(l, ck); s != http.StatusNotFound {
		t.Fatalf("session past its maximum: %d, want 404", s)
	}

	// Per-user session cap (tickets are consumed on redeem, so links keep coming).
	clk.advance(2 * auth.PodProxySessionMax)
	var last int
	for i := 0; i < 40; i++ {
		l := e.linkHost(t, "oper", "team-a", "web", 8080, "/")
		res := e.hostDo(t, l.host, "GET", l.ticketPath, nil, nil, nil)
		readAll(t, res)
		last = res.StatusCode
		if last != http.StatusFound {
			break
		}
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("per-user session cap: last status %d", last)
	}
	// Another user is not affected by oper's cap.
	l = e.linkHost(t, "oper2", "team-a", "web", 8080, "/")
	e.redeemHost(t, l)
}

func TestPodProxyHostWindowLinkPolicy(t *testing.T) {
	e := newPPHostEnv(t, nil)
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
		{"scheme-relative path", "oper", "team-a", `{"port":8080,"path":"//evil.example/"}`, 400},
		{"unknown field", "oper", "team-a", `{"port":8080,"host":"x"}`, 400},
		{"undeclared port", "oper", "team-a", `{"port":22}`, 404},
	}
	for _, c := range cases {
		res, body := e.consoleDo(t, "POST", "/api/v1/pods/"+c.ns+"/web/window-link", c.body, c.user)
		if res.StatusCode != c.want {
			t.Errorf("%s: %d %s, want %d", c.name, res.StatusCode, body, c.want)
		}
		if res.StatusCode == 200 {
			if res.Header.Get("Cache-Control") != "no-store" {
				t.Errorf("%s: link response must not be cached", c.name)
			}
			if !regexp.MustCompile(`"url":"http://[0-9a-f]{32}\.pods\.test:` + e.port + `/p/`).MatchString(body) {
				t.Errorf("%s: link %s", c.name, body)
			}
		}
	}
	// A restricted operator's link reaches its pod, on its own host.
	l := e.linkHost(t, "roper", "team-a", "web", 8080, "/")
	_, ck := e.redeemHost(t, l)
	if b := readAll(t, e.hostDo(t, l.host, "GET", "/", ck, nil, nil)); b != "hello from web GET /" {
		t.Fatalf("restricted operator proxy: %q", b)
	}
	// Capabilities report the mode; the in-origin route is gone.
	if _, b := e.consoleDo(t, "GET", "/api/v1/realtime/capabilities", "", "oper"); !strings.Contains(b, `"podProxy":true`) || !strings.Contains(b, `"podProxyMode":"host"`) {
		t.Fatalf("capabilities: %s", b)
	}
	if res, _ := e.consoleDo(t, "GET", "/api/v1/pods/team-a/web/window/8080/", "", "oper"); res.StatusCode != http.StatusGone {
		t.Fatalf("window route in host mode: %d", res.StatusCode)
	}
}

func TestPodProxyModeCapability(t *testing.T) {
	if _, b := newPPEnv(t, true).consoleDo(t, "GET", "/api/v1/realtime/capabilities", "", "oper"); !strings.Contains(b, `"podProxyMode":"path"`) {
		t.Fatalf("path mode: %s", b)
	}
	if _, b := newPPEnv(t, false).consoleDo(t, "GET", "/api/v1/realtime/capabilities", "", "oper"); strings.Contains(b, "podProxyMode") {
		t.Fatalf("off: %s", b)
	}
}

func TestPodProxyHostSecureCookie(t *testing.T) {
	pp, err := NewPodProxyOrigin(PodProxyOriginConfig{Addr: ":5091", Origin: "https://{id}.pods.example.com", ConsoleAddr: ":5090", ConsoleOrigin: "https://axiom.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	pp.attach(platform.NewDemoProvider(), revocationsChecker{auth.NewRevocations(nil)})
	sid := auth.NewPodProxySessionID()
	tok, _, err := pp.tickets.Issue(auth.PodProxyGrant{User: "oper", Role: "operator", Namespace: "ns", Pod: "p", Port: 80, Path: "/", SessionID: sid})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "https://x/p/"+tok+"/", nil)
	r.Host = sid + ".pods.example.com"
	pp.Handler().ServeHTTP(w, r)
	cks := w.Result().Cookies()
	if w.Code != http.StatusFound || len(cks) != 1 || w.Header().Get("Location") != "/" {
		t.Fatalf("redeem: %d %v %q", w.Code, cks, w.Header().Get("Location"))
	}
	c := cks[0]
	if c.Name != "__Host-axiom_pod_proxy" || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Domain != "" {
		t.Fatalf("https cookie: %+v", c)
	}
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("https proxy origin should send HSTS")
	}
	// The cookie authorizes the session on its host (the demo provider has no
	// proxy target, so the request gets past the session check to the lookup).
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "https://x/", nil)
	r.Host = sid + ".pods.example.com:443"
	r.AddCookie(c)
	pp.Handler().ServeHTTP(w, r)
	if w.Code == http.StatusForbidden || w.Code == http.StatusNotFound && w.Body.Len() == 0 {
		t.Fatalf("session on :443: %d %q", w.Code, w.Body.String())
	}
	// The plain name is not accepted on https.
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "https://x/", nil)
	r.Host = sid + ".pods.example.com"
	r.AddCookie(&http.Cookie{Name: podProxyCookieName, Value: c.Value})
	pp.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("plain cookie name on https: %d", w.Code)
	}
}

func TestPodProxyTLSFiles(t *testing.T) {
	for _, c := range []struct{ oc, ok, cc, ck, wc, wk string }{
		{"p.crt", "p.key", "c.crt", "c.key", "p.crt", "p.key"},
		{"", "", "c.crt", "c.key", "c.crt", "c.key"},
		{"p.crt", "", "c.crt", "c.key", "c.crt", "c.key"},
		{"p.crt", "p.key", "", "", "p.crt", "p.key"},
		{"", "", "", "", "", ""},
	} {
		if gc, gk := PodProxyTLSFiles(c.oc, c.ok, c.cc, c.ck); gc != c.wc || gk != c.wk {
			t.Errorf("%+v: %q %q", c, gc, gk)
		}
	}
}
