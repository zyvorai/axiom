package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
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

// Two replicas of Axiom (console + pod proxy listener each) that share the
// session secret, the revocation store, the redeemed-ticket store and the
// user store, as an HA install does, with no session affinity: a request may
// land on either.

type ppReplica struct {
	console *httptest.Server
	proxy   *httptest.Server
	pp      *PodProxyOrigin
	rev     *auth.Revocations
}

type ppCluster struct {
	a, b    *ppReplica
	users   *platform.UserHub
	webSeen *seen
	port    string // the public proxy port (host mode Host headers)
	host    bool
}

func newPPCluster(t *testing.T, host bool, secret string, tweak func(*PodProxyOrigin)) *ppCluster {
	t.Helper()
	web, webSeen := newPodBackend(t, "web")
	prov := ppProvider{DemoProvider: platform.NewDemoProvider(), targets: map[string]string{
		"team-a/web/8080": web.URL,
		"team-b/web/8080": web.URL,
	}}
	c := &ppCluster{users: platform.NewUserHub(nil), webSeen: webSeen, host: host}
	revStore, redeemed := &auth.MemoryMergingStore{}, &auth.MemoryMergingStore{}
	var handlers [2]http.Handler
	var mu sync.RWMutex
	mk := func(i int) *ppReplica {
		r := &ppReplica{}
		r.proxy = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			mu.RLock()
			h := handlers[i]
			mu.RUnlock()
			h.ServeHTTP(w, req)
		}))
		t.Cleanup(r.proxy.Close)
		return r
	}
	c.a, c.b = mk(0), mk(1)
	pu, _ := url.Parse(c.a.proxy.URL)
	c.port = pu.Port()
	// Both listeners advertise the same public origin (replica A's address
	// stands in for the load balancer).
	origin := c.a.proxy.URL
	if host {
		origin = "http://{id}.pods.test:" + c.port
	}
	for i, r := range []*ppReplica{c.a, c.b} {
		pp, err := NewPodProxyOrigin(PodProxyOriginConfig{Addr: "127.0.0.1:0", Origin: origin, ConsoleAddr: ":5090",
			ConsoleOrigin: "http://axiom.test:5090", SessionSecret: secret, RedeemedStore: redeemed})
		if err != nil {
			t.Fatal(err)
		}
		if tweak != nil {
			tweak(pp)
		}
		r.pp = pp
		r.rev = auth.NewRevocations(revStore)
		mu.Lock()
		handlers[i] = pp.Handler()
		mu.Unlock()
		r.console = httptest.NewServer(NewWithUsers(prov, "demo", idAdminUser, idAdminPass, "", nil, c.users,
			AuthOptions{SessionSecret: "console-" + secret, Revocations: r.rev, PodProxy: pp}))
		t.Cleanup(r.console.Close)
	}
	for _, u := range []struct {
		name, role string
		ns         []string
	}{
		{"oper", "operator", nil},
		{"oper2", "operator", nil},
		{"roper", "operator", []string{"team-a"}},
	} {
		if _, err := c.users.Create(context.Background(), u.name, u.name+"-password", u.role, u.ns); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

// link mints a window link on r's console and returns the ticket path
// ("/p/<ticket>/") and, in host mode, the session's Host header.
func (c *ppCluster) link(t *testing.T, r *ppReplica, user, ns string) (ticketPath, host string) {
	t.Helper()
	req, _ := http.NewRequest("POST", r.console.URL+"/api/v1/pods/"+ns+"/web/window-link", strings.NewReader(`{"port":8080,"path":"/dash"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", basic(user, user+"-password"))
	res, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b := readAll(t, res)
	if res.StatusCode != 200 {
		t.Fatalf("window-link on %s as %s: %d %s", r.console.URL, user, res.StatusCode, b)
	}
	var out struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal([]byte(b), &out)
	if c.host {
		m := regexp.MustCompile(`^http://([0-9a-f]{32}\.pods\.test:` + c.port + `)(/p/[A-Za-z0-9_-]+/)$`).FindStringSubmatch(out.URL)
		if m == nil {
			t.Fatalf("host link %q", out.URL)
		}
		return m[2], m[1]
	}
	if !regexp.MustCompile(`^` + regexp.QuoteMeta(c.a.proxy.URL) + `/p/[A-Za-z0-9_-]+/$`).MatchString(out.URL) {
		t.Fatalf("path link %q", out.URL)
	}
	return strings.TrimPrefix(out.URL, c.a.proxy.URL), ""
}

func (c *ppCluster) do(t *testing.T, r *ppReplica, host, path string, ck *http.Cookie) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", r.proxy.URL+path, nil)
	if host != "" {
		req.Host = host
	}
	if ck != nil {
		req.AddCookie(ck)
	}
	res, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func ppSessionCookie(res *http.Response) *http.Cookie {
	for _, ck := range res.Cookies() {
		if ck.Name == podProxyCookieName {
			return ck
		}
	}
	return nil
}

// redeem opens the ticket on r and returns the session's first path and cookie.
func (c *ppCluster) redeem(t *testing.T, r *ppReplica, ticketPath, host string) (string, *http.Cookie) {
	t.Helper()
	res := c.do(t, r, host, ticketPath, nil)
	b := readAll(t, res)
	ck := ppSessionCookie(res)
	if res.StatusCode != http.StatusFound || ck == nil {
		t.Fatalf("redeem on %s: %d %s %v", r.proxy.URL, res.StatusCode, b, res.Header["Set-Cookie"])
	}
	return res.Header.Get("Location"), ck
}

func tamper(v string) string {
	raw, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil || len(raw) == 0 {
		return v + "x"
	}
	raw[len(raw)/2] ^= 1
	return base64.RawURLEncoding.EncodeToString(raw)
}

func TestPodProxyReplicasPathMode(t *testing.T) {
	c := newPPCluster(t, false, "shared-secret", nil)
	if !c.a.pp.Shared() || !c.b.pp.Shared() {
		t.Fatal("a session secret must make the pod proxy shared")
	}
	tp, _ := c.link(t, c.a, "oper", "team-a")

	// Minted on A, redeemed on B.
	loc, ck := c.redeem(t, c.b, tp, "")
	if !strings.HasPrefix(loc, "/s/") || !strings.HasSuffix(loc, "/dash") {
		t.Fatalf("redirect %q", loc)
	}
	sid := sidOf(loc)
	if ck.Path != "/s/"+sid+"/" || !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie %+v", ck)
	}
	// Single use, on either replica.
	for _, r := range []*ppReplica{c.a, c.b} {
		res := c.do(t, r, "", tp, nil)
		readAll(t, res)
		if res.StatusCode != http.StatusForbidden || ppSessionCookie(res) != nil {
			t.Fatalf("second redeem on %s: %d", r.proxy.URL, res.StatusCode)
		}
	}
	// The cookie from B is accepted by A (and B), with the usual hardening.
	for _, r := range []*ppReplica{c.a, c.b} {
		res := c.do(t, r, "", loc, ck)
		if b := readAll(t, res); res.StatusCode != 200 || b != "hello from web GET /dash" {
			t.Fatalf("session on %s: %d %q", r.proxy.URL, res.StatusCode, b)
		}
		if len(res.Header.Values("Set-Cookie")) != 0 || res.Header.Get("Content-Security-Policy") != podProxyCSP || res.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("hardening on %s: %v", r.proxy.URL, res.Header)
		}
		if got, _ := c.webSeen.last(); got.Header.Get("Cookie") != "" {
			t.Fatalf("cookie reached the pod: %q", got.Header.Get("Cookie"))
		}
	}
	// Tampered, misplaced or foreign tokens are refused.
	other, ck2 := c.redeem(t, c.a, mustLink(t, c, c.b, "oper"), "")
	for name, x := range map[string]struct {
		path string
		ck   *http.Cookie
	}{
		"tampered cookie":           {loc, &http.Cookie{Name: podProxyCookieName, Value: tamper(ck.Value)}},
		"cookie of another session": {loc, ck2},
		"cookie on another path":    {other, ck},
		"session id as cookie":      {loc, &http.Cookie{Name: podProxyCookieName, Value: sid}},
		"no cookie":                 {loc, nil},
	} {
		for _, r := range []*ppReplica{c.a, c.b} {
			res := c.do(t, r, "", x.path, x.ck)
			readAll(t, res)
			if res.StatusCode != http.StatusForbidden {
				t.Errorf("%s on %s: %d", name, r.proxy.URL, res.StatusCode)
			}
		}
	}
	tp3, _ := c.link(t, c.a, "oper", "team-a")
	bad := "/p/" + tamper(strings.TrimSuffix(strings.TrimPrefix(tp3, "/p/"), "/")) + "/"
	if res := c.do(t, c.b, "", bad, nil); readAll(t, res) == "" || res.StatusCode != http.StatusForbidden {
		t.Fatalf("tampered ticket: %d", res.StatusCode)
	}
	c.redeem(t, c.b, tp3, "") // the real one was not burnt by the forgery

	// Revocation on A reaches B through the shared store.
	pending, _ := c.link(t, c.b, "oper", "team-a")
	time.Sleep(2 * time.Millisecond)
	res, err := func() (*http.Response, error) {
		req, _ := http.NewRequest("POST", c.a.console.URL+"/api/v1/auth/logout-all", nil)
		req.Header.Set("Authorization", basic("oper", "oper-password"))
		return http.DefaultClient.Do(req)
	}()
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("logout-all: %v %v", res, err)
	}
	readAll(t, res)
	if err := c.b.rev.Refresh(); err != nil {
		t.Fatal(err)
	}
	for _, r := range []*ppReplica{c.a, c.b} {
		res := c.do(t, r, "", loc, ck)
		readAll(t, res)
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("revoked user's session on %s: %d", r.proxy.URL, res.StatusCode)
		}
	}
	if res := c.do(t, c.a, "", pending, nil); readAll(t, res) == "" || res.StatusCode != http.StatusForbidden {
		t.Fatalf("pre-revocation ticket redeemed: %d", res.StatusCode)
	}
}

func mustLink(t *testing.T, c *ppCluster, r *ppReplica, user string) string {
	t.Helper()
	tp, _ := c.link(t, r, user, "team-a")
	return tp
}

func TestPodProxyReplicasHostMode(t *testing.T) {
	c := newPPCluster(t, true, "shared-secret", nil)
	tp, host := c.link(t, c.a, "oper", "team-a")
	if res := c.do(t, c.b, host, "/", nil); readAll(t, res) != "" || res.StatusCode != http.StatusNotFound {
		t.Fatalf("before redeem: %d", res.StatusCode)
	}
	loc, ck := c.redeem(t, c.b, tp, host)
	if loc != "/dash" || ck.Path != "/" || ck.Domain != "" {
		t.Fatalf("redeem: %q %+v", loc, ck)
	}
	// Second redeem refused on either replica: A has no local session and no
	// cookie came with the request, so it is a redeem attempt and fails.
	for _, r := range []*ppReplica{c.a, c.b} {
		res := c.do(t, r, host, tp, nil)
		readAll(t, res)
		if res.StatusCode != http.StatusForbidden || ppSessionCookie(res) != nil {
			t.Fatalf("second redeem on %s: %d", r.proxy.URL, res.StatusCode)
		}
	}
	// B's cookie works on A at the host root, and /p/ is a pod path there.
	for _, p := range []string{"/", "/static/app.js", tp} {
		res := c.do(t, c.a, host, p, ck)
		if b := readAll(t, res); res.StatusCode != 200 || b != "hello from web GET "+p {
			t.Fatalf("%s on A: %d %q", p, res.StatusCode, b)
		}
	}
	// A replica that did not create the session knows nothing without a
	// valid cookie: empty 404, like an unknown host.
	for name, x := range map[string]*http.Cookie{
		"no cookie":       nil,
		"tampered cookie": {Name: podProxyCookieName, Value: tamper(ck.Value)},
	} {
		res := c.do(t, c.a, host, "/", x)
		if b := readAll(t, res); res.StatusCode != http.StatusNotFound || b != "" {
			t.Errorf("%s on A: %d %q", name, res.StatusCode, b)
		}
	}
	// Another session's cookie on this host is refused on both replicas.
	tp2, host2 := c.link(t, c.b, "oper", "team-a")
	_, ck2 := c.redeem(t, c.a, tp2, host2)
	for _, r := range []*ppReplica{c.a, c.b} {
		res := c.do(t, r, host, "/", ck2)
		readAll(t, res)
		if res.StatusCode == 200 {
			t.Fatalf("cookie of %s accepted on %s via %s", host2, host, r.proxy.URL)
		}
	}
	// Ticket bound to another host is refused and burnt everywhere.
	tp3, host3 := c.link(t, c.a, "oper", "team-a")
	if res := c.do(t, c.b, host2, tp3, nil); readAll(t, res) == "" || res.StatusCode != http.StatusForbidden {
		t.Fatalf("ticket on a foreign live host: %d", res.StatusCode)
	}
	unused := auth.NewPodProxySessionID() + ".pods.test:" + c.port
	if res := c.do(t, c.b, unused, tp3, nil); readAll(t, res) == "" || res.StatusCode != http.StatusForbidden {
		t.Fatalf("ticket on a foreign host: %d", res.StatusCode)
	}
	if res := c.do(t, c.a, host3, tp3, nil); readAll(t, res) == "" || res.StatusCode != http.StatusForbidden {
		t.Fatalf("burnt ticket on its own host: %d", res.StatusCode)
	}

	// Revoked user refused on both replicas: the cut-off is made on A and
	// reaches B through the shared revocation store.
	time.Sleep(2 * time.Millisecond)
	c.a.rev.RevokeUser("oper")
	if err := c.b.rev.Refresh(); err != nil {
		t.Fatal(err)
	}
	for _, r := range []*ppReplica{c.a, c.b} {
		res := c.do(t, r, host, "/", ck)
		readAll(t, res)
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("revoked user's session on %s: %d", r.proxy.URL, res.StatusCode)
		}
	}
}

func TestPodProxyReplicasExpiry(t *testing.T) {
	clk := &ppClock{t: time.Now()}
	c := newPPCluster(t, true, "shared-secret", func(p *PodProxyOrigin) {
		p.tickets = auth.NewSharedPodProxyTicketStoreWithClock("shared-secret", auth.NewPodProxyRedeemedWithClock(nil, clk.now), clk.now, auth.PodProxyTicketTTL)
		p.sess = auth.NewSharedPodProxySessionStoreWithClock("shared-secret", clk.now, auth.PodProxySessionIdle, auth.PodProxySessionMax)
	})
	get := func(r *ppReplica, host string, ck *http.Cookie) (int, *http.Cookie) {
		res := c.do(t, r, host, "/", ck)
		readAll(t, res)
		return res.StatusCode, ppSessionCookie(res)
	}
	// An expired ticket is refused on the other replica.
	tp, host := c.link(t, c.a, "oper", "team-a")
	clk.advance(auth.PodProxyTicketTTL)
	if res := c.do(t, c.b, host, tp, nil); readAll(t, res) == "" || res.StatusCode != http.StatusForbidden {
		t.Fatalf("expired ticket: %d", res.StatusCode)
	}

	tp, host = c.link(t, c.a, "oper", "team-a")
	_, ck := c.redeem(t, c.a, tp, host)
	// B slides the session by re-issuing the cookie (same attributes).
	clk.advance(5 * time.Minute)
	code, renewed := get(c.b, host, ck)
	if code != 200 || renewed == nil || renewed.Value == ck.Value || renewed.Path != "/" || !renewed.HttpOnly ||
		renewed.MaxAge <= 0 || renewed.MaxAge > int(auth.PodProxySessionMax/time.Second) {
		t.Fatalf("renewal: %d %+v", code, renewed)
	}
	if code, again := get(c.b, host, renewed); code != 200 || again != nil {
		t.Fatalf("a fresh cookie is not re-issued: %d %v", code, again)
	}
	clk.advance(6 * time.Minute)
	// The first cookie has passed its signed idle deadline: B (which never
	// held the session) treats the host as unknown, A (idle there for 11
	// minutes) too.
	for _, r := range []*ppReplica{c.a, c.b} {
		if code, _ := get(r, host, ck); code != http.StatusNotFound {
			t.Fatalf("idle-expired cookie on %s: %d", r.proxy.URL, code)
		}
	}
	if code, _ := get(c.a, host, renewed); code != 200 {
		t.Fatalf("renewed cookie on A: %d", code)
	}
	// Absolute maximum, however active.
	cur := renewed
	for elapsed := 11 * time.Minute; elapsed+3*time.Minute < auth.PodProxySessionMax; elapsed += 3 * time.Minute {
		clk.advance(3 * time.Minute)
		code, nx := get(c.b, host, cur)
		if code != 200 {
			t.Fatalf("active session at %v: %d", elapsed+3*time.Minute, code)
		}
		if nx != nil {
			cur = nx
		}
	}
	clk.advance(3 * time.Minute)
	for _, r := range []*ppReplica{c.a, c.b} {
		if code, _ := get(r, host, cur); code != http.StatusNotFound {
			t.Fatalf("past the maximum on %s: %d", r.proxy.URL, code)
		}
	}
}

func TestPodProxyReplicasNamespaceRestriction(t *testing.T) {
	c := newPPCluster(t, false, "shared-secret", nil)
	// The restriction snapshot travels in the token; the console refuses the
	// other namespace up front.
	req, _ := http.NewRequest("POST", c.a.console.URL+"/api/v1/pods/team-b/web/window-link", strings.NewReader(`{"port":8080}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", basic("roper", "roper-password"))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	readAll(t, res)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("restricted operator, other namespace: %d", res.StatusCode)
	}
	tp, _ := c.link(t, c.a, "roper", "team-a")
	loc, ck := c.redeem(t, c.b, tp, "")
	if res := c.do(t, c.a, "", loc, ck); readAll(t, res) == "" || res.StatusCode != 200 {
		t.Fatalf("restricted operator, own namespace: %d", res.StatusCode)
	}
}

func TestPodProxyReplicasWithoutSecretStayLocal(t *testing.T) {
	// Without a session secret the stores are in memory, exactly as before: a
	// ticket only redeems on the replica that minted it.
	c := newPPCluster(t, false, "", nil)
	if c.a.pp.Shared() {
		t.Fatal("no secret must mean in-memory stores")
	}
	tp, _ := c.link(t, c.a, "oper", "team-a")
	if res := c.do(t, c.b, "", tp, nil); readAll(t, res) == "" || res.StatusCode != http.StatusForbidden {
		t.Fatalf("ticket of A on B: %d", res.StatusCode)
	}
	tp, _ = c.link(t, c.a, "oper", "team-a")
	loc, ck := c.redeem(t, c.a, tp, "")
	if res := c.do(t, c.b, "", loc, ck); readAll(t, res) == "" || res.StatusCode != http.StatusForbidden {
		t.Fatalf("session of A on B: %d", res.StatusCode)
	}
	if res := c.do(t, c.a, "", loc, ck); readAll(t, res) == "" || res.StatusCode != 200 {
		t.Fatalf("session on A: %d", res.StatusCode)
	}
}
