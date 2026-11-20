package platform

// Helm chart catalog: a list of chart repositories an administrator added, a
// cached parse of each repository's index.yaml, search over it, and a reader
// that pulls values.yaml / README.md out of a chart archive in memory.
//
// The server fetches URLs an administrator typed (and URLs those indexes name),
// so every request goes through HelmFetcher, which refuses non-public
// addresses at dial time (after DNS resolution, so a rebinding DNS answer
// cannot slip past a check made earlier) and re-validates every redirect hop.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"sigs.k8s.io/yaml"
)

const (
	helmRepoConfigMapName = "zyvor-axiom-helm-repos"
	helmRepoConfigMapKey  = "repos.json"

	helmMaxRepos          = 20
	helmMaxChartsPerIndex = 5000
	helmMaxVersions       = 100
	helmMaxIndexBytes     = 20 << 20
	helmMaxArchiveBytes   = 20 << 20
	helmMaxExtractedFile  = 2 << 20
	helmMaxReadmeBytes    = 64 << 10
	helmMaxRedirects      = 3
	helmRequestTimeout    = 15 * time.Second
	helmCacheTTL          = 10 * time.Minute
	helmRefreshMinGap     = 30 * time.Second
	helmDefaultLimit      = 60
	helmMaxLimit          = 200
)

// helmMaxDecompressed bounds the bytes we are willing to inflate while
// scanning an archive for the two entries we want (CPU guard against a gzip
// bomb hidden in entries we skip). A variable so tests can lower it.
var helmMaxDecompressed int64 = 256 << 20

// ---------------------------------------------------------------------------
// Repository store

// HelmRepo is one chart repository an administrator added.
type HelmRepo struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	AddedAt string `json:"addedAt"`
	// Kind is "oci" for a registry namespace (oci://host/path); empty or
	// "index" for a classic index.yaml repository.
	Kind string `json:"kind,omitempty"`
	// Charts names the charts of an OCI repository explicitly (most hosted
	// registries do not offer /v2/_catalog).
	Charts []string `json:"charts,omitempty"`
	// CredentialsSecret names a stored registry credential (OCI only; see
	// HelmRegistryCredStore). It is a name, never the secret itself.
	CredentialsSecret string `json:"credentialsSecret,omitempty"`
	// PlainHTTP records that the registry answered over plain http when it
	// was added (only possible with AXIOM_HELM_ALLOW_INSECURE_REPOS); helm
	// install/upgrade then get --plain-http.
	PlainHTTP bool `json:"plainHttp,omitempty"`
}

// HelmRepoStore persists the repository list.
type HelmRepoStore interface {
	List(ctx context.Context) ([]HelmRepo, error)
	Add(ctx context.Context, repo HelmRepo) error
	Remove(ctx context.Context, name string) error
}

// helmRepoListAdd returns list+repo, refusing duplicate names/URLs and a list
// that is already full. Shared by every store so the rules are identical.
func helmRepoListAdd(list []HelmRepo, repo HelmRepo) ([]HelmRepo, error) {
	key := helmRepoURLKey(repo.URL)
	for _, r := range list {
		if r.Name == repo.Name {
			return nil, NewStatusError(http.StatusConflict, fmt.Errorf("a chart repository named %q already exists", repo.Name))
		}
		if helmRepoURLKey(r.URL) == key {
			return nil, NewStatusError(http.StatusConflict, fmt.Errorf("that URL is already added as chart repository %q", r.Name))
		}
	}
	if len(list) >= helmMaxRepos {
		return nil, NewStatusError(http.StatusBadRequest, fmt.Errorf("at most %d chart repositories can be added", helmMaxRepos))
	}
	out := append(append([]HelmRepo(nil), list...), repo)
	return out, nil
}

func helmRepoListRemove(list []HelmRepo, name string) ([]HelmRepo, error) {
	out := make([]HelmRepo, 0, len(list))
	found := false
	for _, r := range list {
		if r.Name == name {
			found = true
			continue
		}
		out = append(out, r)
	}
	if !found {
		return nil, NewStatusError(http.StatusNotFound, fmt.Errorf("chart repository %q not found", name))
	}
	return out, nil
}

// helmRepoURLKey compares URLs with a case-insensitive scheme and host.
func helmRepoURLKey(raw string) string {
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil {
		return raw
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.EscapedPath(), "/")
}

// MemoryHelmRepoStore keeps the repository list in memory (demo build, tests).
type MemoryHelmRepoStore struct {
	mu    sync.Mutex
	repos []HelmRepo
}

func NewMemoryHelmRepoStore() *MemoryHelmRepoStore { return &MemoryHelmRepoStore{} }

func (m *MemoryHelmRepoStore) List(context.Context) ([]HelmRepo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]HelmRepo{}, m.repos...), nil
}

func (m *MemoryHelmRepoStore) Add(_ context.Context, repo HelmRepo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	out, err := helmRepoListAdd(m.repos, repo)
	if err != nil {
		return err
	}
	m.repos = out
	return nil
}

func (m *MemoryHelmRepoStore) Remove(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	out, err := helmRepoListRemove(m.repos, name)
	if err != nil {
		return err
	}
	m.repos = out
	return nil
}

// ---------------------------------------------------------------------------
// Validation

var helmRepoNameRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// ValidateHelmRepoName enforces a DNS-1123 label of at most 40 characters.
func ValidateHelmRepoName(name string) error {
	if name == "" {
		return NewStatusError(http.StatusBadRequest, errors.New("repository name is required"))
	}
	if len(name) > 40 || !helmRepoNameRe.MatchString(name) {
		return NewStatusError(http.StatusBadRequest, errors.New("repository name must be a lower-case DNS label (a-z, 0-9, '-'; at most 40 characters)"))
	}
	return nil
}

// HelmFetchPolicy relaxes the default "public HTTPS only" rule.
type HelmFetchPolicy struct {
	AllowInsecure bool // http:// URLs (AXIOM_HELM_ALLOW_INSECURE_REPOS=true)
	AllowPrivate  bool // loopback/private/link-local targets (AXIOM_HELM_ALLOW_PRIVATE_REPOS=true)
}

// HelmFetchPolicyFromEnv reads AXIOM_HELM_ALLOW_INSECURE_REPOS and
// AXIOM_HELM_ALLOW_PRIVATE_REPOS (only the exact value "true" enables them).
func HelmFetchPolicyFromEnv() HelmFetchPolicy {
	return HelmFetchPolicy{
		AllowInsecure: strings.EqualFold(strings.TrimSpace(os.Getenv("AXIOM_HELM_ALLOW_INSECURE_REPOS")), "true"),
		AllowPrivate:  strings.EqualFold(strings.TrimSpace(os.Getenv("AXIOM_HELM_ALLOW_PRIVATE_REPOS")), "true"),
	}
}

// helmPolicyError marks a request the SSRF policy refused.
type helmPolicyError struct{ msg string }

func (e *helmPolicyError) Error() string { return e.msg }

func policyErr(format string, a ...any) error {
	return &helmPolicyError{msg: fmt.Sprintf(format, a...)}
}

var (
	helmCGNAT       = netip.MustParsePrefix("100.64.0.0/10")
	helmThisNetwork = netip.MustParsePrefix("0.0.0.0/8")
	helmBroadcast   = netip.MustParseAddr("255.255.255.255")
)

// helmAddrDisallowed reports whether ip is not a public unicast address:
// loopback, link-local (169.254/16 incl. cloud metadata, fe80::/10), private
// (10/8, 172.16/12, 192.168/16, fc00::/7), unspecified, multicast, CGNAT
// (100.64/10), 0/8 and broadcast. IPv4-mapped IPv6 is checked as IPv4.
func helmAddrDisallowed(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() {
		return true
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsPrivate() ||
		ip.IsUnspecified() || helmCGNAT.Contains(ip) || helmThisNetwork.Contains(ip) || ip == helmBroadcast
}

func isLocalhostName(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(host), ".")
	return h == "localhost" || strings.HasSuffix(h, ".localhost")
}

var helmNumericLabelRe = regexp.MustCompile(`^(0[xX][0-9a-fA-F]*|[0-9]+)$`)

// looksLikeEncodedIP catches hosts some resolvers turn into an IPv4 address
// even though they are not dotted quads: "2130706433", "0x7f000001",
// "0177.0.0.1", "127.1". No real DNS name has only numeric labels.
func looksLikeEncodedIP(host string) bool {
	h := strings.TrimSuffix(host, ".")
	if h == "" {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if !helmNumericLabelRe.MatchString(label) {
			return false
		}
	}
	return true
}

// ValidateHelmURL applies the static part of the policy to a URL (scheme,
// userinfo, host, fragment, literal IPs). Name resolution is checked at dial
// time by HelmFetcher.
func (p HelmFetchPolicy) ValidateHelmURL(u *url.URL) error {
	if u == nil {
		return policyErr("URL is required")
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
	case "http":
		if !p.AllowInsecure {
			return policyErr("only https:// chart repositories are allowed (set AXIOM_HELM_ALLOW_INSECURE_REPOS=true to allow http://)")
		}
	case "":
		return policyErr("URL must start with https://")
	default:
		return policyErr("unsupported URL scheme %q: use https://", u.Scheme)
	}
	if u.Opaque != "" {
		return policyErr("URL must be of the form https://host/path")
	}
	if u.User != nil {
		return policyErr("URL must not contain a user name or password")
	}
	if u.Fragment != "" || u.RawFragment != "" {
		return policyErr("URL must not contain a fragment")
	}
	host := u.Hostname()
	if host == "" {
		return policyErr("URL has no host")
	}
	if strings.Contains(host, "%") {
		return policyErr("URL host must not carry an IPv6 zone")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !p.AllowPrivate && helmAddrDisallowed(ip) {
			return policyErr("refusing to fetch from %s: not a public address (set AXIOM_HELM_ALLOW_PRIVATE_REPOS=true for an in-cluster repository)", host)
		}
		return nil
	}
	if looksLikeEncodedIP(host) {
		return policyErr("URL host %q is a numeric address that is not a dotted-quad IPv4 address", host)
	}
	if !p.AllowPrivate && isLocalhostName(host) {
		return policyErr("refusing to fetch from %s: not a public address (set AXIOM_HELM_ALLOW_PRIVATE_REPOS=true for an in-cluster repository)", host)
	}
	return nil
}

// NormalizeHelmRepoURL trims the URL, strips a trailing slash and validates it.
// Query strings are refused: they usually carry credentials, and the repo URL
// is shown to every role that can read the catalog.
func (p HelmFetchPolicy) NormalizeHelmRepoURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return "", NewStatusError(http.StatusBadRequest, errors.New("repository URL is required"))
	}
	if strings.Contains(raw, "#") {
		return "", NewStatusError(http.StatusBadRequest, errors.New("URL must not contain a fragment"))
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", NewStatusError(http.StatusBadRequest, errors.New("invalid repository URL"))
	}
	if err := p.ValidateHelmURL(u); err != nil {
		return "", NewStatusError(http.StatusBadRequest, err)
	}
	if u.RawQuery != "" || u.ForceQuery {
		return "", NewStatusError(http.StatusBadRequest, errors.New("repository URL must not contain a query string (credentials are not supported)"))
	}
	return raw, nil
}

// redactHelmURL drops the query string and userinfo so URLs are safe to show
// in errors and logs.
func redactHelmURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	c := *u
	c.User = nil
	c.RawQuery = ""
	c.ForceQuery = false
	c.Fragment = ""
	c.RawFragment = ""
	return c.String()
}

// ---------------------------------------------------------------------------
// Fetcher

// HelmResolver resolves host names (net.DefaultResolver in production; tests
// inject a fake to simulate rebinding answers).
type HelmResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// HelmFetcher performs policy-checked GETs.
type HelmFetcher struct {
	Policy    HelmFetchPolicy
	Resolver  HelmResolver
	UserAgent string
	// TLSConfig, when set, is used for https (tests trust an httptest CA).
	TLSConfig *tls.Config
	// dial connects to an already-validated ip:port. Tests override it to
	// route a "public" address to a local test server.
	dial func(ctx context.Context, network, addr string) (net.Conn, error)

	once   sync.Once
	client *http.Client

	// Bearer tokens obtained with registry credentials, by (registry,
	// scope, credential fingerprint); see helmoci.go. now is the clock
	// (tests inject one).
	now      func() time.Time
	tokMu    sync.Mutex
	tokCache map[string]helmCachedToken
}

// NewHelmFetcher returns a fetcher with the production resolver and dialer.
func NewHelmFetcher(p HelmFetchPolicy, userAgent string) *HelmFetcher {
	return &HelmFetcher{Policy: p, UserAgent: userAgent}
}

func (f *HelmFetcher) httpClient() *http.Client {
	f.once.Do(func() {
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		if f.TLSConfig != nil {
			tlsCfg = f.TLSConfig.Clone()
		}
		tr := &http.Transport{
			Proxy:                 nil, // a proxy would dial the target for us, bypassing the address check
			DialContext:           f.dialContext,
			TLSClientConfig:       tlsCfg,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: helmRequestTimeout,
			MaxIdleConns:          10,
			IdleConnTimeout:       60 * time.Second,
			ForceAttemptHTTP2:     true,
		}
		f.client = &http.Client{
			Transport: tr,
			Timeout:   helmRequestTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > helmMaxRedirects {
					return policyErr("too many redirects (at most %d)", helmMaxRedirects)
				}
				if err := f.Policy.ValidateHelmURL(req.URL); err != nil {
					return policyErr("redirect refused: %v", err)
				}
				// Never carry credentials or cookies to the next hop.
				req.Header.Del("Authorization")
				req.Header.Del("Cookie")
				return nil
			},
		}
	})
	return f.client
}

// dialContext resolves the host, refuses the whole host if any address is not
// public (unless private targets are allowed) and dials the validated IP, so
// there is no second lookup an attacker's DNS could answer differently.
func (f *HelmFetcher) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	var ips []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{ip.Unmap()}
	} else {
		if looksLikeEncodedIP(host) {
			return nil, policyErr("refusing numeric host %q", host)
		}
		if !f.Policy.AllowPrivate && isLocalhostName(host) {
			return nil, policyErr("refusing to connect to %s: not a public address", host)
		}
		var res HelmResolver = net.DefaultResolver
		if f.Resolver != nil {
			res = f.Resolver
		}
		addrs, err := res.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", host, err)
		}
		for _, a := range addrs {
			if ip, ok := netip.AddrFromSlice(a.IP); ok {
				ips = append(ips, ip.Unmap())
			}
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("resolve %s: no addresses", host)
	}
	if !f.Policy.AllowPrivate {
		for _, ip := range ips {
			if helmAddrDisallowed(ip) {
				return nil, policyErr("refusing to connect to %s: it resolves to %s, which is not a public address (set AXIOM_HELM_ALLOW_PRIVATE_REPOS=true for an in-cluster repository)", host, ip)
			}
		}
	}
	dial := f.dial
	if dial == nil {
		dial = f.defaultDial
	}
	var lastErr error
	for _, ip := range ips {
		c, err := dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return c, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// defaultDial re-checks the socket address in Control, the last moment before
// connect(2), as a second line of defence.
func (f *HelmFetcher) defaultDial(ctx context.Context, network, addr string) (net.Conn, error) {
	d := net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if f.Policy.AllowPrivate {
				return nil
			}
			ap, err := netip.ParseAddrPort(address)
			if err != nil || helmAddrDisallowed(ap.Addr()) {
				return policyErr("refusing to connect to %s: not a public address", address)
			}
			return nil
		},
	}
	return d.DialContext(ctx, network, addr)
}

// get fetches u (policy-checked, at most max bytes after transfer decoding).
// Errors are StatusErrors: 400 for a refused URL, 413 when too large and 502
// when the server is unreachable or answers with an error.
func (f *HelmFetcher) get(ctx context.Context, u *url.URL, max int64, what string) ([]byte, error) {
	if err := f.Policy.ValidateHelmURL(u); err != nil {
		return nil, NewStatusError(http.StatusBadRequest, err)
	}
	shown := redactHelmURL(u)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, NewStatusError(http.StatusBadRequest, fmt.Errorf("invalid URL %s", shown))
	}
	ua := f.UserAgent
	if ua == "" {
		ua = "zyvor-axiom/dev"
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")
	// Accept-Encoding is left to the transport, which then sends gzip and
	// decodes the response transparently.
	resp, err := f.httpClient().Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // url.Error would print the full URL, query string included
		}
		var pe *helmPolicyError
		if errors.As(err, &pe) {
			return nil, NewStatusError(http.StatusBadRequest, fmt.Errorf("fetch %s: %v", shown, pe))
		}
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("fetch %s: %v", shown, err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("fetch %s: HTTP %d", shown, resp.StatusCode))
	}
	if resp.ContentLength > max && !resp.Uncompressed {
		return nil, NewStatusError(http.StatusRequestEntityTooLarge, fmt.Errorf("%s too large (limit %d MiB)", what, max>>20))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("fetch %s: %v", shown, err))
	}
	if int64(len(body)) > max {
		return nil, NewStatusError(http.StatusRequestEntityTooLarge, fmt.Errorf("%s too large (limit %d MiB)", what, max>>20))
	}
	return body, nil
}

// helmIndexURL is <repo>/index.yaml.
func helmIndexURL(repoURL string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(repoURL, "/") + "/index.yaml")
	if err != nil {
		return nil, NewStatusError(http.StatusBadRequest, errors.New("invalid repository URL"))
	}
	return u, nil
}

// FetchIndex downloads and parses a repository's index.yaml.
func (f *HelmFetcher) FetchIndex(ctx context.Context, repoURL string) (*helmIndex, error) {
	u, err := helmIndexURL(repoURL)
	if err != nil {
		return nil, err
	}
	body, err := f.get(ctx, u, helmMaxIndexBytes, "repository index")
	if err != nil {
		return nil, err
	}
	return parseHelmIndex(body)
}

// ---------------------------------------------------------------------------
// Index parsing

// flexString accepts a JSON string, number or bool (YAML like `version: 1.0`
// or an unquoted timestamp) and keeps its text.
type flexString string

func (s *flexString) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = flexString(v)
		return nil
	}
	if string(b) == "null" {
		*s = ""
		return nil
	}
	*s = flexString(strings.TrimSpace(string(b)))
	return nil
}

type helmIndexMaintainer struct {
	Name  flexString `json:"name"`
	Email flexString `json:"email"`
}

type helmIndexEntry struct {
	Name        flexString            `json:"name"`
	Version     flexString            `json:"version"`
	AppVersion  flexString            `json:"appVersion"`
	Description flexString            `json:"description"`
	Icon        flexString            `json:"icon"`
	Home        flexString            `json:"home"`
	Keywords    []flexString          `json:"keywords"`
	Sources     []flexString          `json:"sources"`
	Maintainers []helmIndexMaintainer `json:"maintainers"`
	URLs        []flexString          `json:"urls"`
	Deprecated  bool                  `json:"deprecated"`
	Created     flexString            `json:"created"`

	sv semver
}

type helmIndex struct {
	charts map[string][]*helmIndexEntry // versions newest first
	names  []string                     // sorted
	// pinned is the entry whose metadata was read (OCI repositories, where
	// only the newest release's Chart.yaml is fetched).
	pinned map[string]*helmIndexEntry
}

func (ix *helmIndex) chartCount() int {
	if ix == nil {
		return 0
	}
	return len(ix.names)
}

func parseHelmIndex(body []byte) (*helmIndex, error) {
	var raw struct {
		APIVersion string                     `json:"apiVersion"`
		Entries    map[string]json.RawMessage `json:"entries"`
	}
	if err := yaml.Unmarshal(body, &raw); err != nil {
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("repository index is not valid YAML: %s", firstLine(err.Error())))
	}
	if raw.APIVersion == "" {
		return nil, NewStatusError(http.StatusBadGateway, errors.New("not a Helm chart repository index (index.yaml has no apiVersion)"))
	}
	names := make([]string, 0, len(raw.Entries))
	for name := range raw.Entries {
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) > helmMaxChartsPerIndex {
		names = names[:helmMaxChartsPerIndex]
	}
	ix := &helmIndex{charts: make(map[string][]*helmIndexEntry, len(names))}
	for _, name := range names {
		var entries []*helmIndexEntry
		if err := json.Unmarshal(raw.Entries[name], &entries); err != nil {
			continue // one malformed chart does not spoil the repository
		}
		kept := entries[:0]
		for _, e := range entries {
			if e == nil || strings.TrimSpace(string(e.Version)) == "" {
				continue
			}
			e.sv = parseSemver(string(e.Version))
			kept = append(kept, e)
		}
		if len(kept) == 0 {
			continue
		}
		sortHelmVersions(kept)
		ix.charts[name] = kept
		ix.names = append(ix.names, name)
	}
	return ix, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// latest is the newest non-deprecated release; if there is none, the newest.
func (ix *helmIndex) latest(name string) *helmIndexEntry {
	if e := ix.pinned[name]; e != nil {
		return e
	}
	vs := ix.charts[name]
	for _, e := range vs {
		if !e.Deprecated && e.sv.valid && len(e.sv.pre) == 0 {
			return e
		}
	}
	if len(vs) > 0 {
		return vs[0]
	}
	return nil
}

func (ix *helmIndex) version(name, version string) *helmIndexEntry {
	for _, e := range ix.charts[name] {
		if string(e.Version) == version {
			return e
		}
	}
	// Tolerate a leading "v" mismatch ("1.2.3" vs "v1.2.3").
	want := strings.TrimPrefix(version, "v")
	for _, e := range ix.charts[name] {
		if strings.TrimPrefix(string(e.Version), "v") == want {
			return e
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Versions

type semver struct {
	valid               bool
	major, minor, patch uint64
	pre                 []string
}

// parseSemver accepts MAJOR[.MINOR[.PATCH]][-PRERELEASE][+BUILD] with an
// optional leading "v"; build metadata is ignored.
func parseSemver(s string) semver {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	core, pre, hasPre := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return semver{}
	}
	var nums [3]uint64
	for i, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return semver{}
		}
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return semver{}
		}
		nums[i] = n
	}
	v := semver{valid: true, major: nums[0], minor: nums[1], patch: nums[2]}
	if hasPre {
		if pre == "" {
			return semver{}
		}
		for _, id := range strings.Split(pre, ".") {
			if id == "" {
				return semver{}
			}
		}
		v.pre = strings.Split(pre, ".")
	}
	return v
}

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func isNumericID(s string) bool { return s != "" && strings.Trim(s, "0123456789") == "" }

// compareSemver orders a and b (-1, 0, 1). Invalid versions sort below every
// valid one; a prerelease sorts below its release.
func compareSemver(a, b semver) int {
	if a.valid != b.valid {
		if a.valid {
			return 1
		}
		return -1
	}
	if !a.valid {
		return 0
	}
	if c := cmpUint(a.major, b.major); c != 0 {
		return c
	}
	if c := cmpUint(a.minor, b.minor); c != 0 {
		return c
	}
	if c := cmpUint(a.patch, b.patch); c != 0 {
		return c
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		x, y := a.pre[i], b.pre[i]
		xn, yn := isNumericID(x), isNumericID(y)
		switch {
		case xn && yn:
			x, y = strings.TrimLeft(x, "0"), strings.TrimLeft(y, "0")
			if len(x) != len(y) {
				if len(x) < len(y) {
					return -1
				}
				return 1
			}
			if c := strings.Compare(x, y); c != 0 {
				return c
			}
		case xn:
			return -1
		case yn:
			return 1
		default:
			if c := strings.Compare(x, y); c != 0 {
				return c
			}
		}
	}
	return cmpUint(uint64(len(a.pre)), uint64(len(b.pre)))
}

// sortHelmVersions sorts newest first; invalid versions last (by text).
func sortHelmVersions(vs []*helmIndexEntry) {
	sort.SliceStable(vs, func(i, j int) bool {
		if c := compareSemver(vs[i].sv, vs[j].sv); c != 0 {
			return c > 0
		}
		if vs[i].Created != vs[j].Created {
			return vs[i].Created > vs[j].Created
		}
		return vs[i].Version > vs[j].Version
	})
}

// ---------------------------------------------------------------------------
// Catalog (cache + views)

// HelmRepoView is a repository as the API reports it.
type HelmRepoView struct {
	Name       string   `json:"name"`
	URL        string   `json:"url"`
	Kind       string   `json:"kind"`
	Charts     []string `json:"charts,omitempty"`
	AddedAt    string   `json:"addedAt"`
	ChartCount int      `json:"chartCount"`
	FetchedAt  string   `json:"fetchedAt"`
	Error      string   `json:"error"`
	// CredentialsSecret is the NAME of the registry credential the
	// repository uses ("" for none); Authenticated says whether it uses one.
	CredentialsSecret string `json:"credentialsSecret"`
	Authenticated     bool   `json:"authenticated"`
	PlainHTTP         bool   `json:"plainHttp,omitempty"`
}

type HelmChartSummary struct {
	Repo        string   `json:"repo"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	AppVersion  string   `json:"appVersion"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Home        string   `json:"home"`
	Keywords    []string `json:"keywords"`
	Deprecated  bool     `json:"deprecated"`
	Created     string   `json:"created"`
	Kind        string   `json:"kind"`
	Ref         string   `json:"ref"` // oci://host/path/chart for OCI charts, "" otherwise
}

type HelmChartSearch struct {
	Charts    []HelmChartSummary `json:"charts"`
	Total     int                `json:"total"`
	FetchedAt string             `json:"fetchedAt"`
	Errors    map[string]string  `json:"errors"`
}

type HelmMaintainer struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type HelmChartVersion struct {
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
	Created    string `json:"created"`
	Deprecated bool   `json:"deprecated"`
}

type HelmChartDetail struct {
	Repo        string             `json:"repo"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Home        string             `json:"home"`
	Sources     []string           `json:"sources"`
	Keywords    []string           `json:"keywords"`
	Icon        string             `json:"icon"`
	Maintainers []HelmMaintainer   `json:"maintainers"`
	Versions    []HelmChartVersion `json:"versions"`
	Kind        string             `json:"kind"`
	Ref         string             `json:"ref"`
}

type HelmChartValues struct {
	Version string `json:"version"`
	Values  string `json:"values"`
	Readme  string `json:"readme"`
}

type helmFlight struct{ done chan struct{} }

type helmCacheEntry struct {
	url       string
	index     *helmIndex
	fetchedAt time.Time // last successful fetch
	attemptAt time.Time // last attempt (success or failure)
	err       string    // last attempt's error, "" after a success
	flight    *helmFlight
}

// HelmCatalog serves the chart catalog over a repository store.
type HelmCatalog struct {
	store   HelmRepoStore
	fetcher *HelmFetcher
	now     func() time.Time
	creds   HelmRegistryCredStore // nil: no registry credentials

	mu    sync.Mutex
	cache map[string]*helmCacheEntry
}

func NewHelmCatalog(store HelmRepoStore, fetcher *HelmFetcher) *HelmCatalog {
	if store == nil {
		store = NewMemoryHelmRepoStore()
	}
	if fetcher == nil {
		fetcher = NewHelmFetcher(HelmFetchPolicyFromEnv(), "")
	}
	return &HelmCatalog{store: store, fetcher: fetcher, now: time.Now, cache: map[string]*helmCacheEntry{}}
}

// SetRegistryCredStore gives the catalog the registry credentials that OCI
// repositories may reference.
func (c *HelmCatalog) SetRegistryCredStore(s HelmRegistryCredStore) { c.creds = s }

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (c *HelmCatalog) view(r HelmRepo) HelmRepoView {
	v := HelmRepoView{Name: r.Name, URL: r.URL, AddedAt: r.AddedAt, Kind: r.kind(), Charts: r.Charts,
		CredentialsSecret: r.CredentialsSecret, Authenticated: r.CredentialsSecret != "", PlainHTTP: r.PlainHTTP}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.cache[r.Name]; e != nil && e.url == r.URL {
		v.ChartCount = e.index.chartCount()
		v.FetchedAt = fmtTime(e.fetchedAt)
		v.Error = e.err
	}
	return v
}

// Repos lists the repositories with what the cache knows; it never fetches.
func (c *HelmCatalog) Repos(ctx context.Context) ([]HelmRepoView, error) {
	repos, err := c.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]HelmRepoView, 0, len(repos))
	for _, r := range repos {
		out = append(out, c.view(r))
	}
	return out, nil
}

// AddRepo validates, fetches the index once and stores the repository.
func (c *HelmCatalog) AddRepo(ctx context.Context, name, rawURL string) (HelmRepoView, error) {
	return c.AddHelmRepo(ctx, HelmRepoRequest{Name: name, URL: rawURL})
}

// HelmRepoRequest is an administrator's request to add a repository.
type HelmRepoRequest struct {
	Name              string
	URL               string
	Kind              string // "", "index" or "oci" ("" infers oci from an oci:// URL)
	Charts            []string
	CredentialsSecret string
}

// AddHelmRepo validates the request, reads the repository once (the index,
// or for OCI the registry, each chart's tags and its newest Chart.yaml) and
// stores it.
func (c *HelmCatalog) AddHelmRepo(ctx context.Context, req HelmRepoRequest) (HelmRepoView, error) {
	name := strings.TrimSpace(req.Name)
	if err := ValidateHelmRepoName(name); err != nil {
		return HelmRepoView{}, err
	}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	isOCIURL := strings.HasPrefix(strings.ToLower(strings.TrimSpace(req.URL)), "oci://")
	switch kind {
	case "":
		kind = HelmRepoKindIndex
		if isOCIURL {
			kind = HelmRepoKindOCI
		}
	case HelmRepoKindIndex:
		if isOCIURL {
			return HelmRepoView{}, NewStatusError(http.StatusBadRequest, errors.New(`an oci:// URL needs kind "oci"`))
		}
	case HelmRepoKindOCI:
	default:
		return HelmRepoView{}, NewStatusError(http.StatusBadRequest, errors.New(`kind must be "index" or "oci"`))
	}
	credName := strings.TrimSpace(req.CredentialsSecret)
	if credName != "" && kind != HelmRepoKindOCI {
		return HelmRepoView{}, NewStatusError(http.StatusBadRequest, errors.New("credentialsSecret applies only to OCI repositories"))
	}
	var repo HelmRepo
	if kind == HelmRepoKindOCI {
		u, err := c.fetcher.Policy.NormalizeHelmOCIURL(req.URL)
		if err != nil {
			return HelmRepoView{}, err
		}
		charts, err := ValidateHelmOCIChartNames(req.Charts)
		if err != nil {
			return HelmRepoView{}, err
		}
		repo = HelmRepo{Name: name, URL: u, AddedAt: fmtTime(c.now()), Kind: HelmRepoKindOCI}
		if len(charts) > 0 {
			repo.Charts = charts
		}
		if credName != "" {
			if err := c.checkRepoCred(ctx, credName, u); err != nil {
				return HelmRepoView{}, err
			}
			repo.CredentialsSecret = credName
		}
	} else {
		if len(req.Charts) > 0 {
			return HelmRepoView{}, NewStatusError(http.StatusBadRequest, errors.New("charts applies only to OCI repositories"))
		}
		u, err := c.fetcher.Policy.NormalizeHelmRepoURL(req.URL)
		if err != nil {
			return HelmRepoView{}, err
		}
		repo = HelmRepo{Name: name, URL: u, AddedAt: fmtTime(c.now())}
	}
	u := repo.URL
	existing, err := c.store.List(ctx)
	if err != nil {
		return HelmRepoView{}, err
	}
	// Check duplicates and the cap before touching the network.
	if _, err := helmRepoListAdd(existing, repo); err != nil {
		return HelmRepoView{}, err
	}
	ix, scheme, err := c.fetchRepoIndexScheme(ctx, repo, true)
	if err != nil {
		// Whatever went wrong (unreachable, refused, too large, not an
		// index), the administrator's input is what failed to verify.
		if repo.kind() == HelmRepoKindOCI {
			return HelmRepoView{}, NewStatusError(http.StatusBadRequest, fmt.Errorf("could not read the OCI registry: %w", err))
		}
		return HelmRepoView{}, NewStatusError(http.StatusBadRequest, fmt.Errorf("could not read the repository index: %w", err))
	}
	if repo.kind() == HelmRepoKindOCI && scheme == "http" {
		repo.PlainHTTP = true
	}
	if err := c.store.Add(ctx, repo); err != nil {
		return HelmRepoView{}, err
	}
	now := c.now()
	c.mu.Lock()
	c.cache[name] = &helmCacheEntry{url: u, index: ix, fetchedAt: now, attemptAt: now}
	c.mu.Unlock()
	return c.view(repo), nil
}

// RemoveRepo deletes a repository and its cached index.
func (c *HelmCatalog) RemoveRepo(ctx context.Context, name string) (HelmRepo, error) {
	repos, err := c.store.List(ctx)
	if err != nil {
		return HelmRepo{}, err
	}
	var found *HelmRepo
	for i := range repos {
		if repos[i].Name == name {
			found = &repos[i]
		}
	}
	if found == nil {
		return HelmRepo{}, NewStatusError(http.StatusNotFound, fmt.Errorf("chart repository %q not found", name))
	}
	if err := c.store.Remove(ctx, name); err != nil {
		return HelmRepo{}, err
	}
	c.mu.Lock()
	delete(c.cache, name)
	c.mu.Unlock()
	return *found, nil
}

func (c *HelmCatalog) repo(ctx context.Context, name string) (HelmRepo, error) {
	repos, err := c.store.List(ctx)
	if err != nil {
		return HelmRepo{}, err
	}
	for _, r := range repos {
		if r.Name == name {
			return r, nil
		}
	}
	return HelmRepo{}, NewStatusError(http.StatusNotFound, fmt.Errorf("chart repository %q not found", name))
}

// index returns the repository's parsed index, fetching it when the cached
// copy is older than 10 minutes (or when refresh is set and the last attempt
// is at least 30 s old). Concurrent callers share one fetch per repository. A
// failed refresh keeps serving the previous copy; err is then the failure
// message (and the returned error is non-nil only when there is no copy).
func (c *HelmCatalog) index(ctx context.Context, r HelmRepo, refresh bool) (*helmIndex, time.Time, string, error) {
	for {
		c.mu.Lock()
		e := c.cache[r.Name]
		if e == nil || e.url != r.URL {
			e = &helmCacheEntry{url: r.URL}
			c.cache[r.Name] = e
		}
		now := c.now()
		sinceAttempt := now.Sub(e.attemptAt)
		need := false
		switch {
		case e.attemptAt.IsZero():
			need = true
		case refresh:
			need = sinceAttempt >= helmRefreshMinGap
		case e.index == nil || now.Sub(e.fetchedAt) >= helmCacheTTL:
			// Stale or never fetched: retry, but a failing repository is
			// retried at most every 30 s.
			need = e.err == "" || sinceAttempt >= helmRefreshMinGap
		}
		if !need {
			ix, at, msg := e.index, e.fetchedAt, e.err
			c.mu.Unlock()
			if ix == nil {
				return nil, time.Time{}, msg, NewStatusError(http.StatusBadGateway, errors.New(msg))
			}
			return ix, at, msg, nil
		}
		if fl := e.flight; fl != nil {
			c.mu.Unlock()
			select {
			case <-fl.done:
				refresh = false // that fetch was the refresh
				continue
			case <-ctx.Done():
				return nil, time.Time{}, "", ctx.Err()
			}
		}
		fl := &helmFlight{done: make(chan struct{})}
		e.flight = fl
		c.mu.Unlock()

		// Detached from the caller: other waiters depend on this fetch.
		fctx, cancel := context.WithTimeout(context.Background(), helmFetchBudget(r))
		ix, err := c.fetchRepoIndex(fctx, r, false)
		cancel()

		c.mu.Lock()
		e.flight = nil
		e.attemptAt = c.now()
		if err != nil {
			e.err = err.Error()
		} else {
			e.index, e.fetchedAt, e.err = ix, e.attemptAt, ""
		}
		close(fl.done)
		c.mu.Unlock()
		refresh = false
	}
}

type helmRanked struct {
	rank int
	s    HelmChartSummary
}

func summarize(repo string, e *helmIndexEntry) HelmChartSummary {
	return HelmChartSummary{
		Repo: repo, Name: string(e.Name), Version: string(e.Version), AppVersion: string(e.AppVersion),
		Description: string(e.Description), Icon: string(e.Icon), Home: string(e.Home),
		Keywords: flexStrings(e.Keywords), Deprecated: e.Deprecated, Created: string(e.Created),
	}
}

func flexStrings(in []flexString) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" {
			out = append(out, string(s))
		}
	}
	return out
}

// helmRank scores a chart against a lower-cased query: 0 exact name, 1 name
// prefix, 2 name contains, 3 description or keyword; -1 no match.
func helmRank(name, description string, keywords []string, q string) int {
	if q == "" {
		return 0
	}
	n := strings.ToLower(name)
	switch {
	case n == q:
		return 0
	case strings.HasPrefix(n, q):
		return 1
	case strings.Contains(n, q):
		return 2
	case strings.Contains(strings.ToLower(description), q):
		return 3
	}
	for _, k := range keywords {
		if strings.Contains(strings.ToLower(k), q) {
			return 3
		}
	}
	return -1
}

// Search lists one entry per chart (its latest version) across one or every
// repository. A failing repository is reported in Errors and does not fail
// the call.
func (c *HelmCatalog) Search(ctx context.Context, repoName, q string, limit int, refresh bool) (HelmChartSearch, error) {
	if limit <= 0 {
		limit = helmDefaultLimit
	}
	if limit > helmMaxLimit {
		limit = helmMaxLimit
	}
	var repos []HelmRepo
	if repoName != "" {
		r, err := c.repo(ctx, repoName)
		if err != nil {
			return HelmChartSearch{}, err
		}
		repos = []HelmRepo{r}
	} else {
		all, err := c.store.List(ctx)
		if err != nil {
			return HelmChartSearch{}, err
		}
		repos = all
	}
	type result struct {
		ix  *helmIndex
		at  time.Time
		msg string
	}
	results := make([]result, len(repos))
	var wg sync.WaitGroup
	for i, r := range repos {
		wg.Add(1)
		go func(i int, r HelmRepo) {
			defer wg.Done()
			ix, at, msg, err := c.index(ctx, r, refresh)
			if err != nil && msg == "" {
				msg = err.Error()
			}
			results[i] = result{ix, at, msg}
		}(i, r)
	}
	wg.Wait()

	out := HelmChartSearch{Charts: []HelmChartSummary{}, Errors: map[string]string{}}
	q = strings.ToLower(strings.TrimSpace(q))
	var ranked []helmRanked
	var oldest time.Time
	for i, r := range repos {
		res := results[i]
		if res.msg != "" {
			out.Errors[r.Name] = res.msg
		}
		if res.ix == nil {
			continue
		}
		if oldest.IsZero() || res.at.Before(oldest) {
			oldest = res.at
		}
		for _, name := range res.ix.names {
			e := res.ix.latest(name)
			if e == nil {
				continue
			}
			s := summarize(r.Name, e)
			s.Name = name
			s.Kind = r.kind()
			if s.Kind == HelmRepoKindOCI {
				s.Ref = helmOCIChartRef(r.URL, name)
			}
			rk := helmRank(name, s.Description, s.Keywords, q)
			if rk < 0 {
				continue
			}
			ranked = append(ranked, helmRanked{rk, s})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if a.s.Name != b.s.Name {
			return a.s.Name < b.s.Name
		}
		return a.s.Repo < b.s.Repo
	})
	out.Total = len(ranked)
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	for _, r := range ranked {
		out.Charts = append(out.Charts, r.s)
	}
	out.FetchedAt = fmtTime(oldest)
	return out, nil
}

func (c *HelmCatalog) chartVersions(ctx context.Context, repoName, chart string) (HelmRepo, *helmIndex, error) {
	r, err := c.repo(ctx, repoName)
	if err != nil {
		return HelmRepo{}, nil, err
	}
	ix, _, _, err := c.index(ctx, r, false)
	if err != nil {
		return HelmRepo{}, nil, err
	}
	if len(ix.charts[chart]) == 0 {
		return HelmRepo{}, nil, NewStatusError(http.StatusNotFound, fmt.Errorf("chart %q not found in repository %q", chart, repoName))
	}
	return r, ix, nil
}

// Chart returns a chart's metadata (from its latest version) and up to 100
// versions, newest first.
func (c *HelmCatalog) Chart(ctx context.Context, repoName, chart string) (HelmChartDetail, error) {
	r, ix, err := c.chartVersions(ctx, repoName, chart)
	if err != nil {
		return HelmChartDetail{}, err
	}
	e := ix.latest(chart)
	d := HelmChartDetail{
		Repo: repoName, Name: chart, Description: string(e.Description), Home: string(e.Home),
		Sources: flexStrings(e.Sources), Keywords: flexStrings(e.Keywords), Icon: string(e.Icon),
		Maintainers: []HelmMaintainer{}, Versions: []HelmChartVersion{}, Kind: r.kind(),
	}
	if d.Kind == HelmRepoKindOCI {
		d.Ref = helmOCIChartRef(r.URL, chart)
	}
	for _, m := range e.Maintainers {
		d.Maintainers = append(d.Maintainers, HelmMaintainer{Name: string(m.Name), Email: string(m.Email)})
	}
	for i, v := range ix.charts[chart] {
		if i >= helmMaxVersions {
			break
		}
		d.Versions = append(d.Versions, HelmChartVersion{Version: string(v.Version), AppVersion: string(v.AppVersion), Created: string(v.Created), Deprecated: v.Deprecated})
	}
	return d, nil
}

// ChartValues downloads the chart archive for version (default: latest) and
// returns its values.yaml and README.md.
func (c *HelmCatalog) ChartValues(ctx context.Context, repoName, chart, version string) (HelmChartValues, error) {
	r, ix, err := c.chartVersions(ctx, repoName, chart)
	if err != nil {
		return HelmChartValues{}, err
	}
	var e *helmIndexEntry
	if strings.TrimSpace(version) == "" {
		e = ix.latest(chart)
	} else {
		e = ix.version(chart, strings.TrimSpace(version))
	}
	if e == nil {
		return HelmChartValues{}, NewStatusError(http.StatusNotFound, fmt.Errorf("chart %q has no version %q", chart, version))
	}
	if r.kind() == HelmRepoKindOCI {
		cred, err := c.repoCred(ctx, r)
		if err != nil {
			return HelmChartValues{}, err
		}
		values, readme, err := c.fetcher.fetchOCIChartValues(ctx, r.URL, cred, chart, string(e.Version))
		if err != nil {
			if code := HTTPStatus(err); code == http.StatusBadRequest || code == http.StatusNotFound {
				// The registry, not the caller, is what failed.
				return HelmChartValues{}, NewStatusError(http.StatusBadGateway, fmt.Errorf("chart download failed: %w", err))
			}
			return HelmChartValues{}, err
		}
		return HelmChartValues{Version: string(e.Version), Values: values, Readme: readme}, nil
	}
	if len(e.URLs) == 0 || strings.TrimSpace(string(e.URLs[0])) == "" {
		return HelmChartValues{}, NewStatusError(http.StatusBadGateway, fmt.Errorf("the repository index lists no download URL for %s %s", chart, e.Version))
	}
	archiveURL, err := resolveHelmChartURL(r.URL, string(e.URLs[0]))
	if err != nil {
		return HelmChartValues{}, err
	}
	// The index is not trusted: the archive URL goes through the same policy.
	body, err := c.fetcher.get(ctx, archiveURL, helmMaxArchiveBytes, "chart archive")
	if err != nil {
		if HTTPStatus(err) == http.StatusBadRequest {
			// The administrator did not type this URL; the repository handed it to us.
			return HelmChartValues{}, NewStatusError(http.StatusBadGateway, fmt.Errorf("chart download refused: %w", err))
		}
		return HelmChartValues{}, err
	}
	values, readme, err := readHelmChartArchive(body, chart)
	if err != nil {
		return HelmChartValues{}, err
	}
	return HelmChartValues{Version: string(e.Version), Values: values, Readme: readme}, nil
}

// resolveHelmChartURL resolves an index "urls" entry against the repository.
func resolveHelmChartURL(repoURL, ref string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimRight(repoURL, "/") + "/")
	if err != nil {
		return nil, NewStatusError(http.StatusBadRequest, errors.New("invalid repository URL"))
	}
	rel, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return nil, NewStatusError(http.StatusBadGateway, errors.New("the repository index has an invalid chart URL"))
	}
	return base.ResolveReference(rel), nil
}

// ---------------------------------------------------------------------------
// Archive reader

var errChartTooLarge = NewStatusError(http.StatusRequestEntityTooLarge, errors.New("chart archive too large"))

// readHelmChartArchive reads <chart>/values.yaml and <chart>/README.md from a
// .tgz held in memory. Nothing is written to disk; every other entry (other
// paths, subcharts, symlinks, directories) is skipped. values.yaml over 2 MiB
// is an error; README.md is truncated to 64 KiB.
func readHelmChartArchive(data []byte, chart string) (values, readme string, err error) {
	if len(data) > helmMaxArchiveBytes {
		return "", "", errChartTooLarge
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", "", NewStatusError(http.StatusBadGateway, errors.New("chart archive is not a gzip file"))
	}
	defer gz.Close()
	lr := &io.LimitedReader{R: gz, N: helmMaxDecompressed}
	tr := tar.NewReader(lr)
	valuesName := chart + "/values.yaml"
	var gotValues, gotReadme bool
	for !(gotValues && gotReadme) {
		hdr, err := tr.Next()
		if err != nil && lr.N <= 0 {
			return "", "", errChartTooLarge
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "", NewStatusError(http.StatusBadGateway, errors.New("chart archive is not a valid tar file"))
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != 0 /* old-style regular file */ {
			continue
		}
		name := hdr.Name
		switch {
		case !gotValues && name == valuesName:
			if hdr.Size > helmMaxExtractedFile {
				return "", "", errChartTooLarge
			}
			b, err := io.ReadAll(io.LimitReader(tr, helmMaxExtractedFile+1))
			if err != nil {
				return "", "", NewStatusError(http.StatusBadGateway, errors.New("chart archive is truncated"))
			}
			if len(b) > helmMaxExtractedFile {
				return "", "", errChartTooLarge
			}
			values, gotValues = string(b), true
		case !gotReadme && strings.HasPrefix(name, chart+"/") && strings.EqualFold(name[len(chart)+1:], "README.md"):
			b, err := io.ReadAll(io.LimitReader(tr, helmMaxReadmeBytes))
			if err != nil {
				return "", "", NewStatusError(http.StatusBadGateway, errors.New("chart archive is truncated"))
			}
			if len(b) == helmMaxReadmeBytes {
				// Truncated: do not cut a character in half.
				for i := 0; i < utf8.UTFMax && len(b) > 0 && !utf8.Valid(b); i++ {
					b = b[:len(b)-1]
				}
			}
			readme, gotReadme = string(b), true
		}
	}
	return values, readme, nil
}
