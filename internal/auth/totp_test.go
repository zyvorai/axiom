package auth

import (
	"strings"
	"testing"
	"time"
)

// RFC 6238 appendix B uses the ASCII secret "12345678901234567890" and lists
// 8-digit codes; our 6-digit codes are the last six digits of each.
var rfcSecret = totpEncoding.EncodeToString([]byte("12345678901234567890"))

func TestTOTPMatchesRFC6238Vectors(t *testing.T) {
	cases := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	}
	for _, c := range cases {
		got, err := TOTPCodeAt(rfcSecret, time.Unix(c.unix, 0))
		if err != nil || got != c.want {
			t.Errorf("t=%d: got %q (err %v), want %q", c.unix, got, err, c.want)
		}
	}
}

func TestVerifyTOTPWindowAndReplay(t *testing.T) {
	now := time.Unix(1111111109, 0)
	code, _ := TOTPCodeAt(rfcSecret, now)
	step, ok := VerifyTOTP(rfcSecret, code, now, 0)
	if !ok || step != now.Unix()/30 {
		t.Fatalf("current code rejected: step=%d ok=%v", step, ok)
	}
	// Replay: the same code, with that step recorded as used, must fail.
	if _, ok := VerifyTOTP(rfcSecret, code, now, step); ok {
		t.Fatal("a code was accepted twice")
	}
	// An older step than the last used one is also refused.
	prev, _ := TOTPCodeAt(rfcSecret, now.Add(-30*time.Second))
	if _, ok := VerifyTOTP(rfcSecret, prev, now, step); ok {
		t.Fatal("a code older than the last used step was accepted")
	}
	// One step of drift either side is tolerated, two is not.
	prev, _ = TOTPCodeAt(rfcSecret, now.Add(-30*time.Second))
	if _, ok := VerifyTOTP(rfcSecret, prev, now, 0); !ok {
		t.Error("code from the previous step should be accepted")
	}
	next, _ := TOTPCodeAt(rfcSecret, now.Add(30*time.Second))
	if _, ok := VerifyTOTP(rfcSecret, next, now, 0); !ok {
		t.Error("code from the next step should be accepted")
	}
	far, _ := TOTPCodeAt(rfcSecret, now.Add(90*time.Second))
	if _, ok := VerifyTOTP(rfcSecret, far, now, 0); ok {
		t.Error("code three steps ahead should be rejected")
	}
}

func TestVerifyTOTPRejectsMalformedCodes(t *testing.T) {
	now := time.Unix(59, 0)
	for _, bad := range []string{"", "12345", "1234567", "abcdef", "28708 2x", "２８７０８２"} {
		if _, ok := VerifyTOTP(rfcSecret, bad, now, 0); ok {
			t.Errorf("accepted %q", bad)
		}
	}
	// A code typed with a space in the middle is fine.
	if _, ok := VerifyTOTP(rfcSecret, "287 082", now, 0); !ok {
		t.Error("rejected a code with a space")
	}
	if _, ok := VerifyTOTP("not base32!", "287082", now, 0); ok {
		t.Error("accepted a malformed secret")
	}
}

func TestGeneratedSecretIsValidAndUnique(t *testing.T) {
	a, b := GenerateTOTPSecret(), GenerateTOTPSecret()
	if a == b || !ValidTOTPSecret(a) || len(a) != 32 {
		t.Fatalf("bad secrets %q %q", a, b)
	}
	if ValidTOTPSecret("ABC") {
		t.Error("a 15-bit secret was accepted")
	}
	// People paste secrets grouped with spaces and in lower case.
	grouped := strings.ToLower(a[:8] + " " + a[8:16] + "-" + a[16:])
	if !ValidTOTPSecret(grouped) || NormalizeTOTPSecret(grouped) != a {
		t.Errorf("grouped secret not normalised: %q", grouped)
	}
}

func TestProvisioningURI(t *testing.T) {
	u := TOTPProvisioningURI("Zyvor Axiom", "alice@example.com", "jbswy3dpehpk3pxp")
	for _, want := range []string{"otpauth://totp/Zyvor%20Axiom:alice@example.com?", "secret=JBSWY3DPEHPK3PXP", "issuer=Zyvor+Axiom", "digits=6", "period=30"} {
		if !strings.Contains(u, want) {
			t.Errorf("%q missing %q", u, want)
		}
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes := GenerateRecoveryCodes()
	if len(codes) != 10 {
		t.Fatalf("got %d codes", len(codes))
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if len(c) != 11 || c[5] != '-' || seen[c] {
			t.Fatalf("malformed or duplicate code %q", c)
		}
		seen[c] = true
		if !LooksLikeRecoveryCode(c) || HashRecoveryCode(c) != HashRecoveryCode(strings.ToUpper(strings.ReplaceAll(c, "-", " "))) {
			t.Errorf("normalisation mismatch for %q", c)
		}
	}
	if LooksLikeRecoveryCode("123456") {
		t.Error("a TOTP code looked like a recovery code")
	}
}
