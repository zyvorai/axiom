package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 mandates HMAC-SHA1 for interoperability with authenticator apps
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP (RFC 6238) with the parameters every authenticator app defaults to:
// HMAC-SHA1, 6 digits, 30-second steps. The standard library is enough; no
// third-party dependency is pulled in for it.
const (
	totpDigits = 6
	totpPeriod = 30
	// totpSkew is how many steps either side of "now" are accepted, which
	// tolerates ordinary clock drift between the phone and the server.
	totpSkew = 1
)

var totpEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateTOTPSecret returns a fresh 160-bit shared secret, base32 encoded
// without padding (the form authenticator apps expect).
func GenerateTOTPSecret() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return totpEncoding.EncodeToString(b)
}

// NormalizeTOTPSecret upper-cases a secret and strips the spaces and dashes
// people add when copying it, so it can be compared and decoded reliably.
func NormalizeTOTPSecret(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.NewReplacer(" ", "", "-", "", "=", "").Replace(s)
	return s
}

// ValidTOTPSecret reports whether s decodes as a base32 secret of a sensible
// length (at least 80 bits, the RFC 4226 minimum).
func ValidTOTPSecret(s string) bool {
	raw, err := totpEncoding.DecodeString(NormalizeTOTPSecret(s))
	return err == nil && len(raw) >= 10
}

// TOTPCodeAt computes the code for the time step containing t.
func TOTPCodeAt(secret string, t time.Time) (string, error) {
	return totpCodeForStep(secret, t.Unix()/totpPeriod)
}

func totpCodeForStep(secret string, step int64) (string, error) {
	raw, err := totpEncoding.DecodeString(NormalizeTOTPSecret(secret))
	if err != nil || len(raw) == 0 {
		return "", fmt.Errorf("invalid TOTP secret")
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, raw)
	_, _ = mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[off])&0x7f)<<24 | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	mod := uint32(1)
	for i := 0; i < totpDigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", totpDigits, bin%mod), nil
}

// NormalizeTOTPCode strips the space people type in the middle of a code
// ("123 456") and rejects anything that is not exactly six digits.
func NormalizeTOTPCode(code string) (string, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return "", false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return "", false
		}
	}
	return code, true
}

// VerifyTOTP checks code against the secret around now and returns the time
// step it matched. A step at or before lastStep is refused, so a code that was
// already used (or an older one) can never be replayed: the caller persists the
// returned step as the new lastStep.
func VerifyTOTP(secret, code string, now time.Time, lastStep int64) (int64, bool) {
	code, ok := NormalizeTOTPCode(code)
	if !ok {
		return 0, false
	}
	cur := now.Unix() / totpPeriod
	matched := int64(0)
	found := false
	// Check every candidate step without an early return so the time taken does
	// not reveal which step (if any) matched.
	for d := int64(-totpSkew); d <= totpSkew; d++ {
		step := cur + d
		want, err := totpCodeForStep(secret, step)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 && step > lastStep && !found {
			matched, found = step, true
		}
	}
	return matched, found
}

// TOTPProvisioningURI is the otpauth:// URI authenticator apps import (also
// what a QR code would encode).
func TOTPProvisioningURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", NormalizeTOTPSecret(secret))
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(totpDigits))
	q.Set("period", fmt.Sprint(totpPeriod))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// Recovery codes let someone who lost their authenticator sign in once and
// re-enrol. They are long random strings, so a fast hash is sufficient to keep
// them unreadable at rest.
const (
	recoveryCodeCount = 10
	recoveryAlphabet  = "abcdefghjkmnpqrstuvwxyz23456789" // no look-alikes (i l o 0 1)
)

// GenerateRecoveryCodes returns n fresh codes formatted xxxxx-xxxxx.
func GenerateRecoveryCodes() []string {
	out := make([]string, recoveryCodeCount)
	for i := range out {
		b := make([]byte, 10)
		_, _ = rand.Read(b)
		var sb strings.Builder
		for j, v := range b {
			if j == 5 {
				sb.WriteByte('-')
			}
			sb.WriteByte(recoveryAlphabet[int(v)%len(recoveryAlphabet)])
		}
		out[i] = sb.String()
	}
	return out
}

// NormalizeRecoveryCode lower-cases a code and drops spaces and dashes so
// "ABCDE FGHJK" and "abcde-fghjk" compare equal.
func NormalizeRecoveryCode(c string) string {
	c = strings.ToLower(strings.TrimSpace(c))
	return strings.NewReplacer("-", "", " ", "").Replace(c)
}

// HashRecoveryCode returns the hex SHA-256 of the normalised code.
func HashRecoveryCode(c string) string {
	sum := sha256.Sum256([]byte(NormalizeRecoveryCode(c)))
	return hex.EncodeToString(sum[:])
}

// LooksLikeRecoveryCode reports whether c has the shape of a recovery code
// (ten characters once separators are removed) rather than a six-digit TOTP.
func LooksLikeRecoveryCode(c string) bool {
	return len(NormalizeRecoveryCode(c)) == 10
}
