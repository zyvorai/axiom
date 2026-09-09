package auth

import (
	"bytes"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// BER/DER encoding helpers. These are pure functions, so we verify them
// against hand-computed byte sequences per the ASN.1 BER rules used by RFC
// 4511 (LDAPv3): a SEQUENCE is tag 0x30, an INTEGER is tag 0x02 (two's
// complement, minimal length, with a leading 0x00 pad byte whenever the
// high bit of the first content byte would otherwise flip the sign), an
// OCTET STRING is tag 0x04, and a context-specific primitive tag N is
// 0x80|N. Lengths under 128 are a single byte; 128-255 use the long form
// 0x81 <byte>.
// ---------------------------------------------------------------------------

func TestEncodeLength(t *testing.T) {
	cases := []struct {
		n    int
		want []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{127, []byte{0x7F}},
		{128, []byte{0x81, 0x80}},
		{255, []byte{0x81, 0xFF}},
		{256, []byte{0x82, 0x01, 0x00}},
	}
	for _, tc := range cases {
		got := encodeLength(tc.n)
		if !bytes.Equal(got, tc.want) {
			t.Errorf("encodeLength(%d) = % X, want % X", tc.n, got, tc.want)
		}
	}
}

func TestEncodeInteger(t *testing.T) {
	cases := []struct {
		name string
		n    int
		want []byte
	}{
		{"zero", 0, []byte{0x02, 0x01, 0x00}},
		{"one", 1, []byte{0x02, 0x01, 0x01}},
		{"ldap protocol version 3", 3, []byte{0x02, 0x01, 0x03}},
		{"max single byte positive (127)", 127, []byte{0x02, 0x01, 0x7F}},
		{"128 needs a leading zero pad byte", 128, []byte{0x02, 0x02, 0x00, 0x80}},
		{"255 needs a leading zero pad byte", 255, []byte{0x02, 0x02, 0x00, 0xFF}},
		{"256 needs two bytes, no padding", 256, []byte{0x02, 0x02, 0x01, 0x00}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := encodeInteger(tc.n)
			if !bytes.Equal(got, tc.want) {
				t.Errorf("encodeInteger(%d) = % X, want % X", tc.n, got, tc.want)
			}
		})
	}
}

func TestEncodeOctetString(t *testing.T) {
	got := encodeOctetString([]byte("cn=admin"))
	want := append([]byte{0x04, 0x08}, []byte("cn=admin")...)
	if !bytes.Equal(got, want) {
		t.Errorf("encodeOctetString(%q) = % X, want % X", "cn=admin", got, want)
	}

	got = encodeOctetString(nil)
	want = []byte{0x04, 0x00}
	if !bytes.Equal(got, want) {
		t.Errorf("encodeOctetString(nil) = % X, want % X", got, want)
	}

	// 200 bytes exercises the long-form length (0x81 0xC8), since 200 > 127.
	long := bytes.Repeat([]byte("a"), 200)
	got = encodeOctetString(long)
	wantHeader := []byte{0x04, 0x81, 0xC8}
	if !bytes.Equal(got[:3], wantHeader) {
		t.Errorf("encodeOctetString(200 bytes) header = % X, want % X", got[:3], wantHeader)
	}
	if !bytes.Equal(got[3:], long) {
		t.Errorf("encodeOctetString(200 bytes) payload was mangled")
	}
}

func TestEncodeContext(t *testing.T) {
	got := encodeContext(0, []byte("pw"))
	want := []byte{0x80, 0x02, 'p', 'w'}
	if !bytes.Equal(got, want) {
		t.Errorf("encodeContext(0, %q) = % X, want % X", "pw", got, want)
	}

	got = encodeContext(3, []byte("x"))
	want = []byte{0x83, 0x01, 'x'}
	if !bytes.Equal(got, want) {
		t.Errorf("encodeContext(3, %q) = % X, want % X", "x", got, want)
	}
}

func TestEncodeRaw(t *testing.T) {
	got := encodeRaw(0x60, encodeInteger(3), encodeOctetString([]byte("cn=x")), encodeContext(0, []byte("secret")))
	want := []byte{
		0x60, 0x11, // application tag 0 (bind request), content length 17
		0x02, 0x01, 0x03, // version = 3
		0x04, 0x04, 'c', 'n', '=', 'x', // name (DN)
		0x80, 0x06, 's', 'e', 'c', 'r', 'e', 't', // authentication choice 0: simple password
	}
	if !bytes.Equal(got, want) {
		t.Errorf("encodeRaw(...) = % X, want % X", got, want)
	}
}

func TestEncodeLDAPBindRequest(t *testing.T) {
	got := encodeLDAPBindRequest(1, "cn=x", "secret")
	want := []byte{
		0x30, 0x16, // LDAPMessage SEQUENCE, content length 22
		0x02, 0x01, 0x01, // messageID = 1
		0x60, 0x11, // bindRequest [APPLICATION 0], content length 17
		0x02, 0x01, 0x03, // version = 3
		0x04, 0x04, 'c', 'n', '=', 'x', // name (DN)
		0x80, 0x06, 's', 'e', 'c', 'r', 'e', 't', // simple password
	}
	if !bytes.Equal(got, want) {
		t.Errorf("encodeLDAPBindRequest(1, %q, %q) = % X, want % X", "cn=x", "secret", got, want)
	}
}

// ---------------------------------------------------------------------------
// parseLDAPBindResult: decoding the (simplified) BindResponse wire format.
// ---------------------------------------------------------------------------

// canonicalBindResponse builds a well-formed LDAPMessage wrapping a
// BindResponse with the given resultCode, empty matchedDN, and empty
// diagnosticMessage — the shape a real directory server sends back.
func canonicalBindResponse(resultCode byte) []byte {
	bindResponseInner := []byte{
		0x0a, 0x01, resultCode, // resultCode ENUMERATED
		0x04, 0x00, // matchedDN OCTET STRING ""
		0x04, 0x00, // diagnosticMessage OCTET STRING ""
	}
	bindResponse := append([]byte{0x61, byte(len(bindResponseInner))}, bindResponseInner...)
	messageID := []byte{0x02, 0x01, 0x01}
	seqInner := append(append([]byte{}, messageID...), bindResponse...)
	return append([]byte{0x30, byte(len(seqInner))}, seqInner...)
}

func TestParseLDAPBindResult(t *testing.T) {
	code, err := parseLDAPBindResult(canonicalBindResponse(0))
	if err != nil || code != 0 {
		t.Fatalf("success response: got code=%d err=%v, want code=0 err=nil", code, err)
	}

	code, err = parseLDAPBindResult(canonicalBindResponse(49)) // invalidCredentials
	if err != nil || code != 49 {
		t.Fatalf("invalidCredentials response: got code=%d err=%v, want code=49 err=nil", code, err)
	}

	if _, err := parseLDAPBindResult([]byte{0x01, 0x02, 0x03}); err == nil {
		t.Fatal("expected an error for a buffer with no BindResponse and no resultCode")
	}
}

// ---------------------------------------------------------------------------
// ldapBindDN: pure bind-DN construction logic.
// ---------------------------------------------------------------------------

func TestLdapEscapeDN(t *testing.T) {
	cases := []struct{ in, want string }{
		{"alice", "alice"},
		{"cn=admin,dc=example,dc=com", `cn\=admin\,dc\=example\,dc\=com`},
		{`back\slash`, `back\\slash`},
		{"quote\"here", `quote\"here`},
		{" leading", `\ leading`},
		{"trailing ", `trailing\ `},
		{"#leading-hash", `\#leading-hash`},
		{"mid#hash", "mid#hash"},
	}
	for _, tc := range cases {
		if got := ldapEscapeDN(tc.in); got != tc.want {
			t.Errorf("ldapEscapeDN(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLdapBindDN(t *testing.T) {
	cases := []struct {
		name     string
		cfg      LDAPConfig
		username string
		want     string
	}{
		{
			// Regression test for a real bug: username is attacker-controlled
			// (submitted at the anonymous login form), so a value that merely
			// *looks* like a DN must never be trusted as one — that would let
			// anyone bind as an arbitrary DN anywhere in the directory,
			// bypassing BaseDN scoping. It's now escaped and treated as an
			// ordinary attribute value instead.
			name:     "a DN-shaped username is escaped into the configured BaseDN, not trusted as a raw DN",
			cfg:      LDAPConfig{BaseDN: "dc=example,dc=com"},
			username: "cn=admin,dc=example,dc=com",
			want:     `uid=cn\=admin\,dc\=example\,dc\=com,dc=example,dc=com`,
		},
		{
			name:     "a comma-shaped username with no BaseDN falls back to the bare (unescaped) username",
			cfg:      LDAPConfig{},
			username: "admin,ou=people",
			want:     "admin,ou=people",
		},
		{
			name:     "UPN form passes through untouched",
			cfg:      LDAPConfig{Domain: "example.com"},
			username: "alice@example.com",
			want:     "alice@example.com",
		},
		{
			name:     "NetBIOS domain produces DOMAIN\\user",
			cfg:      LDAPConfig{Domain: "CORP"},
			username: "alice",
			want:     `CORP\alice`,
		},
		{
			name:     "BaseDN with default uid attribute",
			cfg:      LDAPConfig{BaseDN: "dc=example,dc=com"},
			username: "alice",
			want:     "uid=alice,dc=example,dc=com",
		},
		{
			name:     "BaseDN with a custom UserAttr",
			cfg:      LDAPConfig{BaseDN: "dc=example,dc=com", UserAttr: "sAMAccountName"},
			username: "alice",
			want:     "sAMAccountName=alice,dc=example,dc=com",
		},
		{
			name:     "a Domain that already looks like a DN is not treated as NetBIOS",
			cfg:      LDAPConfig{Domain: "dc=example,dc=com", BaseDN: "dc=example,dc=com"},
			username: "alice",
			want:     "uid=alice,dc=example,dc=com",
		},
		{
			name:     "no Domain and no BaseDN falls back to the bare username",
			cfg:      LDAPConfig{},
			username: "alice",
			want:     "alice",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ldapBindDN(tc.cfg, tc.username)
			if got != tc.want {
				t.Errorf("ldapBindDN(%+v, %q) = %q, want %q", tc.cfg, tc.username, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// LDAPAuthenticate end to end, against a minimal fake LDAP-protocol TCP
// listener that we control on both the encode and decode sides. This
// exercises the real socket read/write path in ldapSimpleBind without a
// live directory server.
// ---------------------------------------------------------------------------

// fakeLDAPServer accepts a single connection and, for each entry in
// responses, blocks for one inbound write (a bind request) then replies with
// the canned bytes — in order, on the same connection. This mirrors what
// ldapSimpleBind actually does: it may issue two sequential binds on one
// connection when trying the UPN-fallback form.
func fakeLDAPServer(t *testing.T, responses [][]byte) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4096)
		for _, resp := range responses {
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			n, err := conn.Read(buf)
			if err != nil || n == 0 {
				return
			}
			if _, err := conn.Write(resp); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String()
}

func TestLDAPAuthenticateSuccess(t *testing.T) {
	addr := fakeLDAPServer(t, [][]byte{canonicalBindResponse(0)})
	cfg := LDAPConfig{Enabled: true, URL: "ldap://" + addr, BaseDN: "dc=example,dc=com"}

	role, ok, err := LDAPAuthenticate(cfg, "alice", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected a successful bind to authenticate")
	}
	if role != "operator" {
		t.Fatalf("expected the default operator role when no groups are configured, got %q", role)
	}
}

// canonicalSearchResultEntry builds a SearchResultEntry LDAPMessage: { objectName
// OCTET STRING, attributes SEQUENCE OF PartialAttribute { type OCTET STRING,
// vals SET OF OCTET STRING } } — mirrors canonicalBindResponse's hand-computed
// style above, all field lengths kept under 128 so single-byte BER lengths apply.
func canonicalSearchResultEntry(messageID byte, dn, attrName string, values []string) []byte {
	var valsInner []byte
	for _, v := range values {
		valsInner = append(valsInner, 0x04, byte(len(v)))
		valsInner = append(valsInner, []byte(v)...)
	}
	valsSet := append([]byte{0x31, byte(len(valsInner))}, valsInner...)

	attrType := append([]byte{0x04, byte(len(attrName))}, []byte(attrName)...)
	partialAttrInner := append(append([]byte{}, attrType...), valsSet...)
	partialAttr := append([]byte{0x30, byte(len(partialAttrInner))}, partialAttrInner...)
	attrsSeq := append([]byte{0x30, byte(len(partialAttr))}, partialAttr...)

	objectName := append([]byte{0x04, byte(len(dn))}, []byte(dn)...)
	entryInner := append(append([]byte{}, objectName...), attrsSeq...)
	entry := append([]byte{0x64, byte(len(entryInner))}, entryInner...)

	seqInner := append([]byte{0x02, 0x01, messageID}, entry...)
	return append([]byte{0x30, byte(len(seqInner))}, seqInner...)
}

// canonicalSearchResultDone builds a successful SearchResultDone LDAPMessage,
// the terminator ldapSearchGroups reads to know the entry list is complete.
func canonicalSearchResultDone(messageID byte) []byte {
	doneInner := []byte{
		0x0a, 0x01, 0x00, // resultCode success
		0x04, 0x00, // matchedDN ""
		0x04, 0x00, // diagnosticMessage ""
	}
	done := append([]byte{0x65, byte(len(doneInner))}, doneInner...)
	seqInner := append([]byte{0x02, 0x01, messageID}, done...)
	return append([]byte{0x30, byte(len(seqInner))}, seqInner...)
}

// joinLDAPMessages concatenates canned LDAPMessage byte sequences the way a
// real server pushes SearchResultEntry/SearchResultDone back to back without
// waiting for another client write in between — see fakeLDAPServer's doc
// comment: it writes one slice entry per client write it reads, so a
// multi-message push has to arrive pre-joined as a single entry.
func joinLDAPMessages(msgs ...[]byte) []byte {
	var out []byte
	for _, m := range msgs {
		out = append(out, m...)
	}
	return out
}

func TestLDAPAuthenticateMapsAdminGroup(t *testing.T) {
	addr := fakeLDAPServer(t, [][]byte{
		canonicalBindResponse(0),
		joinLDAPMessages(
			canonicalSearchResultEntry(2, "uid=alice,dc=example,dc=com", "memberOf", []string{"cn=Axiom Admins,ou=groups,dc=example,dc=com"}),
			canonicalSearchResultDone(2),
		),
	})
	cfg := LDAPConfig{
		Enabled: true, URL: "ldap://" + addr, BaseDN: "dc=example,dc=com",
		AdminGroup: "Axiom Admins", ViewerGroup: "Axiom Viewers",
	}

	role, ok, err := LDAPAuthenticate(cfg, "alice", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected a successful bind to authenticate")
	}
	if role != "admin" {
		t.Fatalf("expected admin role from memberOf CN match, got %q", role)
	}
}

func TestLDAPAuthenticateMapsViewerGroupByFullDN(t *testing.T) {
	addr := fakeLDAPServer(t, [][]byte{
		canonicalBindResponse(0),
		joinLDAPMessages(
			canonicalSearchResultEntry(2, "uid=alice,dc=example,dc=com", "memberOf", []string{"cn=Axiom Viewers,ou=groups,dc=example,dc=com"}),
			canonicalSearchResultDone(2),
		),
	})
	cfg := LDAPConfig{
		Enabled: true, URL: "ldap://" + addr, BaseDN: "dc=example,dc=com",
		AdminGroup: "Axiom Admins", ViewerGroup: "cn=Axiom Viewers,ou=groups,dc=example,dc=com",
	}

	role, ok, err := LDAPAuthenticate(cfg, "alice", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected a successful bind to authenticate")
	}
	if role != "viewer" {
		t.Fatalf("expected viewer role from a full-DN match, got %q", role)
	}
}

func TestLDAPAuthenticateDefaultsToOperatorWhenNoGroupMatches(t *testing.T) {
	addr := fakeLDAPServer(t, [][]byte{
		canonicalBindResponse(0),
		joinLDAPMessages(
			canonicalSearchResultEntry(2, "uid=alice,dc=example,dc=com", "memberOf", []string{"cn=Everyone,ou=groups,dc=example,dc=com"}),
			canonicalSearchResultDone(2),
		),
	})
	cfg := LDAPConfig{
		Enabled: true, URL: "ldap://" + addr, BaseDN: "dc=example,dc=com",
		AdminGroup: "Axiom Admins", ViewerGroup: "Axiom Viewers",
	}

	role, ok, err := LDAPAuthenticate(cfg, "alice", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected a successful bind to authenticate")
	}
	if role != "operator" {
		t.Fatalf("expected the operator default when memberOf matches neither configured group, got %q", role)
	}
}

func TestLDAPAuthenticateSkipsGroupLookupForNonDNBindForm(t *testing.T) {
	// Domain set (NetBIOS bind form) with no BaseDN: ldapBindDN returns a bare
	// "DOMAIN\user" identity, not a real DN, so ldapRoleAfterBind must not
	// attempt a search on it — only the bind response is in the fake server's
	// queue, so any unexpected search write here would hang/fail the read.
	addr := fakeLDAPServer(t, [][]byte{canonicalBindResponse(0)})
	cfg := LDAPConfig{
		Enabled: true, URL: "ldap://" + addr, Domain: "example.com",
		AdminGroup: "Axiom Admins",
	}

	role, ok, err := LDAPAuthenticate(cfg, "alice", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected a successful bind to authenticate")
	}
	if role != "operator" {
		t.Fatalf("expected the operator default for a non-DN bind form, got %q", role)
	}
}

func TestLdapRoleFromGroups(t *testing.T) {
	tests := []struct {
		name                    string
		groups                  []string
		adminGroup, viewerGroup string
		want                    string
	}{
		{"no groups configured matches nothing", []string{"cn=Anything,dc=example,dc=com"}, "", "", "operator"},
		{"admin by CN", []string{"cn=Axiom Admins,ou=groups,dc=example,dc=com"}, "Axiom Admins", "Axiom Viewers", "admin"},
		{"admin by full DN", []string{"cn=Axiom Admins,ou=groups,dc=example,dc=com"}, "cn=Axiom Admins,ou=groups,dc=example,dc=com", "", "admin"},
		{"viewer when admin doesn't match", []string{"cn=Axiom Viewers,ou=groups,dc=example,dc=com"}, "Axiom Admins", "Axiom Viewers", "viewer"},
		{"admin takes priority when a user is in both groups", []string{"cn=Axiom Admins,dc=example,dc=com", "cn=Axiom Viewers,dc=example,dc=com"}, "Axiom Admins", "Axiom Viewers", "admin"},
		{"no match falls back to default", []string{"cn=Everyone,dc=example,dc=com"}, "Axiom Admins", "Axiom Viewers", "operator"},
		{"case-insensitive CN match", []string{"CN=axiom admins,dc=example,dc=com"}, "Axiom Admins", "", "admin"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ldapRoleFromGroups(tc.groups, tc.adminGroup, tc.viewerGroup, "operator")
			if got != tc.want {
				t.Errorf("ldapRoleFromGroups(%v, %q, %q) = %q, want %q", tc.groups, tc.adminGroup, tc.viewerGroup, got, tc.want)
			}
		})
	}
}

func TestLDAPAuthenticateInvalidCredentials(t *testing.T) {
	addr := fakeLDAPServer(t, [][]byte{canonicalBindResponse(49)}) // invalidCredentials
	cfg := LDAPConfig{Enabled: true, URL: "ldap://" + addr, BaseDN: "dc=example,dc=com"}

	_, ok, err := LDAPAuthenticate(cfg, "alice", "wrongpass")
	if err != nil {
		t.Fatalf("expected no error (a rejected bind is reported via ok=false, not err), got %v", err)
	}
	if ok {
		t.Fatal("expected a rejected bind to fail authentication")
	}
}

func TestLDAPAuthenticateFallsBackToUPNForm(t *testing.T) {
	// With cfg.Domain set and no "@" in the username, ldapBindDN first tries
	// the NetBIOS form "example.com\alice" (rejected), then LDAPAuthenticate
	// retries with the UPN form "alice@example.com" (accepted) on the same
	// connection.
	addr := fakeLDAPServer(t, [][]byte{canonicalBindResponse(49), canonicalBindResponse(0)})
	cfg := LDAPConfig{Enabled: true, URL: "ldap://" + addr, Domain: "example.com"}

	_, ok, err := LDAPAuthenticate(cfg, "alice", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected the UPN-form retry to succeed")
	}
}

func TestLDAPAuthenticateUPNFallbackAlsoFails(t *testing.T) {
	addr := fakeLDAPServer(t, [][]byte{canonicalBindResponse(49), canonicalBindResponse(49)})
	cfg := LDAPConfig{Enabled: true, URL: "ldap://" + addr, Domain: "example.com"}

	_, ok, err := LDAPAuthenticate(cfg, "alice", "wrongpass")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ok {
		t.Fatal("expected both the primary and UPN-fallback binds to fail")
	}
}

func TestLDAPAuthenticateDialError(t *testing.T) {
	// Bind and immediately close a listener to get a port nothing is
	// listening on, so the dial fails.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	cfg := LDAPConfig{Enabled: true, URL: "ldap://" + addr}
	_, ok, err := LDAPAuthenticate(cfg, "alice", "secret")
	if ok {
		t.Fatal("expected failure when the LDAP server is unreachable")
	}
	if err == nil {
		t.Fatal("expected a dial error to be returned")
	}
}

func TestLDAPAuthenticateRequiresURL(t *testing.T) {
	_, ok, err := LDAPAuthenticate(LDAPConfig{Enabled: true}, "alice", "secret")
	if ok || err == nil {
		t.Fatalf("expected an error when no LDAP URL is configured, got ok=%v err=%v", ok, err)
	}
}

func TestLDAPAuthenticateRejectsEmptyCredentials(t *testing.T) {
	// cfg deliberately points at a nonexistent server: the empty-credential
	// check must short-circuit before any network I/O is attempted.
	cfg := LDAPConfig{Enabled: true, URL: "ldap://127.0.0.1:1"}

	if _, ok, err := LDAPAuthenticate(cfg, "", "secret"); ok || err != nil {
		t.Fatalf("empty username: got ok=%v err=%v, want ok=false err=nil", ok, err)
	}
	if _, ok, err := LDAPAuthenticate(cfg, "alice", ""); ok || err != nil {
		t.Fatalf("empty password: got ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}

// TestGuardAuthenticatesViaLDAPWhenLocalCredentialsDontMatch exercises the
// public contract described in basic.go: Guard.authorizeDetail falls
// through to LDAPAuthenticate after the env admin account and API key
// checks miss, and grants the "operator" role on a successful bind.
func TestGuardAuthenticatesViaLDAPWhenLocalCredentialsDontMatch(t *testing.T) {
	addr := fakeLDAPServer(t, [][]byte{canonicalBindResponse(0)})
	g := Guard{
		Username: "demo",
		Password: "demo",
		LDAP:     LDAPConfig{Enabled: true, URL: "ldap://" + addr, BaseDN: "dc=example,dc=com"},
	}
	var gotIdentity Identity
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdentity, _ = IdentityFromContext(r.Context())
		w.WriteHeader(204)
	}))

	r := httptest.NewRequest("GET", "/api/v1/overview", nil)
	r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("alice:secret")))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != 204 {
		t.Fatalf("expected the LDAP-authenticated request to reach the handler, got %d", w.Code)
	}
	if gotIdentity.User != "alice" || gotIdentity.Role != "operator" {
		t.Fatalf("expected identity alice/operator, got %+v", gotIdentity)
	}
}

// dialLDAP over plain TCP is exercised indirectly by every fake-server test
// above (LDAPAuthenticate calls it internally). The TLS branch
// (tls.DialWithDialer against ldaps:// or ?tls=true) is not covered here:
// standing up a TLS listener with a certificate the client would accept
// adds real complexity for a branch that's a thin, well-tested stdlib call
// with no bespoke logic of its own — see the plain-TCP coverage above for
// the part of dialLDAP (and all of ldapSimpleBind/parseLDAPBindResult) that
// is actually project-specific.
