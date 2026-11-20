package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

// A pod proxy session is pinned to the cluster its pod was resolved on when
// the link was minted: console tabs switching clusters, a replica's own
// default and other replicas' selections must not re-target it.

type ppMultiReplica struct {
	console *httptest.Server
	proxy   *httptest.Server
	pp      *PodProxyOrigin
	sp      *platform.SwitchableProvider
}

type ppMulti struct {
	r      []*ppMultiReplica
	host   bool
	port   string
	origin string
}

// ppClusterProvider is one cluster whose pod team-a/web:8080 is backend.
func ppClusterProvider(backend string) platform.Provider {
	return ppProvider{DemoProvider: platform.NewDemoProvider(), targets: map[string]string{"team-a/web/8080": backend}}
}

// newPPMulti builds n replicas, each with its own cluster registry holding
// "east" (registered first, so the default) and "west". secret "" keeps the
// pod proxy in memory (n must then be 1).
func newPPMulti(t *testing.T, n int, secret string, host bool) *ppMulti {
	t.Helper()
	east, _ := newPodBackend(t, "east")
	west, _ := newPodBackend(t, "west")
	m := &ppMulti{host: host}
	handlers := make([]http.Handler, n)
	var mu sync.RWMutex
	for i := 0; i < n; i++ {
		i := i
		r := &ppMultiReplica{}
		r.proxy = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			mu.RLock()
			h := handlers[i]
			mu.RUnlock()
			h.ServeHTTP(w, req)
		}))
		t.Cleanup(r.proxy.Close)
		m.r = append(m.r, r)
	}
	pu, _ := url.Parse(m.r[0].proxy.URL)
	m.port = pu.Port()
	m.origin = m.r[0].proxy.URL
	if host {
		m.origin = "http://{id}.pods.test:" + m.port
	}
	revStore, redeemed := &auth.MemoryMergingStore{}, &auth.MemoryMergingStore{}
	for i, r := range m.r {
		pp, err := NewPodProxyOrigin(PodProxyOriginConfig{Addr: "127.0.0.1:0", Origin: m.origin, ConsoleAddr: ":5090",
			ConsoleOrigin: "http://axiom.test:5090", SessionSecret: secret, RedeemedStore: redeemed})
		if err != nil {
			t.Fatal(err)
		}
		r.pp = pp
		r.sp = platform.NewSwitchableProvider()
		r.sp.Register("east", "east", ppClusterProvider(east.URL), "test://east")
		r.sp.Register("west", "west", ppClusterProvider(west.URL), "test://west")
		mu.Lock()
		handlers[i] = pp.Handler()
		mu.Unlock()
		r.console = httptest.NewServer(NewWithUsers(r.sp, "demo", idAdminUser, idAdminPass, "", nil, platform.NewUserHub(nil),
			AuthOptions{SessionSecret: "console-" + secret, Revocations: auth.NewRevocations(revStore), PodProxy: pp}))
		t.Cleanup(r.console.Close)
	}
	return m
}

func (m *ppMulti) console(t *testing.T, r *ppMultiReplica, method, path, tab, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, r.console.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", basic(idAdminUser, idAdminPass))
	if tab != "" {
		req.Header.Set(platform.ClusterSessionHeader, tab)
	}
	res, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, readAll(t, res)
}

// activate selects cluster for console tab tab on replica r.
func (m *ppMulti) activate(t *testing.T, r *ppMultiReplica, tab, cluster string) {
	t.Helper()
	if code, b := m.console(t, r, "POST", "/api/v1/clusters/"+cluster+"/activate", tab, ""); code != 200 {
		t.Fatalf("activate %s: %d %s", cluster, code, b)
	}
}

// link mints a window link for team-a/web:8080 from tab tab on replica r and
// returns the ticket path and (host mode) the session's Host header.
func (m *ppMulti) link(t *testing.T, r *ppMultiReplica, tab string) (string, string) {
	t.Helper()
	code, b := m.console(t, r, "POST", "/api/v1/pods/team-a/web/window-link", tab, `{"port":8080,"path":"/dash"}`)
	if code != 200 {
		t.Fatalf("window-link: %d %s", code, b)
	}
	var out struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal([]byte(b), &out)
	if m.host {
		mm := regexp.MustCompile(`^http://([0-9a-f]{32}\.pods\.test:` + m.port + `)(/p/[A-Za-z0-9_-]+/)$`).FindStringSubmatch(out.URL)
		if mm == nil {
			t.Fatalf("host link %q", out.URL)
		}
		return mm[2], mm[1]
	}
	if !strings.HasPrefix(out.URL, m.origin+"/p/") {
		t.Fatalf("path link %q", out.URL)
	}
	return strings.TrimPrefix(out.URL, m.origin), ""
}

func (m *ppMulti) do(t *testing.T, r *ppMultiReplica, host, path string, ck *http.Cookie) (int, string, *http.Response) {
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
	return res.StatusCode, readAll(t, res), res
}

// redeem opens a ticket on r and returns the session's page path and cookie.
func (m *ppMulti) redeem(t *testing.T, r *ppMultiReplica, ticketPath, host string) (string, *http.Cookie) {
	t.Helper()
	code, b, res := m.do(t, r, host, ticketPath, nil)
	ck := ppSessionCookie(res)
	if code != http.StatusFound || ck == nil {
		t.Fatalf("redeem: %d %s", code, b)
	}
	return res.Header.Get("Location"), ck
}

// get fetches the session page on r and returns status and body.
func (m *ppMulti) get(t *testing.T, r *ppMultiReplica, host, loc string, ck *http.Cookie) (int, string) {
	t.Helper()
	code, b, _ := m.do(t, r, host, loc, ck)
	return code, b
}

func TestPodProxyPinnedToClusterAcrossSwitch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		secret string
		host   bool
	}{
		{"memory path mode", "", false},
		{"memory host mode", "", true},
		{"shared path mode", "shared-secret", false},
		{"shared host mode", "shared-secret", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newPPMulti(t, 1, tc.secret, tc.host)
			a := m.r[0]
			tp, host := m.link(t, a, "tab1") // tab1 is on east (the default)
			loc, ck := m.redeem(t, a, tp, host)
			if code, b := m.get(t, a, host, loc, ck); code != 200 || b != "hello from east GET /dash" {
				t.Fatalf("before switching: %d %q", code, b)
			}
			// The tab switches to west, and so does the replica's default.
			m.activate(t, a, "tab1", "west")
			if err := a.sp.SetActive(context.Background(), "west"); err != nil {
				t.Fatal(err)
			}
			if code, b := m.get(t, a, host, loc, ck); code != 200 || b != "hello from east GET /dash" {
				t.Fatalf("after switching the tab to west the session must stay on east: %d %q", code, b)
			}
			// A ticket minted before the switch is pinned too.
			m.activate(t, a, "tab2", "east")
			tp2, host2 := m.link(t, a, "tab2")
			m.activate(t, a, "tab2", "west")
			loc2, ck2 := m.redeem(t, a, tp2, host2)
			if code, b := m.get(t, a, host2, loc2, ck2); code != 200 || b != "hello from east GET /dash" {
				t.Fatalf("ticket minted on east, redeemed after switching: %d %q", code, b)
			}
			// A new link from the switched tab goes to west.
			tp3, host3 := m.link(t, a, "tab1")
			loc3, ck3 := m.redeem(t, a, tp3, host3)
			if code, b := m.get(t, a, host3, loc3, ck3); code != 200 || b != "hello from west GET /dash" {
				t.Fatalf("link minted on west: %d %q", code, b)
			}
		})
	}
}

func TestPodProxyRemovedClusterRefused(t *testing.T) {
	for _, tc := range []struct {
		name   string
		secret string
		host   bool
	}{
		{"memory path mode", "", false},
		{"shared host mode", "shared-secret", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newPPMulti(t, 1, tc.secret, tc.host)
			a := m.r[0]
			lab, _ := newPodBackend(t, "lab")
			a.sp.Register("lab", "lab", ppClusterProvider(lab.URL), "test://lab")
			m.activate(t, a, "tab1", "lab")
			tp, host := m.link(t, a, "tab1")
			loc, ck := m.redeem(t, a, tp, host)
			if code, b := m.get(t, a, host, loc, ck); code != 200 || b != "hello from lab GET /dash" {
				t.Fatalf("lab session: %d %q", code, b)
			}
			pending, pendingHost := m.link(t, a, "tab1")

			if err := a.sp.Unregister("lab"); err != nil {
				t.Fatal(err)
			}
			// The session is refused, not sent to the tab's (or the
			// replica's) new cluster.
			if code, b := m.get(t, a, host, loc, ck); code != http.StatusGone || !strings.Contains(b, `"lab"`) {
				t.Fatalf("session on a removed cluster: %d %q", code, b)
			}
			// So is a ticket for it, without creating a session.
			code, b, res := m.do(t, a, pendingHost, pending, nil)
			if code != http.StatusGone || ppSessionCookie(res) != nil || !strings.Contains(b, `"lab"`) {
				t.Fatalf("ticket for a removed cluster: %d %q %v", code, b, res.Header["Set-Cookie"])
			}
		})
	}
}

func TestPodProxyClusterAcrossReplicas(t *testing.T) {
	for _, host := range []bool{false, true} {
		name := "path mode"
		if host {
			name = "host mode"
		}
		t.Run(name, func(t *testing.T) {
			m := newPPMulti(t, 2, "shared-secret", host)
			a, b := m.r[0], m.r[1]
			// Replica B defaults to west, and has seen tab1 select west; on A
			// tab1 is on east.
			if err := b.sp.SetActive(context.Background(), "west"); err != nil {
				t.Fatal(err)
			}
			m.activate(t, b, "tab1", "west")
			m.activate(t, a, "tab1", "east")

			tp, h := m.link(t, a, "tab1")
			loc, ck := m.redeem(t, b, tp, h)
			for _, r := range []*ppMultiReplica{a, b} {
				if code, body := m.get(t, r, h, loc, ck); code != 200 || body != "hello from east GET /dash" {
					t.Fatalf("session minted on A's east, served by %s: %d %q", r.proxy.URL, code, body)
				}
			}

			// A cluster only replica A knows (registered at runtime there).
			lab, _ := newPodBackend(t, "lab")
			a.sp.Register("lab", "lab", ppClusterProvider(lab.URL), "test://lab")
			m.activate(t, a, "tab1", "lab")
			tp2, h2 := m.link(t, a, "tab1")
			if code, body, res := m.do(t, b, h2, tp2, nil); code != http.StatusGone || ppSessionCookie(res) != nil {
				t.Fatalf("ticket for a cluster B does not know, redeemed on B: %d %q", code, body)
			}
			tp3, h3 := m.link(t, a, "tab1")
			loc3, ck3 := m.redeem(t, a, tp3, h3)
			if code, body := m.get(t, a, h3, loc3, ck3); code != 200 || body != "hello from lab GET /dash" {
				t.Fatalf("lab session on A: %d %q", code, body)
			}
			if code, body := m.get(t, b, h3, loc3, ck3); code != http.StatusGone || strings.Contains(body, "hello") {
				t.Fatalf("lab session on B, which does not know lab: %d %q", code, body)
			}
		})
	}
}

func TestPodProxyClusterProvider(t *testing.T) {
	single := ppClusterProvider("http://127.0.0.1:1")
	sp := platform.NewSwitchableProvider()
	sp.Register("east", "east", single, "")
	for name, tc := range map[string]struct {
		provider platform.Provider
		cluster  string
		ok       bool
	}{
		"single cluster, no name":           {single, "", true},
		"single cluster, a name":            {single, "east", false},
		"registry, registered name":         {sp, "east", true},
		"registry, unknown name":            {sp, "west", false},
		"registry, grant without a cluster": {sp, "", false},
	} {
		p, err := podProxyClusterProvider(tc.provider, tc.cluster)
		if tc.ok != (err == nil) || (err != nil && !errors.Is(err, errPodProxyClusterGone)) {
			t.Errorf("%s: %v", name, err)
		}
		if _, isRegistry := p.(*platform.SwitchableProvider); tc.ok && (p == nil || isRegistry) {
			t.Errorf("%s: wrong provider %T", name, p)
		}
	}
	if _, ok := sp.ProviderFor("west"); ok {
		t.Fatal("ProviderFor must not resolve an unknown cluster")
	}
}
