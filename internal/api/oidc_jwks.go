package api

import (
	"context"
	"crypto"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ID-token signature verification (OIDC Core §3.1.3.7). The token arrives
// straight from the token endpoint over TLS, which the spec lets a client rely
// on instead — but a JWKS check is stricter: it also catches a compromised or
// mis-routed token endpoint and makes the nonce binding meaningful. Everything
// here fails closed: an unreachable JWKS, an unknown kid, an unexpected
// algorithm or a bad signature all reject the login.

const (
	oidcNonceCookie = "axiom_oidc_nonce"

	jwksTTL          = 15 * time.Minute // how long a fetched key set is trusted
	jwksMinRefetch   = 30 * time.Second // floor between re-fetches on an unknown kid
	jwksMaxBody      = 1 << 20
	jwksMaxKeys      = 64
	jwksMaxCachedURI = 16
	minRSABits       = 2048
)

// oidcJWKS is the process-wide key cache used by the OIDC callback.
var oidcJWKS = newJWKSCache(oidcHTTPClient)

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

type jwksKey struct {
	pub crypto.PublicKey
	alg string // optional alg pin from the JWK
}

type jwksEntry struct {
	keys      map[string]jwksKey // by kid ("" holds a key published without one)
	fetched   time.Time
	lastFetch time.Time
}

type jwksCache struct {
	mu     sync.Mutex
	client *http.Client
	now    func() time.Time
	sets   map[string]*jwksEntry
}

func newJWKSCache(c *http.Client) *jwksCache {
	return &jwksCache{client: c, now: time.Now, sets: map[string]*jwksEntry{}}
}

// lookup returns the key for kid, fetching or refreshing the set as needed.
func (c *jwksCache) lookup(ctx context.Context, uri, kid string) (jwksKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	e := c.sets[uri]
	if e == nil || now.Sub(e.fetched) >= jwksTTL {
		fresh, err := c.fetchLocked(ctx, uri, now)
		if err != nil {
			return jwksKey{}, err
		}
		e = fresh
	}
	if k, ok := pick(e, kid); ok {
		return k, nil
	}
	// Unknown kid: the IdP may have rotated keys. Re-fetch, but rate-limited so
	// a stream of forged kids cannot turn Axiom into a request amplifier.
	if now.Sub(e.lastFetch) >= jwksMinRefetch {
		fresh, err := c.fetchLocked(ctx, uri, now)
		if err != nil {
			return jwksKey{}, err
		}
		if k, ok := pick(fresh, kid); ok {
			return k, nil
		}
	}
	return jwksKey{}, fmt.Errorf("no signing key with kid %q in the provider's JWKS", kid)
}

func pick(e *jwksEntry, kid string) (jwksKey, bool) {
	if kid != "" {
		k, ok := e.keys[kid]
		return k, ok
	}
	// No kid in the token header: only unambiguous if the set has one key.
	if len(e.keys) == 1 {
		for _, k := range e.keys {
			return k, true
		}
	}
	return jwksKey{}, false
}

func (c *jwksCache) fetchLocked(ctx context.Context, uri string, now time.Time) (*jwksEntry, error) {
	if !strings.HasPrefix(strings.ToLower(uri), "https://") && !strings.HasPrefix(strings.ToLower(uri), "http://") {
		return nil, fmt.Errorf("invalid jwks_uri %q", uri)
	}
	if len(c.sets) >= jwksMaxCachedURI && c.sets[uri] == nil {
		c.sets = map[string]*jwksEntry{} // bounded: reset rather than grow
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching JWKS: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, jwksMaxBody+1))
	if err != nil {
		return nil, fmt.Errorf("reading JWKS: %w", err)
	}
	if len(body) > jwksMaxBody {
		return nil, errors.New("JWKS response too large")
	}
	var doc struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parsing JWKS: %w", err)
	}
	if len(doc.Keys) > jwksMaxKeys {
		doc.Keys = doc.Keys[:jwksMaxKeys]
	}
	e := &jwksEntry{keys: map[string]jwksKey{}, fetched: now, lastFetch: now}
	for _, k := range doc.Keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		pub, err := k.publicKey()
		if err != nil {
			continue // skip unusable keys; the verifier fails closed if none match
		}
		e.keys[k.Kid] = jwksKey{pub: pub, alg: k.Alg}
	}
	if len(e.keys) == 0 {
		return nil, errors.New("JWKS contains no usable signing keys")
	}
	c.sets[uri] = e
	return e, nil
}

func b64u(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

func (k jwk) publicKey() (crypto.PublicKey, error) {
	switch k.Kty {
	case "RSA":
		nb, err := b64u(k.N)
		if err != nil {
			return nil, err
		}
		eb, err := b64u(k.E)
		if err != nil {
			return nil, err
		}
		n := new(big.Int).SetBytes(nb)
		e := new(big.Int).SetBytes(eb)
		if n.BitLen() < minRSABits || !e.IsInt64() || e.Int64() < 3 || e.Int64() > 1<<31-1 || e.Int64()%2 == 0 {
			return nil, errors.New("weak or invalid RSA key")
		}
		return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
	case "EC":
		var curve elliptic.Curve
		var ecdhCurve ecdh.Curve
		switch k.Crv {
		case "P-256":
			curve, ecdhCurve = elliptic.P256(), ecdh.P256()
		case "P-384":
			curve, ecdhCurve = elliptic.P384(), ecdh.P384()
		case "P-521":
			curve, ecdhCurve = elliptic.P521(), ecdh.P521()
		default:
			return nil, errors.New("unsupported curve")
		}
		xb, err := b64u(k.X)
		if err != nil {
			return nil, err
		}
		yb, err := b64u(k.Y)
		if err != nil {
			return nil, err
		}
		size := (curve.Params().BitSize + 7) / 8
		if len(xb) > size || len(yb) > size {
			return nil, errors.New("invalid EC coordinates")
		}
		point := make([]byte, 1+2*size)
		point[0] = 4
		copy(point[1+size-len(xb):1+size], xb)
		copy(point[1+2*size-len(yb):], yb)
		if _, err := ecdhCurve.NewPublicKey(point); err != nil { // rejects points not on the curve
			return nil, err
		}
		return &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(xb), Y: new(big.Int).SetBytes(yb)}, nil
	}
	return nil, errors.New("unsupported key type")
}

// jwtAlgs is the allow-list. "none" and every HMAC algorithm are absent on
// purpose: with a public JWKS an HS256 token would be verifiable with the
// public key as the MAC secret (algorithm confusion).
var jwtAlgs = map[string]struct {
	family string // RSA | PSS | EC
	hash   crypto.Hash
	curve  string
}{
	"RS256": {"RSA", crypto.SHA256, ""}, "RS384": {"RSA", crypto.SHA384, ""}, "RS512": {"RSA", crypto.SHA512, ""},
	"PS256": {"PSS", crypto.SHA256, ""}, "PS384": {"PSS", crypto.SHA384, ""}, "PS512": {"PSS", crypto.SHA512, ""},
	"ES256": {"EC", crypto.SHA256, "P-256"}, "ES384": {"EC", crypto.SHA384, "P-384"}, "ES512": {"EC", crypto.SHA512, "P-521"},
}

func digestOf(h crypto.Hash, data []byte) []byte {
	switch h {
	case crypto.SHA384:
		s := sha512.Sum384(data)
		return s[:]
	case crypto.SHA512:
		s := sha512.Sum512(data)
		return s[:]
	}
	s := sha256.Sum256(data)
	return s[:]
}

// verifyJWTSignature checks the compact JWS signature of token against the key
// the provider publishes for its kid, and returns the decoded claims payload.
func (c *jwksCache) verifyJWTSignature(ctx context.Context, token, jwksURI string) ([]byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[2] == "" {
		return nil, errors.New("id_token is not a signed JWT")
	}
	hb, err := b64u(parts[0])
	if err != nil {
		return nil, errors.New("malformed id_token header")
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(hb, &hdr); err != nil {
		return nil, errors.New("malformed id_token header")
	}
	spec, ok := jwtAlgs[hdr.Alg]
	if !ok {
		return nil, fmt.Errorf("id_token signing algorithm %q is not accepted", hdr.Alg)
	}
	key, err := c.lookup(ctx, jwksURI, hdr.Kid)
	if err != nil {
		return nil, err
	}
	if key.alg != "" && key.alg != hdr.Alg {
		return nil, fmt.Errorf("id_token alg %q does not match the signing key's alg %q", hdr.Alg, key.alg)
	}
	sig, err := b64u(parts[2])
	if err != nil {
		return nil, errors.New("malformed id_token signature")
	}
	digest := digestOf(spec.hash, []byte(parts[0]+"."+parts[1]))
	switch pub := key.pub.(type) {
	case *rsa.PublicKey:
		switch spec.family {
		case "RSA":
			err = rsa.VerifyPKCS1v15(pub, spec.hash, digest, sig)
		case "PSS":
			err = rsa.VerifyPSS(pub, spec.hash, digest, sig, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
		default:
			err = errors.New("algorithm does not match key type")
		}
	case *ecdsa.PublicKey:
		if spec.family != "EC" || pub.Curve.Params().Name != spec.curve {
			err = errors.New("algorithm does not match key type")
			break
		}
		size := (pub.Curve.Params().BitSize + 7) / 8
		if len(sig) != 2*size {
			err = errors.New("bad ECDSA signature length")
			break
		}
		r := new(big.Int).SetBytes(sig[:size])
		s := new(big.Int).SetBytes(sig[size:])
		if !ecdsa.Verify(pub, digest, r, s) {
			err = errors.New("signature mismatch")
		}
	default:
		err = errors.New("unsupported key")
	}
	if err != nil {
		return nil, errors.New("id_token signature is invalid")
	}
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
}

// verifyIDToken is the full ID-token check: signature against the provider's
// JWKS, then iss / aud / exp (validateIDTokenClaims), azp for multi-audience
// tokens, sub presence, and — when expectedNonce is set — the nonce bound to the
// browser session that started the login.
func verifyIDToken(ctx context.Context, cache *jwksCache, idToken, jwksURI, issuer, clientID, expectedNonce string, now time.Time) error {
	if jwksURI == "" {
		return errors.New("the OIDC provider publishes no jwks_uri, so the id_token signature cannot be verified")
	}
	payload, err := cache.verifyJWTSignature(ctx, idToken, jwksURI)
	if err != nil {
		return err
	}
	if err := validateIDTokenClaims(idToken, issuer, clientID, now); err != nil {
		return err
	}
	var c map[string]any
	if err := json.Unmarshal(payload, &c); err != nil {
		return errors.New("malformed id_token claims")
	}
	if sub, _ := c["sub"].(string); sub == "" {
		return errors.New("id_token has no sub")
	}
	if aud, ok := c["aud"].([]any); ok && len(aud) > 1 {
		if azp, _ := c["azp"].(string); azp != clientID {
			return errors.New("id_token azp does not match this client")
		}
	}
	if nbf, ok := c["nbf"].(float64); ok && now.Add(2*time.Minute).Before(time.Unix(int64(nbf), 0)) {
		return errors.New("id_token is not yet valid")
	}
	if expectedNonce == "" {
		return errors.New("missing OIDC nonce: restart the sign-in")
	}
	got, _ := c["nonce"].(string)
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expectedNonce)) != 1 {
		return errors.New("id_token nonce does not match this sign-in")
	}
	return nil
}
