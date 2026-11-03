package platform

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// helpers

type fakeResolver map[string][]string

func (f fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	ips, ok := f[strings.ToLower(host)]
	if !ok {
		return nil, fmt.Errorf("no such host %s", host)
	}
	out := make([]net.IPAddr, 0, len(ips))
	for _, s := range ips {
		out = append(out, net.IPAddr{IP: net.ParseIP(s)})
	}
	return out, nil
}

// routedFetcher resolves names with res and connects every validated address
// to the test server at target, recording which addresses were dialed.
func routedFetcher(p HelmFetchPolicy, res fakeResolver, target string, dialed *[]string) *HelmFetcher {
	f := &HelmFetcher{Policy: p, Resolver: res, UserAgent: "zyvor-axiom/test"}
	var mu sync.Mutex
	f.dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
		mu.Lock()
		if dialed != nil {
			*dialed = append(*dialed, addr)
		}
		mu.Unlock()
		var d net.Dialer
		return d.DialContext(ctx, network, target)
	}
	return f
}

func trustServer(f *HelmFetcher, srv *httptest.Server) {
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	f.TLSConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

type tarEntry struct {
	name, body, link string
	typ              byte
	size             int64 // header size override (0 = len(body))
}

func makeTgz(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		hdr := &tar.Header{Name: e.name, Mode: 0o644, Typeflag: typ, Linkname: e.link}
		if typ == tar.TypeReg {
			hdr.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *testClock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

const testIndex = `apiVersion: v1
entries:
  nginx:
  - name: nginx
    version: 2.0.0-rc.1
    description: Web server (preview)
    urls: [charts/nginx-2.0.0-rc.1.tgz]
  - name: nginx
    version: 1.9.0
    appVersion: "1.24"
    description: Web server
    urls: [charts/nginx-1.9.0.tgz]
  - name: nginx
    version: 1.10.0
    appVersion: "1.25"
    description: Web server
    home: https://nginx.org
    icon: https://example.com/nginx.png
    keywords: [http, proxy]
    sources: [https://github.com/example/nginx]
    maintainers:
    - name: Ada
      email: ada@example.com
    created: 2024-01-02T03:04:05Z
    urls: [charts/nginx-1.10.0.tgz]
  - name: nginx
    version: 1.11.0
    deprecated: true
    urls: [charts/nginx-1.11.0.tgz]
  nginx-ingress:
  - name: nginx-ingress
    version: 0.5.0
    description: Ingress controller
    urls: [charts/nginx-ingress-0.5.0.tgz]
  my-nginx-proxy:
  - name: my-nginx-proxy
    version: 1.0.0
    urls: [charts/my-nginx-proxy-1.0.0.tgz]
  cache:
  - name: cache
    version: 3.0.0
    description: Sits in front of NGINX
    urls: [charts/cache-3.0.0.tgz]
  tagged:
  - name: tagged
    version: 1.0.0
    keywords: [webserver, Nginx-compatible]
    urls: [https://cdn.example.org/tagged-1.0.0.tgz]
  redis:
  - name: redis
    version: 7.0.0
    description: Key-value store
    urls: [charts/redis-7.0.0.tgz]
  onlypre:
  - name: onlypre
    version: 1.0.0-alpha
    urls: [x.tgz]
  - name: onlypre
    version: 1.0.0-beta
    urls: [x.tgz]
  olddep:
  - name: olddep
    version: 1.0.0
    deprecated: true
    urls: [x.tgz]
  - name: olddep
    version: 2.0.0
    deprecated: true
    urls: [x.tgz]
  numeric:
  - name: numeric
    version: 1.0
    urls: [x.tgz]
  postgres:
  - name: postgres
    version: 15.0.0
    urls: [charts/postgres-15.0.0.tgz]
  broken: "not a list"
`

// repoServer serves an index and chart archives, counting index hits.
type repoServer struct {
	*httptest.Server
	mu       sync.Mutex
	index    string
	status   int
	archives map[string][]byte
	hits     atomic.Int32
	block    chan struct{} // when non-nil, index requests wait for it
	headers  http.Header
}

func newRepoServer(t *testing.T, tlsServer bool) *repoServer {
	t.Helper()
	rs := &repoServer{index: testIndex, status: 200, archives: map[string][]byte{}}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		rs.headers = r.Header.Clone()
		block, status, index := rs.block, rs.status, rs.index
		rs.mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/index.yaml") {
			rs.hits.Add(1)
			if block != nil {
				<-block
			}
			if status != 200 {
				w.WriteHeader(status)
				return
			}
			_, _ = w.Write([]byte(index))
			return
		}
		rs.mu.Lock()
		b, ok := rs.archives[r.URL.Path]
		rs.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	})
	if tlsServer {
		rs.Server = httptest.NewTLSServer(h)
	} else {
		rs.Server = httptest.NewServer(h)
	}
	t.Cleanup(rs.Close)
	return rs
}

var openPolicy = HelmFetchPolicy{AllowInsecure: true, AllowPrivate: true}

func newTestCatalog(t *testing.T, rs *repoServer) (*HelmCatalog, *testClock) {
	t.Helper()
	clk := &testClock{t: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	c := NewHelmCatalog(NewMemoryHelmRepoStore(), &HelmFetcher{Policy: openPolicy, UserAgent: "zyvor-axiom/test"})
	c.now = clk.Now
	if rs != nil {
		if _, err := c.AddRepo(context.Background(), "main", rs.URL+"/"); err != nil {
			t.Fatalf("AddRepo: %v", err)
		}
	}
	return c, clk
}

func wantHelmStatus(t *testing.T, err error, code int) {
	t.Helper()
	if err == nil {
		t.Fatalf("want an error with status %d, got nil", code)
	}
	if got := HTTPStatus(err); got != code {
		t.Fatalf("want status %d, got %d (%v)", code, got, err)
	}
}

// ---------------------------------------------------------------------------
// SSRF: static URL validation

func TestHelmValidateURL(t *testing.T) {
	strict := HelmFetchPolicy{}
	insecure := HelmFetchPolicy{AllowInsecure: true}
	private := HelmFetchPolicy{AllowPrivate: true}
	cases := []struct {
		name string
		url  string
		pol  HelmFetchPolicy
		ok   bool
	}{
		{"https public name", "https://charts.example.com/stable", strict, true},
		{"https public IPv4", "https://8.8.8.8/charts", strict, true},
		{"https public IPv6", "https://[2606:4700:4700::1111]/charts", strict, true},
		{"http refused by default", "http://charts.example.com", strict, false},
		{"http allowed when insecure", "http://charts.example.com", insecure, true},
		{"ftp refused", "ftp://charts.example.com", insecure, false},
		{"file refused", "file:///etc/passwd", insecure, false},
		{"gopher refused", "gopher://charts.example.com", insecure, false},
		{"no scheme", "charts.example.com/stable", strict, false},
		{"opaque", "https:charts.example.com", strict, false},
		{"userinfo", "https://user:pw@charts.example.com", strict, false},
		{"user only", "https://user@charts.example.com", strict, false},
		{"fragment", "https://charts.example.com/#x", strict, false},
		{"empty host", "https:///index", strict, false},
		{"port only", "https://:443/index", strict, false},
		{"ipv6 zone", "https://[fe80::1%25en0]/", private, false},
		{"loopback v4", "https://127.0.0.1/", strict, false},
		{"loopback v4 other", "https://127.8.9.10/", strict, false},
		{"loopback v6", "https://[::1]/", strict, false},
		{"mapped loopback", "https://[::ffff:127.0.0.1]/", strict, false},
		{"private 10", "https://10.1.2.3/", strict, false},
		{"private 172.16", "https://172.16.5.4/", strict, false},
		{"private 172.31", "https://172.31.255.255/", strict, false},
		{"not private 172.32", "https://172.32.0.1/", strict, true},
		{"private 192.168", "https://192.168.1.1/", strict, false},
		{"link-local metadata", "https://169.254.169.254/latest/meta-data", strict, false},
		{"link-local v6", "https://[fe80::1]/", strict, false},
		{"ula fc00", "https://[fc00::1]/", strict, false},
		{"ula fd", "https://[fd12:3456::1]/", strict, false},
		{"cgnat", "https://100.64.0.1/", strict, false},
		{"cgnat top", "https://100.127.255.254/", strict, false},
		{"not cgnat", "https://100.128.0.1/", strict, true},
		{"unspecified v4", "https://0.0.0.0/", strict, false},
		{"this network", "https://0.1.2.3/", strict, false},
		{"unspecified v6", "https://[::]/", strict, false},
		{"multicast v4", "https://224.0.0.1/", strict, false},
		{"multicast v6", "https://[ff02::1]/", strict, false},
		{"broadcast", "https://255.255.255.255/", strict, false},
		{"decimal IP", "https://2130706433/", strict, false},
		{"hex IP", "https://0x7f000001/", strict, false},
		{"octal IP", "https://0177.0.0.1/", strict, false},
		{"short IP", "https://127.1/", strict, false},
		{"decimal IP even when private allowed", "https://2130706433/", private, false},
		{"hex IP even when private allowed", "https://0x7f.1/", private, false},
		{"localhost", "https://localhost/", strict, false},
		{"localhost upper with dot", "https://LOCALHOST./", strict, false},
		{"sub.localhost", "https://charts.localhost/", strict, false},
		{"numeric-looking but real name", "https://cafe.beef/", strict, true},
		{"name with digits", "https://123.example.com/", strict, true},
		{"private allowed", "https://10.1.2.3/", private, true},
		{"loopback allowed", "https://127.0.0.1:8080/", private, true},
		{"localhost allowed", "https://localhost:8080/", private, true},
		{"metadata allowed when private allowed", "https://169.254.169.254/", private, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := url.Parse(tc.url)
			if err == nil {
				err = tc.pol.ValidateHelmURL(u)
			}
			if (err == nil) != tc.ok {
				t.Fatalf("ValidateHelmURL(%q) = %v, want ok=%v", tc.url, err, tc.ok)
			}
		})
	}
}

func TestHelmAddrDisallowed(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1", "169.254.169.254", "100.64.0.1", "0.0.0.0", "224.0.0.251", "::1", "fe80::1", "fc00::1", "::", "ff05::1", "::ffff:10.0.0.1", "::ffff:169.254.169.254"} {
		if !helmAddrDisallowed(netip.MustParseAddr(s)) {
			t.Errorf("%s should be refused", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "93.184.216.34", "2606:4700::1111", "172.32.0.1", "100.128.0.1"} {
		if helmAddrDisallowed(netip.MustParseAddr(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
}

func TestHelmRepoNameAndURLNormalization(t *testing.T) {
	for _, n := range []string{"bitnami", "a", "my-repo-2", strings.Repeat("a", 40)} {
		if err := ValidateHelmRepoName(n); err != nil {
			t.Errorf("%q: %v", n, err)
		}
	}
	for _, n := range []string{"", "Bitnami", "-x", "x-", "a_b", "a.b", "a/b", strings.Repeat("a", 41), "ä"} {
		if err := ValidateHelmRepoName(n); err == nil {
			t.Errorf("%q should be refused", n)
		}
	}
	p := HelmFetchPolicy{}
	got, err := p.NormalizeHelmRepoURL("  https://charts.example.com/stable//  ")
	if err != nil || got != "https://charts.example.com/stable" {
		t.Fatalf("normalize: %q %v", got, err)
	}
	for _, bad := range []string{"", "https://charts.example.com/?token=x", "https://charts.example.com/?", "https://charts.example.com/#", "http://charts.example.com", "https://u:p@charts.example.com"} {
		if _, err := p.NormalizeHelmRepoURL(bad); err == nil {
			t.Errorf("%q should be refused", bad)
		} else if HTTPStatus(err) != 400 {
			t.Errorf("%q: status %d", bad, HTTPStatus(err))
		}
	}
}

// ---------------------------------------------------------------------------
// SSRF: dial-time checks

func TestHelmDialRefusesNamesResolvingToPrivateAddresses(t *testing.T) {
	rs := newRepoServer(t, false)
	target := rs.Listener.Addr().String()
	res := fakeResolver{
		"rebind.example.com": {"127.0.0.1"},
		"meta.example.com":   {"169.254.169.254"},
		"mixed.example.com":  {"93.184.216.34", "10.0.0.7"},
		"v6.example.com":     {"::1"},
		"mapped.example.com": {"::ffff:192.168.1.1"},
		"cgnat.example.com":  {"100.64.1.1"},
		"public.example.com": {"93.184.216.34"},
	}
	pol := HelmFetchPolicy{AllowInsecure: true}
	for _, host := range []string{"rebind", "meta", "mixed", "v6", "mapped", "cgnat"} {
		t.Run(host, func(t *testing.T) {
			var dialed []string
			f := routedFetcher(pol, res, target, &dialed)
			_, err := f.FetchIndex(context.Background(), "http://"+host+".example.com")
			wantHelmStatus(t, err, 400)
			if !strings.Contains(err.Error(), "not a public address") {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(dialed) != 0 {
				t.Fatalf("must not dial at all, dialed %v", dialed)
			}
		})
	}
	t.Run("public name is fetched by validated IP", func(t *testing.T) {
		var dialed []string
		f := routedFetcher(pol, res, target, &dialed)
		ix, err := f.FetchIndex(context.Background(), "http://public.example.com")
		if err != nil {
			t.Fatal(err)
		}
		if ix.chartCount() == 0 || len(dialed) != 1 || !strings.HasPrefix(dialed[0], "93.184.216.34:") {
			t.Fatalf("charts=%d dialed=%v", ix.chartCount(), dialed)
		}
	})
	t.Run("private allowed", func(t *testing.T) {
		f := &HelmFetcher{Policy: openPolicy, Resolver: fakeResolver{"inside.example.com": {"127.0.0.1"}}}
		_, port, _ := net.SplitHostPort(target)
		if _, err := f.FetchIndex(context.Background(), "http://inside.example.com:"+port); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("literal localhost refused before resolving", func(t *testing.T) {
		f := routedFetcher(pol, fakeResolver{"localhost": {"93.184.216.34"}}, target, nil)
		_, err := f.dialContext(context.Background(), "tcp", "localhost:80")
		if err == nil {
			t.Fatal("localhost must be refused")
		}
	})
	t.Run("encoded IP refused at dial", func(t *testing.T) {
		f := routedFetcher(openPolicy, res, target, nil)
		if _, err := f.dialContext(context.Background(), "tcp", "2130706433:80"); err == nil {
			t.Fatal("decimal host must be refused")
		}
	})
	t.Run("default dialer re-checks the socket address", func(t *testing.T) {
		f := &HelmFetcher{Policy: HelmFetchPolicy{}}
		_, err := f.defaultDial(context.Background(), "tcp", target)
		var pe *helmPolicyError
		if err == nil || !errors.As(err, &pe) {
			t.Fatalf("Control must refuse a loopback connect: %v", err)
		}
	})
}

func TestHelmRedirects(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/to-loopback/index.yaml":
			http.Redirect(w, r, "https://127.0.0.1:"+srvPort(srv)+"/ok/index.yaml", http.StatusFound)
		case "/to-private-name/index.yaml":
			http.Redirect(w, r, "https://internal.example.com/ok/index.yaml", http.StatusFound)
		case "/to-http/index.yaml":
			http.Redirect(w, r, "http://example.com/ok/index.yaml", http.StatusMovedPermanently)
		case "/to-file/index.yaml":
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		case "/to-userinfo/index.yaml":
			http.Redirect(w, r, "https://a:b@example.com/ok/index.yaml", http.StatusFound)
		case "/hop1/index.yaml":
			http.Redirect(w, r, "/hop2/index.yaml", http.StatusFound)
		case "/hop2/index.yaml":
			http.Redirect(w, r, "/hop3/index.yaml", http.StatusFound)
		case "/hop3/index.yaml":
			http.Redirect(w, r, "/ok/index.yaml", http.StatusFound)
		case "/hop0/index.yaml":
			http.Redirect(w, r, "/hop1/index.yaml", http.StatusFound)
		case "/ok/index.yaml":
			if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
				w.WriteHeader(400)
				return
			}
			_, _ = w.Write([]byte(testIndex))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	res := fakeResolver{"example.com": {"93.184.216.34"}, "internal.example.com": {"10.0.0.5"}}
	cases := []struct {
		path string
		ok   bool
		msg  string
	}{
		{"ok", true, ""},
		{"hop1", true, ""}, // three redirects are allowed
		{"hop0", false, "too many redirects"},
		{"to-loopback", false, "not a public address"},
		{"to-private-name", false, "not a public address"},
		{"to-http", false, "only https://"},
		{"to-file", false, "unsupported URL scheme"},
		{"to-userinfo", false, "user name or password"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			f := routedFetcher(HelmFetchPolicy{}, res, srv.Listener.Addr().String(), nil)
			trustServer(f, srv)
			_, err := f.FetchIndex(context.Background(), "https://example.com/"+tc.path)
			if tc.ok {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.msg) {
				t.Fatalf("want error containing %q, got %v", tc.msg, err)
			}
		})
	}
}

func srvPort(s *httptest.Server) string {
	_, p, _ := net.SplitHostPort(s.Listener.Addr().String())
	return p
}

func TestHelmFetchHeadersGzipAndLimits(t *testing.T) {
	var gotHeaders http.Header
	var mu sync.Mutex
	big := strings.Repeat("#", helmMaxIndexBytes+10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotHeaders = r.Header.Clone()
		mu.Unlock()
		switch r.URL.Path {
		case "/gz/index.yaml":
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				w.WriteHeader(400)
				return
			}
			w.Header().Set("Content-Encoding", "gzip")
			gz := gzip.NewWriter(w)
			_, _ = gz.Write([]byte(testIndex))
			_ = gz.Close()
		case "/big/index.yaml":
			_, _ = w.Write([]byte(big))
		case "/bigchunked/index.yaml":
			for i := 0; i < 21; i++ {
				_, _ = w.Write([]byte(strings.Repeat("#", 1<<20)))
				w.(http.Flusher).Flush()
			}
		case "/html/index.yaml":
			_, _ = w.Write([]byte("<html><body>hello</body></html>"))
		case "/noapi/index.yaml":
			_, _ = w.Write([]byte("entries: {}\n"))
		case "/empty/index.yaml":
			_, _ = w.Write([]byte("apiVersion: v1\nentries: {}\n"))
		case "/secret/index.yaml":
			w.WriteHeader(500)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	f := &HelmFetcher{Policy: openPolicy, UserAgent: "zyvor-axiom/1.2.3"}
	ix, err := f.FetchIndex(context.Background(), srv.URL+"/gz")
	if err != nil || ix.chartCount() == 0 {
		t.Fatalf("gzip index: %v", err)
	}
	mu.Lock()
	if gotHeaders.Get("User-Agent") != "zyvor-axiom/1.2.3" || gotHeaders.Get("Authorization") != "" || gotHeaders.Get("Cookie") != "" {
		t.Errorf("headers: %v", gotHeaders)
	}
	mu.Unlock()
	_, err = f.FetchIndex(context.Background(), srv.URL+"/big")
	wantHelmStatus(t, err, 413)
	_, err = f.FetchIndex(context.Background(), srv.URL+"/bigchunked")
	wantHelmStatus(t, err, 413)
	_, err = f.FetchIndex(context.Background(), srv.URL+"/html")
	wantHelmStatus(t, err, 502)
	_, err = f.FetchIndex(context.Background(), srv.URL+"/noapi")
	wantHelmStatus(t, err, 502)
	if ix, err := f.FetchIndex(context.Background(), srv.URL+"/empty"); err != nil || ix.chartCount() != 0 {
		t.Fatalf("empty index: %v", err)
	}
	_, err = f.FetchIndex(context.Background(), srv.URL+"/missing")
	wantHelmStatus(t, err, 502)

	// Query strings never appear in errors.
	_, err = f.get(context.Background(), mustURL(t, srv.URL+"/secret/index.yaml?token=hunter2"), 1<<20, "x")
	wantHelmStatus(t, err, 502)
	if strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), "token") {
		t.Fatalf("query string leaked: %v", err)
	}
	// Nor in transport errors (url.Error prints the full URL).
	_, err = f.get(context.Background(), mustURL(t, "http://127.0.0.1:1/x.tgz?sig=hunter2"), 1<<20, "x")
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("query string leaked: %v", err)
	}
}

func TestHelmFetchTimeoutIsSet(t *testing.T) {
	f := NewHelmFetcher(HelmFetchPolicy{}, "ua")
	c := f.httpClient()
	if c.Timeout != 15*time.Second || c.Jar != nil {
		t.Fatalf("client: timeout=%v jar=%v", c.Timeout, c.Jar)
	}
	if tr := c.Transport.(*http.Transport); tr.Proxy != nil {
		t.Fatal("a proxy would bypass the dial-time address check")
	}
}

// ---------------------------------------------------------------------------
// Index parsing, versions, search

func TestHelmSemverOrdering(t *testing.T) {
	ordered := []string{"2.0.0", "2.0.0-rc.10", "2.0.0-rc.2", "2.0.0-rc.1", "2.0.0-beta", "2.0.0-alpha.beta", "2.0.0-alpha.1", "2.0.0-alpha", "1.10.0", "v1.9.0+build.5", "1.2", "1"}
	for i := 0; i+1 < len(ordered); i++ {
		a, b := parseSemver(ordered[i]), parseSemver(ordered[i+1])
		if compareSemver(a, b) <= 0 || compareSemver(b, a) >= 0 {
			t.Errorf("%s should sort above %s", ordered[i], ordered[i+1])
		}
	}
	if compareSemver(parseSemver("1.0.0+a"), parseSemver("1.0.0+b")) != 0 {
		t.Error("build metadata must be ignored")
	}
	for _, bad := range []string{"banana", "1.2.3.4", "1..2", "1.2.x", "1.0.0-", "1.0.0-a..b", ""} {
		if parseSemver(bad).valid {
			t.Errorf("%q should be invalid", bad)
		}
		if compareSemver(parseSemver(bad), parseSemver("0.0.1")) >= 0 {
			t.Errorf("invalid %q must sort below valid versions", bad)
		}
	}
	vs := []*helmIndexEntry{{Version: "banana"}, {Version: "1.0.0-rc.1"}, {Version: "0.9.0"}, {Version: "1.0.0"}, {Version: "apple"}}
	for _, v := range vs {
		v.sv = parseSemver(string(v.Version))
	}
	sortHelmVersions(vs)
	var got []string
	for _, v := range vs {
		got = append(got, string(v.Version))
	}
	if strings.Join(got, ",") != "1.0.0,1.0.0-rc.1,0.9.0,banana,apple" {
		t.Fatalf("order: %v", got)
	}
}

func TestHelmParseIndex(t *testing.T) {
	ix, err := parseHelmIndex([]byte(testIndex))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ix.charts["broken"]; ok {
		t.Error("a malformed chart should be skipped")
	}
	if ix.chartCount() != 10 {
		t.Errorf("chart count %d: %v", ix.chartCount(), ix.names)
	}
	cases := map[string]string{
		"nginx":   "1.10.0",     // 2.0.0-rc.1 is a prerelease, 1.11.0 deprecated
		"onlypre": "1.0.0-beta", // nothing else: newest
		"olddep":  "2.0.0",      // all deprecated: newest
		"numeric": "1",          // an unquoted YAML number still parses (as its numeric text)
	}
	for chart, want := range cases {
		if got := string(ix.latest(chart).Version); got != want {
			t.Errorf("latest(%s) = %s, want %s", chart, got, want)
		}
	}
	var order []string
	for _, e := range ix.charts["nginx"] {
		order = append(order, string(e.Version))
	}
	if strings.Join(order, ",") != "2.0.0-rc.1,1.11.0,1.10.0,1.9.0" {
		t.Errorf("version order: %v", order)
	}
	if c := string(ix.latest("nginx").Created); !strings.HasPrefix(c, "2024-01-02T03:04:05") {
		t.Errorf("created: %q", c)
	}
}

func TestHelmParseIndexCapsCharts(t *testing.T) {
	var b strings.Builder
	b.WriteString("apiVersion: v1\nentries:\n")
	for i := 0; i < helmMaxChartsPerIndex+50; i++ {
		fmt.Fprintf(&b, "  c%05d:\n  - version: 1.0.0\n", i)
	}
	ix, err := parseHelmIndex([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if ix.chartCount() != helmMaxChartsPerIndex {
		t.Fatalf("parsed %d charts", ix.chartCount())
	}
}

func TestHelmSearch(t *testing.T) {
	rs := newRepoServer(t, false)
	c, _ := newTestCatalog(t, rs)
	ctx := context.Background()

	res, err := c.Search(ctx, "", "NGINX", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, ch := range res.Charts {
		names = append(names, ch.Name)
	}
	// exact > prefix > contains > description/keyword; ties by name.
	if want := "nginx,nginx-ingress,my-nginx-proxy,cache,tagged"; strings.Join(names, ",") != want {
		t.Fatalf("ranking: got %v, want %s", names, want)
	}
	if res.Total != 5 || len(res.Errors) != 0 || res.FetchedAt == "" {
		t.Fatalf("result: %+v", res)
	}
	top := res.Charts[0]
	if top.Repo != "main" || top.Version != "1.10.0" || top.AppVersion != "1.25" || top.Home != "https://nginx.org" || len(top.Keywords) != 2 {
		t.Fatalf("summary: %+v", top)
	}

	res, _ = c.Search(ctx, "", "", 3, false)
	if len(res.Charts) != 3 || res.Total != 10 {
		t.Fatalf("limit: %d of %d", len(res.Charts), res.Total)
	}
	if res.Charts[0].Name != "cache" {
		t.Errorf("an empty query lists by name: %v", res.Charts[0].Name)
	}
	if res.Charts[0].Keywords == nil {
		t.Error("keywords must be [] not null")
	}
	res, _ = c.Search(ctx, "", "", 5000, false)
	if len(res.Charts) != 10 {
		t.Fatalf("limit capped: %d", len(res.Charts))
	}
	res, _ = c.Search(ctx, "main", "zzz", 0, false)
	if res.Total != 0 || res.Charts == nil {
		t.Fatalf("no match: %+v", res)
	}
	_, err = c.Search(ctx, "nope", "", 0, false)
	wantHelmStatus(t, err, 404)
}

func TestHelmSearchLimitDefaultAndMax(t *testing.T) {
	var b strings.Builder
	b.WriteString("apiVersion: v1\nentries:\n")
	for i := 0; i < 250; i++ {
		fmt.Fprintf(&b, "  c%03d:\n  - version: 1.0.0\n", i)
	}
	rs := newRepoServer(t, false)
	rs.index = b.String()
	c, _ := newTestCatalog(t, rs)
	res, _ := c.Search(context.Background(), "", "", 0, false)
	if len(res.Charts) != 60 || res.Total != 250 {
		t.Fatalf("default limit: %d/%d", len(res.Charts), res.Total)
	}
	res, _ = c.Search(context.Background(), "", "", 1000, false)
	if len(res.Charts) != 200 {
		t.Fatalf("max limit: %d", len(res.Charts))
	}
}

func TestHelmSearchOneFailingRepo(t *testing.T) {
	good := newRepoServer(t, false)
	bad := newRepoServer(t, false)
	c, clk := newTestCatalog(t, good)
	if _, err := c.AddRepo(context.Background(), "flaky", bad.URL); err != nil {
		t.Fatal(err)
	}
	bad.mu.Lock()
	bad.status = 503
	bad.mu.Unlock()
	clk.Add(11 * time.Minute)

	res, err := c.Search(context.Background(), "", "redis", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	// The stale copy of the failing repo is still served, with the error reported.
	if res.Total != 2 || res.Errors["flaky"] == "" || res.Errors["main"] != "" {
		t.Fatalf("result: %+v", res)
	}

	// A repo that never fetched successfully: no charts, error reported, call succeeds.
	c2, _ := newTestCatalog(t, good)
	if err := c2.store.Add(context.Background(), HelmRepo{Name: "dead", URL: "http://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	res, err = c2.Search(context.Background(), "", "redis", 0, false)
	if err != nil || res.Total != 1 || res.Errors["dead"] == "" {
		t.Fatalf("result: %+v %v", res, err)
	}
	if strings.Contains(res.Errors["dead"], "?") {
		t.Fatalf("error: %q", res.Errors["dead"])
	}
}

// ---------------------------------------------------------------------------
// Caching

func TestHelmCacheTTLAndRefreshThrottle(t *testing.T) {
	rs := newRepoServer(t, false)
	c, clk := newTestCatalog(t, rs) // AddRepo fetched once
	ctx := context.Background()
	if n := rs.hits.Load(); n != 1 {
		t.Fatalf("add should fetch once, got %d", n)
	}
	for i := 0; i < 3; i++ {
		_, _ = c.Search(ctx, "", "", 0, false)
		_, _ = c.Chart(ctx, "main", "nginx")
	}
	if n := rs.hits.Load(); n != 1 {
		t.Fatalf("cached reads must not fetch, got %d", n)
	}
	clk.Add(10 * time.Second)
	_, _ = c.Search(ctx, "", "", 0, true)
	if n := rs.hits.Load(); n != 1 {
		t.Fatalf("refresh within 30 s must not fetch, got %d", n)
	}
	clk.Add(25 * time.Second)
	_, _ = c.Search(ctx, "", "", 0, true)
	if n := rs.hits.Load(); n != 2 {
		t.Fatalf("refresh after 30 s must fetch, got %d", n)
	}
	_, _ = c.Search(ctx, "", "", 0, true)
	if n := rs.hits.Load(); n != 2 {
		t.Fatalf("second refresh straight away must not fetch, got %d", n)
	}
	clk.Add(9 * time.Minute)
	_, _ = c.Search(ctx, "", "", 0, false)
	if n := rs.hits.Load(); n != 2 {
		t.Fatalf("fresh within 10 min, got %d", n)
	}
	clk.Add(2 * time.Minute)
	_, _ = c.Search(ctx, "", "", 0, false)
	if n := rs.hits.Load(); n != 3 {
		t.Fatalf("stale after 10 min must fetch, got %d", n)
	}
}

func TestHelmCacheSingleFlight(t *testing.T) {
	rs := newRepoServer(t, false)
	c, clk := newTestCatalog(t, rs)
	clk.Add(11 * time.Minute) // stale
	rs.mu.Lock()
	rs.block = make(chan struct{})
	rs.mu.Unlock()
	start := rs.hits.Load()

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := c.Search(context.Background(), "main", "", 0, false)
			if err == nil && res.Total != 10 {
				err = fmt.Errorf("total %d", res.Total)
			}
			errs <- err
		}()
	}
	deadline := time.Now().Add(5 * time.Second)
	for rs.hits.Load() == start && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // let the others pile up behind the flight
	rs.mu.Lock()
	close(rs.block)
	rs.block = nil
	rs.mu.Unlock()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := rs.hits.Load() - start; n != 1 {
		t.Fatalf("10 concurrent readers caused %d fetches", n)
	}
}

func TestHelmCacheStaleOnError(t *testing.T) {
	rs := newRepoServer(t, false)
	c, clk := newTestCatalog(t, rs)
	ctx := context.Background()
	rs.mu.Lock()
	rs.status = 500
	rs.mu.Unlock()
	clk.Add(11 * time.Minute)

	res, err := c.Search(ctx, "main", "", 0, false)
	if err != nil || res.Total != 10 || !strings.Contains(res.Errors["main"], "HTTP 500") {
		t.Fatalf("stale copy must be served with the error: %+v %v", res, err)
	}
	repos, _ := c.Repos(ctx)
	if len(repos) != 1 || repos[0].ChartCount != 10 || !strings.Contains(repos[0].Error, "HTTP 500") || repos[0].FetchedAt != "2026-11-03T12:00:00Z" {
		t.Fatalf("repo view: %+v", repos)
	}
	// A failing repo is not hammered: no retry within 30 s.
	n := rs.hits.Load()
	_, _ = c.Search(ctx, "main", "", 0, false)
	if rs.hits.Load() != n {
		t.Fatal("failing repo retried immediately")
	}
	// Recovery clears the error.
	rs.mu.Lock()
	rs.status = 200
	rs.mu.Unlock()
	clk.Add(31 * time.Second)
	res, _ = c.Search(ctx, "main", "", 0, false)
	if len(res.Errors) != 0 {
		t.Fatalf("errors after recovery: %v", res.Errors)
	}
	if repos, _ := c.Repos(ctx); repos[0].Error != "" {
		t.Fatalf("repo error after recovery: %q", repos[0].Error)
	}
}

// ---------------------------------------------------------------------------
// Repositories

func TestHelmAddRemoveRepo(t *testing.T) {
	rs := newRepoServer(t, false)
	c, _ := newTestCatalog(t, nil)
	ctx := context.Background()

	v, err := c.AddRepo(ctx, "main", rs.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if v.URL != rs.URL || v.ChartCount != 10 || v.AddedAt == "" || v.FetchedAt == "" || v.Error != "" {
		t.Fatalf("view: %+v", v)
	}
	_, err = c.AddRepo(ctx, "main", "http://127.0.0.2:9")
	wantHelmStatus(t, err, 409)
	_, err = c.AddRepo(ctx, "other", strings.Replace(rs.URL, "http://", "HTTP://", 1)+"/")
	wantHelmStatus(t, err, 409)
	_, err = c.AddRepo(ctx, "Bad_Name", rs.URL+"/x")
	wantHelmStatus(t, err, 400)
	_, err = c.AddRepo(ctx, "missing", rs.URL+"/nothing-here/sub") // index answers, see below
	if err != nil {
		t.Fatalf("index under a sub-path: %v", err)
	}
	rs.mu.Lock()
	rs.index = "<html>not an index</html>"
	rs.mu.Unlock()
	_, err = c.AddRepo(ctx, "html", rs.URL+"/html")
	wantHelmStatus(t, err, 400)
	if !strings.Contains(err.Error(), "index") {
		t.Fatalf("reason: %v", err)
	}
	_, err = c.AddRepo(ctx, "dead", "http://127.0.0.1:1")
	wantHelmStatus(t, err, 400)
	_, err = c.AddRepo(ctx, "query", rs.URL+"/q?token=x")
	wantHelmStatus(t, err, 400)

	repos, _ := c.Repos(ctx)
	if len(repos) != 2 {
		t.Fatalf("repos: %+v", repos)
	}
	if _, err := c.RemoveRepo(ctx, "main"); err != nil {
		t.Fatal(err)
	}
	_, err = c.RemoveRepo(ctx, "main")
	wantHelmStatus(t, err, 404)
	c.mu.Lock()
	_, cached := c.cache["main"]
	c.mu.Unlock()
	if cached {
		t.Error("removing a repo must drop its cache")
	}
}

func TestHelmRepoCap(t *testing.T) {
	rs := newRepoServer(t, false)
	c, _ := newTestCatalog(t, nil)
	for i := 0; i < helmMaxRepos; i++ {
		if err := c.store.Add(context.Background(), HelmRepo{Name: fmt.Sprintf("r%d", i), URL: fmt.Sprintf("https://r%d.example.com", i)}); err != nil {
			t.Fatal(err)
		}
	}
	start := rs.hits.Load()
	_, err := c.AddRepo(context.Background(), "one-more", rs.URL)
	wantHelmStatus(t, err, 400)
	if rs.hits.Load() != start {
		t.Error("the cap must be checked before fetching")
	}
}

func TestMemoryHelmRepoStore(t *testing.T) {
	testHelmRepoStore(t, NewMemoryHelmRepoStore())
}

// testHelmRepoStore is shared with the kubernetes-build ConfigMap store test.
func testHelmRepoStore(t *testing.T, s HelmRepoStore) {
	t.Helper()
	ctx := context.Background()
	if l, err := s.List(ctx); err != nil || len(l) != 0 || l == nil {
		t.Fatalf("empty list: %v %v", l, err)
	}
	if err := s.Add(ctx, HelmRepo{Name: "a", URL: "https://a.example.com", AddedAt: "t"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(ctx, HelmRepo{Name: "b", URL: "https://b.example.com"}); err != nil {
		t.Fatal(err)
	}
	wantHelmStatus(t, s.Add(ctx, HelmRepo{Name: "a", URL: "https://c.example.com"}), 409)
	wantHelmStatus(t, s.Add(ctx, HelmRepo{Name: "c", URL: "https://A.example.com/"}), 409)
	l, _ := s.List(ctx)
	if len(l) != 2 || l[0].Name != "a" || l[0].AddedAt != "t" || l[1].Name != "b" {
		t.Fatalf("list: %+v", l)
	}
	wantHelmStatus(t, s.Remove(ctx, "zzz"), 404)
	if err := s.Remove(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.List(ctx); len(l) != 1 || l[0].Name != "b" {
		t.Fatalf("after remove: %+v", l)
	}
	for i := 0; i < helmMaxRepos-1; i++ {
		if err := s.Add(ctx, HelmRepo{Name: fmt.Sprintf("r%d", i), URL: fmt.Sprintf("https://r%d.example.com", i)}); err != nil {
			t.Fatal(err)
		}
	}
	wantHelmStatus(t, s.Add(ctx, HelmRepo{Name: "full", URL: "https://full.example.com"}), 400)
}

// ---------------------------------------------------------------------------
// Chart detail and values

func TestHelmChartDetail(t *testing.T) {
	rs := newRepoServer(t, false)
	c, _ := newTestCatalog(t, rs)
	d, err := c.Chart(context.Background(), "main", "nginx")
	if err != nil {
		t.Fatal(err)
	}
	if d.Description != "Web server" || d.Home != "https://nginx.org" || len(d.Sources) != 1 || len(d.Maintainers) != 1 || d.Maintainers[0].Email != "ada@example.com" {
		t.Fatalf("detail: %+v", d)
	}
	if len(d.Versions) != 4 || d.Versions[0].Version != "2.0.0-rc.1" || !d.Versions[1].Deprecated {
		t.Fatalf("versions: %+v", d.Versions)
	}
	_, err = c.Chart(context.Background(), "main", "nope")
	wantHelmStatus(t, err, 404)
	_, err = c.Chart(context.Background(), "nope", "nginx")
	wantHelmStatus(t, err, 404)

	var b strings.Builder
	b.WriteString("apiVersion: v1\nentries:\n  many:\n")
	for i := 0; i < 150; i++ {
		fmt.Fprintf(&b, "  - version: 1.%d.0\n", i)
	}
	rs.mu.Lock()
	rs.index = b.String()
	rs.mu.Unlock()
	c2, _ := newTestCatalog(t, rs)
	d, _ = c2.Chart(context.Background(), "main", "many")
	if len(d.Versions) != 100 || d.Versions[0].Version != "1.149.0" {
		t.Fatalf("versions capped: %d %s", len(d.Versions), d.Versions[0].Version)
	}
}

func TestHelmChartValues(t *testing.T) {
	rs := newRepoServer(t, false)
	cdn := newRepoServer(t, false)
	rs.archives["/charts/nginx-1.10.0.tgz"] = makeTgz(t,
		tarEntry{name: "nginx/Chart.yaml", body: "name: nginx"},
		tarEntry{name: "nginx/values.yaml", body: "replicaCount: 1\n"},
		tarEntry{name: "nginx/README.md", body: "# NGINX"},
	)
	rs.archives["/charts/nginx-1.9.0.tgz"] = makeTgz(t, tarEntry{name: "nginx/values.yaml", body: "old: true\n"})
	cdn.archives["/tagged-1.0.0.tgz"] = makeTgz(t, tarEntry{name: "tagged/values.yaml", body: "cdn: true\n"})
	rs.index = strings.Replace(testIndex, "https://cdn.example.org", cdn.URL, 1)
	c, _ := newTestCatalog(t, rs)
	ctx := context.Background()

	v, err := c.ChartValues(ctx, "main", "nginx", "")
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != "1.10.0" || v.Values != "replicaCount: 1\n" || v.Readme != "# NGINX" {
		t.Fatalf("values: %+v", v)
	}
	v, err = c.ChartValues(ctx, "main", "nginx", "1.9.0")
	if err != nil || v.Values != "old: true\n" || v.Readme != "" {
		t.Fatalf("pinned version: %+v %v", v, err)
	}
	// An absolute URL on another host is fine when it passes the policy.
	if v, err := c.ChartValues(ctx, "main", "tagged", ""); err != nil || v.Values != "cdn: true\n" {
		t.Fatalf("absolute archive URL: %+v %v", v, err)
	}
	_, err = c.ChartValues(ctx, "main", "nginx", "9.9.9")
	wantHelmStatus(t, err, 404)
	_, err = c.ChartValues(ctx, "main", "nope", "")
	wantHelmStatus(t, err, 404)
	_, err = c.ChartValues(ctx, "main", "redis", "") // archive missing upstream
	wantHelmStatus(t, err, 502)

	rs.archives["/charts/cache-3.0.0.tgz"] = bytes.Repeat([]byte{1}, helmMaxArchiveBytes+1)
	_, err = c.ChartValues(ctx, "main", "cache", "")
	wantHelmStatus(t, err, 413)
}

func TestHelmChartArchiveURLIsNotTrusted(t *testing.T) {
	rs := newRepoServer(t, false)
	rs.index = `apiVersion: v1
entries:
  meta:
  - version: 1.0.0
    urls: ["http://169.254.169.254/latest/meta-data/x.tgz"]
  internal:
  - version: 1.0.0
    urls: ["http://internal.example.com/x.tgz"]
  ftp:
  - version: 1.0.0
    urls: ["ftp://example.com/x.tgz"]
  creds:
  - version: 1.0.0
    urls: ["http://user:pw@example.com/x.tgz"]
  nourl:
  - version: 1.0.0
`
	pol := HelmFetchPolicy{AllowInsecure: true}
	res := fakeResolver{"repo.example.com": {"93.184.216.34"}, "internal.example.com": {"10.1.1.1"}}
	var dialed []string
	f := routedFetcher(pol, res, rs.Listener.Addr().String(), &dialed)
	c := NewHelmCatalog(NewMemoryHelmRepoStore(), f)
	if _, err := c.AddRepo(context.Background(), "public", "http://repo.example.com"); err != nil {
		t.Fatal(err)
	}
	for _, chart := range []string{"meta", "internal", "ftp", "creds", "nourl"} {
		_, err := c.ChartValues(context.Background(), "public", chart, "")
		wantHelmStatus(t, err, 502)
	}
	for _, d := range dialed {
		if !strings.HasPrefix(d, "93.184.216.34:") {
			t.Fatalf("dialed a refused address: %v", dialed)
		}
	}
}

func TestHelmResolveChartURL(t *testing.T) {
	cases := map[string]string{
		"charts/x.tgz":                  "https://h.example.com/repo/charts/x.tgz",
		"x.tgz":                         "https://h.example.com/repo/x.tgz",
		"/abs/x.tgz":                    "https://h.example.com/abs/x.tgz",
		"https://cdn.example.org/x.tgz": "https://cdn.example.org/x.tgz",
	}
	for ref, want := range cases {
		u, err := resolveHelmChartURL("https://h.example.com/repo", ref)
		if err != nil || u.String() != want {
			t.Errorf("%s: %v %v", ref, u, err)
		}
	}
}

func TestHelmReadChartArchive(t *testing.T) {
	bigValues := strings.Repeat("a", helmMaxExtractedFile+1)
	longReadme := strings.Repeat("é", helmMaxReadmeBytes) // 2 bytes each
	cases := []struct {
		name         string
		entries      []tarEntry
		values       string
		readme       string
		status       int
		readmeLength int
	}{
		{name: "valid", entries: []tarEntry{{name: "c/Chart.yaml", body: "x"}, {name: "c/values.yaml", body: "a: 1"}, {name: "c/README.md", body: "hi"}}, values: "a: 1", readme: "hi"},
		{name: "lower-case readme", entries: []tarEntry{{name: "c/readme.md", body: "hi"}}, readme: "hi"},
		{name: "missing values", entries: []tarEntry{{name: "c/Chart.yaml", body: "x"}}},
		{name: "oversized values", entries: []tarEntry{{name: "c/values.yaml", body: bigValues}}, status: 413},
		{name: "readme truncated", entries: []tarEntry{{name: "c/README.md", body: longReadme}}, readmeLength: helmMaxReadmeBytes},
		{name: "path traversal ignored", entries: []tarEntry{
			{name: "../values.yaml", body: "evil"}, {name: "c/../c/values.yaml", body: "evil"}, {name: "/c/values.yaml", body: "evil"},
			{name: "./c/values.yaml", body: "evil"}, {name: "c//values.yaml", body: "evil"}, {name: "c/values.yaml", body: "good"},
		}, values: "good"},
		{name: "symlink ignored", entries: []tarEntry{{name: "c/values.yaml", typ: tar.TypeSymlink, link: "/etc/passwd"}, {name: "c/README.md", typ: tar.TypeLink, link: "c/x"}}},
		{name: "symlink then real", entries: []tarEntry{{name: "c/values.yaml", typ: tar.TypeSymlink, link: "/etc/passwd"}, {name: "c/values.yaml", body: "real"}}, values: "real"},
		{name: "directory ignored", entries: []tarEntry{{name: "c/values.yaml", typ: tar.TypeDir}}},
		{name: "subchart values ignored", entries: []tarEntry{{name: "c/charts/sub/values.yaml", body: "sub"}, {name: "c/charts/sub/README.md", body: "sub"}, {name: "sub/values.yaml", body: "sub"}}},
		{name: "first values wins", entries: []tarEntry{{name: "c/values.yaml", body: "one"}, {name: "c/values.yaml", body: "two"}}, values: "one"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values, readme, err := readHelmChartArchive(makeTgz(t, tc.entries...), "c")
			if tc.status != 0 {
				wantHelmStatus(t, err, tc.status)
				if !strings.Contains(err.Error(), "chart archive too large") {
					t.Fatalf("message: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if values != tc.values {
				t.Errorf("values %q, want %q", values, tc.values)
			}
			if tc.readmeLength != 0 {
				if len(readme) != tc.readmeLength {
					t.Errorf("readme length %d", len(readme))
				}
			} else if readme != tc.readme {
				t.Errorf("readme %q, want %q", readme, tc.readme)
			}
		})
	}
	t.Run("not gzip", func(t *testing.T) {
		_, _, err := readHelmChartArchive([]byte("plain"), "c")
		wantHelmStatus(t, err, 502)
	})
	t.Run("not tar", func(t *testing.T) {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		_, _ = gz.Write(bytes.Repeat([]byte("x"), 2048))
		_ = gz.Close()
		_, _, err := readHelmChartArchive(buf.Bytes(), "c")
		wantHelmStatus(t, err, 502)
	})
	t.Run("decompression bomb in a skipped entry", func(t *testing.T) {
		old := helmMaxDecompressed
		helmMaxDecompressed = 1 << 20
		defer func() { helmMaxDecompressed = old }()
		bomb := makeTgz(t, tarEntry{name: "c/blob.bin", body: strings.Repeat("\x00", 4<<20)}, tarEntry{name: "c/values.yaml", body: "late"})
		if len(bomb) > 64<<10 {
			t.Fatalf("test archive is not a bomb: %d bytes", len(bomb))
		}
		_, _, err := readHelmChartArchive(bomb, "c")
		wantHelmStatus(t, err, 413)
	})
	t.Run("oversized compressed input", func(t *testing.T) {
		_, _, err := readHelmChartArchive(make([]byte, helmMaxArchiveBytes+1), "c")
		wantHelmStatus(t, err, 413)
	})
}
