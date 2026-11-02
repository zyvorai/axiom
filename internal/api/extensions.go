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

	operatorLogin := !oidcEnabled && !samlEnabled && !ldapEnabled

	// This endpoint is unauthenticated. Only the demo build advertises the
	// demo credentials and the default username; a real deployment must not
	// tell the internet which account to attack.
	labHint, defaultUser := "", ""
	if s.mode == "demo" {
		labHint, defaultUser = "demo / demo", s.username
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"lab": map[string]any{
			"operator_login": operatorLogin,
			"hint":           labHint,
		},
		// Public kubectl / cluster identity for the login "This machine"
		// chapter — safe to expose before auth so operators can confirm
		// which context they are about to sign into during lab testing.
		"cluster": s.publicClusterContext(r),
		"local": map[string]any{
			"enabled":          true,
			"default_username": defaultUser,
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

// publicClusterContext returns the active kubectl-style context for the
// unauthenticated login page. Names and API server URLs match what
// `kubectl config get-contexts` would show for the wired kubeconfig.
func (s *Server) publicClusterContext(r *http.Request) map[string]any {
	out := map[string]any{
		"mode": s.mode,
	}
	sp := s.switchable()
	if sp == nil {
		return out
	}
	clusters := sp.Clusters(r.Context())
	out["contexts"] = len(clusters)
	names := make([]string, 0, len(clusters))
	for _, c := range clusters {
		names = append(names, c.Name)
		if !c.Current {
			continue
		}
		out["context"] = c.Name
		out["label"] = c.Label
		out["server"] = c.Server
		out["healthy"] = c.Healthy
	}
	if len(names) > 0 {
		out["context_names"] = names
	}
	if _, ok := out["context"]; !ok && len(clusters) > 0 {
		c := clusters[0]
		out["context"] = c.Name
		out["label"] = c.Label
		out["server"] = c.Server
		out["healthy"] = c.Healthy
	}
	if s.mode != "demo" {
		// Pre-auth: don't reveal the API-server address or every registered
		// cluster's name outside the demo.
		delete(out, "server")
		delete(out, "context_names")
	}
	return out
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
	ns, kind, name, action := r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name"), r.PathValue("action")
	err := s.provider.RolloutAction(r.Context(), ns, kind, name, action)
	if err == nil {
		platform.RecordAxiomAudit("workload.rollout."+action, ns+"/"+kind+"/"+name, "", s.auditUser(r))
		EmitStateChanged("workload", "rollout")
	}
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
		return "SAML ACS at /api/v1/auth/saml/acs — set AXIOM_SAML_IDP_CERT (or AXIOM_SAML_ALLOW_UNSIGNED=true for lab)."
	}
	return "Set AXIOM_SAML_LOGIN_URL (and AXIOM_SAML_IDP_CERT) to enable Sign in with SAML."
}

func ldapNote(enabled bool) string {
	if enabled {
		return "Directory sign-in: use the local username/password form — Axiom binds to AXIOM_LDAP_URL with those credentials."
	}
	return "Set AXIOM_LDAP_ENABLED=true and AXIOM_LDAP_URL to enable Active Directory / LDAP bind sign-in."
}

func apiKeyNote(enabled bool) string {
	if enabled {
		return "Send Authorization: Bearer <key> or sign in with username apikey."
	}
	return "Set AXIOM_API_KEY to enable bearer API key sign-in."
}
