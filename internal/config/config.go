package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
)

type Config struct {
	Addr       string
	Mode       string
	Kubeconfig string
	Username   string
	Password   string
	// SessionSecret signs OIDC session cookies/tokens. Optional — when unset,
	// the server generates a random per-process secret instead of falling
	// back to Password (see api.AuthOptions.SessionSecret).
	SessionSecret string
	// TrustedProxyHeader, when set (e.g. "X-Forwarded-For"), tells the login
	// rate limiter to key on that header instead of the raw connection
	// address. Only set this when a reverse proxy in front of Axiom itself
	// sets/overwrites the header — see api.AuthOptions.TrustedProxyHeader.
	TrustedProxyHeader string
	AllowOrigin        string
	TLSCert            string
	TLSKey             string
	APIKey             string
	OIDCLogin          string
	OIDCIssuer         string
	OIDCClient         string
	SAMLLogin         string
	SAMLEntityID      string
	SAMLIDPCert       string
	SAMLAllowUnsigned bool
	LDAPEnabled       bool
	LDAPDomain        string
	LDAPURL           string
	LDAPBaseDN        string
	LDAPBindDN        string
	LDAPBindPW        string
	LDAPUserAttr      string
	LDAPGroupAttr     string
	LDAPAdminGroup    string
	LDAPViewerGroup   string
	AtlasURL          string
	AtlasToken        string
	ClusterName       string
	AuditRingSize     int

	OIDCGroupsClaim string
	OIDCAdminGroup  string
	OIDCViewerGroup string
	OIDCDefaultRole string

	// TOTPSecret adds a TOTP second factor to the built-in admin account
	// (AXIOM_TOTP_SECRET). SessionTTL is how long a login session lasts
	// (AXIOM_SESSION_TTL, a Go duration such as 8h; default 12h).
	TOTPSecret string
	SessionTTL time.Duration
	// FleetProbeInterval is how often registered clusters are health-probed
	// (AXIOM_FLEET_PROBE_INTERVAL, default 30s; "off" or 0 disables the
	// background probing so clusters are only probed when /fleet is read).
	FleetProbeInterval time.Duration
	// FleetHistoryPersistInterval is how often the fleet health history is
	// saved for restarts (AXIOM_FLEET_HISTORY_PERSIST_INTERVAL, default 5m,
	// minimum 1m; "off" or 0 disables persistence and restore).
	FleetHistoryPersistInterval time.Duration
	// BudgetCheckInterval is how often cost budgets are evaluated for
	// warning/over notifications (AXIOM_BUDGET_CHECK_INTERVAL, default 5m,
	// minimum 1m; "off" or 0 disables the watcher).
	BudgetCheckInterval time.Duration
	// RequireMFA (AXIOM_REQUIRE_MFA: off|admin|operator|all, default off) is
	// which local accounts must enrol a second factor before using the console.
	RequireMFA auth.MFAPolicy
	// RevocationRefresh is how often session revocations are re-read from the
	// shared store so other replicas' logouts apply here
	// (AXIOM_REVOCATION_REFRESH, default 15s, minimum 5s; "off" disables).
	RevocationRefresh time.Duration
	// SAMLAllowIdPInitiated accepts unsolicited (IdP-initiated) SAML
	// responses, i.e. ones with no InResponseTo. Off by default.
	SAMLAllowIdPInitiated bool
	// PrometheusURL (AXIOM_PROMETHEUS_URL) backs cost rightsizing with p95/p99
	// usage history; PrometheusToken (AXIOM_PROMETHEUS_TOKEN) is an optional
	// bearer token for it. Never logged.
	PrometheusURL   string
	PrometheusToken string
	// PodProxyAddr (AXIOM_POD_PROXY_ADDR, e.g. ":5091") and PodProxyOrigin
	// (AXIOM_POD_PROXY_ORIGIN, e.g. "https://pods.example.com") serve pod
	// windows from a separate origin. Both must be set; off by default.
	PodProxyAddr   string
	PodProxyOrigin string
	// OpenCostURL (AXIOM_OPENCOST_URL) switches GET /cost from request-based
	// estimates to actual allocation cost; OpenCostToken
	// (AXIOM_OPENCOST_TOKEN) is an optional bearer token for it. Never logged.
	OpenCostURL   string
	OpenCostToken string
}

func FromEnv() Config {
	return Config{
		Addr:               env("AXIOM_ADDR", ":5090"),
		Mode:               strings.ToLower(env("AXIOM_MODE", "demo")),
		Kubeconfig:         os.Getenv("KUBECONFIG"),
		Username:           env("AXIOM_USERNAME", "demo"),
		Password:           env("AXIOM_PASSWORD", "demo"),
		SessionSecret:      strings.TrimSpace(os.Getenv("AXIOM_SESSION_SECRET")),
		TrustedProxyHeader: strings.TrimSpace(os.Getenv("AXIOM_TRUSTED_PROXY_HEADER")),
		AllowOrigin:        env("AXIOM_ALLOW_ORIGIN", "http://localhost:5173"),
		TLSCert:            os.Getenv("AXIOM_TLS_CERT"),
		TLSKey:             os.Getenv("AXIOM_TLS_KEY"),
		APIKey:             strings.TrimSpace(os.Getenv("AXIOM_API_KEY")),
		OIDCLogin:          strings.TrimSpace(os.Getenv("AXIOM_OIDC_LOGIN_URL")),
		OIDCIssuer:         strings.TrimSpace(os.Getenv("AXIOM_OIDC_ISSUER")),
		OIDCClient:         strings.TrimSpace(os.Getenv("AXIOM_OIDC_CLIENT_ID")),
		SAMLLogin:          strings.TrimSpace(os.Getenv("AXIOM_SAML_LOGIN_URL")),
		SAMLEntityID:       strings.TrimSpace(os.Getenv("AXIOM_SAML_ENTITY_ID")),
		SAMLIDPCert:        strings.TrimSpace(os.Getenv("AXIOM_SAML_IDP_CERT")),
		SAMLAllowUnsigned:  truthy(os.Getenv("AXIOM_SAML_ALLOW_UNSIGNED")),
		LDAPEnabled:        truthy(os.Getenv("AXIOM_LDAP_ENABLED")),
		LDAPDomain:         strings.TrimSpace(os.Getenv("AXIOM_LDAP_DOMAIN")),
		LDAPURL:            strings.TrimSpace(os.Getenv("AXIOM_LDAP_URL")),
		LDAPBaseDN:         strings.TrimSpace(os.Getenv("AXIOM_LDAP_BASE_DN")),
		LDAPBindDN:         strings.TrimSpace(os.Getenv("AXIOM_LDAP_BIND_DN")),
		LDAPBindPW:         strings.TrimSpace(os.Getenv("AXIOM_LDAP_BIND_PASSWORD")),
		LDAPUserAttr:       strings.TrimSpace(os.Getenv("AXIOM_LDAP_USER_ATTR")),
		LDAPGroupAttr:      strings.TrimSpace(os.Getenv("AXIOM_LDAP_GROUP_ATTR")),
		LDAPAdminGroup:     strings.TrimSpace(os.Getenv("AXIOM_LDAP_ADMIN_GROUP")),
		LDAPViewerGroup:    strings.TrimSpace(os.Getenv("AXIOM_LDAP_VIEWER_GROUP")),
		AtlasURL:           strings.TrimSpace(os.Getenv("AXIOM_ATLAS_URL")),
		AtlasToken:         strings.TrimSpace(os.Getenv("AXIOM_ATLAS_TOKEN")),
		ClusterName:        strings.TrimSpace(os.Getenv("AXIOM_CLUSTER_NAME")),
		AuditRingSize:      envInt("AXIOM_AUDIT_RING_SIZE", 5000),

		OIDCGroupsClaim: env("AXIOM_OIDC_GROUPS_CLAIM", "groups"),
		OIDCAdminGroup:  strings.TrimSpace(os.Getenv("AXIOM_OIDC_ADMIN_GROUP")),
		OIDCViewerGroup: strings.TrimSpace(os.Getenv("AXIOM_OIDC_VIEWER_GROUP")),
		OIDCDefaultRole: env("AXIOM_OIDC_DEFAULT_ROLE", "admin"),

		TOTPSecret: strings.TrimSpace(os.Getenv("AXIOM_TOTP_SECRET")),
		SessionTTL: envDuration("AXIOM_SESSION_TTL", 12*time.Hour),

		FleetProbeInterval: fleetProbeInterval(),
		FleetHistoryPersistInterval: fleetHistoryPersistInterval(),
		BudgetCheckInterval: budgetCheckInterval(),
		RequireMFA:         requireMFA(),
		RevocationRefresh:  revocationRefresh(),
		SAMLAllowIdPInitiated: truthy(os.Getenv("AXIOM_SAML_ALLOW_IDP_INITIATED")),

		PrometheusURL:   strings.TrimSpace(os.Getenv("AXIOM_PROMETHEUS_URL")),
		PrometheusToken: strings.TrimSpace(os.Getenv("AXIOM_PROMETHEUS_TOKEN")),

		PodProxyAddr:   strings.TrimSpace(os.Getenv("AXIOM_POD_PROXY_ADDR")),
		PodProxyOrigin: strings.TrimSpace(os.Getenv("AXIOM_POD_PROXY_ORIGIN")),
		OpenCostURL:   strings.TrimSpace(os.Getenv("AXIOM_OPENCOST_URL")),
		OpenCostToken: strings.TrimSpace(os.Getenv("AXIOM_OPENCOST_TOKEN")),
	}
}

func fleetProbeInterval() time.Duration {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("AXIOM_FLEET_PROBE_INTERVAL")))
	if v == "off" || v == "0" || v == "false" {
		return 0
	}
	d := envDuration("AXIOM_FLEET_PROBE_INTERVAL", 30*time.Second)
	if d < 5*time.Second {
		d = 5 * time.Second // a probe may take several seconds; never run them back to back
	}
	return d
}

func fleetHistoryPersistInterval() time.Duration {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("AXIOM_FLEET_HISTORY_PERSIST_INTERVAL")))
	if v == "off" || v == "0" || v == "false" {
		return 0
	}
	d := envDuration("AXIOM_FLEET_HISTORY_PERSIST_INTERVAL", 5*time.Minute)
	if d < time.Minute {
		d = time.Minute // each save rewrites a ConfigMap of up to 900 KiB
	}
	return d
}

func budgetCheckInterval() time.Duration {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("AXIOM_BUDGET_CHECK_INTERVAL")))
	if v == "off" || v == "0" || v == "false" {
		return 0
	}
	d := envDuration("AXIOM_BUDGET_CHECK_INTERVAL", 5*time.Minute)
	if d < time.Minute {
		d = time.Minute // each check estimates cost for the whole cluster
	}
	return d
}

func requireMFA() auth.MFAPolicy {
	v := os.Getenv("AXIOM_REQUIRE_MFA")
	p, ok := auth.ParseMFAPolicy(v)
	if !ok {
		log.Printf("config: ignoring AXIOM_REQUIRE_MFA=%q (want off, admin, operator or all); two-factor sign-in is NOT being required", v)
	}
	return p
}

func revocationRefresh() time.Duration {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("AXIOM_REVOCATION_REFRESH")))
	if v == "off" || v == "0" || v == "false" {
		return 0
	}
	d := envDuration("AXIOM_REVOCATION_REFRESH", 15*time.Second)
	if d < 5*time.Second {
		d = 5 * time.Second // each refresh is an API read; do not hammer the store
	}
	return d
}

func envDuration(k string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		log.Printf("config: ignoring %s=%q (want a duration such as 8h)", k, v)
		return fallback
	}
	return d
}

func envInt(k string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n <= 0 {
		return fallback
	}
	return n
}

func env(k, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return fallback
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
