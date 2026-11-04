package api

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

// Per-session-host mode of the pod proxy origin.
//
// In the fixed-origin mode every pod session lives under /s/<sid>/ on one
// origin, so pod pages opened through the proxy share localStorage, IndexedDB
// and non-HttpOnly cookies with each other. When AXIOM_POD_PROXY_ORIGIN holds
// the placeholder {id} as its leftmost host label
// (https://{id}.pods.example.com, or http://{id}.localhost:5091 for a lab),
// every session gets a hostname, and so an origin, of its own:
//
//	console: POST /api/v1/pods/{ns}/{name}/window-link
//	         → {"url":"https://<sid>.pods.example.com/p/<ticket>/","expiresIn":60}
//	         (<sid>: 128-bit random, 32 hex characters, chosen now; the
//	         ticket is bound to it)
//	proxy, Host <sid>.pods.example.com:
//	         GET /p/<ticket>/   → only while no session <sid> exists: redeems
//	                              the ticket (which must be bound to <sid>),
//	                              sets a host-only cookie (Path=/), 302 to
//	                              the link's base path
//	         ANY /…             → once the session exists: proxied to the pod
//	                              at the same path (absolute paths work)
//	         any other Host, unknown or expired <sid> → 404, empty body
//
// The id in the host is public; the secret in the cookie authorizes. A cookie
// from one session's host is never sent to another's, and even when replayed
// by hand it does not match the other session's secret.

const (
	podProxyIDPlaceholder = "{id}"
	// podProxyIDSentinel stands in for {id} while the template is parsed.
	podProxyIDSentinel = "axiom-pod-proxy-id-placeholder"
	// podProxyHostCookieName is the session cookie on an https per-session
	// host: host-only, Path=/, Secure, which the __Host- prefix enforces.
	podProxyHostCookieName = "__Host-axiom_pod_proxy"
)

// podProxyHostMode describes the per-session hostnames.
type podProxyHostMode struct {
	scheme      string // "http" or "https"
	base        string // lowercase base domain, e.g. "pods.example.com"
	hostSuffix  string // "." + base + optional ":port" as configured
	port        string // effective port (explicit, else the scheme's default)
	defaultPort string // the scheme's default port
}

// originFor is the origin of session id.
func (m *podProxyHostMode) originFor(id string) string {
	return m.scheme + "://" + id + m.hostSuffix
}

// sessionID extracts the session id from a Host header of the form
// <32 lowercase hex>.<base>[:port], or reports false.
func (m *podProxyHostMode) sessionID(hostHeader string) (string, bool) {
	h := strings.ToLower(hostHeader)
	host, port := h, ""
	if i := strings.LastIndexByte(h, ':'); i >= 0 {
		host, port = h[:i], h[i+1:]
	}
	if port == "" {
		port = m.defaultPort
	}
	if port != m.port {
		return "", false
	}
	id, rest, ok := strings.Cut(host, ".")
	if !ok || rest != m.base || !auth.ValidPodProxySessionID(id) {
		return "", false
	}
	return id, true
}

// newPodProxyHostOrigin validates an AXIOM_POD_PROXY_ORIGIN template with an
// {id} placeholder (addr has already been checked).
func newPodProxyHostOrigin(cfg PodProxyOriginConfig, addr, origin string) (*PodProxyOrigin, error) {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("AXIOM_POD_PROXY_ORIGIN %q: "+format, append([]any{origin}, a...)...)
	}
	if strings.Count(origin, podProxyIDPlaceholder) != 1 {
		return nil, bad("the %s placeholder must appear exactly once", podProxyIDPlaceholder)
	}
	u, err := url.Parse(strings.Replace(origin, podProxyIDPlaceholder, podProxyIDSentinel, 1))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return nil, bad("must be an origin template such as https://{id}.pods.example.com")
	}
	host := strings.ToLower(u.Hostname())
	base, ok := strings.CutPrefix(host, podProxyIDSentinel+".")
	if !ok || strings.Contains(base, podProxyIDSentinel) {
		return nil, bad("%s must be the whole leftmost host label, as in https://{id}.pods.example.com", podProxyIDPlaceholder)
	}
	if !validDNSName(base) {
		return nil, bad("the host after %s. must be a DNS name such as pods.example.com", podProxyIDPlaceholder)
	}
	_, port := originHostPort(u)
	defaultPort := "80"
	if u.Scheme == "https" {
		defaultPort = "443"
	}
	if sameListenAddr(addr, cfg.ConsoleAddr) {
		return nil, fmt.Errorf("AXIOM_POD_PROXY_ADDR %q is the console's own listen address", addr)
	}
	// A pod page may set cookies for any parent domain of its own host. The
	// console must therefore not live on the base domain or under it (the pod
	// could set cookies the console receives), and the base domain must not be
	// under the console's host either.
	if cu, err := url.Parse(strings.TrimSpace(cfg.ConsoleOrigin)); err == nil && cu.Host != "" {
		cHost, _ := originHostPort(cu)
		switch {
		case cHost == base:
			return nil, bad("the console's host %q (AXIOM_ALLOW_ORIGIN) is the pod base domain; use a base domain the console is not on", cHost)
		case strings.HasSuffix(cHost, "."+base):
			return nil, bad("the console's host %q (AXIOM_ALLOW_ORIGIN) is inside the pod base domain %q; pod pages could set cookies it receives", cHost, base)
		case strings.HasSuffix(base, "."+cHost):
			return nil, bad("the pod base domain %q is inside the console's host %q (AXIOM_ALLOW_ORIGIN); use a separate domain", base, cHost)
		}
	}
	if lh, lp, err := net.SplitHostPort(strings.TrimSpace(cfg.ConsoleAddr)); err == nil && lp == port {
		lh = strings.ToLower(lh)
		loopBase := base == "localhost" || strings.HasSuffix(base, ".localhost")
		if loopBase && (lh == "" || lh == "0.0.0.0" || lh == "::" || isLoopbackHost(lh)) {
			return nil, bad("points at the console's listen address %q; it must differ by port", cfg.ConsoleAddr)
		}
	}
	tickets, sess := newPodProxyStores(cfg)
	return &PodProxyOrigin{
		addr:    addr,
		origin:  u.Scheme + "://" + podProxyIDPlaceholder + "." + strings.ToLower(u.Host)[len(podProxyIDSentinel)+1:],
		secure:  u.Scheme == "https",
		tickets: tickets,
		sess:    sess,
		host: &podProxyHostMode{
			scheme:      u.Scheme,
			base:        base,
			hostSuffix:  strings.ToLower(u.Host)[len(podProxyIDSentinel):],
			port:        port,
			defaultPort: defaultPort,
		},
	}, nil
}

// validDNSName reports whether s is a dot-separated DNS name of LDH labels
// (not an IP literal, no trailing dot).
func validDNSName(s string) bool {
	if s == "" || len(s) > 253 || net.ParseIP(s) != nil {
		return false
	}
	for _, l := range strings.Split(s, ".") {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
		for i := 0; i < len(l); i++ {
			c := l[i]
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

// podProxyNotFound is the host-mode 404: no body, nothing about why.
func podProxyNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Length", "0")
	w.WriteHeader(http.StatusNotFound)
}

// serveHost is the host-mode handler (the policy headers are already set).
func (p *PodProxyOrigin) serveHost(w http.ResponseWriter, r *http.Request) {
	sid, ok := p.host.sessionID(r.Host)
	if !ok {
		podProxyNotFound(w)
		return
	}
	// A session is live if this replica holds it or (shared mode) the request
	// carries a valid signed cookie for it, which another replica may have
	// issued.
	if p.sess.Live(sid) || (p.sess.Shared() && p.hasValidSessionCookie(r, sid)) {
		// After the first use /p/ is an ordinary pod path.
		g, provider, ok := p.authorize(w, r, sid)
		if !ok {
			return
		}
		p.forward(w, r, provider, g, r.URL.Path)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/p/") {
		p.redeemHost(w, r, sid)
		return
	}
	podProxyNotFound(w)
}

// redeemHost handles GET /p/<ticket>/… on the host of session sid before that
// session exists: the ticket is consumed whatever happens next and must have
// been minted for sid.
func (p *PodProxyOrigin) redeemHost(w http.ResponseWriter, r *http.Request, sid string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		podProxyDeny(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	token, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/p/"), "/")
	_, revoked := p.deps()
	if revoked == nil {
		podProxyDeny(w, http.StatusServiceUnavailable, "pod proxy is not ready")
		return
	}
	g, ok := p.tickets.Consume(token)
	if !ok {
		podProxyDeny(w, http.StatusForbidden, "this pod link has expired or was already used; open the port again from the console")
		return
	}
	if g.SessionID != sid || !podProxyGrantAllowed(revoked, g) {
		podProxyDeny(w, http.StatusForbidden, "this pod link is no longer valid")
		return
	}
	if !p.grantClusterServable(w, g) {
		return
	}
	secret, err := p.sess.CreateWithID(sid, g)
	if err != nil {
		code := http.StatusForbidden
		if errors.Is(err, auth.ErrPodProxyTableFull) {
			code = http.StatusTooManyRequests
		}
		podProxyDeny(w, code, err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     p.cookieName(),
		Value:    secret,
		Path:     "/",
		MaxAge:   int(auth.PodProxySessionMax / time.Second),
		HttpOnly: true,
		Secure:   p.secure,
		SameSite: http.SameSiteLaxMode,
	})
	platform.IncWindow(g.Namespace, g.Pod)
	http.Redirect(w, r, g.Path, http.StatusFound)
}

// PodProxyTLSFiles picks the pod proxy listener's certificate: its own pair
// (AXIOM_POD_PROXY_TLS_CERT/KEY, e.g. a wildcard certificate for host mode)
// when both are set, else the console's. A half-set own pair is logged and
// ignored.
func PodProxyTLSFiles(ownCert, ownKey, consoleCert, consoleKey string) (string, string) {
	if ownCert != "" && ownKey != "" {
		return ownCert, ownKey
	}
	if ownCert != "" || ownKey != "" {
		log.Printf("pod proxy origin: AXIOM_POD_PROXY_TLS_CERT and AXIOM_POD_PROXY_TLS_KEY must both be set; using the console's certificate")
	}
	return consoleCert, consoleKey
}

// podProxyCapabilities adds the pod proxy fields to a capabilities response:
// "podProxy" (on/off) and, when on, "podProxyMode" ("path" or "host").
func podProxyCapabilities(caps map[string]any, mode func() string) map[string]any {
	m := ""
	if mode != nil {
		m = mode()
	}
	caps["podProxy"] = m != ""
	if m != "" {
		caps["podProxyMode"] = m
	}
	return caps
}
