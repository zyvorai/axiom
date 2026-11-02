package auth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"math"
	"net/http"
	"strconv"
	"strings"
)

// UserAuthenticator checks local multi-user credentials beyond the single
// env-configured operator account. Implemented by platform.UserHub.
// namespaces is non-empty only for a namespace-restricted viewer account.
type UserAuthenticator interface {
	Authenticate(username, password string) (role string, namespaces []string, ok bool)
}

// Identity is the resolved caller of an authorized request, attached to the
// request context so handlers and audit logging can read who acted and
// with what role without re-parsing credentials.
type Identity struct {
	User string
	Role string
	// Namespaces restricts this identity to the listed namespaces when
	// non-empty. Only ever set for local viewer accounts — the env admin
	// account, API keys, and OIDC/SAML sessions are always unrestricted.
	Namespaces []string
}

func (id Identity) namespaceRestricted() bool {
	return len(id.Namespaces) > 0
}

func (id Identity) allowsNamespace(ns string) bool {
	for _, allowed := range id.Namespaces {
		if allowed == ns {
			return true
		}
	}
	return false
}

type ctxKey string

const identityCtxKey ctxKey = "axiom-identity"

// IdentityFromContext returns the authenticated caller for this request, if any.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	v, ok := ctx.Value(identityCtxKey).(Identity)
	return v, ok
}

type Guard struct {
	Username string
	Password string
	APIKey   string
	// SessionSecret signs OIDC session cookies. Defaults to Password when empty.
	SessionSecret string
	// Users resolves local multi-user accounts created under Settings → Users.
	// Optional — nil means only the single env-configured account exists.
	Users UserAuthenticator
	// LDAP, when enabled, tries a directory simple-bind for Basic auth
	// credentials before falling through to local users.
	LDAP LDAPConfig
	// Limiter throttles repeated failed logins per client IP. Optional — a
	// fresh one is created automatically in Middleware when left nil.
	Limiter *RateLimiter
	// Audit records failed logins and rate-limit lockouts to the platform
	// audit trail. Optional — nil means these events aren't audited (they
	// still fully function, just without a security-review trail).
	Audit AuditLogger
	// TrustedProxyHeader, when set, is the header (e.g. "X-Forwarded-For")
	// the rate limiter reads the real client IP from instead of
	// RemoteAddr. Leave empty unless Axiom sits behind a reverse proxy
	// that itself sets/overwrites this header — see clientKey's doc
	// comment in ratelimit.go for why trusting it by default is unsafe.
	TrustedProxyHeader string
}

// AuditLogger records a security-relevant event. Implemented by an adapter
// around platform.RecordAxiomAudit — kept as an interface here so this
// package doesn't need to import platform.
type AuditLogger interface {
	Record(action, target, detail, user string)
}

func (g Guard) sessionSecret() string {
	if g.SessionSecret != "" {
		return g.SessionSecret
	}
	return g.Password
}

func (g Guard) Middleware(next http.Handler) http.Handler {
	limiter := g.Limiter
	if limiter == nil {
		limiter = NewRateLimiter()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r) {
			next.ServeHTTP(w, r)
			return
		}
		key := clientKey(r, g.TrustedProxyHeader)
		if !limiter.Allowed(key) {
			if g.Audit != nil {
				g.Audit.Record("auth.rate_limited", key, r.URL.Path, attemptedUsername(r))
			}
			w.Header().Set("Content-Type", "application/json")
			// Round up so Retry-After never tells a client to retry before
			// the lockout has actually cleared.
			retryAfter := int(math.Ceil(limiter.RetryAfter(key).Seconds()))
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"too many failed login attempts — try again shortly"}`))
			return
		}
		id, ok := g.authorizeDetail(r)
		if !ok {
			limiter.RecordFailure(key)
			if g.Audit != nil {
				g.Audit.Record("auth.login_failed", key, r.URL.Path, attemptedUsername(r))
			}
			// Do not set WWW-Authenticate — that triggers the browser's native
			// Basic-auth popup on top of Axiom's custom login form.
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		limiter.RecordSuccess(key)
		deny := func(code int, msg string) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
		}
		if id.Role == "viewer" && isMutatingMethod(r.Method) {
			deny(http.StatusForbidden, "read-only user: this account cannot perform write actions")
			return
		}
		if id.Role == "viewer" && isInteractivePath(r.URL.Path) {
			deny(http.StatusForbidden, "read-only user: shells, consoles and the pod proxy require the operator role")
			return
		}
		if sns, isVals := secretValuesTarget(r); isVals {
			if id.Role == "viewer" {
				deny(http.StatusForbidden, "read-only user: reading secret values requires the operator role")
				return
			}
			if id.Role != "admin" && strings.EqualFold(sns, axiomNamespace) {
				deny(http.StatusForbidden, "secret values in the Axiom namespace require the admin role")
				return
			}
		}
		if id.Role == "operator" && ((isMutatingMethod(r.Method) && isAdminOnlyPath(r.URL.Path)) || isOperatorForbidden(r)) {
			deny(http.StatusForbidden, "operator role cannot manage users, integrations, RBAC, Helm installs, or the cluster registry")
			return
		}
		if id.namespaceRestricted() {
			if restrictedDenied(r) {
				deny(http.StatusForbidden, "this account is restricted to specific namespaces")
				return
			}
			if nss, scoped := namespacesOfRequest(r); scoped {
				for _, ns := range nss {
					if ns == "" || strings.EqualFold(ns, "all") || !id.allowsNamespace(ns) {
						deny(http.StatusForbidden, "this account is restricted to specific namespaces")
						return
					}
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityCtxKey, id)))
	})
}

// namespacedResourcePrefixes lists the API paths that expose namespaced
// resources — via a `?namespace=` query parameter on the list form, or a
// `{namespace}` path segment on the single-resource GET form. Deliberately
// conservative: cluster-wide administrative views (RBAC, Cost, Audit, Users,
// Clusters, Nodes, Settings/Integrations) are intentionally left out, since
// namespace-scoping a viewer is about workload/resource visibility, not
// administrative surfaces.
var namespacedResourcePrefixes = []string{
	"/api/v1/workloads",
	"/api/v1/pods",
	"/api/v1/services",
	"/api/v1/secrets",
	"/api/v1/configmaps",
	"/api/v1/resourcequotas",
	"/api/v1/limitranges",
	"/api/v1/ingresses",
	"/api/v1/jobs",
	"/api/v1/cronjobs",
	"/api/v1/events",
	"/api/v1/network",
	"/api/v1/volumes",
	"/api/v1/backups",
	"/api/v1/external-secrets",
	"/api/v1/vm-snapshots",
	"/api/v1/vm-clones",
	"/api/v1/vm-exports",
	"/api/v1/vm-restores",
	"/api/v1/vms",
	"/api/v1/alerts",
	"/api/v1/agent-runs",
	"/api/v1/platform-intents",
	"/api/v1/resources",
	"/api/v1/helm/releases",
	"/api/v1/velero/backups",
	"/api/v1/velero/schedules",
	"/api/v1/haven/identity",
	"/api/v1/haven/planes",
	"/api/v1/hpas",
	"/api/v1/poddisruptionbudgets",
	"/api/v1/networkpolicies",
	"/api/v1/gateways",
	// Also blocks a restricted viewer from GET /api/v1/crds (the bare list
	// of installed CRD definitions, which is cluster-scoped metadata with
	// no namespace of its own) when no ?namespace= is given — an
	// acceptable over-restriction given this list's existing fail-closed
	// philosophy, versus leaving CRD instances readable across every
	// namespace.
	"/api/v1/crds",
}

func matchNamespacedPrefix(path string) (string, bool) {
	for _, prefix := range namespacedResourcePrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return prefix, true
		}
	}
	return "", false
}

func isMutatingMethod(m string) bool {
	switch m {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isPublicPath(r *http.Request) bool {
	path := r.URL.Path
	if r.Method == http.MethodGet && path == "/api/v1/health" {
		return true
	}
	// Public like /health: a Prometheus scraper shouldn't need Basic Auth
	// creds baked into its scrape config, and the exposed labels (route
	// prefix, method, status code, request counts) carry no cluster data.
	if r.Method == http.MethodGet && path == "/api/v1/metrics" {
		return true
	}
	if r.Method == http.MethodGet && path == "/api/v1/auth/providers" {
		return true
	}
	if r.Method == http.MethodGet && (path == "/api/v1/auth/oidc/login" || path == "/api/v1/auth/oidc/callback" || path == "/api/v1/auth/saml/login") {
		return true
	}
	if (r.Method == http.MethodPost || r.Method == http.MethodGet) && path == "/api/v1/auth/saml/acs" {
		return true
	}
	if r.Method == http.MethodPost && path == "/api/v1/auth/logout" {
		return true
	}
	return false
}

func (g Guard) authorize(r *http.Request) bool {
	_, ok := g.authorizeDetail(r)
	return ok
}

func (g Guard) authorizeDetail(r *http.Request) (Identity, bool) {
	if sess, ok := SessionFromRequest(g.sessionSecret(), r); ok {
		role := sess.Role
		if role == "" {
			role = "admin"
		}
		return Identity{User: sess.User, Role: role}, true
	}
	if g.APIKey != "" {
		if bearer := bearerToken(r.Header.Get("Authorization")); bearer != "" {
			if subtle.ConstantTimeCompare([]byte(bearer), []byte(g.APIKey)) == 1 {
				return Identity{User: "apikey", Role: "admin"}, true
			}
		}
		if key := strings.TrimSpace(r.URL.Query().Get("api_key")); key != "" {
			if subtle.ConstantTimeCompare([]byte(key), []byte(g.APIKey)) == 1 {
				return Identity{User: "apikey", Role: "admin"}, true
			}
		}
	}
	u, p, ok := credentialsFromRequest(r)
	if !ok {
		return Identity{}, false
	}
	if g.APIKey != "" && (u == "apikey" || u == "__apikey__") {
		if subtle.ConstantTimeCompare([]byte(p), []byte(g.APIKey)) == 1 {
			return Identity{User: "apikey", Role: "admin"}, true
		}
		return Identity{}, false
	}
	if userMatches(g.Username, u) && subtle.ConstantTimeCompare([]byte(p), []byte(g.Password)) == 1 {
		return Identity{User: g.Username, Role: "admin"}, true
	}
	if g.LDAP.Enabled || g.LDAP.URL != "" {
		if role, ok, err := LDAPAuthenticate(g.LDAP, u, p); ok && err == nil {
			return Identity{User: u, Role: role}, true
		}
	}
	if g.Users != nil {
		if role, namespaces, ok := g.Users.Authenticate(u, p); ok {
			return Identity{User: u, Role: role, Namespaces: namespaces}, true
		}
	}
	return Identity{}, false
}

// isAdminOnlyPath lists mutating API prefixes reserved for RoleAdmin.
func isAdminOnlyPath(path string) bool {
	if strings.HasPrefix(path, "/api/v1/users") {
		return true
	}
	if path == "/api/v1/integrations" {
		return true
	}
	if path == "/api/v1/clusters" {
		// POST register — GET list is fine for operators; Guard only runs this
		// on mutating methods, so GET /clusters stays allowed.
		return true
	}
	if strings.HasPrefix(path, "/api/v1/clusters/") {
		// activate is OK for operators; register/unregister need admin.
		if strings.HasSuffix(path, "/activate") {
			return false
		}
		return true
	}
	return false
}

// userMatches accepts the configured username, and when that is "demo"
// also accepts demo@zyvor.dev (preferred login identity).
func userMatches(configured, provided string) bool {
	configured = strings.TrimSpace(configured)
	provided = strings.TrimSpace(provided)
	if subtle.ConstantTimeCompare([]byte(provided), []byte(configured)) == 1 {
		return true
	}
	if strings.EqualFold(configured, "demo") && strings.EqualFold(provided, "demo@zyvor.dev") {
		return true
	}
	if strings.EqualFold(configured, "demo@zyvor.dev") && strings.EqualFold(provided, "demo") {
		return true
	}
	return false
}

func bearerToken(v string) string {
	if !strings.HasPrefix(v, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(v, "Bearer "))
}

func parseBasic(v string) (string, string, bool) {
	if !strings.HasPrefix(v, "Basic ") {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(v, "Basic ")))
	if err != nil {
		return "", "", false
	}
	user, pass, ok := strings.Cut(string(raw), ":")
	return user, pass, ok
}

// attemptedUsername extracts just the username from a request's credentials,
// for audit logging — never the password, and never validated.
func attemptedUsername(r *http.Request) string {
	u, _, ok := credentialsFromRequest(r)
	if !ok {
		return ""
	}
	return u
}

// credentialsFromRequest reads Basic auth from the header or access_token query
// (base64 user:pass) so browser EventSource/WebSocket clients can authenticate.
func credentialsFromRequest(r *http.Request) (string, string, bool) {
	if u, p, ok := parseBasic(r.Header.Get("Authorization")); ok {
		return u, p, true
	}
	token := strings.TrimSpace(r.URL.Query().Get("access_token"))
	if token == "" {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", "", false
	}
	return strings.Cut(string(raw), ":")
}

// Basic keeps the legacy constructor used by older call sites / tests.
type Basic struct{ Username, Password string }

func (b Basic) Middleware(next http.Handler) http.Handler {
	return Guard{Username: b.Username, Password: b.Password}.Middleware(next)
}
