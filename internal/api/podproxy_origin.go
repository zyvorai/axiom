package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

// The separate pod-proxy origin.
//
// The in-origin window route serves arbitrary pod web content from the
// console's own origin; even sandboxed, that is one browser bug or one
// relaxed header away from a pod UI scripting the console. When
// AXIOM_POD_PROXY_ADDR and AXIOM_POD_PROXY_ORIGIN are set, Axiom instead runs a
// second listener for pod content on an origin of its own:
//
//	console: POST /api/v1/pods/{ns}/{name}/window-link {"port":8080,"path":"/"}
//	         → {"url":"<origin>/p/<ticket>/","expiresIn":60}
//	proxy:   GET /p/<ticket>/   → redeems the single-use ticket, sets a
//	                              session cookie scoped to /s/<sid>/, 302
//	         ANY /s/<sid>/…     → proxied to the pod (session cookie required)
//	         anything else      → 404
//
// The proxy listener never reads the console's axiom_session cookie and never
// accepts Basic, Bearer or API-key credentials: a different port on the same
// host is a different origin, but browsers send host-scoped cookies to every
// port, so cookies seen there prove nothing about the console. It accepts only
// its own tickets and sessions (internal/auth/podproxy.go), and re-checks on
// every request that the user who created the session has not been revoked
// since.
//
// That layout puts every pod session on one origin. When AXIOM_POD_PROXY_ORIGIN
// carries an {id} placeholder in its leftmost host label instead
// (https://{id}.pods.example.com), every session gets a hostname — and so an
// origin — of its own and lives at the root of it; see podproxy_host.go.

const (
	podProxyCookieName       = "axiom_pod_proxy"
	podProxySecureCookieName = "__Secure-axiom_pod_proxy"
	// podProxyMaxBody caps a request body forwarded to a pod.
	podProxyMaxBody = 32 << 20
	// podProxyMaxHeaderBytes caps request headers on the proxy listener.
	podProxyMaxHeaderBytes = 32 << 10
	// podProxyCSP is the policy put on every proxied response. The origin is
	// isolated from the console, so the sandbox keeps allow-same-origin: the
	// session cookie is SameSite=Lax and an opaque-origin document would not
	// send it with its own sub-resource requests. Framing is forbidden.
	podProxyCSP = "sandbox allow-scripts allow-forms allow-same-origin allow-popups; frame-ancestors 'none'"
)

// PodProxyRevocationChecker reports whether user's credentials issued at
// issuedMs (Unix ms) have since been revoked (auth.Revocations.RevokeUser).
type PodProxyRevocationChecker interface {
	UserRevokedSince(user string, issuedMs int64) bool
}

type revocationsChecker struct{ r *auth.Revocations }

func (c revocationsChecker) UserRevokedSince(user string, issuedMs int64) bool {
	return c.r.Revoked(auth.Session{User: user, IatMs: issuedMs})
}

// PodProxyOriginConfig is the configuration of the separate pod-proxy origin.
type PodProxyOriginConfig struct {
	// Addr is the listen address of the proxy listener (AXIOM_POD_PROXY_ADDR).
	Addr string
	// Origin is the public origin browsers reach it on (AXIOM_POD_PROXY_ORIGIN).
	Origin string
	// ConsoleAddr and ConsoleOrigin (AXIOM_ADDR / AXIOM_ALLOW_ORIGIN) describe
	// the console, so a proxy origin equal to it is refused.
	ConsoleAddr   string
	ConsoleOrigin string
	// SessionSecret (AXIOM_SESSION_SECRET), when set, makes tickets and
	// session cookies signed tokens every replica with the same secret
	// accepts, so no session affinity is needed. Empty: tickets and sessions
	// stay in this process's memory, as with a random per-process secret.
	SessionSecret string
	// RedeemedStore shares the ids of redeemed tickets between replicas
	// (platform.OpenPodProxyRedeemedStore), so a ticket is single use across
	// them, best effort. Only used with SessionSecret; nil: per replica only.
	RedeemedStore auth.RevocationStore
}

// newPodProxyStores builds the ticket and session stores for cfg: shared
// (signed) with a session secret, in memory otherwise.
func newPodProxyStores(cfg PodProxyOriginConfig) (*auth.PodProxyTicketStore, *auth.PodProxySessionStore) {
	if secret := strings.TrimSpace(cfg.SessionSecret); secret != "" {
		return auth.NewSharedPodProxyTicketStore(secret, auth.NewPodProxyRedeemed(cfg.RedeemedStore)), auth.NewSharedPodProxySessionStore(secret)
	}
	return auth.NewPodProxyTicketStore(), auth.NewPodProxySessionStore()
}

// ErrPodProxyDisabled is returned by NewPodProxyOrigin when neither setting
// is present (the feature is simply off).
var ErrPodProxyDisabled = errors.New("pod proxy origin not configured")

// PodProxyOrigin is the separate pod-proxy origin: the ticket and session
// stores the console's window-link endpoint and the proxy listener share, and
// the listener's handler.
type PodProxyOrigin struct {
	addr    string
	origin  string // normalized, no trailing slash
	secure  bool   // origin is https
	tickets *auth.PodProxyTicketStore
	sess    *auth.PodProxySessionStore

	// host is set in per-session-host mode (AXIOM_POD_PROXY_ORIGIN with an
	// {id} placeholder, podproxy_host.go); nil in fixed-origin mode.
	host *podProxyHostMode

	mu       sync.RWMutex
	provider platform.Provider
	revoked  PodProxyRevocationChecker
}

// NewPodProxyOrigin validates cfg. It returns ErrPodProxyDisabled when both
// settings are empty and another error when the configuration is unsafe or
// incomplete; in either case the caller keeps the in-origin window route.
func NewPodProxyOrigin(cfg PodProxyOriginConfig) (*PodProxyOrigin, error) {
	addr, origin := strings.TrimSpace(cfg.Addr), strings.TrimSpace(cfg.Origin)
	if addr == "" && origin == "" {
		return nil, ErrPodProxyDisabled
	}
	if addr == "" || origin == "" {
		return nil, errors.New("AXIOM_POD_PROXY_ADDR and AXIOM_POD_PROXY_ORIGIN must both be set")
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return nil, fmt.Errorf("AXIOM_POD_PROXY_ADDR %q: %v", addr, err)
	}
	if strings.Contains(origin, podProxyIDPlaceholder) {
		return newPodProxyHostOrigin(cfg, addr, origin)
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return nil, fmt.Errorf("AXIOM_POD_PROXY_ORIGIN %q must be an origin such as https://pods.example.com", origin)
	}
	pHost, pPort := originHostPort(u)
	if sameListenAddr(addr, cfg.ConsoleAddr) {
		return nil, fmt.Errorf("AXIOM_POD_PROXY_ADDR %q is the console's own listen address", addr)
	}
	if cu, err := url.Parse(strings.TrimSpace(cfg.ConsoleOrigin)); err == nil && cu.Host != "" {
		cHost, cPort := originHostPort(cu)
		if cHost == pHost && cPort == pPort {
			return nil, fmt.Errorf("AXIOM_POD_PROXY_ORIGIN %q is the console's origin (AXIOM_ALLOW_ORIGIN); it must differ by host or port", origin)
		}
	}
	if lh, lp, err := net.SplitHostPort(strings.TrimSpace(cfg.ConsoleAddr)); err == nil && lp == pPort {
		lh = strings.ToLower(lh)
		if lh == pHost || ((lh == "" || lh == "0.0.0.0" || lh == "::") && isLoopbackHost(pHost)) || (isLoopbackHost(lh) && isLoopbackHost(pHost)) {
			return nil, fmt.Errorf("AXIOM_POD_PROXY_ORIGIN %q points at the console's listen address %q; it must differ by host or port", origin, cfg.ConsoleAddr)
		}
	}
	tickets, sess := newPodProxyStores(cfg)
	return &PodProxyOrigin{
		addr:    addr,
		origin:  u.Scheme + "://" + u.Host,
		secure:  u.Scheme == "https",
		tickets: tickets,
		sess:    sess,
	}, nil
}

// NewPodProxyOriginFromEnv wraps NewPodProxyOrigin for main: an unset
// configuration returns nil quietly, a bad one is logged and returns nil.
func NewPodProxyOriginFromEnv(cfg PodProxyOriginConfig) *PodProxyOrigin {
	p, err := NewPodProxyOrigin(cfg)
	switch {
	case errors.Is(err, ErrPodProxyDisabled):
		return nil
	case err != nil:
		log.Printf("pod proxy origin disabled: %v; pod windows stay on the console origin", err)
		return nil
	}
	return p
}

func originHostPort(u *url.URL) (string, string) {
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	return host, port
}

func isLoopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func sameListenAddr(a, b string) bool {
	ah, ap, err1 := net.SplitHostPort(strings.TrimSpace(a))
	bh, bp, err2 := net.SplitHostPort(strings.TrimSpace(b))
	if err1 != nil || err2 != nil || ap != bp {
		return false
	}
	wild := func(h string) bool { return h == "" || h == "0.0.0.0" || h == "::" }
	return ah == bh || wild(ah) || wild(bh)
}

// Addr is the proxy listener's listen address.
func (p *PodProxyOrigin) Addr() string { return p.addr }

// Origin is the public origin of the proxy listener (in per-session-host mode
// the template, with its {id} placeholder).
func (p *PodProxyOrigin) Origin() string { return p.origin }

// Mode is "host" when every proxy session gets its own hostname and "path"
// for the fixed origin with /s/<sid>/ prefixes.
func (p *PodProxyOrigin) Mode() string {
	if p.host != nil {
		return "host"
	}
	return "path"
}

// Shared reports whether tickets and sessions are signed tokens any replica
// with the same session secret accepts (false: in this process's memory).
func (p *PodProxyOrigin) Shared() bool { return p.tickets.Shared() && p.sess.Shared() }

// attach wires the console's provider and revocation set; NewWithUsers calls
// it. Until then the proxy listener answers 503.
func (p *PodProxyOrigin) attach(provider platform.Provider, revoked PodProxyRevocationChecker) {
	p.mu.Lock()
	p.provider, p.revoked = provider, revoked
	p.mu.Unlock()
}

func (p *PodProxyOrigin) deps() (platform.Provider, PodProxyRevocationChecker) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.provider, p.revoked
}

func (p *PodProxyOrigin) cookieName() string {
	if p.host != nil && p.secure {
		return podProxyHostCookieName
	}
	if p.secure {
		return podProxySecureCookieName
	}
	return podProxyCookieName
}

// Server returns the proxy listener's http.Server: its own handler (no CORS,
// none of the console's routes), bounded headers, a header read timeout, and
// HTTP/1.1 only (WebSocket upgrades). There is no whole-request or write
// timeout because proxied responses may stream and WebSockets are long-lived;
// request bodies are capped instead.
func (p *PodProxyOrigin) Server() *http.Server {
	return &http.Server{
		Addr:              p.addr,
		Handler:           p.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    podProxyMaxHeaderBytes,
		TLSNextProto:      map[string]func(*http.Server, *tls.Conn, http.Handler){},
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
}

// Handler serves /p/<ticket>/ and /s/<sid>/… and 404s everything else.
func (p *PodProxyOrigin) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-store")
		if p.secure {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if p.host != nil {
			p.serveHost(w, r)
			return
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/p/"):
			p.redeem(w, r)
		case strings.HasPrefix(r.URL.Path, "/s/"):
			p.proxy(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

func podProxyDeny(w http.ResponseWriter, code int, msg string) {
	http.Error(w, msg, code)
}

// redeem handles GET /p/<ticket>/…: the ticket is consumed (whatever happens
// next), a session is created and the browser is sent to /s/<sid>/<path>.
func (p *PodProxyOrigin) redeem(w http.ResponseWriter, r *http.Request) {
	// GET only: a HEAD (link preview, prefetch probe) must not burn the ticket.
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
	if g.SessionID != "" || !podProxyGrantAllowed(revoked, g) {
		podProxyDeny(w, http.StatusForbidden, "this pod link is no longer valid")
		return
	}
	if !p.grantClusterServable(w, g) {
		return
	}
	sid, secret, err := p.sess.Create(g)
	if err != nil {
		podProxyDeny(w, http.StatusTooManyRequests, err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     p.cookieName(),
		Value:    secret,
		Path:     "/s/" + sid + "/",
		MaxAge:   int(auth.PodProxySessionMax / time.Second),
		HttpOnly: true,
		Secure:   p.secure,
		SameSite: http.SameSiteLaxMode,
	})
	platform.IncWindow(g.Namespace, g.Pod)
	http.Redirect(w, r, "/s/"+sid+g.Path, http.StatusFound)
}

func podProxyRoleAllowed(role string) bool { return role == "operator" || role == "admin" }

// podProxyGrantAllowed is the check every redeem and every proxied request
// makes: the creating user has not been revoked since the ticket was minted,
// the role snapshot may use the proxy and the namespace restriction snapshot
// covers the pod's namespace.
func podProxyGrantAllowed(revoked PodProxyRevocationChecker, g auth.PodProxyGrant) bool {
	return !revoked.UserRevokedSince(g.User, g.IssuedMs) && podProxyRoleAllowed(g.Role) && g.NamespaceAllowed()
}

// sessionCookiePath is where session sid's cookie is scoped.
func (p *PodProxyOrigin) sessionCookiePath(sid string) string {
	if p.host != nil {
		return "/"
	}
	return "/s/" + sid + "/"
}

// hasValidSessionCookie reports whether r carries a live cookie for session
// sid (without sliding it). In shared mode that is how a replica that did not
// create the session knows it exists.
func (p *PodProxyOrigin) hasValidSessionCookie(r *http.Request, sid string) bool {
	for _, c := range r.Cookies() {
		if c.Name == p.cookieName() && p.sess.Valid(sid, c.Value) {
			return true
		}
	}
	return false
}

// proxy handles /s/<sid>/…: the session cookie must belong to the session
// named in the path, the creating user must not have been revoked since, and
// then the request goes to the pod through the shared proxy core.
func (p *PodProxyOrigin) proxy(w http.ResponseWriter, r *http.Request) {
	sid, rest, hasSlash := strings.Cut(strings.TrimPrefix(r.URL.Path, "/s/"), "/")
	if sid == "" {
		http.NotFound(w, r)
		return
	}
	g, provider, ok := p.authorize(w, r, sid)
	if !ok {
		return
	}
	if !hasSlash {
		http.Redirect(w, r, "/s/"+sid+"/", http.StatusFound)
		return
	}
	p.forward(w, r, provider, g, "/"+rest)
}

// authorize checks the method, the session cookie for session sid and the
// creating user's revocation state, answering the request itself when one of
// them fails.
func (p *PodProxyOrigin) authorize(w http.ResponseWriter, r *http.Request, sid string) (auth.PodProxyGrant, platform.Provider, bool) {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		w.Header().Set("Allow", "GET, HEAD, POST, PUT, PATCH, DELETE")
		podProxyDeny(w, http.StatusMethodNotAllowed, "method not allowed")
		return auth.PodProxyGrant{}, nil, false
	}
	provider, revoked := p.deps()
	if provider == nil || revoked == nil {
		podProxyDeny(w, http.StatusServiceUnavailable, "pod proxy is not ready")
		return auth.PodProxyGrant{}, nil, false
	}
	var (
		g       auth.PodProxyGrant
		renewed string
		maxAge  time.Duration
		ok      bool
	)
	for _, c := range r.Cookies() {
		if c.Name != p.cookieName() {
			continue
		}
		if g, renewed, maxAge, ok = p.sess.Authorize(sid, c.Value); ok {
			break
		}
	}
	if !ok {
		podProxyDeny(w, http.StatusForbidden, "no valid pod proxy session; open the port again from the console")
		return auth.PodProxyGrant{}, nil, false
	}
	if !podProxyGrantAllowed(revoked, g) {
		p.sess.Delete(sid)
		podProxyDeny(w, http.StatusForbidden, "this pod proxy session was revoked")
		return auth.PodProxyGrant{}, nil, false
	}
	if renewed != "" {
		// Shared mode: slide the idle expiry by re-issuing the signed cookie
		// (same name, path and attributes; never past the absolute maximum).
		// The pod's own Set-Cookie headers are still stripped.
		http.SetCookie(w, &http.Cookie{
			Name:     p.cookieName(),
			Value:    renewed,
			Path:     p.sessionCookiePath(sid),
			MaxAge:   max(1, int(maxAge/time.Second)),
			HttpOnly: true,
			Secure:   p.secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
	return g, provider, true
}

// forward sends an authorized request to the session's pod, with upstreamPath
// as the path, through the shared proxy core.
func (p *PodProxyOrigin) forward(w http.ResponseWriter, r *http.Request, provider platform.Provider, g auth.PodProxyGrant, upstreamPath string) {
	// A service worker registered here could intercept other sessions on this
	// origin (in host mode: outlive the session on its host); pod content may
	// not install one.
	if r.Header.Get("Service-Worker") != "" {
		podProxyDeny(w, http.StatusForbidden, "service workers are not allowed on the pod proxy origin")
		return
	}
	pinned, ok := podProxyPinnedProvider(w, provider, g)
	if !ok {
		return
	}
	target, err := pinned.PodProxyTarget(r.Context(), g.Namespace, g.Pod, g.Port)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	if r.ContentLength > podProxyMaxBody {
		podProxyDeny(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	if r.Body != nil && r.Body != http.NoBody {
		r.Body = http.MaxBytesReader(w, r.Body, podProxyMaxBody)
	}
	r.Header.Del(platform.ClusterSessionHeader)
	r.Header.Del("X-Api-Key")
	// The listener's policy headers are already on w; ReverseProxy adds the
	// pod's headers to them, so the hardening step removes the pod's copies
	// of those (and a few more it must not control) instead of setting them.
	w.Header().Set("Content-Security-Policy", podProxyCSP)
	w.Header().Del("Cache-Control")
	servePodProxy(w, r, target, upstreamPath, hardenPodProxyOrigin)
}

// errPodProxyClusterGone is wrapped by podProxyClusterProvider when a pod
// proxy grant names a cluster this replica cannot serve it from.
var errPodProxyClusterGone = errors.New("pod proxy cluster unavailable")

// podProxyLinkCluster resolves, for a window-link request, the cluster the
// pod is looked up on and the Provider of exactly that cluster: on a
// multi-cluster registry the calling tab's active cluster (by registered
// name), otherwise "" and the provider itself.
func podProxyLinkCluster(ctx context.Context, provider platform.Provider) (string, platform.Provider, error) {
	sp, ok := provider.(*platform.SwitchableProvider)
	if !ok {
		return "", provider, nil
	}
	name := sp.ActiveName(ctx)
	p, ok := sp.ProviderFor(name)
	if !ok {
		return "", nil, platform.NewStatusError(http.StatusNotFound, fmt.Errorf("no active cluster to open the pod on"))
	}
	return name, p, nil
}

// podProxyClusterProvider returns the Provider of the cluster a pod proxy
// grant was minted on, independent of any cluster selection: a session never
// follows a console tab (or a replica's default) to another cluster. A grant
// naming a cluster that is not registered here (removed since, or never known
// to this replica), a grant without a cluster on a multi-cluster registry
// (minted before grants were pinned) and a grant naming a cluster on a
// single-cluster provider are all refused.
func podProxyClusterProvider(provider platform.Provider, cluster string) (platform.Provider, error) {
	sp, multi := provider.(*platform.SwitchableProvider)
	switch {
	case multi && cluster == "":
		return nil, fmt.Errorf("%w: this pod session does not record its cluster", errPodProxyClusterGone)
	case multi:
		p, ok := sp.ProviderFor(cluster)
		if !ok {
			return nil, fmt.Errorf("%w: cluster %q this pod session was opened on is no longer registered", errPodProxyClusterGone, cluster)
		}
		return p, nil
	case cluster != "":
		return nil, fmt.Errorf("%w: cluster %q this pod session was opened on is not known here", errPodProxyClusterGone, cluster)
	}
	return provider, nil
}

// podProxyPinnedProvider is podProxyClusterProvider for a request: when the
// grant's cluster cannot be served here it logs why and answers 410.
func podProxyPinnedProvider(w http.ResponseWriter, provider platform.Provider, g auth.PodProxyGrant) (platform.Provider, bool) {
	pinned, err := podProxyClusterProvider(provider, g.Cluster)
	if err != nil {
		log.Printf("pod proxy: refusing %s/%s port %d for user %s: %v", g.Namespace, g.Pod, g.Port, g.User, err)
		podProxyDeny(w, http.StatusGone, err.Error()+"; open the port again from the console")
		return nil, false
	}
	return pinned, true
}

// grantClusterServable is the redeem-time cluster check: a ticket for a
// cluster this replica cannot serve does not become a session.
func (p *PodProxyOrigin) grantClusterServable(w http.ResponseWriter, g auth.PodProxyGrant) bool {
	provider, _ := p.deps()
	if provider == nil {
		podProxyDeny(w, http.StatusServiceUnavailable, "pod proxy is not ready")
		return false
	}
	_, ok := podProxyPinnedProvider(w, provider, g)
	return ok
}

// podProxyPolicyHeaders are set by the proxy listener itself on every
// response; a pod cannot override or add to them.
var podProxyPolicyHeaders = []string{
	"Content-Security-Policy", "X-Content-Type-Options", "Referrer-Policy",
	"Cross-Origin-Opener-Policy", "X-Frame-Options", "Strict-Transport-Security",
}

// hardenPodProxyOrigin strips from a pod's response the headers the proxy
// origin controls (see Handler and proxy, which set them on the writer).
func hardenPodProxyOrigin(h http.Header) {
	for _, k := range podProxyPolicyHeaders {
		h.Del(k)
	}
	h.Del("Content-Security-Policy-Report-Only")
	h.Del("Clear-Site-Data")
	h.Del("Service-Worker-Allowed")
	for k := range h {
		if strings.HasPrefix(http.CanonicalHeaderKey(k), "Access-Control-") {
			h.Del(k)
		}
	}
}

// --- console side -----------------------------------------------------------

type windowLinkRequest struct {
	Port int64  `json:"port"`
	Path string `json:"path"`
}

// podWindowLink is POST /api/v1/pods/{namespace}/{name}/window-link. The guard
// gives it the window route's policy (operator/admin, namespace-checked).
func (s *Server) podWindowLink(w http.ResponseWriter, r *http.Request) {
	if s.podProxy == nil {
		fail(w, http.StatusNotFound, errors.New("the separate pod proxy origin is not configured (AXIOM_POD_PROXY_ADDR / AXIOM_POD_PROXY_ORIGIN); use the window route"))
		return
	}
	id, ok := auth.IdentityFromContext(r.Context())
	if !ok || id.User == "" {
		fail(w, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	if !podProxyRoleAllowed(id.Role) {
		fail(w, http.StatusForbidden, errors.New("the pod proxy requires the operator role"))
		return
	}
	var req windowLinkRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 8<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, fmt.Errorf("invalid body: %v", err))
		return
	}
	if req.Port <= 0 || req.Port > 65535 {
		fail(w, http.StatusBadRequest, errors.New("port must be between 1 and 65535"))
		return
	}
	if req.Path == "" {
		req.Path = "/"
	}
	if !auth.ValidPodProxyBasePath(req.Path) {
		fail(w, http.StatusBadRequest, errors.New("path must be an absolute path such as /"))
		return
	}
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	// The link (and every session made from it) is pinned to the cluster the
	// pod is resolved on now, by its registered name.
	cluster, pinned, err := podProxyLinkCluster(r.Context(), s.provider)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	// Refuse unknown pods and undeclared ports now rather than on the proxy
	// origin.
	if _, err := pinned.PodProxyTarget(r.Context(), ns, name, int32(req.Port)); err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	// In host mode the session id is chosen now: it is the link's host label,
	// and the ticket can only be redeemed on that host.
	sessionID := ""
	if s.podProxy.host != nil {
		sessionID = auth.NewPodProxySessionID()
	}
	tok, _, err := s.podProxy.tickets.Issue(auth.PodProxyGrant{
		User: id.User, Role: id.Role, Namespaces: id.Namespaces, Namespace: ns, Pod: name, Port: int32(req.Port),
		Path: req.Path, Cluster: cluster, SessionID: sessionID,
	})
	if err != nil {
		if errors.Is(err, auth.ErrPodProxyTableFull) {
			fail(w, http.StatusTooManyRequests, err)
		} else {
			fail(w, http.StatusBadRequest, err)
		}
		return
	}
	detail := "port " + strconv.FormatInt(req.Port, 10) + " via pod proxy origin"
	if cluster != "" {
		detail += " on cluster " + cluster
	}
	platform.RecordAxiomAudit("pod.window", ns+"/"+name, detail, id.User)
	w.Header().Set("Cache-Control", "no-store")
	origin := s.podProxy.origin
	if s.podProxy.host != nil {
		origin = s.podProxy.host.originFor(sessionID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"url":       origin + "/p/" + tok + "/",
		"expiresIn": int(auth.PodProxyTicketTTL / time.Second),
	})
}

// podProxyMode is what the capabilities endpoint reports: "" when the pod
// proxy origin is off, else "path" or "host" (PodProxyOrigin.Mode).
func (s *Server) podProxyMode() string {
	if s.podProxy == nil {
		return ""
	}
	return s.podProxy.Mode()
}

// StartPodProxyOrigin runs p's listener until ctx is cancelled, with TLS when
// certFile/keyFile are set. It returns once the listener is serving or failed
// to bind; serving errors after that are logged.
func StartPodProxyOrigin(ctx context.Context, p *PodProxyOrigin, certFile, keyFile string) (*http.Server, error) {
	srv := p.Server()
	ln, err := net.Listen("tcp", p.addr)
	if err != nil {
		return nil, err
	}
	srv.Addr = ln.Addr().String()
	go func() {
		var err error
		if certFile != "" && keyFile != "" {
			err = srv.ServeTLS(ln, certFile, keyFile)
		} else {
			err = srv.Serve(ln)
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("pod proxy origin: %v", err)
		}
	}()
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	return srv, nil
}
