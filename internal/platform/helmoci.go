package platform

// OCI Helm chart repositories: a registry namespace such as
// oci://ghcr.io/org/charts in which every chart is a repository
// (<namespace>/<chart>) and every version a tag. Registries have no index, so
// the catalog lists the charts an administrator named (or, where the registry
// allows it, the ones /v2/_catalog reports), lists each chart's tags, and reads
// Chart.yaml from the newest version's config blob. The chart archive itself is
// only downloaded to show values.yaml / README.md.
//
// Every request goes through HelmFetcher (dial-time address checks, redirect
// re-validation, no proxy, 15 s timeouts), the bearer-token realm included.
// Only anonymous access is supported: the token request never carries
// credentials.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	HelmRepoKindIndex = "index"
	HelmRepoKindOCI   = "oci"

	helmOCIMaxCharts     = 50
	helmOCIMaxTags       = 500
	helmOCIMaxListBytes  = 20 << 20 // tag list or catalog, all pages together
	helmOCIMaxPages      = 50
	helmOCIPageSize      = 100
	helmOCIMaxManifest   = 4 << 20
	helmOCIMaxConfig     = 1 << 20
	helmOCIMaxToken      = 64 << 10
	helmOCIMaxPing       = 64 << 10
	helmOCIConcurrency   = 4
	helmOCIFetchBudget   = 2 * time.Minute
	helmOCIMaxRefLength  = 512
	helmOCIMaxChartName  = 128
	helmOCIManifestOCI   = "application/vnd.oci.image.manifest.v1+json"
	helmOCIManifestV2    = "application/vnd.docker.distribution.manifest.v2+json"
	helmOCIChartLayer    = "application/vnd.cncf.helm.chart.content.v1.tar+gzip"
	helmOCIChartLayerOld = "application/tar+gzip" // Helm 3.0-3.7 pushes
	helmOCIConfigMedia   = "application/vnd.cncf.helm.config.v1+json"
)

var (
	// A path component of an OCI repository name (distribution spec).
	helmOCIComponentRe = regexp.MustCompile(`^[a-z0-9]+(?:(?:\.|_|__|-+)[a-z0-9]+)*$`)
	helmOCIChartNameRe = regexp.MustCompile(`^[a-z0-9]([-._a-z0-9]*[a-z0-9])?$`)
	helmOCIHostRe      = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	helmOCITagRe       = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$`)
	helmOCIDigestRe    = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

// kind is the repository kind, "index" for repositories stored before OCI
// support existed.
func (r HelmRepo) kind() string {
	if r.Kind == HelmRepoKindOCI {
		return HelmRepoKindOCI
	}
	return HelmRepoKindIndex
}

// ---------------------------------------------------------------------------
// Validation

// helmOCILocation is a parsed oci://host[:port][/path] reference.
type helmOCILocation struct {
	host string // lower-case host[:port]
	path string // namespace without leading or trailing slash ("" at the registry root)
}

func (l helmOCILocation) String() string {
	if l.path == "" {
		return "oci://" + l.host
	}
	return "oci://" + l.host + "/" + l.path
}

// repository is the registry repository name of a chart in the namespace.
func (l helmOCILocation) repository(chart string) string {
	if l.path == "" {
		return chart
	}
	return l.path + "/" + chart
}

// parseHelmOCIURL checks the syntax of oci://host[:port][/path]: a DNS name
// or IP literal, an optional port, lower-case OCI path components and nothing
// else (no userinfo, query, fragment, escapes or whitespace).
func parseHelmOCIURL(raw string) (helmOCILocation, error) {
	if len(raw) > helmOCIMaxRefLength {
		return helmOCILocation{}, errors.New("OCI reference is too long")
	}
	for _, r := range raw {
		if unicode.IsSpace(r) || unicode.IsControl(r) || r > unicode.MaxASCII {
			return helmOCILocation{}, errors.New("OCI reference must not contain whitespace, control or non-ASCII characters")
		}
	}
	if !strings.HasPrefix(strings.ToLower(raw), "oci://") {
		return helmOCILocation{}, errors.New("OCI reference must start with oci://")
	}
	rest := raw[len("oci://"):]
	if strings.ContainsAny(rest, "@") {
		return helmOCILocation{}, errors.New("OCI reference must not contain a user name or password")
	}
	if strings.ContainsAny(rest, "?#") {
		return helmOCILocation{}, errors.New("OCI reference must not contain a query string or fragment")
	}
	if strings.ContainsAny(rest, `%\`) {
		return helmOCILocation{}, errors.New("OCI reference must not contain escapes")
	}
	hostPort, path, _ := strings.Cut(rest, "/")
	if hostPort == "" {
		return helmOCILocation{}, errors.New("OCI reference has no host")
	}
	u, err := url.Parse("https://" + hostPort + "/")
	if err != nil || u.Host != hostPort {
		return helmOCILocation{}, errors.New("OCI reference has an invalid host")
	}
	host := strings.ToLower(u.Hostname())
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Zone() != "" {
			return helmOCILocation{}, errors.New("OCI reference host must not carry an IPv6 zone")
		}
	} else if len(host) > 253 || !helmOCIHostRe.MatchString(host) {
		return helmOCILocation{}, fmt.Errorf("OCI reference host %q is not a valid DNS name", host)
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return helmOCILocation{}, errors.New("OCI reference has an invalid port")
		}
	} else if strings.HasSuffix(hostPort, ":") {
		return helmOCILocation{}, errors.New("OCI reference has an invalid port")
	}
	if path != "" {
		for _, seg := range strings.Split(path, "/") {
			if !helmOCIComponentRe.MatchString(seg) {
				return helmOCILocation{}, fmt.Errorf("OCI reference path component %q is invalid (lower-case letters, digits and . _ - separators)", seg)
			}
		}
	}
	return helmOCILocation{host: strings.ToLower(u.Host), path: path}, nil
}

// NormalizeHelmOCIURL validates an oci:// repository URL against the syntax
// rules and the static part of the fetch policy and returns its canonical
// form (lower-case host, no trailing slash).
func (p HelmFetchPolicy) NormalizeHelmOCIURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", NewStatusError(http.StatusBadRequest, errors.New("repository URL is required"))
	}
	for len(raw) > len("oci://") && strings.HasSuffix(raw, "/") {
		raw = raw[:len(raw)-1]
	}
	loc, err := parseHelmOCIURL(raw)
	if err != nil {
		return "", NewStatusError(http.StatusBadRequest, err)
	}
	// The registry is reached over https (http only as a fallback when
	// insecure repositories are allowed), so check the host as https.
	if err := p.ValidateHelmURL(&url.URL{Scheme: "https", Host: loc.host, Path: "/"}); err != nil {
		return "", NewStatusError(http.StatusBadRequest, err)
	}
	return loc.String(), nil
}

// ValidateHelmOCIChartNames trims and checks an explicit chart list: at most
// 50 distinct DNS-1123-ish names.
func ValidateHelmOCIChartNames(charts []string) ([]string, error) {
	if len(charts) > helmOCIMaxCharts {
		return nil, NewStatusError(http.StatusBadRequest, fmt.Errorf("at most %d charts can be listed", helmOCIMaxCharts))
	}
	out := make([]string, 0, len(charts))
	seen := map[string]bool{}
	for _, c := range charts {
		c = strings.TrimSpace(c)
		if c == "" || len(c) > helmOCIMaxChartName || !helmOCIChartNameRe.MatchString(c) {
			return nil, NewStatusError(http.StatusBadRequest, fmt.Errorf("invalid chart name %q (lower-case letters, digits, '-', '.', '_')", c))
		}
		if seen[c] {
			return nil, NewStatusError(http.StatusBadRequest, fmt.Errorf("chart %q is listed twice", c))
		}
		seen[c] = true
		out = append(out, c)
	}
	return out, nil
}

// ValidateHelmOCIRef checks an install reference oci://host[:port]/path/chart.
func ValidateHelmOCIRef(ref string) error {
	loc, err := parseHelmOCIURL(ref)
	if err != nil {
		return NewStatusError(http.StatusBadRequest, err)
	}
	if !strings.HasPrefix(ref, "oci://") {
		return NewStatusError(http.StatusBadRequest, errors.New("OCI reference must start with oci://"))
	}
	if loc.path == "" {
		return NewStatusError(http.StatusBadRequest, errors.New("OCI reference must name a chart: oci://host/path/chart"))
	}
	if strings.HasSuffix(ref, "/") {
		return NewStatusError(http.StatusBadRequest, errors.New("OCI reference must not end with '/'"))
	}
	return nil
}

// ValidateHelmOCIInstall validates an install/upgrade request that names its
// chart with ociRef. Every value becomes a literal argv element of the helm
// Job (no shell), and none may look like a flag.
func ValidateHelmOCIInstall(req InstallHelmRequest) error {
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.ReleaseName) == "" || strings.TrimSpace(req.OCIRef) == "" {
		return NewStatusError(http.StatusBadRequest, errors.New("namespace, releaseName, and ociRef are required"))
	}
	if strings.TrimSpace(req.RepoURL) != "" {
		return NewStatusError(http.StatusBadRequest, errors.New("give either repoUrl and chartName, or ociRef, not both"))
	}
	for _, f := range []string{req.Namespace, req.ReleaseName, req.ChartName, req.ChartVersion, req.OCIRef} {
		if strings.HasPrefix(strings.TrimSpace(f), "-") {
			return NewStatusError(http.StatusBadRequest, fmt.Errorf("invalid value %q: must not start with '-'", f))
		}
	}
	if strings.IndexFunc(req.ChartVersion, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return NewStatusError(http.StatusBadRequest, errors.New("chartVersion must not contain whitespace"))
	}
	return ValidateHelmOCIRef(req.OCIRef)
}

// helmOCIChartRef is the install reference of a chart in an OCI repository.
func helmOCIChartRef(repoURL, chart string) string {
	return strings.TrimRight(repoURL, "/") + "/" + chart
}

// helmOCITagToVersion / helmOCIVersionToTag: OCI tags cannot hold '+', so
// Helm pushes a version's build metadata with '_' instead.
func helmOCITagToVersion(tag string) string { return strings.ReplaceAll(tag, "_", "+") }
func helmOCIVersionToTag(v string) string   { return strings.ReplaceAll(v, "+", "_") }

// ---------------------------------------------------------------------------
// HTTP

type helmOCIResponse struct {
	status int
	header http.Header
	body   []byte
	url    *url.URL // after redirects
}

// ociRequest performs one policy-checked GET. A non-200 answer is returned as
// a response (the caller decides); transport failures, refused URLs and
// oversized bodies are StatusErrors (502, 400, 413). When digest is set, the
// body is hashed while it is read and a mismatch is refused. Errors never
// carry the query string, the Authorization header or the body.
func (f *HelmFetcher) ociRequest(ctx context.Context, u *url.URL, hdr http.Header, max int64, what, digest string) (*helmOCIResponse, error) {
	if err := f.Policy.ValidateHelmURL(u); err != nil {
		return nil, NewStatusError(http.StatusBadRequest, err)
	}
	shown := redactHelmURL(u)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, NewStatusError(http.StatusBadRequest, fmt.Errorf("invalid URL %s", shown))
	}
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	ua := f.UserAgent
	if ua == "" {
		ua = "zyvor-axiom/dev"
	}
	req.Header.Set("User-Agent", ua)
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "*/*")
	}
	resp, err := f.httpClient().Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		var pe *helmPolicyError
		if errors.As(err, &pe) {
			return nil, NewStatusError(http.StatusBadRequest, fmt.Errorf("fetch %s: %v", shown, pe))
		}
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("fetch %s: %v", shown, err))
	}
	defer resp.Body.Close()
	out := &helmOCIResponse{status: resp.StatusCode, header: resp.Header, url: resp.Request.URL}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return out, nil
	}
	tooLarge := NewStatusError(http.StatusRequestEntityTooLarge, fmt.Errorf("%s too large (limit %s)", what, helmSizeText(max)))
	if resp.ContentLength > max && !resp.Uncompressed {
		return nil, tooLarge
	}
	var h hash.Hash
	var r io.Reader = io.LimitReader(resp.Body, max+1)
	if digest != "" {
		h = sha256.New()
		r = io.TeeReader(r, h)
	}
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("fetch %s: %v", shown, err))
	}
	if int64(len(body)) > max {
		return nil, tooLarge
	}
	if h != nil && "sha256:"+hex.EncodeToString(h.Sum(nil)) != digest {
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("%s does not match its digest %s: refusing it", what, digest))
	}
	out.body = body
	return out, nil
}

func helmSizeText(n int64) string {
	if n >= 1<<20 {
		return fmt.Sprintf("%d MiB", n>>20)
	}
	return fmt.Sprintf("%d KiB", n>>10)
}

// ---------------------------------------------------------------------------
// Bearer challenge

type helmBearerChallenge struct {
	realm, service, scope string
}

// parseHelmAuthChallenge reads a WWW-Authenticate value. It returns the
// scheme (lower-case) and its auth-params.
func parseHelmAuthChallenge(v string) (string, map[string]string) {
	v = strings.TrimSpace(v)
	scheme, rest, _ := strings.Cut(v, " ")
	params := map[string]string{}
	rest = strings.TrimSpace(rest)
	for rest != "" {
		eq := strings.IndexByte(rest, '=')
		if eq <= 0 {
			break
		}
		key := strings.ToLower(strings.TrimSpace(rest[:eq]))
		rest = strings.TrimLeft(rest[eq+1:], " ")
		var val string
		if strings.HasPrefix(rest, `"`) {
			var b strings.Builder
			i := 1
			for ; i < len(rest); i++ {
				c := rest[i]
				if c == '\\' && i+1 < len(rest) {
					i++
					b.WriteByte(rest[i])
					continue
				}
				if c == '"' {
					break
				}
				b.WriteByte(c)
			}
			val = b.String()
			if i < len(rest) {
				i++
			}
			rest = rest[i:]
		} else {
			end := strings.IndexByte(rest, ',')
			if end < 0 {
				end = len(rest)
			}
			val = strings.TrimSpace(rest[:end])
			rest = rest[end:]
		}
		params[key] = val
		rest = strings.TrimLeft(rest, " ,")
	}
	return strings.ToLower(scheme), params
}

func helmBearerFromHeader(h http.Header) (*helmBearerChallenge, string) {
	var schemes []string
	for _, v := range h.Values("WWW-Authenticate") {
		scheme, params := parseHelmAuthChallenge(v)
		if scheme == "bearer" && params["realm"] != "" {
			return &helmBearerChallenge{realm: params["realm"], service: params["service"], scope: params["scope"]}, ""
		}
		if scheme != "" {
			schemes = append(schemes, scheme)
		}
	}
	return nil, strings.Join(schemes, ",")
}

// ---------------------------------------------------------------------------
// Registry client

// helmOCIClient talks to one registry. It is used for one catalog fetch or
// one values download and is safe for concurrent use.
type helmOCIClient struct {
	f      *HelmFetcher
	loc    helmOCILocation
	scheme string

	mu     sync.Mutex
	strict *HelmFetcher      // public-https-only fetcher for a token realm on another host
	tokens map[string]string // scope -> anonymous bearer token
}

func newHelmOCIClient(f *HelmFetcher, repoURL string) (*helmOCIClient, error) {
	loc, err := parseHelmOCIURL(repoURL)
	if err != nil {
		return nil, NewStatusError(http.StatusBadRequest, err)
	}
	return &helmOCIClient{f: f, loc: loc, scheme: "https", tokens: map[string]string{}}, nil
}

func (c *helmOCIClient) url(path string, q url.Values) *url.URL {
	u := &url.URL{Scheme: c.scheme, Host: c.loc.host, Path: path}
	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}
	return u
}

func (c *helmOCIClient) strictFetcher() *HelmFetcher {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.strict == nil {
		c.strict = &HelmFetcher{Policy: HelmFetchPolicy{}, Resolver: c.f.Resolver, UserAgent: c.f.UserAgent, TLSConfig: c.f.TLSConfig, dial: c.f.dial}
	}
	return c.strict
}

// ping checks GET /v2/ and settles the scheme: https, or http when insecure
// repositories are allowed and https cannot be spoken.
func (c *helmOCIClient) ping(ctx context.Context) error {
	schemes := []string{"https"}
	if c.f.Policy.AllowInsecure {
		schemes = append(schemes, "http")
	}
	var firstErr error
	for _, s := range schemes {
		c.scheme = s
		resp, err := c.f.ociRequest(ctx, c.url("/v2/", nil), nil, helmOCIMaxPing, "registry ping", "")
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if HTTPStatus(err) == http.StatusBadGateway {
				continue // transport failure: maybe a plain-http registry
			}
			return err
		}
		switch resp.status {
		case http.StatusOK, http.StatusUnauthorized:
			// 401 is the usual answer of a registry that hands out tokens;
			// requests below fetch one when they need it.
			return nil
		case http.StatusNotFound:
			return NewStatusError(http.StatusBadGateway, fmt.Errorf("%s is not an OCI registry (GET /v2/ returned HTTP 404)", c.loc.host))
		default:
			return NewStatusError(http.StatusBadGateway, fmt.Errorf("registry %s answered GET /v2/ with HTTP %d", c.loc.host, resp.status))
		}
	}
	return firstErr
}

func (c *helmOCIClient) cachedToken(scope string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokens[scope]
}

// fetchToken performs the anonymous half of the token flow: GET realm with
// service and scope, no credentials. A realm on the registry's own host is
// held to the registry's policy; any other host must be public https.
func (c *helmOCIClient) fetchToken(ctx context.Context, ch *helmBearerChallenge, scope string) (string, error) {
	realm, err := url.Parse(ch.realm)
	if err != nil || !realm.IsAbs() || realm.Host == "" {
		return "", NewStatusError(http.StatusBadGateway, errors.New("registry token realm is not an absolute URL"))
	}
	f := c.f
	if !strings.EqualFold(realm.Host, c.loc.host) {
		if !strings.EqualFold(realm.Scheme, "https") {
			return "", NewStatusError(http.StatusBadGateway, fmt.Errorf("registry token realm %s must use https", redactHelmURL(realm)))
		}
		f = c.strictFetcher()
	}
	q := realm.Query()
	if ch.service != "" {
		q.Set("service", ch.service)
	}
	q.Set("scope", scope)
	tu := *realm
	tu.RawQuery = q.Encode()
	resp, err := f.ociRequest(ctx, &tu, nil, helmOCIMaxToken, "registry token response", "")
	if err != nil {
		if HTTPStatus(err) == http.StatusBadRequest {
			return "", NewStatusError(http.StatusBadGateway, fmt.Errorf("registry token realm refused: %w", err))
		}
		return "", err
	}
	if resp.status != http.StatusOK {
		return "", NewStatusError(http.StatusBadGateway, fmt.Errorf("registry token endpoint %s returned HTTP %d (only anonymous access is supported)", redactHelmURL(&tu), resp.status))
	}
	var tok struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(resp.body, &tok); err != nil {
		return "", NewStatusError(http.StatusBadGateway, errors.New("registry token response is not valid JSON"))
	}
	t := tok.Token
	if t == "" {
		t = tok.AccessToken
	}
	if t == "" || strings.IndexFunc(t, func(r rune) bool { return r <= ' ' || r > '~' }) >= 0 {
		return "", NewStatusError(http.StatusBadGateway, errors.New("registry token response carries no usable token"))
	}
	c.mu.Lock()
	c.tokens[scope] = t
	c.mu.Unlock()
	return t, nil
}

// get performs a registry GET, answering a bearer challenge once with an
// anonymous token for scope. The returned response may have any status
// except 401.
func (c *helmOCIClient) get(ctx context.Context, u *url.URL, accept, scope string, max int64, what, digest string) (*helmOCIResponse, error) {
	hdr := func(tok string) http.Header {
		h := http.Header{}
		if accept != "" {
			h.Set("Accept", accept)
		}
		if tok != "" {
			h.Set("Authorization", "Bearer "+tok)
		}
		return h
	}
	tok := c.cachedToken(scope)
	resp, err := c.f.ociRequest(ctx, u, hdr(tok), max, what, digest)
	if err != nil {
		return nil, err
	}
	if resp.status != http.StatusUnauthorized {
		return resp, nil
	}
	ch, other := helmBearerFromHeader(resp.header)
	if ch == nil {
		if other != "" {
			return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("registry %s requires %s credentials: only anonymous registries are supported", c.loc.host, other))
		}
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("registry %s requires credentials: only anonymous registries are supported", c.loc.host))
	}
	newTok, err := c.fetchToken(ctx, ch, scope)
	if err != nil {
		return nil, err
	}
	if newTok == tok {
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("registry %s refused its own anonymous token", c.loc.host))
	}
	resp, err = c.f.ociRequest(ctx, u, hdr(newTok), max, what, digest)
	if err != nil {
		return nil, err
	}
	if resp.status == http.StatusUnauthorized {
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("registry %s denied anonymous access to %s: only anonymous registries are supported", c.loc.host, what))
	}
	return resp, nil
}

// nextLink follows a `Link: <...>; rel="next"` header. The next page must be
// on the same registry and path.
func (c *helmOCIClient) nextLink(cur *url.URL, h http.Header) (*url.URL, bool, error) {
	for _, v := range h.Values("Link") {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if !strings.HasPrefix(part, "<") {
				continue
			}
			end := strings.IndexByte(part, '>')
			if end < 0 {
				continue
			}
			params := strings.ToLower(part[end+1:])
			if !strings.Contains(params, `rel="next"`) && !strings.Contains(params, "rel=next") {
				continue
			}
			ref, err := url.Parse(part[1:end])
			if err != nil {
				return nil, false, NewStatusError(http.StatusBadGateway, errors.New("registry sent an invalid pagination link"))
			}
			next := cur.ResolveReference(ref)
			if next.Scheme != cur.Scheme || !strings.EqualFold(next.Host, cur.Host) || next.Path != cur.Path || next.User != nil {
				return nil, false, NewStatusError(http.StatusBadGateway, errors.New("registry pagination link leaves the listed path: refusing it"))
			}
			return next, true, nil
		}
	}
	return nil, false, nil
}

// list reads a paginated JSON list (tags or catalog). It stops at maxItems
// and refuses more than 20 MiB over all pages.
func (c *helmOCIClient) list(ctx context.Context, path, field, scope, what string, maxItems int) ([]string, int, error) {
	u := c.url(path, url.Values{"n": {strconv.Itoa(helmOCIPageSize)}})
	var items []string
	var used int64
	for page := 0; page < helmOCIMaxPages; page++ {
		resp, err := c.get(ctx, u, "application/json", scope, helmOCIMaxListBytes-used, what, "")
		if err != nil {
			if HTTPStatus(err) == http.StatusRequestEntityTooLarge {
				return nil, 0, NewStatusError(http.StatusRequestEntityTooLarge, fmt.Errorf("%s too large (limit %d MiB)", what, helmOCIMaxListBytes>>20))
			}
			return nil, 0, err
		}
		if resp.status != http.StatusOK {
			return nil, resp.status, nil
		}
		used += int64(len(resp.body))
		var body map[string]json.RawMessage
		if err := json.Unmarshal(resp.body, &body); err != nil {
			return nil, 0, NewStatusError(http.StatusBadGateway, fmt.Errorf("%s is not valid JSON", what))
		}
		var vals []string
		if raw, ok := body[field]; ok && string(raw) != "null" {
			if err := json.Unmarshal(raw, &vals); err != nil {
				return nil, 0, NewStatusError(http.StatusBadGateway, fmt.Errorf("%s is malformed", what))
			}
		}
		items = append(items, vals...)
		if len(items) >= maxItems {
			return items[:maxItems], http.StatusOK, nil
		}
		next, ok, err := c.nextLink(resp.url, resp.header)
		if err != nil {
			return nil, 0, err
		}
		if !ok || len(vals) == 0 {
			break
		}
		u = next
	}
	return items, http.StatusOK, nil
}

func (c *helmOCIClient) repoScope(name string) string { return "repository:" + name + ":pull" }

// tags lists a chart's tags (at most 500).
func (c *helmOCIClient) tags(ctx context.Context, chart string) ([]string, error) {
	name := c.loc.repository(chart)
	tags, status, err := c.list(ctx, "/v2/"+name+"/tags/list", "tags", c.repoScope(name), "tag list of "+chart, helmOCIMaxTags)
	if err != nil {
		return nil, err
	}
	switch status {
	case http.StatusOK:
		return tags, nil
	case http.StatusNotFound:
		return nil, NewStatusError(http.StatusNotFound, fmt.Errorf("chart %q not found in %s", chart, c.loc))
	case http.StatusForbidden:
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("registry denied anonymous access to chart %q", chart))
	default:
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("registry answered the tag list of %q with HTTP %d", chart, status))
	}
}

// catalog lists the charts directly under the namespace via /v2/_catalog,
// which most hosted registries do not offer.
func (c *helmOCIClient) catalog(ctx context.Context) ([]string, error) {
	repos, status, err := c.list(ctx, "/v2/_catalog", "repositories", "registry:catalog:*", "registry catalog", 10000)
	if err != nil {
		if HTTPStatus(err) == http.StatusRequestEntityTooLarge {
			return nil, err
		}
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("could not list the registry catalog (%v): name the charts explicitly with \"charts\"", err))
	}
	if status != http.StatusOK {
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("the registry does not allow listing its catalog (HTTP %d): name the charts explicitly with \"charts\"", status))
	}
	prefix := ""
	if c.loc.path != "" {
		prefix = c.loc.path + "/"
	}
	var out []string
	seen := map[string]bool{}
	for _, r := range repos {
		if !strings.HasPrefix(r, prefix) {
			continue
		}
		name := r[len(prefix):]
		if strings.Contains(name, "/") || len(name) > helmOCIMaxChartName || !helmOCIChartNameRe.MatchString(name) || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	if len(out) > helmOCIMaxCharts {
		out = out[:helmOCIMaxCharts]
	}
	return out, nil
}

type helmOCIDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

type helmOCIManifest struct {
	SchemaVersion int                 `json:"schemaVersion"`
	MediaType     string              `json:"mediaType"`
	Config        helmOCIDescriptor   `json:"config"`
	Layers        []helmOCIDescriptor `json:"layers"`
}

// chartLayer is the chart archive layer of a Helm manifest.
func (m *helmOCIManifest) chartLayer() (helmOCIDescriptor, bool) {
	for _, l := range m.Layers {
		if l.MediaType == helmOCIChartLayer {
			return l, true
		}
	}
	for _, l := range m.Layers {
		if l.MediaType == helmOCIChartLayerOld {
			return l, true
		}
	}
	return helmOCIDescriptor{}, false
}

// manifest fetches and checks the manifest of chart:tag.
func (c *helmOCIClient) manifest(ctx context.Context, chart, tag string) (*helmOCIManifest, error) {
	if !helmOCITagRe.MatchString(tag) {
		return nil, NewStatusError(http.StatusBadRequest, fmt.Errorf("invalid tag %q", tag))
	}
	name := c.loc.repository(chart)
	resp, err := c.get(ctx, c.url("/v2/"+name+"/manifests/"+tag, nil), helmOCIManifestOCI+", "+helmOCIManifestV2, c.repoScope(name), helmOCIMaxManifest, "manifest of "+chart+":"+tag, "")
	if err != nil {
		return nil, err
	}
	switch resp.status {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, NewStatusError(http.StatusNotFound, fmt.Errorf("chart %q has no version %q in the registry", chart, tag))
	default:
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("registry answered the manifest of %s:%s with HTTP %d", chart, tag, resp.status))
	}
	var m helmOCIManifest
	if err := json.Unmarshal(resp.body, &m); err != nil {
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("manifest of %s:%s is not valid JSON", chart, tag))
	}
	mt := m.MediaType
	if mt == "" {
		mt, _, _ = strings.Cut(resp.header.Get("Content-Type"), ";")
		mt = strings.TrimSpace(mt)
	}
	switch mt {
	case helmOCIManifestOCI, helmOCIManifestV2, "":
	default:
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("%s:%s is not a Helm chart (manifest media type %q)", chart, tag, mt))
	}
	if m.Config.MediaType != helmOCIConfigMedia {
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("%s:%s is not a Helm chart (config media type %q)", chart, tag, m.Config.MediaType))
	}
	return &m, nil
}

// blob downloads a blob, verifying its sha256 digest and size while reading.
func (c *helmOCIClient) blob(ctx context.Context, chart string, d helmOCIDescriptor, max int64, what string) ([]byte, error) {
	if !helmOCIDigestRe.MatchString(d.Digest) {
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("%s has an unsupported digest (only sha256 is accepted)", what))
	}
	if d.Size > max {
		return nil, NewStatusError(http.StatusRequestEntityTooLarge, fmt.Errorf("%s too large (limit %s)", what, helmSizeText(max)))
	}
	name := c.loc.repository(chart)
	resp, err := c.get(ctx, c.url("/v2/"+name+"/blobs/"+d.Digest, nil), "", c.repoScope(name), max, what, d.Digest)
	if err != nil {
		if HTTPStatus(err) == http.StatusBadRequest {
			// The registry, not the administrator, chose where the blob lives.
			return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("%s download refused: %w", what, err))
		}
		return nil, err
	}
	if resp.status != http.StatusOK {
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("registry answered the %s download with HTTP %d", what, resp.status))
	}
	if d.Size > 0 && int64(len(resp.body)) != d.Size {
		return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("%s is %d bytes, the manifest says %d: refusing it", what, len(resp.body), d.Size))
	}
	return resp.body, nil
}

// ---------------------------------------------------------------------------
// Catalog

// helmOCIVersions turns tags into index entries: only tags that parse as
// semver are kept ("latest", "main", digests-as-tags are ignored).
func helmOCIVersions(chart string, tags []string) []*helmIndexEntry {
	var out []*helmIndexEntry
	seen := map[string]bool{}
	for _, t := range tags {
		if !helmOCITagRe.MatchString(t) {
			continue
		}
		v := helmOCITagToVersion(t)
		sv := parseSemver(v)
		if !sv.valid || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, &helmIndexEntry{Name: flexString(chart), Version: flexString(v), sv: sv})
	}
	sortHelmVersions(out)
	return out
}

// chartEntry lists one chart's versions and reads Chart.yaml (as JSON) from
// the config blob of the newest non-prerelease version.
func (c *helmOCIClient) chartEntry(ctx context.Context, chart string) ([]*helmIndexEntry, *helmIndexEntry, error) {
	tags, err := c.tags(ctx, chart)
	if err != nil {
		return nil, nil, err
	}
	vs := helmOCIVersions(chart, tags)
	if len(vs) == 0 {
		return nil, nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("chart %q has no semver version tags", chart))
	}
	pick := vs[0]
	for _, e := range vs {
		if len(e.sv.pre) == 0 {
			pick = e
			break
		}
	}
	m, err := c.manifest(ctx, chart, helmOCIVersionToTag(string(pick.Version)))
	if err != nil {
		return nil, nil, err
	}
	raw, err := c.blob(ctx, chart, m.Config, helmOCIMaxConfig, "chart config of "+chart)
	if err != nil {
		return nil, nil, err
	}
	var cfg helmIndexEntry
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("chart config of %s is not valid JSON", chart))
	}
	pick.AppVersion, pick.Description, pick.Icon, pick.Home = cfg.AppVersion, cfg.Description, cfg.Icon, cfg.Home
	pick.Keywords, pick.Sources, pick.Maintainers, pick.Deprecated = cfg.Keywords, cfg.Sources, cfg.Maintainers, cfg.Deprecated
	return vs, pick, nil
}

// FetchOCIIndex builds a catalog index for an OCI repository. With strict
// set (adding a repository) any chart that cannot be read fails the call;
// otherwise unreadable charts are left out and only a registry that yields
// no chart at all is an error.
func (f *HelmFetcher) FetchOCIIndex(ctx context.Context, repo HelmRepo, strict bool) (*helmIndex, error) {
	c, err := newHelmOCIClient(f, repo.URL)
	if err != nil {
		return nil, err
	}
	if err := c.ping(ctx); err != nil {
		return nil, err
	}
	charts := repo.Charts
	if len(charts) == 0 {
		if charts, err = c.catalog(ctx); err != nil {
			return nil, err
		}
		if len(charts) == 0 {
			return nil, NewStatusError(http.StatusBadGateway, fmt.Errorf("the registry catalog lists no charts under %s", c.loc))
		}
	}
	type result struct {
		vs   []*helmIndexEntry
		pick *helmIndexEntry
		err  error
	}
	results := make([]result, len(charts))
	sem := make(chan struct{}, helmOCIConcurrency)
	var wg sync.WaitGroup
	for i, ch := range charts {
		wg.Add(1)
		go func(i int, ch string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = result{err: ctx.Err()}
				return
			}
			defer func() { <-sem }()
			vs, pick, err := c.chartEntry(ctx, ch)
			results[i] = result{vs, pick, err}
		}(i, ch)
	}
	wg.Wait()
	ix := &helmIndex{charts: map[string][]*helmIndexEntry{}, pinned: map[string]*helmIndexEntry{}}
	var firstErr error
	for i, ch := range charts {
		r := results[i]
		if r.err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("chart %q: %w", ch, r.err)
			}
			continue
		}
		ix.charts[ch] = r.vs
		ix.pinned[ch] = r.pick
		ix.names = append(ix.names, ch)
	}
	if firstErr != nil && (strict || len(ix.names) == 0) {
		return nil, firstErr
	}
	sort.Strings(ix.names)
	return ix, nil
}

// FetchOCIChartValues downloads the chart layer of chart:version and returns
// its values.yaml and README.md through the index archive reader.
func (f *HelmFetcher) FetchOCIChartValues(ctx context.Context, repoURL, chart, version string) (string, string, error) {
	c, err := newHelmOCIClient(f, repoURL)
	if err != nil {
		return "", "", err
	}
	if err := c.ping(ctx); err != nil {
		return "", "", err
	}
	m, err := c.manifest(ctx, chart, helmOCIVersionToTag(version))
	if err != nil {
		return "", "", err
	}
	layer, ok := m.chartLayer()
	if !ok {
		return "", "", NewStatusError(http.StatusBadGateway, fmt.Errorf("%s %s has no Helm chart layer", chart, version))
	}
	body, err := c.blob(ctx, chart, layer, helmMaxArchiveBytes, "chart archive")
	if err != nil {
		return "", "", err
	}
	return readHelmChartArchive(body, chart)
}

// fetchRepoIndex fetches a repository's catalog index by kind.
func (c *HelmCatalog) fetchRepoIndex(ctx context.Context, r HelmRepo, strict bool) (*helmIndex, error) {
	if r.kind() == HelmRepoKindOCI {
		return c.fetcher.FetchOCIIndex(ctx, r, strict)
	}
	return c.fetcher.FetchIndex(ctx, r.URL)
}

func helmFetchBudget(r HelmRepo) time.Duration {
	if r.kind() == HelmRepoKindOCI {
		return helmOCIFetchBudget
	}
	return 2 * helmRequestTimeout
}
