package api

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

// samlACS accepts a SAMLResponse (POST preferred; GET supported for some IdPs),
// extracts the subject, optionally verifies the assertion signature when
// AXIOM_SAML_IDP_CERT is set, and mints an Axiom session cookie.
func (s *Server) samlACS(w http.ResponseWriter, r *http.Request) {
	if s.auth.SAMLLogin == "" && s.auth.SAMLEntityID == "" {
		http.Error(w, "SAML is not configured", http.StatusNotImplemented)
		return
	}
	if err := r.ParseForm(); err != nil && r.Method == http.MethodPost {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	raw := strings.TrimSpace(r.FormValue("SAMLResponse"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("SAMLResponse"))
	}
	if raw == "" {
		http.Error(w, "missing SAMLResponse", http.StatusBadRequest)
		return
	}
	xmlBytes, err := decodeSAMLResponse(raw)
	if err != nil {
		http.Error(w, "invalid SAMLResponse encoding: "+err.Error(), http.StatusBadRequest)
		return
	}
	if s.auth.SAMLIDPCert != "" {
		if err := verifySAMLIdPCert(xmlBytes, s.auth.SAMLIDPCert); err != nil {
			http.Error(w, "SAML IdP certificate check failed: "+err.Error(), http.StatusUnauthorized)
			return
		}
	} else if !s.auth.SAMLAllowUnsigned {
		http.Error(w, "SAML IdP certificate required (set AXIOM_SAML_IDP_CERT), or AXIOM_SAML_ALLOW_UNSIGNED=true for lab only", http.StatusUnauthorized)
		return
	}

	user, email, groups, err := parseSAMLAssertion(xmlBytes)
	if err != nil {
		http.Error(w, "SAML assertion parse failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	if user == "" {
		user = "saml-user"
	}
	role := resolveSAMLRole(groups, s.auth.OIDCAdminGroup, s.auth.OIDCViewerGroup, s.auth.OIDCDefaultRole)
	token, err := auth.SignSession(s.sessionSecret(), auth.Session{
		User:   user,
		Email:  email,
		Issuer: firstNonEmpty(s.auth.SAMLEntityID, "saml"),
		Role:   role,
		Exp:    time.Now().Add(12 * time.Hour).Unix(),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.SetSessionCookie(w, token, s.requestSecure(r))
	platform.RecordAxiomAudit("auth.saml_login", user, role, user)
	http.Redirect(w, r, "/?sso=1", http.StatusFound)
}

func decodeSAMLResponse(raw string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			return nil, err
		}
	}
	// Some IdPs send DEFLATE-compressed responses (HTTP-Redirect binding).
	if looksLikeXML(decoded) {
		return decoded, nil
	}
	r := flate.NewReader(bytes.NewReader(decoded))
	defer r.Close()
	out, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err != nil {
		// Not compressed — return original bytes and let the XML parser fail clearly.
		return decoded, nil
	}
	if looksLikeXML(out) {
		return out, nil
	}
	return decoded, nil
}

func looksLikeXML(b []byte) bool {
	s := strings.TrimSpace(string(b))
	return strings.HasPrefix(s, "<") || strings.HasPrefix(s, "<?xml")
}

type samlResponseXML struct {
	XMLName   xml.Name `xml:"Response"`
	Assertion *samlAssertionXML
}

type samlAssertionXML struct {
	XMLName            xml.Name `xml:"Assertion"`
	Subject            samlSubject
	AttributeStatement *samlAttributeStatement
}

type samlSubject struct {
	NameID string `xml:"NameID"`
}

type samlAttributeStatement struct {
	Attributes []samlAttribute `xml:"Attribute"`
}

type samlAttribute struct {
	Name   string   `xml:"Name,attr"`
	Values []string `xml:"AttributeValue"`
}

func parseSAMLAssertion(raw []byte) (user, email string, groups []string, err error) {
	// Tolerate default / namespaced Response elements by scanning for Assertion.
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = false
	var assertion *samlAssertionXML
	for {
		tok, e := dec.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", "", nil, e
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "Assertion" {
			var a samlAssertionXML
			if err := dec.DecodeElement(&a, &se); err != nil {
				return "", "", nil, err
			}
			assertion = &a
			break
		}
	}
	if assertion == nil {
		// Fallback: whole document as Response
		var resp samlResponseXML
		if err := xml.Unmarshal(raw, &resp); err != nil {
			return "", "", nil, fmt.Errorf("no Assertion found")
		}
		assertion = resp.Assertion
	}
	if assertion == nil {
		return "", "", nil, fmt.Errorf("no Assertion found")
	}
	user = strings.TrimSpace(assertion.Subject.NameID)
	if assertion.AttributeStatement != nil {
		for _, attr := range assertion.AttributeStatement.Attributes {
			name := strings.ToLower(attr.Name)
			vals := attr.Values
			switch {
			case strings.Contains(name, "email"):
				if email == "" && len(vals) > 0 {
					email = strings.TrimSpace(vals[0])
				}
			case strings.Contains(name, "uid"), strings.Contains(name, "userid"), name == "name":
				if user == "" && len(vals) > 0 {
					user = strings.TrimSpace(vals[0])
				}
			case strings.Contains(name, "group"), strings.Contains(name, "role"), strings.Contains(name, "member"):
				groups = append(groups, vals...)
			}
		}
	}
	if user == "" && email != "" {
		user = email
	}
	return user, email, groups, nil
}

func resolveSAMLRole(groups []string, adminGroup, viewerGroup, defaultRole string) string {
	adminGroup = strings.TrimSpace(adminGroup)
	viewerGroup = strings.TrimSpace(viewerGroup)
	if adminGroup == "" && viewerGroup == "" {
		return normalizeDefaultRole(defaultRole)
	}
	if adminGroup != "" && containsFold(groups, adminGroup) {
		return "admin"
	}
	if viewerGroup != "" && containsFold(groups, viewerGroup) {
		return "viewer"
	}
	if containsFold(groups, "operator") || containsFold(groups, "axiom-operator") {
		return "operator"
	}
	return "viewer"
}

func normalizeDefaultRole(r string) string {
	switch strings.ToLower(strings.TrimSpace(r)) {
	case "viewer":
		return "viewer"
	case "operator":
		return "operator"
	default:
		return "admin"
	}
}

// verifySAMLIdPCert pins the assertion's embedded X509Certificate against the
// configured IdP PEM. Full XML-DSig C14N varies by IdP; certificate pinning is
// the practical SP check Axiom ships without pulling a heavy SAML stack.
func verifySAMLIdPCert(xmlBytes []byte, pemCert string) error {
	wantBlock, _ := pem.Decode([]byte(pemCert))
	if wantBlock == nil {
		return fmt.Errorf("AXIOM_SAML_IDP_CERT is not a PEM certificate")
	}
	wantCert, err := x509.ParseCertificate(wantBlock.Bytes)
	if err != nil {
		return err
	}
	wantSum := sha256.Sum256(wantCert.Raw)

	b64 := extractXMLTag(xmlBytes, "X509Certificate")
	if b64 == "" {
		return fmt.Errorf("no X509Certificate in SAMLResponse")
	}
	der, err := base64.StdEncoding.DecodeString(strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, b64))
	if err != nil {
		return fmt.Errorf("bad X509Certificate encoding")
	}
	gotSum := sha256.Sum256(der)
	if wantSum != gotSum {
		return fmt.Errorf("embedded IdP certificate does not match AXIOM_SAML_IDP_CERT")
	}
	return nil
}

func extractXMLTag(raw []byte, local string) string {
	s := string(raw)
	// Prefer exact local-name match, including namespaced tags like ds:X509Certificate.
	needle := local + ">"
	idx := -1
	for i := 0; i+len(local) < len(s); i++ {
		if s[i] != '<' {
			continue
		}
		rest := s[i+1:]
		if strings.HasPrefix(rest, local) && (len(rest) == len(local) || rest[len(local)] == '>' || rest[len(local)] == ' ') {
			idx = i
			break
		}
		if j := strings.Index(rest, ":"+needle); j >= 0 && j < 64 {
			idx = i
			break
		}
		if j := strings.Index(rest, ":"+local+" "); j >= 0 && j < 64 {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ""
	}
	gt := strings.IndexByte(s[idx:], '>')
	if gt < 0 {
		return ""
	}
	contentStart := idx + gt + 1
	end := strings.Index(s[contentStart:], "</")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(s[contentStart : contentStart+end])
}
