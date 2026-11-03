package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

// Account security: session revocation and two-factor (TOTP) sign-in for local
// accounts. The sign-in itself (POST /api/v1/auth/login) lives in auth.Guard so
// that it is throttled like every other credential check; this file holds the
// authenticated self-service and admin endpoints around it.

const mfaIssuer = "Zyvor Axiom"

// registerIdentity wires the account-security routes.
func (s *Server) registerIdentity(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/logout-all", s.logoutAll)
	mux.HandleFunc("GET /api/v1/auth/mfa", s.mfaStatus)
	mux.HandleFunc("POST /api/v1/auth/mfa/setup", s.mfaSetup)
	mux.HandleFunc("POST /api/v1/auth/mfa/enable", s.mfaEnable)
	mux.HandleFunc("POST /api/v1/auth/mfa/disable", s.mfaDisable)
	mux.HandleFunc("DELETE /api/v1/users/{username}/mfa", s.resetUserMFA)
	mux.HandleFunc("POST /api/v1/users/{username}/revoke-sessions", s.revokeUserSessions)
}

// callerKind says how the caller's account is managed.
type callerKind int

const (
	callerLocal    callerKind = iota // a local account in the user store
	callerEnv                        // the single env-configured account
	callerExternal                   // SSO, LDAP, API key: managed elsewhere
)

// caller resolves who is asking and how their credentials are managed. An SSO
// session whose user name happens to match a local account is still external.
func (s *Server) caller(r *http.Request) (string, callerKind) {
	id, ok := auth.IdentityFromContext(r.Context())
	if !ok {
		return "", callerExternal
	}
	if sess, ok := auth.SessionFromRequest(s.sessionSecret(), r); ok && sess.Issuer != auth.LocalSessionIssuer {
		return id.User, callerExternal
	}
	if strings.EqualFold(id.User, s.username) {
		return s.username, callerEnv
	}
	if s.users != nil {
		if _, _, ok := s.users.Lookup(id.User); ok {
			return id.User, callerLocal
		}
	}
	return id.User, callerExternal
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	if sess, ok := auth.SessionFromRequest(s.sessionSecret(), r); ok {
		s.revocations.RevokeSession(sess.ID, sess.Exp)
		platform.RecordAxiomAudit("auth.logout", "session", "", sess.User)
	}
	auth.ClearSessionCookie(w, s.requestSecure(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// logoutAll ends every session the caller has opened, on every device.
func (s *Server) logoutAll(w http.ResponseWriter, r *http.Request) {
	user, _ := s.caller(r)
	if user == "" || user == "apikey" {
		fail(w, http.StatusBadRequest, errSSO("API keys are not sessions; rotate the key instead"))
		return
	}
	s.revocations.RevokeUser(user)
	platform.RecordAxiomAudit("auth.logout_all", user, "", user)
	auth.ClearSessionCookie(w, s.requestSecure(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type mfaStatusView struct {
	// Available is true when this account's second factor is managed here.
	Available bool `json:"available"`
	Enabled   bool `json:"enabled"`
	// RecoveryCodes is how many unused recovery codes remain.
	RecoveryCodes int `json:"recoveryCodes"`
	// Managed explains where the second factor lives when it is not managed
	// here (the identity provider, or an environment variable).
	Managed string `json:"managed,omitempty"`
}

func (s *Server) mfaStatus(w http.ResponseWriter, r *http.Request) {
	user, kind := s.caller(r)
	switch kind {
	case callerLocal:
		enabled, left, _ := s.users.MFAStatus(user)
		writeJSON(w, http.StatusOK, mfaStatusView{Available: true, Enabled: enabled, RecoveryCodes: left})
	case callerEnv:
		writeJSON(w, http.StatusOK, mfaStatusView{Enabled: s.envTOTP != nil, Managed: "AXIOM_TOTP_SECRET"})
	default:
		writeJSON(w, http.StatusOK, mfaStatusView{Managed: "your identity provider"})
	}
}

// localCaller returns the signed-in local account or writes why there is none.
func (s *Server) localCaller(w http.ResponseWriter, r *http.Request) (string, bool) {
	user, kind := s.caller(r)
	switch kind {
	case callerLocal:
		return user, true
	case callerEnv:
		fail(w, http.StatusBadRequest, errSSO("the built-in admin account's second factor is set with the AXIOM_TOTP_SECRET environment variable"))
	default:
		fail(w, http.StatusBadRequest, errSSO("this account is managed by your identity provider; enable two-factor sign-in there"))
	}
	return "", false
}

// groupSecret formats a secret in blocks of four so it is easy to type.
func groupSecret(s string) string {
	var sb strings.Builder
	for i, c := range s {
		if i > 0 && i%4 == 0 {
			sb.WriteByte(' ')
		}
		sb.WriteRune(c)
	}
	return sb.String()
}

// mfaSetup starts enrolment: it issues a secret for the person to add to their
// authenticator app. Re-entering the password stops someone with only a stolen
// session from enrolling their own device.
func (s *Server) mfaSetup(w http.ResponseWriter, r *http.Request) {
	user, ok := s.localCaller(w, r)
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if !s.guard.VerifyPassword(user, req.Password) {
		platform.RecordAxiomAudit("auth.mfa_setup_denied", user, "wrong password", user)
		fail(w, http.StatusForbidden, errSSO("password is incorrect"))
		return
	}
	secret, err := s.users.BeginMFA(r.Context(), user)
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	platform.RecordAxiomAudit("auth.mfa_setup", user, "", user)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"secret": groupSecret(secret),
		"uri":    auth.TOTPProvisioningURI(mfaIssuer, user, secret),
		"issuer": mfaIssuer,
	})
}

// mfaEnable confirms the secret with a code, turns two-factor on and returns
// the recovery codes. This is the only time they are shown.
func (s *Server) mfaEnable(w http.ResponseWriter, r *http.Request) {
	user, ok := s.localCaller(w, r)
	if !ok {
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	codes, err := s.users.EnableMFA(r.Context(), user, req.Code, time.Now())
	if err != nil {
		platform.RecordAxiomAudit("auth.mfa_enable_failed", user, "", user)
		fail(w, http.StatusBadRequest, err)
		return
	}
	platform.RecordAxiomAudit("auth.mfa_enabled", user, "", user)
	// Enabling revoked the account's sessions; make the browser forget its own.
	auth.ClearSessionCookie(w, s.requestSecure(r))
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recoveryCodes": codes})
}

// mfaDisable turns two-factor off for the caller after they prove both the
// password and a current code (or recovery code).
func (s *Server) mfaDisable(w http.ResponseWriter, r *http.Request) {
	user, ok := s.localCaller(w, r)
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if !s.guard.VerifyPassword(user, req.Password) || !s.users.VerifySecondFactor(user, req.Code, time.Now()) {
		platform.RecordAxiomAudit("auth.mfa_disable_denied", user, "", user)
		fail(w, http.StatusForbidden, errSSO("password or code is incorrect"))
		return
	}
	if err := s.users.DisableMFA(r.Context(), user); err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("auth.mfa_disabled", user, "", user)
	auth.ClearSessionCookie(w, s.requestSecure(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// resetUserMFA lets an administrator remove a locked-out person's second
// factor (they can then sign in with their password and re-enrol). Admin only,
// audited.
func (s *Server) resetUserMFA(w http.ResponseWriter, r *http.Request) {
	if s.users == nil {
		fail(w, http.StatusServiceUnavailable, errSSO("user management is not available on this deployment"))
		return
	}
	username := r.PathValue("username")
	if err := s.users.DisableMFA(r.Context(), username); err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("user.mfa_reset", username, "", s.auditUser(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// revokeUserSessions signs a user out everywhere. Admin only, audited.
func (s *Server) revokeUserSessions(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.PathValue("username"))
	if username == "" {
		fail(w, http.StatusBadRequest, errSSO("username is required"))
		return
	}
	s.revocations.RevokeUser(username)
	platform.RecordAxiomAudit("user.sessions_revoked", username, "", s.auditUser(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
