package auth

import (
	"net/http"
	"strings"
)

// MFAPolicy says which accounts must have a second factor before they may use
// the console (AXIOM_REQUIRE_MFA). It applies to accounts whose second factor
// Axiom manages: local user-store accounts, the built-in env admin, and
// directory (LDAP) accounts, by the role mapped from the directory. SSO
// accounts (their second factor belongs to the identity provider) and API keys
// are never affected.
type MFAPolicy int

const (
	// MFAPolicyOff requires nothing (the default).
	MFAPolicyOff MFAPolicy = iota
	// MFAPolicyAdmin requires a second factor for admins.
	MFAPolicyAdmin
	// MFAPolicyOperator requires it for operators and admins.
	MFAPolicyOperator
	// MFAPolicyAll requires it for every local account, viewers included.
	MFAPolicyAll
)

// ParseMFAPolicy reads an AXIOM_REQUIRE_MFA value. Empty means off; ok is
// false for an unrecognised value.
func ParseMFAPolicy(s string) (MFAPolicy, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "off":
		return MFAPolicyOff, true
	case "admin":
		return MFAPolicyAdmin, true
	case "operator":
		return MFAPolicyOperator, true
	case "all":
		return MFAPolicyAll, true
	}
	return MFAPolicyOff, false
}

func (p MFAPolicy) String() string {
	switch p {
	case MFAPolicyAdmin:
		return "admin"
	case MFAPolicyOperator:
		return "operator"
	case MFAPolicyAll:
		return "all"
	}
	return "off"
}

// Covers reports whether an account with role must have a second factor.
func (p MFAPolicy) Covers(role string) bool {
	switch p {
	case MFAPolicyAdmin:
		return role == "admin"
	case MFAPolicyOperator:
		return role == "admin" || role == "operator"
	case MFAPolicyAll:
		return true
	}
	return false
}

// identitySource records how the caller's credentials are managed, which is
// what decides whether the require-MFA policy applies to them.
type identitySource int

const (
	sourceExternal identitySource = iota // SSO, LDAP, API key: managed elsewhere
	sourceLocal                          // a local account in the user store
	sourceEnv                            // the built-in env admin (AXIOM_USERNAME)
	sourceLDAP                           // a directory account (LDAP bind, or the session it led to)
)

// ViaLDAP reports whether this identity was established by an LDAP directory
// bind (with this request, or through the session the login endpoint issued
// after one). It is false for SSO sessions, local and env accounts and API keys.
func (id Identity) ViaLDAP() bool { return id.source == sourceLDAP }

// ldapMFAProvider is where the second factor of the directory account named
// user lives: a local account of the same name takes precedence (local first,
// as for passwords), otherwise the external (directory) MFA store.
func (g Guard) ldapMFAProvider(user string) MFAProvider {
	if lk, ok := g.Users.(UserLookup); ok && g.MFA != nil {
		if _, _, exists := lk.Lookup(user); exists {
			return g.MFA
		}
	}
	return g.ExternalMFA
}

// MFAState reports, for an authenticated identity, whether the require-MFA
// policy applies to it and whether it has a second factor. The built-in env
// admin is satisfied when AXIOM_TOTP_SECRET is set and otherwise exempt (break
// glass), so it is never "required but not enrolled".
func (g Guard) MFAState(id Identity) (required, enrolled bool) {
	switch id.source {
	case sourceEnv:
		enrolled = g.EnvTOTP != nil
		return enrolled && g.RequireMFA.Covers(id.Role), enrolled
	case sourceLocal:
		enrolled = g.MFA != nil && g.MFA.MFAEnrolled(id.User)
		return g.RequireMFA.Covers(id.Role), enrolled
	case sourceLDAP:
		// Only when this deployment can hold a directory user's second factor;
		// requiring one it cannot store would lock directory users out.
		mfa := g.ldapMFAProvider(id.User)
		if mfa == nil {
			return false, false
		}
		return g.RequireMFA.Covers(id.Role), mfa.MFAEnrolled(id.User)
	}
	return false, false
}

// EnvAdminExemptFromMFA reports whether the policy would cover the built-in
// admin but it has no AXIOM_TOTP_SECRET, so it is exempt to keep a way in.
func (g Guard) EnvAdminExemptFromMFA() bool {
	return g.RequireMFA.Covers("admin") && g.EnvTOTP == nil && strings.TrimSpace(g.Username) != ""
}

// mfaEnrollmentPending reports whether the policy blocks id until it enrols.
func (g Guard) mfaEnrollmentPending(id Identity) bool {
	required, enrolled := g.MFAState(id)
	return required && !enrolled
}

// isMFAEnrollmentPath lists the only requests an account that must enrol (but
// has not) may make: read its session, enrol, and sign out. /auth/providers
// and /auth/logout are public and never reach this check.
func isMFAEnrollmentPath(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet:
		switch r.URL.Path {
		case "/api/v1/session", "/api/v1/auth/providers", "/api/v1/auth/mfa":
			return true
		}
	case http.MethodPost:
		switch r.URL.Path {
		case "/api/v1/auth/logout", "/api/v1/auth/logout-all",
			"/api/v1/auth/mfa/setup", "/api/v1/auth/mfa/enable", "/api/v1/auth/mfa/disable":
			return true
		}
	}
	return false
}

const mfaEnrollmentRequiredBody = `{"error":"two-factor sign-in is required for your role: set it up under Account security","mfaEnrollmentRequired":true}`
