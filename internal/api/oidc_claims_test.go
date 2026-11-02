package api

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func fakeIDToken(claims map[string]any) string {
	b, _ := json.Marshal(claims)
	return "e30." + base64.RawURLEncoding.EncodeToString(b) + ".sig"
}

func TestValidateIDTokenClaims(t *testing.T) {
	now := time.Now()
	good := map[string]any{"iss": "https://idp.example/realms/x", "aud": "axiom", "exp": float64(now.Add(5 * time.Minute).Unix())}
	with := func(k string, v any) map[string]any {
		m := map[string]any{}
		for kk, vv := range good {
			m[kk] = vv
		}
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
		return m
	}
	cases := []struct {
		name   string
		claims map[string]any
		ok     bool
	}{
		{"valid", good, true},
		{"issuer trailing slash tolerated", with("iss", "https://idp.example/realms/x/"), true},
		{"aud array containing client", with("aud", []any{"other", "axiom"}), true},
		{"wrong issuer", with("iss", "https://evil.example"), false},
		{"wrong audience", with("aud", "some-other-client"), false},
		{"aud array without client", with("aud", []any{"a", "b"}), false},
		{"expired", with("exp", float64(now.Add(-time.Hour).Unix())), false},
		{"missing exp", with("exp", nil), false},
	}
	for _, c := range cases {
		err := validateIDTokenClaims(fakeIDToken(c.claims), "https://idp.example/realms/x", "axiom", now)
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v, wantOK=%v", c.name, err, c.ok)
		}
	}
	if validateIDTokenClaims("not-a-jwt", "i", "c", now) == nil {
		t.Error("a malformed token must be rejected")
	}
}
