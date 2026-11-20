package api

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
	"github.com/russellhaering/goxmldsig/etreeutils"
)

// samlClockSkew tolerates small clock differences between the IdP and Axiom.
const samlClockSkew = 2 * time.Minute

// verifySAMLSignature verifies the enveloped XML-DSig signature on the
// SAMLResponse (or, failing that, on a direct child Assertion) against the
// pinned IdP certificate, and returns ONLY the signed XML.
//
// Returning the validated subtree (not the original bytes) matters: it defeats
// XML signature-wrapping, where an attacker keeps a genuinely signed element
// somewhere in the document and adds an unsigned Assertion that the parser
// would otherwise pick up first.
func verifySAMLSignature(xmlBytes []byte, pemCert string) ([]byte, error) {
	block, _ := pem.Decode([]byte(pemCert))
	if block == nil {
		return nil, errors.New("AXIOM_SAML_IDP_CERT is not a PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("AXIOM_SAML_IDP_CERT: %w", err)
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(xmlBytes); err != nil {
		return nil, fmt.Errorf("malformed SAMLResponse XML: %w", err)
	}
	root := doc.Root()
	if root == nil {
		return nil, errors.New("empty SAMLResponse")
	}

	var signed *etree.Element
	if hasDirectSignature(root) {
		signed = root
	} else {
		for _, c := range root.ChildElements() {
			if c.Tag == "Assertion" && hasDirectSignature(c) {
				signed = c
				break
			}
		}
	}
	if signed == nil {
		return nil, errors.New("SAMLResponse is not signed")
	}

	// A signed Assertion usually inherits namespace declarations (saml:, ds:)
	// from the Response root; canonicalization needs them on the element itself.
	nsctx, err := etreeutils.NSBuildParentContext(signed)
	if err != nil {
		return nil, fmt.Errorf("namespace context: %w", err)
	}
	signed, err = etreeutils.NSDetatch(nsctx, signed)
	if err != nil {
		return nil, fmt.Errorf("namespace detach: %w", err)
	}

	vctx := dsig.NewDefaultValidationContext(&dsig.MemoryX509CertificateStore{Roots: []*x509.Certificate{cert}})
	validated, err := vctx.Validate(signed)
	if err != nil {
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}
	out := etree.NewDocument()
	out.SetRoot(validated.Copy())
	return out.WriteToBytes()
}

func hasDirectSignature(e *etree.Element) bool {
	for _, c := range e.ChildElements() {
		if c.Tag == "Signature" {
			return true
		}
	}
	return false
}

// checkSAMLConditions enforces the assertion's validity window and audience on
// already-signature-verified XML. A missing NotOnOrAfter is rejected (it would
// make the assertion replayable forever); a configured entity ID must appear
// as an Audience.
func checkSAMLConditions(xmlBytes []byte, audience string, now time.Time) error {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(xmlBytes); err != nil {
		return err
	}
	var conds []*etree.Element
	var walk func(e *etree.Element)
	walk = func(e *etree.Element) {
		if e.Tag == "Conditions" {
			conds = append(conds, e)
		}
		for _, c := range e.ChildElements() {
			walk(c)
		}
	}
	walk(doc.Root())
	if len(conds) == 0 {
		return errors.New("assertion has no Conditions (validity window required)")
	}
	for _, c := range conds {
		nooa := c.SelectAttrValue("NotOnOrAfter", "")
		if nooa == "" {
			return errors.New("assertion has no NotOnOrAfter")
		}
		end, err := time.Parse(time.RFC3339, nooa)
		if err != nil {
			return fmt.Errorf("bad NotOnOrAfter: %w", err)
		}
		if !now.Add(-samlClockSkew).Before(end) {
			return errors.New("assertion has expired")
		}
		if nb := c.SelectAttrValue("NotBefore", ""); nb != "" {
			start, err := time.Parse(time.RFC3339, nb)
			if err != nil {
				return fmt.Errorf("bad NotBefore: %w", err)
			}
			if now.Add(samlClockSkew).Before(start) {
				return errors.New("assertion is not valid yet")
			}
		}
		if audience != "" {
			matched := false
			for _, ar := range c.ChildElements() {
				if ar.Tag != "AudienceRestriction" {
					continue
				}
				for _, a := range ar.ChildElements() {
					if a.Tag == "Audience" && strings.TrimSpace(a.Text()) == audience {
						matched = true
					}
				}
			}
			if !matched {
				return errors.New("assertion audience does not match this service provider")
			}
		}
	}
	return nil
}

const samlBearerMethod = "urn:oasis:names:tc:SAML:2.0:cm:bearer"

// samlElements returns every element under (and including) root with the
// given local name.
func samlElements(root *etree.Element, tag string) []*etree.Element {
	var out []*etree.Element
	var walk func(e *etree.Element)
	walk = func(e *etree.Element) {
		if e.Tag == tag {
			out = append(out, e)
		}
		for _, c := range e.ChildElements() {
			walk(c)
		}
	}
	if root != nil {
		walk(root)
	}
	return out
}

// samlInResponseTo returns the InResponseTo carried by trusted XML (the
// signature-verified content, or the whole document in unsigned lab mode):
// the Response's own attribute when the Response itself is the trusted root,
// plus every SubjectConfirmationData InResponseTo. All values present must
// agree; "" means the response is unsolicited (IdP-initiated).
func samlInResponseTo(xmlBytes []byte) (string, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(xmlBytes); err != nil {
		return "", err
	}
	root := doc.Root()
	if root == nil {
		return "", errors.New("empty SAML document")
	}
	var vals []string
	if root.Tag == "Response" {
		if v := strings.TrimSpace(root.SelectAttrValue("InResponseTo", "")); v != "" {
			vals = append(vals, v)
		}
	}
	for _, scd := range samlElements(root, "SubjectConfirmationData") {
		if v := strings.TrimSpace(scd.SelectAttrValue("InResponseTo", "")); v != "" {
			vals = append(vals, v)
		}
	}
	irt := ""
	for _, v := range vals {
		if irt == "" {
			irt = v
		} else if v != irt {
			return "", errors.New("conflicting InResponseTo values in SAML response")
		}
	}
	return irt, nil
}

// samlOuterInResponseTo reads the InResponseTo attribute of the document's
// root element without trusting it (it lies outside the signed Assertion when
// only the Assertion is signed); it is used only as a consistency check.
func samlOuterInResponseTo(xmlBytes []byte) string {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(xmlBytes); err != nil || doc.Root() == nil {
		return ""
	}
	return strings.TrimSpace(doc.Root().SelectAttrValue("InResponseTo", ""))
}

// checkSAMLSubjectConfirmation enforces the Web Browser SSO profile rules
// (SAML Profiles §4.1.4.3) on trusted XML: every SubjectConfirmation must use
// the bearer method, and a SubjectConfirmationData, when present, must name
// this ACS as Recipient and must not have expired (NotOnOrAfter, with the same
// clock skew as checkSAMLConditions).
func checkSAMLSubjectConfirmation(xmlBytes []byte, acsURL string, now time.Time) error {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(xmlBytes); err != nil {
		return err
	}
	for _, sc := range samlElements(doc.Root(), "SubjectConfirmation") {
		if m := strings.TrimSpace(sc.SelectAttrValue("Method", "")); m != samlBearerMethod {
			return fmt.Errorf("unsupported SubjectConfirmation Method %q (only bearer is accepted)", m)
		}
		for _, scd := range sc.ChildElements() {
			if scd.Tag != "SubjectConfirmationData" {
				continue
			}
			if rcpt := strings.TrimSpace(scd.SelectAttrValue("Recipient", "")); rcpt != "" && rcpt != acsURL {
				return fmt.Errorf("SubjectConfirmationData Recipient %q does not match this ACS (%s)", rcpt, acsURL)
			}
			if nooa := scd.SelectAttrValue("NotOnOrAfter", ""); nooa != "" {
				end, err := time.Parse(time.RFC3339, nooa)
				if err != nil {
					return fmt.Errorf("bad SubjectConfirmationData NotOnOrAfter: %w", err)
				}
				if !now.Add(-samlClockSkew).Before(end) {
					return errors.New("SubjectConfirmationData has expired")
				}
			}
			if nb := scd.SelectAttrValue("NotBefore", ""); nb != "" {
				start, err := time.Parse(time.RFC3339, nb)
				if err != nil {
					return fmt.Errorf("bad SubjectConfirmationData NotBefore: %w", err)
				}
				if now.Add(samlClockSkew).Before(start) {
					return errors.New("SubjectConfirmationData is not valid yet")
				}
			}
		}
	}
	return nil
}
