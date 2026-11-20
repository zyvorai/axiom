package platform

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/crypto/ssh"
)

// GitOps repository credentials: write-only Secrets for Argo CD and Flux.
//
// SECURITY: secret values (password, token, private key, known hosts) flow in
// one direction only — from the request body into the Secret's data. They are
// never returned, logged, audited or put into an error message. Listing reads
// only labels, annotations, and for Argo CD the non-secret "url" and "type"
// keys; it never enumerates arbitrary Secrets, only those carrying the Argo
// CD repository labels or Axiom's own credential labels. There is no update:
// rotating a credential is delete + recreate.
//
// This file is pure (no cluster access) and compiles in both build modes.

const (
	GitOpsCredentialPurposeLabel = "axiom.zyvor.dev/purpose"
	GitOpsCredentialPurposeValue = "gitops-credentials"
	ArgoSecretTypeLabel          = "argocd.argoproj.io/secret-type"

	gitOpsCredURLAnnotation  = "axiom.zyvor.dev/repo-url"
	gitOpsCredKindAnnotation = "axiom.zyvor.dev/repo-kind"
	gitOpsCredAuthAnnotation = "axiom.zyvor.dev/auth-type"

	gitOpsCredMaxField    = 16 << 10
	gitOpsCredMaxUsername = 256
	// GitOpsCredentialBodyLimit bounds the POST body (http.MaxBytesReader).
	GitOpsCredentialBodyLimit = 64 << 10

	GitOpsCredEngineArgoCD = "argocd"
	GitOpsCredEngineFlux   = "flux"
)

// GitOpsCredential is one listed credential. It carries no secret data.
type GitOpsCredential struct {
	Name           string `json:"name"`
	Engine         string `json:"engine"`
	Kind           string `json:"kind"` // git | helm | oci
	URL            string `json:"url"`
	AuthType       string `json:"authType"` // basic | token | ssh (or another Argo CD type)
	Namespace      string `json:"namespace"`
	ManagedByAxiom bool   `json:"managedByAxiom"`
	Age            string `json:"age"`
}

// GitOpsCredentialRequest is the body of POST /api/v1/gitops/credentials.
type GitOpsCredentialRequest struct {
	Engine        string `json:"engine"`
	Name          string `json:"name"`
	Namespace     string `json:"namespace,omitempty"`
	URL           string `json:"url"`
	AuthType      string `json:"authType"`
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	Token         string `json:"token,omitempty"`
	SSHPrivateKey string `json:"sshPrivateKey,omitempty"`
	KnownHosts    string `json:"knownHosts,omitempty"`
	Kind          string `json:"kind,omitempty"`
}

// String and GoString keep the secret fields out of any %v / %#v formatting.
func (r GitOpsCredentialRequest) String() string {
	return "GitOpsCredentialRequest{engine=" + r.Engine + " namespace=" + r.Namespace + " name=" + r.Name + " authType=" + r.AuthType + " <secrets redacted>}"
}
func (r GitOpsCredentialRequest) GoString() string { return r.String() }

// GitOpsCredentialResult is the 201 response. It never echoes secrets.
type GitOpsCredentialResult struct {
	OK        bool   `json:"ok"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Engine    string `json:"engine"`
}

// gitOpsCredSecret is the Secret to create.
type gitOpsCredSecret struct {
	Engine, Namespace, Name string
	Labels, Annotations     map[string]string
	Data                    map[string][]byte
}

func (s gitOpsCredSecret) String() string {
	return "gitOpsCredSecret{" + s.Namespace + "/" + s.Name + " <data redacted>}"
}
func (s gitOpsCredSecret) GoString() string { return s.String() }

// wipe zeroes the secret bytes once the Secret has been sent.
func (s gitOpsCredSecret) wipe() {
	for _, b := range s.Data {
		for i := range b {
			b[i] = 0
		}
	}
}

func gitOpsCredDefaultNamespace(engine string) string {
	if engine == GitOpsCredEngineFlux {
		return "flux-system"
	}
	return "argocd"
}

func checkGitOpsCredEngine(engine string) error {
	if engine != GitOpsCredEngineArgoCD && engine != GitOpsCredEngineFlux {
		return gitOpsBad("engine must be argocd or flux")
	}
	return nil
}

// gitOpsCredNamespace applies the engine default and validates ns.
func gitOpsCredNamespace(engine, ns string) (string, error) {
	if ns == "" {
		ns = gitOpsCredDefaultNamespace(engine)
	}
	if err := checkDNSLabel("namespace", ns, 63); err != nil {
		return "", err
	}
	return ns, nil
}

// checkGitOpsCredTarget validates the DELETE path values. Secrets created
// outside Axiom may have DNS-1123 subdomain names, so dots are allowed.
func checkGitOpsCredTarget(engine, ns, name string) error {
	if err := checkGitOpsCredEngine(engine); err != nil {
		return err
	}
	if ns == "" || len(ns) > 63 || !gitOpsDNSLabel.MatchString(ns) {
		return gitOpsBad("invalid namespace")
	}
	if name == "" || len(name) > 253 || !gitOpsObjectName.MatchString(name) {
		return gitOpsBad("invalid name")
	}
	return nil
}

// gitOpsCredScope is one (engine, namespace) pair to list.
type gitOpsCredScope struct{ Engine, Namespace string }

// gitOpsCredListScopes validates the listing query. An empty engine lists
// both engines in their default namespaces.
func gitOpsCredListScopes(engine, ns string) ([]gitOpsCredScope, error) {
	if engine == "" {
		if ns != "" {
			return nil, gitOpsBad("namespace requires engine=argocd or engine=flux")
		}
		return []gitOpsCredScope{{GitOpsCredEngineArgoCD, "argocd"}, {GitOpsCredEngineFlux, "flux-system"}}, nil
	}
	if err := checkGitOpsCredEngine(engine); err != nil {
		return nil, err
	}
	n, err := gitOpsCredNamespace(engine, ns)
	if err != nil {
		return nil, err
	}
	return []gitOpsCredScope{{engine, n}}, nil
}

// gitOpsCredSelectors are the label selectors listing uses: Argo CD's own
// repository labels, or Axiom's credential labels for Flux. Nothing else.
func gitOpsCredSelectors(engine string) []string {
	if engine == GitOpsCredEngineArgoCD {
		return []string{ArgoSecretTypeLabel + "=repository", ArgoSecretTypeLabel + "=repo-creds"}
	}
	return []string{GitOpsManagedByLabel + "=" + GitOpsManagedByValue + "," + GitOpsCredentialPurposeLabel + "=" + GitOpsCredentialPurposeValue}
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// checkGitOpsCredURL validates the repository URL for the credential kind and
// auth type. Like checkRepoURL its messages never echo the URL.
func checkGitOpsCredURL(raw, kind, authType string, flux bool) error {
	if raw == "" {
		return gitOpsBad("url is required")
	}
	if strings.Contains(raw, "://") && !hasSpaceOrControl(raw) {
		if u, err := url.Parse(raw); err == nil && u.User != nil {
			_, hasPass := u.User.Password()
			if hasPass || !strings.EqualFold(u.Scheme, "ssh") {
				return gitOpsBad("url must not contain credentials: pass them in username/password/token/sshPrivateKey")
			}
		}
	}
	if err := checkRepoURL(raw, kind != "git", flux); err != nil {
		return gitOpsBad("%s", strings.ReplaceAll(err.Error(), "source.repoUrl", "url"))
	}
	scheme := "scp"
	if i := strings.Index(raw, "://"); i > 0 {
		scheme = strings.ToLower(raw[:i])
	}
	switch {
	case authType == "ssh" && scheme != "ssh" && scheme != "scp":
		return gitOpsBad("authType ssh requires an ssh:// (or, for Argo CD, git@host:org/repo) url")
	case authType != "ssh" && (scheme == "ssh" || scheme == "scp" || scheme == "git"):
		return gitOpsBad("authType %s requires an https url; use authType ssh for ssh urls", authType)
	case kind == "oci" && scheme != "oci":
		return gitOpsBad("kind oci requires an oci:// url")
	case kind == "helm" && scheme == "oci":
		return gitOpsBad("an oci:// url requires kind oci")
	}
	return nil
}

var errSSHKey = gitOpsBad("sshPrivateKey must be an unencrypted PEM or OpenSSH private key (-----BEGIN ... PRIVATE KEY-----)")

// checkSSHPrivateKey parses the key. Errors never include key material.
func checkSSHPrivateKey(key string) error {
	if !strings.Contains(key, "-----BEGIN ") || !strings.Contains(key, "PRIVATE KEY-----") {
		return errSSHKey
	}
	if _, err := ssh.ParseRawPrivateKey([]byte(key)); err != nil {
		var pm *ssh.PassphraseMissingError
		if errors.As(err, &pm) {
			return gitOpsBad("sshPrivateKey is passphrase-protected; use an unencrypted deploy key")
		}
		return errSSHKey
	}
	return nil
}

func checkKnownHosts(kh string) error {
	n := 0
	for _, line := range strings.Split(kh, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(strings.Fields(line)) < 3 {
			return gitOpsBad("knownHosts must be in known_hosts format (host key-type key per line)")
		}
		n++
	}
	if n == 0 {
		return gitOpsBad("knownHosts must contain at least one host key line")
	}
	return nil
}

// buildGitOpsCredential validates req and renders the Secret. No message
// returned from here includes a secret value.
func buildGitOpsCredential(req GitOpsCredentialRequest) (gitOpsCredSecret, error) {
	var s gitOpsCredSecret
	if err := checkGitOpsCredEngine(req.Engine); err != nil {
		return s, err
	}
	flux := req.Engine == GitOpsCredEngineFlux
	if err := checkDNSLabel("name", req.Name, 63); err != nil {
		return s, err
	}
	ns, err := gitOpsCredNamespace(req.Engine, req.Namespace)
	if err != nil {
		return s, err
	}
	kind := req.Kind
	if kind == "" {
		kind = "git"
	}
	if kind != "git" && kind != "helm" && kind != "oci" {
		return s, gitOpsBad("kind must be git, helm or oci")
	}
	for _, f := range []struct {
		name, v string
		max     int
	}{
		{"username", req.Username, gitOpsCredMaxUsername}, {"password", req.Password, gitOpsCredMaxField},
		{"token", req.Token, gitOpsCredMaxField}, {"sshPrivateKey", req.SSHPrivateKey, gitOpsCredMaxField},
		{"knownHosts", req.KnownHosts, gitOpsCredMaxField},
	} {
		if len(f.v) > f.max {
			return s, gitOpsBad("%s must be at most %d bytes", f.name, f.max)
		}
	}
	if hasSpaceOrControl(req.Username) {
		return s, gitOpsBad("username must not contain whitespace or control characters")
	}
	if hasControl(req.Password) || hasControl(req.Token) {
		return s, gitOpsBad("password and token must not contain control characters or newlines")
	}
	mustBeEmpty := func(authType string, fields map[string]string) error {
		for name, v := range fields {
			if v != "" {
				return gitOpsBad("%s is not used with authType %s", name, authType)
			}
		}
		return nil
	}
	switch req.AuthType {
	case "basic":
		if req.Username == "" || req.Password == "" {
			return s, gitOpsBad("authType basic requires username and password")
		}
		if err := mustBeEmpty("basic", map[string]string{"token": req.Token, "sshPrivateKey": req.SSHPrivateKey, "knownHosts": req.KnownHosts}); err != nil {
			return s, err
		}
	case "token":
		if req.Token == "" {
			return s, gitOpsBad("authType token requires token")
		}
		if err := mustBeEmpty("token", map[string]string{"password": req.Password, "sshPrivateKey": req.SSHPrivateKey, "knownHosts": req.KnownHosts}); err != nil {
			return s, err
		}
	case "ssh":
		if kind != "git" {
			return s, gitOpsBad("authType ssh is only supported for kind git")
		}
		if req.SSHPrivateKey == "" {
			return s, gitOpsBad("authType ssh requires sshPrivateKey")
		}
		if err := mustBeEmpty("ssh", map[string]string{"username": req.Username, "password": req.Password, "token": req.Token}); err != nil {
			return s, err
		}
		if err := checkSSHPrivateKey(req.SSHPrivateKey); err != nil {
			return s, err
		}
		if flux {
			if req.KnownHosts == "" {
				return s, gitOpsBad("knownHosts is required for Flux ssh credentials (e.g. the output of ssh-keyscan)")
			}
			if err := checkKnownHosts(req.KnownHosts); err != nil {
				return s, err
			}
		} else if req.KnownHosts != "" {
			return s, gitOpsBad("knownHosts is not stored per repository for Argo CD; configure it in the argocd-ssh-known-hosts-cm ConfigMap")
		}
	case "":
		return s, gitOpsBad("authType is required (basic, token or ssh)")
	default:
		return s, gitOpsBad("authType must be basic, token or ssh")
	}
	if err := checkGitOpsCredURL(req.URL, kind, req.AuthType, flux); err != nil {
		return s, err
	}

	s = gitOpsCredSecret{
		Engine: req.Engine, Namespace: ns, Name: req.Name,
		Labels: map[string]string{
			GitOpsManagedByLabel:         GitOpsManagedByValue,
			GitOpsCredentialPurposeLabel: GitOpsCredentialPurposeValue,
		},
		Annotations: map[string]string{
			gitOpsCredURLAnnotation: req.URL, gitOpsCredKindAnnotation: kind, gitOpsCredAuthAnnotation: req.AuthType,
		},
		Data: map[string][]byte{},
	}
	username := req.Username
	if req.AuthType == "token" && username == "" {
		username = "git" // GitHub/GitLab/Bitbucket accept any username with a token
	}
	secret := req.Password
	if req.AuthType == "token" {
		secret = req.Token
	}
	if flux {
		if req.AuthType == "ssh" {
			s.Data["identity"] = []byte(req.SSHPrivateKey)
			s.Data["known_hosts"] = []byte(req.KnownHosts)
		} else {
			s.Data["username"] = []byte(username)
			s.Data["password"] = []byte(secret)
		}
		return s, nil
	}
	s.Labels[ArgoSecretTypeLabel] = "repository"
	argoURL, argoType := req.URL, kind
	if kind == "oci" {
		// Argo CD addresses OCI Helm registries without the scheme, with enableOCI.
		argoURL, argoType = strings.TrimPrefix(req.URL, "oci://"), "helm"
		s.Data["enableOCI"] = []byte("true")
	}
	s.Data["type"] = []byte(argoType)
	s.Data["url"] = []byte(argoURL)
	s.Data["name"] = []byte(req.Name)
	if req.AuthType == "ssh" {
		s.Data["sshPrivateKey"] = []byte(req.SSHPrivateKey)
	} else {
		s.Data["username"] = []byte(username)
		s.Data["password"] = []byte(secret)
	}
	return s, nil
}

// checkGitOpsCredentialDeletable allows deleting only Secrets Axiom labelled
// itself: Flux credentials with the managed-by + purpose labels, Argo CD
// repository Secrets that also carry the managed-by label.
func checkGitOpsCredentialDeletable(engine string, labels map[string]string) error {
	managed := labels[GitOpsManagedByLabel] == GitOpsManagedByValue
	ok := false
	switch engine {
	case GitOpsCredEngineFlux:
		ok = managed && labels[GitOpsCredentialPurposeLabel] == GitOpsCredentialPurposeValue
	case GitOpsCredEngineArgoCD:
		t := labels[ArgoSecretTypeLabel]
		ok = managed && (t == "repository" || t == "repo-creds")
	}
	if !ok {
		return NewStatusError(http.StatusConflict, errors.New("this Secret was not created by Axiom as a GitOps credential; it is never deleted from here"))
	}
	return nil
}

// gitOpsCredentialSummary builds the listing row. data is read ONLY for the
// non-secret Argo CD "url", "type" and "enableOCI" keys and for key presence.
func gitOpsCredentialSummary(engine, ns, name string, labels, annotations map[string]string, data map[string][]byte, ageStr string) GitOpsCredential {
	c := GitOpsCredential{
		Name: name, Namespace: ns, Engine: engine,
		ManagedByAxiom: labels[GitOpsManagedByLabel] == GitOpsManagedByValue,
		URL:            annotations[gitOpsCredURLAnnotation],
		Kind:           annotations[gitOpsCredKindAnnotation],
		AuthType:       annotations[gitOpsCredAuthAnnotation],
		Age:            ageStr,
	}
	has := func(k string) bool { _, ok := data[k]; return ok }
	if engine == GitOpsCredEngineArgoCD {
		if u := string(data["url"]); u != "" && c.URL == "" {
			c.URL = u
		}
		if c.Kind == "" {
			switch string(data["type"]) {
			case "helm":
				c.Kind = "helm"
				if string(data["enableOCI"]) == "true" {
					c.Kind = "oci"
				}
			case "oci":
				c.Kind = "oci"
			default:
				c.Kind = "git"
			}
		}
		if c.AuthType == "" {
			switch {
			case has("sshPrivateKey"):
				c.AuthType = "ssh"
			case has("githubAppPrivateKey"):
				c.AuthType = "githubApp"
			case has("password"):
				c.AuthType = "basic"
			default:
				c.AuthType = "none"
			}
		}
		return c
	}
	if c.AuthType == "" {
		switch {
		case has("identity"):
			c.AuthType = "ssh"
		case has("password"):
			c.AuthType = "basic"
		}
	}
	return c
}
