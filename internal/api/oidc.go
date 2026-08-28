package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
)

type oidcDiscovery struct {
	Issuer        string `json:"issuer"`
	AuthEndpoint  string `json:"authorization_endpoint"`
	TokenEndpoint string `json:"token_endpoint"`
	JWKSURI       string `json:"jwks_uri"`
	Userinfo      string `json:"userinfo_endpoint"`
}

func (s *Server) oidcConfig() (issuer, loginURL, clientID string) {
	issuer = s.auth.OIDCIssuer
	loginURL = s.auth.OIDCLogin
	clientID = s.auth.OIDCClient
	if s.hub != nil {
		cur := s.hub.Get()
		if cur.OIDCIssuer != "" {
			issuer = cur.OIDCIssuer
		}
		if cur.OIDCLoginURL != "" {
			loginURL = cur.OIDCLoginURL
		}
		if cur.OIDCClientID != "" {
			clientID = cur.OIDCClientID
		}
	}
	if clientID == "" {
		clientID = "axiom"
	}
	if loginURL == "" && issuer != "" {
		loginURL = strings.TrimRight(issuer, "/") + "/protocol/openid-connect/auth"
	}
	return issuer, loginURL, clientID
}

func (s *Server) sessionSecret() string {
	if s.password != "" {
		return s.password
	}
	return "axiom-dev-session"
}

func (s *Server) requestSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) publicBase(r *http.Request) string {
	scheme := "https"
	if !s.requestSecure(r) {
		scheme = "http"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	host := r.Host
	if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
		host = fh
	}
	return scheme + "://" + host
}

func (s *Server) oidcLogin(w http.ResponseWriter, r *http.Request) {
	issuer, login, clientID := s.oidcConfig()
	if login == "" && issuer == "" {
		fail(w, http.StatusNotImplemented, errSSO("OIDC is not configured. Set it in Settings → Integrations."))
		return
	}
	authURL := login
	if issuer != "" {
		if disc, err := s.fetchOIDCDiscovery(r, issuer); err == nil && disc.AuthEndpoint != "" {
			authURL = disc.AuthEndpoint
		}
	}
	target, err := url.Parse(authURL)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}

	verifier := auth.RandomURLToken(32)
	state := auth.RandomURLToken(24)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	secure := s.requestSecure(r)
	http.SetCookie(w, &http.Cookie{Name: auth.OIDCVerifier, Value: verifier, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure, MaxAge: 600})
	http.SetCookie(w, &http.Cookie{Name: auth.OIDCState, Value: state, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure, MaxAge: 600})

	q := target.Query()
	q.Set("client_id", clientID)
	q.Set("response_type", "code")
	q.Set("scope", "openid profile email")
	q.Set("redirect_uri", s.publicBase(r)+"/api/v1/auth/oidc/callback")
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	target.RawQuery = q.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		http.Error(w, "OIDC error: "+errMsg+" — "+r.URL.Query().Get("error_description"), http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}
	wantState, _ := r.Cookie(auth.OIDCState)
	if wantState == nil || wantState.Value == "" || state == "" || wantState.Value != state {
		http.Error(w, "invalid OIDC state", http.StatusBadRequest)
		return
	}
	verifierCookie, _ := r.Cookie(auth.OIDCVerifier)
	if verifierCookie == nil || verifierCookie.Value == "" {
		http.Error(w, "missing PKCE verifier", http.StatusBadRequest)
		return
	}

	issuer, _, clientID := s.oidcConfig()
	disc, err := s.fetchOIDCDiscovery(r, issuer)
	if err != nil {
		http.Error(w, "OIDC discovery failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", s.publicBase(r)+"/api/v1/auth/oidc/callback")
	form.Set("client_id", clientID)
	form.Set("code_verifier", verifierCookie.Value)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, disc.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		http.Error(w, fmt.Sprintf("token exchange HTTP %d: %s", res.StatusCode, string(body)), http.StatusBadGateway)
		return
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		http.Error(w, "invalid token response", http.StatusBadGateway)
		return
	}

	user, email := oidcIdentity(tok.IDToken, tok.AccessToken, disc.Userinfo, r)
	if user == "" {
		user = "oidc-user"
	}
	role := resolveOIDCRole(tok.IDToken, s.auth.OIDCGroupsClaim, s.auth.OIDCAdminGroup, s.auth.OIDCViewerGroup)
	token, err := auth.SignSession(s.sessionSecret(), auth.Session{
		User:   user,
		Email:  email,
		Issuer: issuer,
		Role:   role,
		Exp:    time.Now().Add(12 * time.Hour).Unix(),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	secure := s.requestSecure(r)
	auth.SetSessionCookie(w, token, secure)
	// Clear PKCE cookies
	http.SetCookie(w, &http.Cookie{Name: auth.OIDCVerifier, Value: "", Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: auth.OIDCState, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/?sso=1", http.StatusFound)
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	auth.ClearSessionCookie(w, s.requestSecure(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) fetchOIDCDiscovery(r *http.Request, issuer string) (oidcDiscovery, error) {
	var out oidcDiscovery
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	if issuer == "" {
		return out, fmt.Errorf("issuer not configured")
	}
	u := issuer + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u, nil)
	if err != nil {
		return out, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return out, fmt.Errorf("discovery HTTP %d: %s", res.StatusCode, string(b))
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return out, err
	}
	// Lab Keycloak sometimes advertises :8080 while NodePort is elsewhere — rewrite to issuer origin.
	if issuerURL, err := url.Parse(issuer); err == nil && issuerURL.Host != "" {
		rewrite := func(ep string) string {
			u, err := url.Parse(ep)
			if err != nil || u.Host == "" {
				return ep
			}
			if u.Hostname() == issuerURL.Hostname() && u.Host != issuerURL.Host {
				u.Host = issuerURL.Host
				u.Scheme = issuerURL.Scheme
				return u.String()
			}
			return ep
		}
		out.AuthEndpoint = rewrite(out.AuthEndpoint)
		out.TokenEndpoint = rewrite(out.TokenEndpoint)
		out.Userinfo = rewrite(out.Userinfo)
		out.JWKSURI = rewrite(out.JWKSURI)
	}
	return out, nil
}

func oidcIdentity(idToken, accessToken, userinfoURL string, r *http.Request) (user, email string) {
	if claims := decodeJWTClaims(idToken); claims != nil {
		user = firstNonEmpty(claims["preferred_username"], claims["email"], claims["sub"])
		email = claims["email"]
		if user != "" {
			return user, email
		}
	}
	if accessToken != "" && userinfoURL != "" {
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, userinfoURL, nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+accessToken)
			if res, err := http.DefaultClient.Do(req); err == nil {
				defer res.Body.Close()
				var info map[string]any
				if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&info) == nil {
					user = firstNonEmpty(strAny(info["preferred_username"]), strAny(info["email"]), strAny(info["sub"]))
					email = strAny(info["email"])
				}
			}
		}
	}
	return user, email
}

// resolveOIDCRole maps an ID token's groups claim to viewer/admin. When
// adminGroup and viewerGroup are both unset, every OIDC session stays
// admin, matching Axiom's behavior before this mapping existed.
func resolveOIDCRole(idToken, groupsClaim, adminGroup, viewerGroup string) string {
	adminGroup = strings.TrimSpace(adminGroup)
	viewerGroup = strings.TrimSpace(viewerGroup)
	if adminGroup == "" && viewerGroup == "" {
		return "admin"
	}
	groups := decodeJWTClaimGroups(idToken, groupsClaim)
	if adminGroup != "" && containsFold(groups, adminGroup) {
		return "admin"
	}
	if viewerGroup != "" && containsFold(groups, viewerGroup) {
		return "viewer"
	}
	// Authenticated via OIDC but in neither configured group: default to
	// the more restrictive role rather than silently granting admin.
	return "viewer"
}

func containsFold(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

// decodeJWTClaimGroups reads a single claim from a JWT's payload and
// coerces it to a string slice, handling both a JSON array of strings and
// a single string value (some IdPs emit a bare string for a one-group user).
func decodeJWTClaimGroups(jwtToken, claim string) []string {
	parts := strings.Split(jwtToken, ".")
	if len(parts) < 2 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	switch v := m[claim].(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return []string{v}
	default:
		return nil
	}
}

func decodeJWTClaims(jwt string) map[string]string {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	out := map[string]string{}
	for k, v := range m {
		out[k] = strAny(v)
	}
	return out
}

func strAny(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return ""
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
