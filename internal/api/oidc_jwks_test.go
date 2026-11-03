package api

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

const (
	testClient = "axiom"
	testNonce  = "nonce-123"
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

type testIdP struct {
	srv      *httptest.Server
	rsaKey   *rsa.PrivateKey
	ecKey    *ecdsa.PrivateKey
	hits     atomic.Int32
	jwksJSON atomic.Value // string
	status   atomic.Int32
	idToken  atomic.Value // string returned by the token endpoint
}

func (i *testIdP) rsaJWK(kid string, key *rsa.PrivateKey) map[string]any {
	return map[string]any{"kty": "RSA", "kid": kid, "use": "sig", "n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes())}
}

func (i *testIdP) ecJWK(kid string, key *ecdsa.PrivateKey) map[string]any {
	size := (key.Curve.Params().BitSize + 7) / 8
	pad := func(b []byte) []byte { return append(make([]byte, size-len(b)), b...) }
	return map[string]any{"kty": "EC", "kid": kid, "use": "sig", "crv": "P-256", "x": b64(pad(key.X.Bytes())), "y": b64(pad(key.Y.Bytes()))}
}

func (i *testIdP) setKeys(keys ...map[string]any) {
	b, _ := json.Marshal(map[string]any{"keys": keys})
	i.jwksJSON.Store(string(b))
}

func newTestIdP(t *testing.T) *testIdP {
	t.Helper()
	i := &testIdP{}
	var err error
	if i.rsaKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		t.Fatal(err)
	}
	if i.ecKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
		t.Fatal(err)
	}
	i.status.Store(200)
	i.setKeys(i.rsaJWK("k1", i.rsaKey))
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": i.srv.URL, "authorization_endpoint": i.srv.URL + "/auth", "token_endpoint": i.srv.URL + "/token",
			"jwks_uri": i.srv.URL + "/jwks", "userinfo_endpoint": i.srv.URL + "/userinfo",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		i.hits.Add(1)
		if st := int(i.status.Load()); st != 200 {
			http.Error(w, "boom", st)
			return
		}
		_, _ = w.Write([]byte(i.jwksJSON.Load().(string)))
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "id_token": i.idToken.Load(), "token_type": "Bearer"})
	})
	i.srv = httptest.NewServer(mux)
	t.Cleanup(i.srv.Close)
	return i
}

func (i *testIdP) claims(over map[string]any) map[string]any {
	c := map[string]any{
		"iss": i.srv.URL, "aud": testClient, "sub": "user-1", "preferred_username": "alice",
		"exp": float64(time.Now().Add(5 * time.Minute).Unix()), "nonce": testNonce,
	}
	for k, v := range over {
		if v == nil {
			delete(c, k)
		} else {
			c[k] = v
		}
	}
	return c
}

func signRS256(t *testing.T, key *rsa.PrivateKey, header map[string]any, claims map[string]any) string {
	t.Helper()
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	signing := b64(hb) + "." + b64(cb)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + b64(sig)
}

func signES256(t *testing.T, key *ecdsa.PrivateKey, header map[string]any, claims map[string]any) string {
	t.Helper()
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	signing := b64(hb) + "." + b64(cb)
	sum := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := append(append(make([]byte, 32-len(r.Bytes())), r.Bytes()...), append(make([]byte, 32-len(s.Bytes())), s.Bytes()...)...)
	return signing + "." + b64(sig)
}

func verify(i *testIdP, cache *jwksCache, tok, nonce string) error {
	return verifyIDToken(context.Background(), cache, tok, i.srv.URL+"/jwks", i.srv.URL, testClient, nonce, time.Now())
}

func TestVerifyIDToken(t *testing.T) {
	idp := newTestIdP(t)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	rs := map[string]any{"alg": "RS256", "kid": "k1", "typ": "JWT"}

	hsToken := func() string { // HS256 "signed" with the public modulus as the MAC secret
		hb, _ := json.Marshal(map[string]any{"alg": "HS256", "kid": "k1"})
		cb, _ := json.Marshal(idp.claims(nil))
		signing := b64(hb) + "." + b64(cb)
		m := hmac.New(sha256.New, idp.rsaKey.N.Bytes())
		m.Write([]byte(signing))
		return signing + "." + b64(m.Sum(nil))
	}
	unsigned := func(alg string) string {
		hb, _ := json.Marshal(map[string]any{"alg": alg, "kid": "k1"})
		cb, _ := json.Marshal(idp.claims(nil))
		return b64(hb) + "." + b64(cb) + "."
	}
	tampered := func() string {
		tok := signRS256(t, idp.rsaKey, rs, idp.claims(nil))
		parts := strings.Split(tok, ".")
		cb, _ := json.Marshal(idp.claims(map[string]any{"sub": "admin-1", "preferred_username": "root"}))
		return parts[0] + "." + b64(cb) + "." + parts[2]
	}

	cases := []struct {
		name  string
		tok   func() string
		nonce string
		ok    bool
	}{
		{"valid RS256", func() string { return signRS256(t, idp.rsaKey, rs, idp.claims(nil)) }, testNonce, true},
		{"signed by a different key under the same kid", func() string { return signRS256(t, other, rs, idp.claims(nil)) }, testNonce, false},
		{"alg=none, empty signature", func() string { return unsigned("none") }, testNonce, false},
		{"alg=None casing", func() string { return unsigned("None") }, testNonce, false},
		{"alg=RS256 but signature stripped", func() string { return unsigned("RS256") }, testNonce, false},
		{"HS256 using the public key as the secret", hsToken, testNonce, false},
		{"payload tampered after signing", tampered, testNonce, false},
		{"unknown kid", func() string {
			return signRS256(t, idp.rsaKey, map[string]any{"alg": "RS256", "kid": "nope"}, idp.claims(nil))
		}, testNonce, false},
		{"no kid in the header is fine when the set has exactly one key", func() string { return signRS256(t, idp.rsaKey, map[string]any{"alg": "RS256"}, idp.claims(nil)) }, testNonce, true}, // single-key set: unambiguous
		{"nonce mismatch", func() string { return signRS256(t, idp.rsaKey, rs, idp.claims(map[string]any{"nonce": "other"})) }, testNonce, false},
		{"nonce missing from token", func() string { return signRS256(t, idp.rsaKey, rs, idp.claims(map[string]any{"nonce": nil})) }, testNonce, false},
		{"nonce cookie missing", func() string { return signRS256(t, idp.rsaKey, rs, idp.claims(nil)) }, "", false},
		{"expired", func() string {
			return signRS256(t, idp.rsaKey, rs, idp.claims(map[string]any{"exp": float64(time.Now().Add(-time.Hour).Unix())}))
		}, testNonce, false},
		{"wrong issuer", func() string {
			return signRS256(t, idp.rsaKey, rs, idp.claims(map[string]any{"iss": "https://evil.example"}))
		}, testNonce, false},
		{"wrong audience", func() string { return signRS256(t, idp.rsaKey, rs, idp.claims(map[string]any{"aud": "someone-else"})) }, testNonce, false},
		{"multi-audience without azp", func() string {
			return signRS256(t, idp.rsaKey, rs, idp.claims(map[string]any{"aud": []string{testClient, "other"}}))
		}, testNonce, false},
		{"multi-audience with azp", func() string {
			return signRS256(t, idp.rsaKey, rs, idp.claims(map[string]any{"aud": []string{testClient, "other"}, "azp": testClient}))
		}, testNonce, true},
		{"missing sub", func() string { return signRS256(t, idp.rsaKey, rs, idp.claims(map[string]any{"sub": nil})) }, testNonce, false},
		{"not a JWT", func() string { return "garbage" }, testNonce, false},
		{"two segments", func() string { return "a.b" }, testNonce, false},
		{"unsupported alg claimed", func() string {
			return signRS256(t, idp.rsaKey, map[string]any{"alg": "RS1", "kid": "k1"}, idp.claims(nil))
		}, testNonce, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := verify(idp, newJWKSCache(http.DefaultClient), tc.tok(), tc.nonce)
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v, wantOK=%v", err, tc.ok)
			}
		})
	}

	t.Run("ES256 valid and tampered", func(t *testing.T) {
		idp.setKeys(idp.ecJWK("e1", idp.ecKey))
		hdr := map[string]any{"alg": "ES256", "kid": "e1"}
		cache := newJWKSCache(http.DefaultClient)
		good := signES256(t, idp.ecKey, hdr, idp.claims(nil))
		if err := verify(idp, cache, good, testNonce); err != nil {
			t.Fatalf("valid ES256 rejected: %v", err)
		}
		parts := strings.Split(good, ".")
		cb, _ := json.Marshal(idp.claims(map[string]any{"sub": "x"}))
		if err := verify(idp, cache, parts[0]+"."+b64(cb)+"."+parts[2], testNonce); err == nil {
			t.Fatal("tampered ES256 payload accepted")
		}
		// An RS256-labelled token must not verify against an EC key (key-type confusion).
		if err := verify(idp, cache, signRS256(t, idp.rsaKey, map[string]any{"alg": "RS256", "kid": "e1"}, idp.claims(nil)), testNonce); err == nil {
			t.Fatal("RS256 token accepted against an EC key")
		}
		idp.setKeys(idp.rsaJWK("k1", idp.rsaKey))
	})

	t.Run("key alg pin must match header alg", func(t *testing.T) {
		k := idp.rsaJWK("k1", idp.rsaKey)
		k["alg"] = "RS512"
		idp.setKeys(k)
		defer idp.setKeys(idp.rsaJWK("k1", idp.rsaKey))
		if err := verify(idp, newJWKSCache(http.DefaultClient), signRS256(t, idp.rsaKey, rs, idp.claims(nil)), testNonce); err == nil {
			t.Fatal("token alg differing from the key's pinned alg was accepted")
		}
	})

	t.Run("no jwks_uri", func(t *testing.T) {
		tok := signRS256(t, idp.rsaKey, rs, idp.claims(nil))
		if err := verifyIDToken(context.Background(), newJWKSCache(http.DefaultClient), tok, "", idp.srv.URL, testClient, testNonce, time.Now()); err == nil {
			t.Fatal("accepted a token with no way to verify its signature")
		}
	})
}

func TestJWKSFailClosedAndCaching(t *testing.T) {
	idp := newTestIdP(t)
	rs := map[string]any{"alg": "RS256", "kid": "k1"}
	good := signRS256(t, idp.rsaKey, rs, idp.claims(nil))

	t.Run("endpoint down", func(t *testing.T) {
		idp.status.Store(500)
		defer idp.status.Store(200)
		if err := verify(idp, newJWKSCache(http.DefaultClient), good, testNonce); err == nil {
			t.Fatal("accepted a token while the JWKS endpoint is failing")
		}
	})
	t.Run("garbage JWKS", func(t *testing.T) {
		idp.jwksJSON.Store("not json")
		defer idp.setKeys(idp.rsaJWK("k1", idp.rsaKey))
		if err := verify(idp, newJWKSCache(http.DefaultClient), good, testNonce); err == nil {
			t.Fatal("accepted with an unparsable JWKS")
		}
	})
	t.Run("weak RSA key ignored", func(t *testing.T) {
		weak, _ := rsa.GenerateKey(rand.Reader, 1024)
		idp.setKeys(idp.rsaJWK("k1", weak))
		defer idp.setKeys(idp.rsaJWK("k1", idp.rsaKey))
		if err := verify(idp, newJWKSCache(http.DefaultClient), signRS256(t, weak, rs, idp.claims(nil)), testNonce); err == nil {
			t.Fatal("accepted a signature from a 1024-bit key")
		}
	})
	t.Run("enc-only key ignored", func(t *testing.T) {
		k := idp.rsaJWK("k1", idp.rsaKey)
		k["use"] = "enc"
		idp.setKeys(k)
		defer idp.setKeys(idp.rsaJWK("k1", idp.rsaKey))
		if err := verify(idp, newJWKSCache(http.DefaultClient), good, testNonce); err == nil {
			t.Fatal("accepted a key published for encryption")
		}
	})
	t.Run("keys are cached", func(t *testing.T) {
		idp.hits.Store(0)
		cache := newJWKSCache(http.DefaultClient)
		for i := 0; i < 5; i++ {
			if err := verify(idp, cache, good, testNonce); err != nil {
				t.Fatal(err)
			}
		}
		if n := idp.hits.Load(); n != 1 {
			t.Fatalf("expected 1 JWKS fetch for 5 verifications, got %d", n)
		}
	})
	t.Run("unknown kid refetches at most once per interval and picks up rotation", func(t *testing.T) {
		idp.hits.Store(0)
		now := time.Now()
		cache := newJWKSCache(http.DefaultClient)
		cache.now = func() time.Time { return now }
		if err := verify(idp, cache, good, testNonce); err != nil {
			t.Fatal(err)
		}
		forged := signRS256(t, idp.rsaKey, map[string]any{"alg": "RS256", "kid": "forged"}, idp.claims(nil))
		for i := 0; i < 10; i++ {
			if err := verify(idp, cache, forged, testNonce); err == nil {
				t.Fatal("unknown kid accepted")
			}
		}
		if n := idp.hits.Load(); n != 1 {
			t.Fatalf("forged kids must not amplify into fetches inside the refetch window; got %d fetches", n)
		}
		// IdP rotates to a new key; after the floor elapses the unknown kid triggers one refresh.
		rotated, _ := rsa.GenerateKey(rand.Reader, 2048)
		idp.setKeys(idp.rsaJWK("k2", rotated))
		now = now.Add(jwksMinRefetch + time.Second)
		if err := verify(idp, cache, signRS256(t, rotated, map[string]any{"alg": "RS256", "kid": "k2"}, idp.claims(nil)), testNonce); err != nil {
			t.Fatalf("rotated key not picked up: %v", err)
		}
		// The retired key no longer verifies.
		now = now.Add(jwksTTL + time.Second)
		if err := verify(idp, cache, good, testNonce); err == nil {
			t.Fatal("retired key still accepted after the cache refreshed")
		}
	})
}

// End-to-end through the real callback handler: a fake IdP issues the token.
func TestOIDCCallbackVerifiesSignatureAndNonce(t *testing.T) {
	idp := newTestIdP(t)
	rs := map[string]any{"alg": "RS256", "kid": "k1"}
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{OIDCIssuer: idp.srv.URL, OIDCClient: testClient, SessionSecret: "s3cret-for-tests"})

	call := func(idToken string, nonceCookie string) *httptest.ResponseRecorder {
		idp.idToken.Store(idToken)
		r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=abc&state=st", nil)
		r.AddCookie(&http.Cookie{Name: auth.OIDCState, Value: "st"})
		r.AddCookie(&http.Cookie{Name: auth.OIDCVerifier, Value: "ver"})
		if nonceCookie != "" {
			r.AddCookie(&http.Cookie{Name: oidcNonceCookie, Value: nonceCookie})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	hasSession := func(w *httptest.ResponseRecorder) bool {
		for _, c := range w.Result().Cookies() {
			if c.Name == auth.SessionCookie && c.Value != "" {
				return true
			}
		}
		return false
	}

	w := call(signRS256(t, idp.rsaKey, rs, idp.claims(nil)), testNonce)
	if w.Code != http.StatusFound || !hasSession(w) {
		t.Fatalf("valid sign-in: code=%d body=%s", w.Code, w.Body.String())
	}
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	for name, tc := range map[string]struct {
		tok, nonce string
	}{
		"forged signature":   {signRS256(t, other, rs, idp.claims(nil)), testNonce},
		"alg none":           {b64([]byte(`{"alg":"none"}`)) + "." + b64([]byte(`{"iss":"x","aud":"axiom","sub":"a","exp":9999999999,"nonce":"nonce-123"}`)) + ".", testNonce},
		"nonce mismatch":     {signRS256(t, idp.rsaKey, rs, idp.claims(nil)), "a-different-browser"},
		"nonce cookie gone":  {signRS256(t, idp.rsaKey, rs, idp.claims(nil)), ""},
		"replayed old nonce": {signRS256(t, idp.rsaKey, rs, idp.claims(map[string]any{"nonce": "older"})), testNonce},
	} {
		w := call(tc.tok, tc.nonce)
		if w.Code != http.StatusUnauthorized || hasSession(w) {
			t.Errorf("%s: code=%d session=%v (want 401 and no session): %s", name, w.Code, hasSession(w), w.Body.String())
		}
	}
}

func TestOIDCLoginSetsNonce(t *testing.T) {
	idp := newTestIdP(t)
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{OIDCIssuer: idp.srv.URL, OIDCClient: testClient})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/login", nil))
	if w.Code != http.StatusFound {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	var cookie string
	for _, c := range w.Result().Cookies() {
		if c.Name == oidcNonceCookie {
			cookie = c.Value
		}
	}
	if cookie == "" || !strings.Contains(loc, "nonce="+cookie) {
		t.Fatalf("authorization request must carry the nonce held in the cookie; cookie=%q location=%s", cookie, loc)
	}
}
