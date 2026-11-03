package config

import (
	"fmt"
	"os"
	"strings"
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

	// SAMLAllowIdPInitiated accepts unsolicited (IdP-initiated) SAML
	// responses, i.e. ones with no InResponseTo. Off by default.
	SAMLAllowIdPInitiated bool
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

		SAMLAllowIdPInitiated: truthy(os.Getenv("AXIOM_SAML_ALLOW_IDP_INITIATED")),
	}
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
