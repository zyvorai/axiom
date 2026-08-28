package config

import (
	"os"
	"strings"
)

type Config struct {
	Addr        string
	Mode        string
	Kubeconfig  string
	Username    string
	Password    string
	AllowOrigin string
	TLSCert     string
	TLSKey      string
	APIKey      string
	OIDCLogin   string
	OIDCIssuer  string
	OIDCClient  string
	SAMLLogin   string
	LDAPEnabled bool
	LDAPDomain  string
	LDAPURL     string
	AtlasURL    string
	AtlasToken  string
	ClusterName string
}

func FromEnv() Config {
	return Config{
		Addr:        env("AXIOM_ADDR", ":5090"),
		Mode:        strings.ToLower(env("AXIOM_MODE", "demo")),
		Kubeconfig:  os.Getenv("KUBECONFIG"),
		Username:    env("AXIOM_USERNAME", "demo"),
		Password:    env("AXIOM_PASSWORD", "demo"),
		AllowOrigin: env("AXIOM_ALLOW_ORIGIN", "http://localhost:5173"),
		TLSCert:     os.Getenv("AXIOM_TLS_CERT"),
		TLSKey:      os.Getenv("AXIOM_TLS_KEY"),
		APIKey:      strings.TrimSpace(os.Getenv("AXIOM_API_KEY")),
		OIDCLogin:   strings.TrimSpace(os.Getenv("AXIOM_OIDC_LOGIN_URL")),
		OIDCIssuer:  strings.TrimSpace(os.Getenv("AXIOM_OIDC_ISSUER")),
		OIDCClient:  strings.TrimSpace(os.Getenv("AXIOM_OIDC_CLIENT_ID")),
		SAMLLogin:   strings.TrimSpace(os.Getenv("AXIOM_SAML_LOGIN_URL")),
		LDAPEnabled: truthy(os.Getenv("AXIOM_LDAP_ENABLED")),
		LDAPDomain:  strings.TrimSpace(os.Getenv("AXIOM_LDAP_DOMAIN")),
		LDAPURL:     strings.TrimSpace(os.Getenv("AXIOM_LDAP_URL")),
		AtlasURL:    strings.TrimSpace(os.Getenv("AXIOM_ATLAS_URL")),
		AtlasToken:  strings.TrimSpace(os.Getenv("AXIOM_ATLAS_TOKEN")),
		ClusterName: strings.TrimSpace(os.Getenv("AXIOM_CLUSTER_NAME")),
	}
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
