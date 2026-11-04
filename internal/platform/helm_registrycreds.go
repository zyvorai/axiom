package platform

// OCI registry credentials for the Helm chart catalog.
//
// SECURITY: a password flows in one direction only — from the request body
// into the store (one Secret in the Axiom namespace), and from there to
// exactly two consumers inside this package: the registry client (as HTTP
// Basic credentials, only towards the registry host named in the credential)
// and the install Job builder (as a Docker-style config.json in a short-lived
// Secret owned by the Job). It is never returned by the API, never logged,
// audited or put into an error message. The secret-bearing accessor is the
// unexported get method of HelmRegistryCredStore, so code outside this
// package (the API handlers) cannot read it. There is no update: rotating a
// credential is delete + recreate.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// HelmRegistryCredSecretName is the Secret (in the Axiom namespace) that
	// holds every registry credential as one JSON map.
	HelmRegistryCredSecretName = "zyvor-axiom-helm-registry-creds"
	helmRegistryCredSecretKey  = "credentials.json"

	// HelmRegistryCredBodyLimit bounds the POST body (http.MaxBytesReader).
	HelmRegistryCredBodyLimit = 16 << 10

	helmRegistryCredMaxPassword = 4 << 10
	helmRegistryCredMaxUsername = 256
	helmRegistryCredMaxCount    = 50

	// helmOCINeedsCredentials is appended to every "the registry wants
	// credentials we do not have" error.
	helmOCINeedsCredentials = "this registry needs credentials: add them under Helm → Registries"
)

// HelmRegistryCredRequest is the body of POST /api/v1/helm/registry-credentials.
type HelmRegistryCredRequest struct {
	Name     string `json:"name"`
	Registry string `json:"registry"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// String and GoString keep the password out of any %v / %#v formatting.
func (r HelmRegistryCredRequest) String() string {
	return "HelmRegistryCredRequest{name=" + r.Name + " registry=" + r.Registry + " <secrets redacted>}"
}
func (r HelmRegistryCredRequest) GoString() string { return r.String() }

// HelmRegistryCredInfo is the non-secret part of a stored credential.
type HelmRegistryCredInfo struct {
	Name      string `json:"name"`
	Registry  string `json:"registry"`
	Username  string `json:"username"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// HelmRegistryCredResult is the 201 response. It never echoes the password.
type HelmRegistryCredResult struct {
	OK       bool   `json:"ok"`
	Name     string `json:"name"`
	Registry string `json:"registry"`
	Username string `json:"username"`
}

// HelmRegistryCredential is one row of GET /api/v1/helm/registry-credentials.
type HelmRegistryCredential struct {
	Name     string   `json:"name"`
	Registry string   `json:"registry"`
	Username string   `json:"username"`
	Age      string   `json:"age"`
	InUse    []string `json:"inUse"`
}

// helmRegistryCred is a stored credential, password included. It is the
// JSON value of the stored map.
type helmRegistryCred struct {
	Registry  string `json:"registry"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	CreatedAt string `json:"createdAt"`
}

func (c helmRegistryCred) String() string {
	return "helmRegistryCred{registry=" + c.Registry + " <secrets redacted>}"
}
func (c helmRegistryCred) GoString() string { return c.String() }

// basicAuth is the value of an Authorization header for the credential.
func (c *helmRegistryCred) basicAuth() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.Username+":"+c.Password))
}

// fingerprint identifies the credential in cache keys without carrying it.
func (c *helmRegistryCred) fingerprint() string {
	h := sha256.Sum256([]byte(c.Registry + "\x00" + c.Username + "\x00" + c.Password))
	return hex.EncodeToString(h[:12])
}

// HelmRegistryCredStore persists registry credentials. List, Add and Delete
// never return a password; get (unexported, so only this package can call it)
// does, for the registry client and the install Job builder.
type HelmRegistryCredStore interface {
	List(ctx context.Context) ([]HelmRegistryCredInfo, error)
	// Add validates req and stores it; a name that exists is a 409 (an
	// existing credential is never overwritten or read).
	Add(ctx context.Context, req HelmRegistryCredRequest) (HelmRegistryCredInfo, error)
	// Delete removes a credential and returns what it was (without secrets).
	Delete(ctx context.Context, name string) (HelmRegistryCredInfo, error)
	get(ctx context.Context, name string) (helmRegistryCred, error)
}

// ---------------------------------------------------------------------------
// Validation

// NormalizeHelmRegistryHost checks a registry given as host[:port] — a DNS
// name or IP literal with an optional port, no scheme, path, userinfo, query
// or whitespace — and returns it lower-cased.
func NormalizeHelmRegistryHost(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	bad := NewStatusError(http.StatusBadRequest, errors.New("registry must be a host name with an optional port (for example ghcr.io or registry.example.com:5000), without a scheme or path"))
	if raw == "" {
		return "", NewStatusError(http.StatusBadRequest, errors.New("registry is required"))
	}
	if strings.Contains(raw, "://") || strings.ContainsAny(raw, "/@?#") {
		return "", bad
	}
	loc, err := parseHelmOCIURL("oci://" + raw)
	if err != nil || loc.path != "" {
		return "", bad
	}
	return loc.host, nil
}

// validateHelmRegistryCredRequest checks a create request and returns the
// record to store. No message returned from here includes a secret value.
func validateHelmRegistryCredRequest(req HelmRegistryCredRequest, now time.Time) (string, helmRegistryCred, error) {
	if err := checkDNSLabel("name", req.Name, 63); err != nil {
		return "", helmRegistryCred{}, err
	}
	registry, err := NormalizeHelmRegistryHost(req.Registry)
	if err != nil {
		return "", helmRegistryCred{}, err
	}
	switch {
	case req.Username == "":
		return "", helmRegistryCred{}, gitOpsBad("username is required")
	case len(req.Username) > helmRegistryCredMaxUsername:
		return "", helmRegistryCred{}, gitOpsBad("username must be at most %d bytes", helmRegistryCredMaxUsername)
	case hasSpaceOrControl(req.Username) || strings.Contains(req.Username, ":"):
		return "", helmRegistryCred{}, gitOpsBad("username must not contain whitespace, control characters or ':'")
	case req.Password == "":
		return "", helmRegistryCred{}, gitOpsBad("password is required (a password or an access token)")
	case len(req.Password) > helmRegistryCredMaxPassword:
		return "", helmRegistryCred{}, gitOpsBad("password must be at most %d bytes", helmRegistryCredMaxPassword)
	case hasControl(req.Password):
		return "", helmRegistryCred{}, gitOpsBad("password must not contain control characters or newlines")
	}
	return req.Name, helmRegistryCred{Registry: registry, Username: req.Username, Password: req.Password, CreatedAt: fmtTime(now)}, nil
}

func checkHelmRegistryCredName(name string) error {
	if name == "" || len(name) > 63 || !gitOpsDNSLabel.MatchString(name) {
		return NewStatusError(http.StatusBadRequest, errors.New("invalid registry credential name"))
	}
	return nil
}

// helmRegistryCredMapAdd and helmRegistryCredMapDelete are shared by every
// store so the rules are identical.
func helmRegistryCredMapAdd(m map[string]helmRegistryCred, name string, c helmRegistryCred) error {
	if _, ok := m[name]; ok {
		return NewStatusError(http.StatusConflict, fmt.Errorf("a registry credential named %q already exists (delete it first to replace it)", name))
	}
	if len(m) >= helmRegistryCredMaxCount {
		return NewStatusError(http.StatusBadRequest, fmt.Errorf("at most %d registry credentials can be stored", helmRegistryCredMaxCount))
	}
	m[name] = c
	return nil
}

func helmRegistryCredMapDelete(m map[string]helmRegistryCred, name string) (helmRegistryCred, error) {
	c, ok := m[name]
	if !ok {
		return helmRegistryCred{}, NewStatusError(http.StatusNotFound, fmt.Errorf("registry credential %q not found", name))
	}
	delete(m, name)
	return c, nil
}

func helmRegistryCredNotFound(name string) error {
	return NewStatusError(http.StatusNotFound, fmt.Errorf("registry credential %q not found", name))
}

func helmRegistryCredInfos(m map[string]helmRegistryCred) []HelmRegistryCredInfo {
	out := make([]HelmRegistryCredInfo, 0, len(m))
	for name, c := range m {
		out = append(out, HelmRegistryCredInfo{Name: name, Registry: c.Registry, Username: c.Username, CreatedAt: c.CreatedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ---------------------------------------------------------------------------
// Memory store

// MemoryHelmRegistryCredStore keeps credentials in memory (demo build, tests).
type MemoryHelmRegistryCredStore struct {
	mu    sync.Mutex
	creds map[string]helmRegistryCred
	now   func() time.Time
}

func NewMemoryHelmRegistryCredStore() *MemoryHelmRegistryCredStore {
	return &MemoryHelmRegistryCredStore{creds: map[string]helmRegistryCred{}, now: time.Now}
}

func (m *MemoryHelmRegistryCredStore) List(context.Context) ([]HelmRegistryCredInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return helmRegistryCredInfos(m.creds), nil
}

func (m *MemoryHelmRegistryCredStore) Add(_ context.Context, req HelmRegistryCredRequest) (HelmRegistryCredInfo, error) {
	name, c, err := validateHelmRegistryCredRequest(req, m.now())
	if err != nil {
		return HelmRegistryCredInfo{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := helmRegistryCredMapAdd(m.creds, name, c); err != nil {
		return HelmRegistryCredInfo{}, err
	}
	return HelmRegistryCredInfo{Name: name, Registry: c.Registry, Username: c.Username, CreatedAt: c.CreatedAt}, nil
}

func (m *MemoryHelmRegistryCredStore) Delete(_ context.Context, name string) (HelmRegistryCredInfo, error) {
	if err := checkHelmRegistryCredName(name); err != nil {
		return HelmRegistryCredInfo{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c, err := helmRegistryCredMapDelete(m.creds, name)
	if err != nil {
		return HelmRegistryCredInfo{}, err
	}
	return HelmRegistryCredInfo{Name: name, Registry: c.Registry, Username: c.Username, CreatedAt: c.CreatedAt}, nil
}

func (m *MemoryHelmRegistryCredStore) get(_ context.Context, name string) (helmRegistryCred, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.creds[name]
	if !ok {
		return helmRegistryCred{}, helmRegistryCredNotFound(name)
	}
	return c, nil
}

// ---------------------------------------------------------------------------
// Operations used by the API (none returns a password)

// ListHelmRegistryCreds lists the credentials with the repositories that use
// each one.
func ListHelmRegistryCreds(ctx context.Context, creds HelmRegistryCredStore, repos HelmRepoStore) ([]HelmRegistryCredential, error) {
	infos, err := creds.List(ctx)
	if err != nil {
		return nil, err
	}
	var list []HelmRepo
	if repos != nil {
		if list, err = repos.List(ctx); err != nil {
			return nil, err
		}
	}
	out := make([]HelmRegistryCredential, 0, len(infos))
	for _, in := range infos {
		row := HelmRegistryCredential{Name: in.Name, Registry: in.Registry, Username: in.Username, Age: helmCredAge(in.CreatedAt, time.Now()), InUse: []string{}}
		for _, r := range list {
			if r.CredentialsSecret == in.Name {
				row.InUse = append(row.InUse, r.Name)
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// DeleteHelmRegistryCred deletes a credential no repository references.
func DeleteHelmRegistryCred(ctx context.Context, creds HelmRegistryCredStore, repos HelmRepoStore, name string) (HelmRegistryCredInfo, error) {
	if err := checkHelmRegistryCredName(name); err != nil {
		return HelmRegistryCredInfo{}, err
	}
	if repos != nil {
		list, err := repos.List(ctx)
		if err != nil {
			return HelmRegistryCredInfo{}, err
		}
		var users []string
		for _, r := range list {
			if r.CredentialsSecret == name {
				users = append(users, r.Name)
			}
		}
		if len(users) > 0 {
			return HelmRegistryCredInfo{}, NewStatusError(http.StatusConflict, fmt.Errorf("registry credential %q is used by chart repository %s: remove or edit repository %s first", name, strings.Join(users, ", "), users[0]))
		}
	}
	return creds.Delete(ctx, name)
}

func helmCredAge(createdAt string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return "-"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// ---------------------------------------------------------------------------
// Install access

// HelmOCIAccess is what an OCI install or upgrade needs beyond its reference:
// whether the registry speaks plain http, and the credentials of the
// repository the reference belongs to. Its fields are unexported: it is built
// only by ResolveHelmOCIAccess and read only by the install Job builder, so
// the API layer can carry it without being able to read the password.
type HelmOCIAccess struct {
	repo      string
	credName  string
	registry  string
	username  string
	password  string
	plainHTTP bool
}

func (a *HelmOCIAccess) String() string {
	if a == nil {
		return "HelmOCIAccess{}"
	}
	return fmt.Sprintf("HelmOCIAccess{repo=%s registry=%s plainHttp=%t credential=%s <secrets redacted>}", a.repo, a.registry, a.plainHTTP, a.credName)
}
func (a *HelmOCIAccess) GoString() string { return a.String() }

// Repo is the name of the matched repository.
func (a *HelmOCIAccess) Repo() string {
	if a == nil {
		return ""
	}
	return a.repo
}

// CredentialName is the name of the credential used, "" for none.
func (a *HelmOCIAccess) CredentialName() string {
	if a == nil {
		return ""
	}
	return a.credName
}

// PlainHTTP reports whether helm must be told --plain-http.
func (a *HelmOCIAccess) PlainHTTP() bool { return a != nil && a.plainHTTP }

func (a *HelmOCIAccess) authenticated() bool { return a != nil && a.credName != "" && a.password != "" }

// helmRegistryDockerConfig is the config.json helm reads through
// HELM_REGISTRY_CONFIG: {"auths":{"<registry>":{"auth":"<base64 user:pass>"}}}.
func (a *HelmOCIAccess) dockerConfig() ([]byte, error) {
	type entry struct {
		Auth string `json:"auth"`
	}
	return json.Marshal(map[string]map[string]entry{"auths": {
		a.registry: {Auth: base64.StdEncoding.EncodeToString([]byte(a.username + ":" + a.password))},
	}})
}

// helmOCIMatchRepo picks the stored OCI repository a reference belongs to:
// same registry host, and the repository's path is a prefix of the
// reference's path at a component boundary. The longest path wins.
func helmOCIMatchRepo(repos []HelmRepo, ref string) (HelmRepo, bool) {
	loc, err := parseHelmOCIURL(ref)
	if err != nil {
		return HelmRepo{}, false
	}
	best, bestLen, found := HelmRepo{}, -1, false
	for _, r := range repos {
		if r.kind() != HelmRepoKindOCI {
			continue
		}
		rl, err := parseHelmOCIURL(r.URL)
		if err != nil || rl.host != loc.host {
			continue
		}
		if rl.path != "" && loc.path != rl.path && !strings.HasPrefix(loc.path, rl.path+"/") {
			continue
		}
		if len(rl.path) > bestLen {
			best, bestLen, found = r, len(rl.path), true
		}
	}
	return best, found
}

// ResolveHelmOCIAccess finds the repository an oci:// install reference
// belongs to and returns its plain-http flag and credentials. nil means
// "anonymous over https", exactly as before credentials existed.
func ResolveHelmOCIAccess(ctx context.Context, repos HelmRepoStore, creds HelmRegistryCredStore, ref string) (*HelmOCIAccess, error) {
	if repos == nil || strings.TrimSpace(ref) == "" {
		return nil, nil
	}
	list, err := repos.List(ctx)
	if err != nil {
		return nil, err
	}
	r, ok := helmOCIMatchRepo(list, ref)
	if !ok || (!r.PlainHTTP && r.CredentialsSecret == "") {
		return nil, nil
	}
	loc, _ := parseHelmOCIURL(r.URL)
	a := &HelmOCIAccess{repo: r.Name, registry: loc.host, plainHTTP: r.PlainHTTP}
	if r.CredentialsSecret != "" {
		if creds == nil {
			return nil, NewStatusError(http.StatusConflict, fmt.Errorf("chart repository %q needs registry credential %q, which is not available", r.Name, r.CredentialsSecret))
		}
		c, err := creds.get(ctx, r.CredentialsSecret)
		if err != nil {
			if HTTPStatus(err) == http.StatusNotFound {
				return nil, NewStatusError(http.StatusConflict, fmt.Errorf("chart repository %q uses registry credential %q, which no longer exists", r.Name, r.CredentialsSecret))
			}
			return nil, err
		}
		if c.Registry != loc.host {
			return nil, NewStatusError(http.StatusConflict, fmt.Errorf("registry credential %q is for registry %s, not %s", r.CredentialsSecret, c.Registry, loc.host))
		}
		a.credName, a.username, a.password = r.CredentialsSecret, c.Username, c.Password
	}
	return a, nil
}
