package api

import (
	"net/http"
	"net/url"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) authProviders(w http.ResponseWriter, r *http.Request) {
	oidcLogin := s.auth.OIDCLogin
	oidcIssuer := s.auth.OIDCIssuer
	oidcClient := s.auth.OIDCClient
	if s.hub != nil {
		cur := s.hub.Get()
		if cur.OIDCLoginURL != "" {
			oidcLogin = cur.OIDCLoginURL
		}
		if cur.OIDCIssuer != "" {
			oidcIssuer = cur.OIDCIssuer
		}
		if cur.OIDCClientID != "" {
			oidcClient = cur.OIDCClientID
		}
	}
	oidcEnabled := oidcLogin != "" || oidcIssuer != ""
	samlEnabled := s.auth.SAMLLogin != ""
	ldapEnabled := s.auth.LDAPEnabled || s.auth.LDAPURL != ""
	apiKeyEnabled := s.auth.APIKey != ""

	writeJSON(w, http.StatusOK, map[string]any{
		"local": map[string]any{
			"enabled":          true,
			"default_username": s.username,
			"note":             "Local Basic auth via AXIOM_USERNAME / AXIOM_PASSWORD",
		},
		"oidc": map[string]any{
			"enabled":   oidcEnabled,
			"login_url": "/api/v1/auth/oidc/login",
			"issuer":    nullIfEmpty(oidcIssuer),
			"client_id": nullIfEmpty(oidcClient),
			"note":      oidcNote(oidcEnabled),
		},
		"saml": map[string]any{
			"enabled":   samlEnabled,
			"login_url": "/api/v1/auth/saml/login",
			"note":      samlNote(samlEnabled),
		},
		"ldap": map[string]any{
			"enabled": ldapEnabled,
			"domain":  nullIfEmpty(s.auth.LDAPDomain),
			"url":     nullIfEmpty(s.auth.LDAPURL),
			"note":    ldapNote(ldapEnabled),
		},
		"api_key": map[string]any{
			"enabled": apiKeyEnabled,
			"note":    apiKeyNote(apiKeyEnabled),
		},
	})
}

func (s *Server) samlLogin(w http.ResponseWriter, r *http.Request) {
	if s.auth.SAMLLogin == "" {
		fail(w, http.StatusNotImplemented, errSSO("SAML is not configured. Set AXIOM_SAML_LOGIN_URL to your IdP SSO URL."))
		return
	}
	target, err := url.Parse(s.auth.SAMLLogin)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	next := r.URL.Query().Get("next")
	q := target.Query()
	if next != "" && q.Get("RelayState") == "" {
		q.Set("RelayState", next)
		target.RawQuery = q.Encode()
	}
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func (s *Server) nodeDetail(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.GetNode(r.Context(), r.PathValue("name"))
	respond(w, v, err)
}

func (s *Server) rolloutStatus(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.GetRolloutStatus(r.Context(), r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name"))
	respond(w, v, err)
}

func (s *Server) rolloutAction(w http.ResponseWriter, r *http.Request) {
	err := s.provider.RolloutAction(r.Context(), r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name"), r.PathValue("action"))
	respondAction(w, err)
}

func (s *Server) resourceDetail(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.GetResource(r.Context(), r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name"))
	respond(w, v, err)
}

func (s *Server) packetwolfStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, platform.FetchPacketWolfStatus(r.Context()))
}

func (s *Server) packetwolfProxy(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	body, code, err := platform.PacketWolfProxy(r.Context(), path, r.URL.RawQuery)
	if err != nil {
		fail(w, code, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(body)
}

func (s *Server) workloadNetworkGuard(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.WorkloadNetworkGuard(r.Context(), r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name"))
	respond(w, v, err)
}

type ssoError string

func (e ssoError) Error() string { return string(e) }

func errSSO(msg string) error { return ssoError(msg) }

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func oidcNote(enabled bool) string {
	if enabled {
		return "Redirects to the OIDC authorize URL from Settings → Integrations."
	}
	return "Configure OIDC issuer / login URL in Settings → Integrations to enable Sign in with OIDC."
}

func samlNote(enabled bool) string {
	if enabled {
		return "Redirects to AXIOM_SAML_LOGIN_URL."
	}
	return "Set AXIOM_SAML_LOGIN_URL to enable Sign in with SAML."
}

func ldapNote(enabled bool) string {
	if enabled {
		return "Directory sign-in uses the local operator credentials bridge, or place Axiom behind an LDAP-aware proxy."
	}
	return "Set AXIOM_LDAP_ENABLED=true (optional AXIOM_LDAP_DOMAIN / AXIOM_LDAP_URL) to surface Active Directory sign-in."
}

func apiKeyNote(enabled bool) string {
	if enabled {
		return "Send Authorization: Bearer <key> or sign in with username apikey."
	}
	return "Set AXIOM_API_KEY to enable bearer API key sign-in."
}
