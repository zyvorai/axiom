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
}

// LDAPAuthenticate performs a simple bind as the user. Returns role (always
// "admin" unless mapped later — callers may override), ok.
func LDAPAuthenticate(cfg LDAPConfig, username, password string) (ok bool, err error) {
	if strings.TrimSpace(username) == "" || password == "" {
		return false, nil
	}
	if cfg.URL == "" {
		return false, fmt.Errorf("LDAP URL not configured")
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return false, err
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
		return false, err
	}
	defer conn.Close()

	dn := ldapBindDN(cfg, username)
	if err := ldapSimpleBind(conn, dn, password); err != nil {
		// Try UPN form user@domain
		if cfg.Domain != "" && !strings.Contains(username, "@") {
			alt := username + "@" + strings.TrimPrefix(cfg.Domain, "@")
			if err2 := ldapSimpleBind(conn, alt, password); err2 == nil {
				return true, nil
			}
		}
		return false, nil
	}
	return true, nil
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
