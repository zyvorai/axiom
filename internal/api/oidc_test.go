package api

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func fakeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	body := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + body + ".sig"
}

func TestResolveOIDCRoleNoMappingConfigured(t *testing.T) {
	tok := fakeJWT(t, map[string]any{"groups": []string{"engineers"}})
	role := resolveOIDCRole(tok, "groups", "", "")
	if role != "admin" {
		t.Fatalf("expected admin when no groups are configured, got %q", role)
	}
}

func TestResolveOIDCRoleAdminGroupMatch(t *testing.T) {
	tok := fakeJWT(t, map[string]any{"groups": []string{"platform-admins", "engineers"}})
	role := resolveOIDCRole(tok, "groups", "platform-admins", "platform-viewers")
	if role != "admin" {
		t.Fatalf("expected admin, got %q", role)
	}
}

func TestResolveOIDCRoleViewerGroupMatch(t *testing.T) {
	tok := fakeJWT(t, map[string]any{"groups": []string{"platform-viewers"}})
	role := resolveOIDCRole(tok, "groups", "platform-admins", "platform-viewers")
	if role != "viewer" {
		t.Fatalf("expected viewer, got %q", role)
	}
}

func TestResolveOIDCRoleNoGroupMatchDefaultsToViewer(t *testing.T) {
	tok := fakeJWT(t, map[string]any{"groups": []string{"unrelated-group"}})
	role := resolveOIDCRole(tok, "groups", "platform-admins", "platform-viewers")
	if role != "viewer" {
		t.Fatalf("expected viewer as the safe default for an unmapped group, got %q", role)
	}
}

func TestResolveOIDCRoleSingleStringGroupClaim(t *testing.T) {
	tok := fakeJWT(t, map[string]any{"groups": "platform-admins"})
	role := resolveOIDCRole(tok, "groups", "platform-admins", "platform-viewers")
	if role != "admin" {
		t.Fatalf("expected admin for a bare-string groups claim, got %q", role)
	}
}

func TestResolveOIDCRoleCaseInsensitive(t *testing.T) {
	tok := fakeJWT(t, map[string]any{"groups": []string{"Platform-Admins"}})
	role := resolveOIDCRole(tok, "groups", "platform-admins", "platform-viewers")
	if role != "admin" {
		t.Fatalf("expected case-insensitive group match to produce admin, got %q", role)
	}
}

func TestDecodeJWTClaimGroupsCustomClaimName(t *testing.T) {
	tok := fakeJWT(t, map[string]any{"realm_access": nil, "roles": []string{"admin-role"}})
	groups := decodeJWTClaimGroups(tok, "roles")
	if len(groups) != 1 || groups[0] != "admin-role" {
		t.Fatalf("expected [admin-role], got %v", groups)
	}
}
