package auth

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// LDAPConfig holds directory bind settings. When Enabled (or URL is set),
// Guard tries an LDAP simple-bind before falling through to local users.
type LDAPConfig struct {
	Enabled bool
	URL     string // ldap:// or ldaps:// host[:port]
	Domain  string // AD-style DOMAIN or email domain for UPN bind
	BaseDN  string // optional search base; when set with BindDN, look up DN by uid/sAMAccountName
	BindDN  string // optional service account for user search
	BindPW  string
	UserAttr string // default sAMAccountName; also tries uid / userPrincipalName

	// GroupAttr/AdminGroup/ViewerGroup map the bound user's directory group
	// membership to a role. GroupAttr defaults to "memberOf" when AdminGroup
	// or ViewerGroup is set. Both unset (the default) keeps every successful
	// bind at the long-standing "operator" role.
	GroupAttr   string
	AdminGroup  string
	ViewerGroup string
}

// LDAPAuthenticate performs a simple bind as the user. Returns the mapped
// role (AdminGroup/ViewerGroup membership via a follow-up search, else the
// "operator" default) and ok.
func LDAPAuthenticate(cfg LDAPConfig, username, password string) (role string, ok bool, err error) {
	if strings.TrimSpace(username) == "" || password == "" {
		return "", false, nil
	}
	if cfg.URL == "" {
		return "", false, fmt.Errorf("LDAP URL not configured")
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return "", false, err
	}
	host := u.Host
	if host == "" {
		host = u.Path
	}
	if !strings.Contains(host, ":") {
		if u.Scheme == "ldaps" {
			host += ":636"
		} else {
			host += ":389"
		}
	}
	useTLS := u.Scheme == "ldaps" || u.Query().Get("tls") == "true"

	conn, err := dialLDAP(host, useTLS)
	if err != nil {
		return "", false, err
	}
	defer conn.Close()

	dn := ldapBindDN(cfg, username)
	if err := ldapSimpleBind(conn, dn, password); err != nil {
		// Try UPN form user@domain
		if cfg.Domain != "" && !strings.Contains(username, "@") {
			alt := username + "@" + strings.TrimPrefix(cfg.Domain, "@")
			if err2 := ldapSimpleBind(conn, alt, password); err2 == nil {
				return ldapRoleAfterBind(cfg, conn, alt), true, nil
			}
		}
		return "", false, nil
	}
	return ldapRoleAfterBind(cfg, conn, dn), true, nil
}

// ldapRoleAfterBind maps the just-authenticated connection's bound DN to a
// role via AdminGroup/ViewerGroup, when configured. Group lookup is a
// base-scope search on the bound entry's own DN, so it only works for the
// BaseDN-constructed bind form ("uid=alice,ou=...") — a bare UPN or NetBIOS
// bind identity (the AD-style Domain branch of ldapBindDN) isn't a real DN to
// search on directly, so that case falls back to the default role. Any
// search/parse failure degrades to the default role rather than failing the
// login — group mapping is best-effort, authentication already succeeded.
func ldapRoleAfterBind(cfg LDAPConfig, conn net.Conn, boundDN string) string {
	const defaultRole = "operator"
	if cfg.AdminGroup == "" && cfg.ViewerGroup == "" {
		return defaultRole
	}
	if !strings.Contains(boundDN, "=") {
		return defaultRole
	}
	attr := cfg.GroupAttr
	if attr == "" {
		attr = "memberOf"
	}
	groups := ldapSearchGroups(conn, boundDN, attr)
	return ldapRoleFromGroups(groups, cfg.AdminGroup, cfg.ViewerGroup, defaultRole)
}

// ldapRoleFromGroups matches each group value (a full DN, e.g.
// "cn=Axiom Admins,ou=groups,dc=example,dc=com") against the configured
// AdminGroup/ViewerGroup, either as an exact case-insensitive match or by its
// "cn" RDN component, so operators can configure either a full DN or just the
// group's common name. Admin is checked before viewer, so a user in both
// groups gets the more privileged role.
func ldapRoleFromGroups(groups []string, adminGroup, viewerGroup, defaultRole string) string {
	matches := func(group, want string) bool {
		if want == "" {
			return false
		}
		if strings.EqualFold(group, want) {
			return true
		}
		for _, part := range strings.Split(group, ",") {
			part = strings.TrimSpace(part)
			if kv := strings.SplitN(part, "=", 2); len(kv) == 2 && strings.EqualFold(kv[0], "cn") && strings.EqualFold(kv[1], want) {
				return true
			}
		}
		return false
	}
	for _, g := range groups {
		if matches(g, adminGroup) {
			return "admin"
		}
	}
	for _, g := range groups {
		if matches(g, viewerGroup) {
			return "viewer"
		}
	}
	return defaultRole
}

// ldapEscapeDN escapes RFC 4514 special characters in a DN attribute value.
// username here is attacker-controlled (submitted at the anonymous login
// form), so it must never be embedded in a constructed DN unescaped — an
// unescaped "," or "=" lets it inject additional RDN components and bind as
// an arbitrary DN elsewhere in the directory, defeating BaseDN scoping.
func ldapEscapeDN(v string) string {
	var b strings.Builder
	for i, r := range v {
		switch r {
		case ',', '+', '"', '\\', '<', '>', ';', '=':
			b.WriteByte('\\')
			b.WriteRune(r)
		case ' ':
			if i == 0 || i == len(v)-1 {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		case '#':
			if i == 0 {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func ldapBindDN(cfg LDAPConfig, username string) string {
	// Deliberately no "username already looks like a DN, pass it through"
	// branch here: username comes straight from the anonymous login form, so
	// trusting it as pre-formed DN syntax would let anyone bind as an
	// arbitrary DN anywhere in the directory instead of the configured scope.
	if strings.Contains(username, "@") {
		return username // UPN — a standard AD bind form, not raw DN syntax
	}
	if cfg.Domain != "" && !strings.Contains(cfg.Domain, "=") {
		// NetBIOS DOMAIN\user
		return cfg.Domain + `\` + username
	}
	attr := cfg.UserAttr
	if attr == "" {
		attr = "uid"
	}
	if cfg.BaseDN != "" {
		return fmt.Sprintf("%s=%s,%s", attr, ldapEscapeDN(username), cfg.BaseDN)
	}
	return username
}

func dialLDAP(host string, useTLS bool) (net.Conn, error) {
	d := net.Dialer{Timeout: 8 * time.Second}
	if useTLS {
		return tls.DialWithDialer(&d, "tcp", host, &tls.Config{MinVersion: tls.VersionTLS12})
	}
	return d.Dial("tcp", host)
}

// Minimal LDAP simple-bind (RFC 4511) without an external LDAP library.
func ldapSimpleBind(conn net.Conn, dn, password string) error {
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	msg := encodeLDAPBindRequest(1, dn, password)
	if _, err := conn.Write(msg); err != nil {
		return err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	code, err := parseLDAPBindResult(buf[:n])
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("ldap bind result code %d", code)
	}
	return nil
}

func encodeLDAPBindRequest(messageID int, dn, password string) []byte {
	bindApp := encodeRaw(0x60, encodeInteger(3), encodeOctetString([]byte(dn)), encodeContext(0, []byte(password)))
	return encodeSequence(encodeInteger(messageID), bindApp)
}

// ldapSearchGroups issues a base-scope Search for a single attribute
// (typically "memberOf") on dn, the just-authenticated user's own entry, and
// returns its values. Best-effort: any protocol error or malformed response
// returns whatever was parsed so far rather than propagating an error — role
// mapping degrades to the caller's default role, it never fails the login
// that already succeeded.
func ldapSearchGroups(conn net.Conn, dn, attr string) []string {
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	req := encodeLDAPSearchRequest(2, dn, attr)
	if _, err := conn.Write(req); err != nil {
		return nil
	}
	// A server answers one SearchRequest with zero or more SearchResultEntry
	// messages followed by exactly one SearchResultDone — all of it pushed
	// without any further write from us, and possibly split or coalesced
	// arbitrarily across TCP reads. So this buffers reads and drains however
	// many complete messages each one contains, rather than assuming one
	// message per Read call.
	var groups []string
	var pending []byte
	buf := make([]byte, 8192)
	for reads := 0; reads < 20; reads++ {
		for len(pending) > 0 {
			_, opTag, opContent, consumed, ok := parseLDAPMessage(pending)
			if !ok {
				return groups
			}
			switch opTag {
			case 0x64: // SearchResultEntry
				groups = append(groups, parseSearchResultEntryAttr(opContent, attr)...)
			case 0x65: // SearchResultDone
				return groups
			default:
				return groups
			}
			pending = pending[consumed:]
		}
		n, err := conn.Read(buf)
		if err != nil || n == 0 {
			return groups
		}
		pending = append(pending, buf[:n]...)
	}
	return groups
}

// SearchRequest ::= [APPLICATION 3] SEQUENCE { baseObject, scope, derefAliases,
// sizeLimit, timeLimit, typesOnly, filter, attributes } (RFC 4511 §4.5.1).
// Scope is always baseObject(0) and the filter is always the "present"
// filter (objectClass=*) — this only ever reads attr back off dn itself.
func encodeLDAPSearchRequest(messageID int, dn, attr string) []byte {
	filter := encodeContext(7, []byte("objectClass")) // [7] present
	attrs := encodeSequence(encodeOctetString([]byte(attr)))
	searchApp := encodeRaw(0x63,
		encodeOctetString([]byte(dn)),
		encodeEnumerated(0), // scope: baseObject
		encodeEnumerated(0), // derefAliases: never
		encodeInteger(0),    // sizeLimit: no limit
		encodeInteger(0),    // timeLimit: no limit
		encodeBoolean(false),
		filter,
		attrs,
	)
	return encodeSequence(encodeInteger(messageID), searchApp)
}

func encodeEnumerated(n int) []byte {
	b := encodeInteger(n)
	b[0] = 0x0a
	return b
}

func encodeBoolean(v bool) []byte {
	if v {
		return []byte{0x01, 0x01, 0xff}
	}
	return []byte{0x01, 0x01, 0x00}
}

func encodeRaw(tag byte, parts ...[]byte) []byte {
	var inner []byte
	for _, p := range parts {
		inner = append(inner, p...)
	}
	return append([]byte{tag}, append(encodeLength(len(inner)), inner...)...)
}

func encodeSequence(parts ...[]byte) []byte {
	var inner []byte
	for _, p := range parts {
		inner = append(inner, p...)
	}
	return append([]byte{0x30}, append(encodeLength(len(inner)), inner...)...)
}

func encodeInteger(n int) []byte {
	if n == 0 {
		return []byte{0x02, 0x01, 0x00}
	}
	var b []byte
	v := n
	for v > 0 {
		b = append([]byte{byte(v & 0xff)}, b...)
		v >>= 8
	}
	if b[0]&0x80 != 0 {
		b = append([]byte{0x00}, b...)
	}
	return append([]byte{0x02}, append(encodeLength(len(b)), b...)...)
}

func encodeOctetString(b []byte) []byte {
	return append([]byte{0x04}, append(encodeLength(len(b)), b...)...)
}

func encodeContext(num int, b []byte) []byte {
	return append([]byte{byte(0x80 | num)}, append(encodeLength(len(b)), b...)...)
}

func encodeLength(n int) []byte {
	if n < 128 {
		return []byte{byte(n)}
	}
	if n < 256 {
		return []byte{0x81, byte(n)}
	}
	return []byte{0x82, byte(n >> 8), byte(n)}
}

func parseLDAPBindResult(b []byte) (int, error) {
	// Walk ASN.1 enough to find resultCode INTEGER inside BindResponse (0x61).
	for i := 0; i+2 < len(b); i++ {
		if b[i] == 0x61 || (b[i] == 0x30) {
			// search for INTEGER resultCode after messageID
		}
	}
	// Naive scan: find APPLICATION 1 (BindResponse 0x61) then first INTEGER after version-ish.
	idx := -1
	for i := 0; i < len(b); i++ {
		if b[i] == 0x61 {
			idx = i
			break
		}
	}
	if idx < 0 {
		// Some servers wrap differently — look for resultCode 0x0a ENUMERATED
		for i := 0; i+2 < len(b); i++ {
			if b[i] == 0x0a && b[i+1] == 0x01 {
				return int(b[i+2]), nil
			}
		}
		return -1, fmt.Errorf("ldap: no bind response")
	}
	for i := idx; i+2 < len(b); i++ {
		if b[i] == 0x0a && b[i+1] == 0x01 { // ENUMERATED resultCode
			return int(b[i+2]), nil
		}
		if b[i] == 0x02 && b[i+1] == 0x01 && i > idx+4 { // INTEGER after messageID
			// first INTEGER inside BindResponse is often resultCode in older encodings
			return int(b[i+2]), nil
		}
	}
	return -1, fmt.Errorf("ldap: result code not found")
}

// --- Minimal generic BER TLV walk, used only by the Search response parsing
// below. The bind-result parser above predates this and stays a byte scan;
// Search responses have nested SEQUENCEs (attribute list, per-attribute
// values) that a byte scan can't reliably delimit, so this walks real
// tag/length/value boundaries instead.

type berNode struct {
	tag   byte
	value []byte
}

// decodeBERLength reads a BER length at buf[pos] (the byte right after the
// tag). Returns the decoded length and how many bytes the length itself took.
func decodeBERLength(buf []byte, pos int) (length, consumed int, ok bool) {
	if pos >= len(buf) {
		return 0, 0, false
	}
	b := buf[pos]
	if b&0x80 == 0 {
		return int(b), 1, true
	}
	n := int(b & 0x7f)
	if n == 0 || n > 4 || pos+1+n > len(buf) {
		return 0, 0, false
	}
	for i := 0; i < n; i++ {
		length = length<<8 | int(buf[pos+1+i])
	}
	return length, 1 + n, true
}

// berChildren walks content as a flat sequence of top-level TLVs.
func berChildren(content []byte) []berNode {
	var out []berNode
	i := 0
	for i < len(content) {
		tag := content[i]
		length, consumed, ok := decodeBERLength(content, i+1)
		if !ok {
			break
		}
		start := i + 1 + consumed
		end := start + length
		if end > len(content) {
			break
		}
		out = append(out, berNode{tag: tag, value: content[start:end]})
		i = end
	}
	return out
}

// parseLDAPMessage unwraps the outer LDAPMessage SEQUENCE { messageID
// INTEGER, protocolOp [APPLICATION n] ... } and returns the protocolOp's tag
// and raw content.
// parseLDAPMessage parses exactly one LDAPMessage from the start of b and
// reports consumed, how many leading bytes it occupied — b commonly holds
// more than one message back to back (e.g. a SearchResultEntry immediately
// followed by SearchResultDone), so callers loop over b[consumed:] rather
// than assuming b holds a single message.
func parseLDAPMessage(b []byte) (messageID int, opTag byte, opContent []byte, consumed int, ok bool) {
	if len(b) == 0 || b[0] != 0x30 {
		return 0, 0, nil, 0, false
	}
	length, lenBytes, lok := decodeBERLength(b, 1)
	if !lok || 1+lenBytes+length > len(b) {
		return 0, 0, nil, 0, false
	}
	total := 1 + lenBytes + length
	children := berChildren(b[1+lenBytes : total])
	if len(children) < 2 {
		return 0, 0, nil, 0, false
	}
	for _, by := range children[0].value {
		messageID = messageID<<8 | int(by)
	}
	return messageID, children[1].tag, children[1].value, total, true
}

// parseSearchResultEntryAttr reads a SearchResultEntry's protocolOp content
// — { objectName LDAPDN, attributes SEQUENCE OF PartialAttribute } — and
// returns the values of the named attribute (case-insensitive match).
func parseSearchResultEntryAttr(opContent []byte, attrName string) []string {
	children := berChildren(opContent)
	if len(children) < 2 {
		return nil
	}
	var vals []string
	for _, pa := range berChildren(children[1].value) {
		paChildren := berChildren(pa.value)
		if len(paChildren) < 2 {
			continue
		}
		if !strings.EqualFold(string(paChildren[0].value), attrName) {
			continue
		}
		for _, v := range berChildren(paChildren[1].value) {
			vals = append(vals, string(v.value))
		}
	}
	return vals
}
