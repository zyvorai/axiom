package platform

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

const testRegistryPassword = "Reg-PASS-7c1e-NEVER-SHOWN"

func addTestCred(t *testing.T, s HelmRegistryCredStore, name, registry, user, pass string) {
	t.Helper()
	if _, err := s.Add(context.Background(), HelmRegistryCredRequest{Name: name, Registry: registry, Username: user, Password: pass}); err != nil {
		t.Fatalf("add %s: %v", name, err)
	}
}

func basicOf(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func TestHelmRegistryHostValidation(t *testing.T) {
	for in, want := range map[string]string{
		"ghcr.io": "ghcr.io", "Registry.Example.com:5000": "registry.example.com:5000", " quay.io ": "quay.io",
		"10.0.0.5:5000": "10.0.0.5:5000", "[::1]:5000": "[::1]:5000",
	} {
		got, err := NormalizeHelmRegistryHost(in)
		if err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	for _, in := range []string{"", "https://ghcr.io", "ghcr.io/org", "user:pw@ghcr.io", "oci://ghcr.io", "ghcr.io:0", "ghcr.io:99999", "ghcr io", "ghcr.io?x=1", "ghcr.io#f", "-bad.io", "ghcr.io:", "bad_host.io"} {
		if _, err := NormalizeHelmRegistryHost(in); err == nil || HTTPStatus(err) != 400 {
			t.Errorf("%q accepted (%v)", in, err)
		}
	}
}

func TestHelmRegistryCredValidationNeverEchoesSecrets(t *testing.T) {
	s := NewMemoryHelmRegistryCredStore()
	ctx := context.Background()
	ok := HelmRegistryCredRequest{Name: "ghcr", Registry: "ghcr.io", Username: "bot", Password: testRegistryPassword}
	cases := map[string]func(r *HelmRegistryCredRequest){
		"bad name":         func(r *HelmRegistryCredRequest) { r.Name = "Bad_Name" },
		"long name":        func(r *HelmRegistryCredRequest) { r.Name = strings.Repeat("a", 64) },
		"no registry":      func(r *HelmRegistryCredRequest) { r.Registry = "" },
		"scheme":           func(r *HelmRegistryCredRequest) { r.Registry = "https://ghcr.io" },
		"path":             func(r *HelmRegistryCredRequest) { r.Registry = "ghcr.io/org" },
		"userinfo":         func(r *HelmRegistryCredRequest) { r.Registry = "bot:" + testRegistryPassword + "@ghcr.io" },
		"no user":          func(r *HelmRegistryCredRequest) { r.Username = "" },
		"long user":        func(r *HelmRegistryCredRequest) { r.Username = strings.Repeat("u", 257) },
		"colon user":       func(r *HelmRegistryCredRequest) { r.Username = "a:b" },
		"control user":     func(r *HelmRegistryCredRequest) { r.Username = "a\x01" },
		"no password":      func(r *HelmRegistryCredRequest) { r.Password = "" },
		"long password":    func(r *HelmRegistryCredRequest) { r.Password = testRegistryPassword + strings.Repeat("p", 4096) },
		"newline password": func(r *HelmRegistryCredRequest) { r.Password = testRegistryPassword + "\n" },
	}
	for name, mut := range cases {
		r := ok
		mut(&r)
		_, err := s.Add(ctx, r)
		if err == nil || HTTPStatus(err) != 400 {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if strings.Contains(err.Error(), testRegistryPassword) {
			t.Errorf("%s: error leaks the password: %v", name, err)
		}
	}
	if got := fmt.Sprintf("%v %+v %#v %s", ok, ok, ok, ok); strings.Contains(got, testRegistryPassword) {
		t.Fatalf("request formatting leaks: %s", got)
	}
	c := helmRegistryCred{Registry: "ghcr.io", Username: "bot", Password: testRegistryPassword}
	if got := fmt.Sprintf("%v %+v %#v", c, c, &c); strings.Contains(got, testRegistryPassword) {
		t.Fatalf("record formatting leaks: %s", got)
	}
	if len(ok.Password) > helmRegistryCredMaxPassword {
		t.Fatal("test password too long")
	}
	// 4 KiB exactly is accepted.
	big := ok
	big.Name, big.Password = "big", strings.Repeat("p", helmRegistryCredMaxPassword)
	if _, err := s.Add(ctx, big); err != nil {
		t.Fatalf("4 KiB password: %v", err)
	}
}

func TestMemoryHelmRegistryCredStore(t *testing.T) {
	s := NewMemoryHelmRegistryCredStore()
	ctx := context.Background()
	info, err := s.Add(ctx, HelmRegistryCredRequest{Name: "ghcr", Registry: "GHCR.io", Username: "bot", Password: testRegistryPassword})
	if err != nil || info.Registry != "ghcr.io" || info.Name != "ghcr" || info.Username != "bot" || info.CreatedAt == "" {
		t.Fatalf("add: %+v %v", info, err)
	}
	// Never overwritten.
	_, err = s.Add(ctx, HelmRegistryCredRequest{Name: "ghcr", Registry: "quay.io", Username: "x", Password: "other"})
	if HTTPStatus(err) != 409 {
		t.Fatalf("dup: %v", err)
	}
	if c, _ := s.get(ctx, "ghcr"); c.Registry != "ghcr.io" || c.Password != testRegistryPassword {
		t.Fatalf("dup overwrote: %v", c)
	}
	list, _ := s.List(ctx)
	b, _ := json.Marshal(list)
	if len(list) != 1 || strings.Contains(string(b), testRegistryPassword) {
		t.Fatalf("list: %s", b)
	}
	if _, err := s.get(ctx, "nope"); HTTPStatus(err) != 404 {
		t.Fatalf("get missing: %v", err)
	}
	if _, err := s.Delete(ctx, "Bad Name"); HTTPStatus(err) != 400 {
		t.Fatalf("delete bad name: %v", err)
	}
	if gone, err := s.Delete(ctx, "ghcr"); err != nil || gone.Registry != "ghcr.io" {
		t.Fatalf("delete: %+v %v", gone, err)
	}
	if _, err := s.Delete(ctx, "ghcr"); HTTPStatus(err) != 404 {
		t.Fatalf("delete twice: %v", err)
	}
	for i := 0; i < helmRegistryCredMaxCount; i++ {
		addTestCred(t, s, fmt.Sprintf("c%d", i), "ghcr.io", "u", "p")
	}
	if _, err := s.Add(ctx, HelmRegistryCredRequest{Name: "over", Registry: "ghcr.io", Username: "u", Password: "p"}); HTTPStatus(err) != 400 {
		t.Fatalf("cap: %v", err)
	}
}

func TestHelmRegistryCredListAndDeleteInUse(t *testing.T) {
	ctx := context.Background()
	creds := NewMemoryHelmRegistryCredStore()
	repos := NewMemoryHelmRepoStore()
	addTestCred(t, creds, "ghcr", "ghcr.io", "bot", testRegistryPassword)
	addTestCred(t, creds, "quay", "quay.io", "bot", testRegistryPassword)
	_ = repos.Add(ctx, HelmRepo{Name: "org-charts", URL: "oci://ghcr.io/org/charts", Kind: "oci", CredentialsSecret: "ghcr"})
	_ = repos.Add(ctx, HelmRepo{Name: "plain", URL: "oci://quay.io/x", Kind: "oci"})
	rows, err := ListHelmRegistryCreds(ctx, creds, repos)
	if err != nil || len(rows) != 2 {
		t.Fatalf("list: %+v %v", rows, err)
	}
	if rows[0].Name != "ghcr" || strings.Join(rows[0].InUse, ",") != "org-charts" || rows[1].InUse == nil || len(rows[1].InUse) != 0 || rows[0].Age == "" {
		t.Fatalf("rows: %+v", rows)
	}
	b, _ := json.Marshal(rows)
	if strings.Contains(string(b), testRegistryPassword) || strings.Contains(string(b), "password") {
		t.Fatalf("list leaks: %s", b)
	}
	_, err = DeleteHelmRegistryCred(ctx, creds, repos, "ghcr")
	if HTTPStatus(err) != 409 || !strings.Contains(err.Error(), "remove or edit repository org-charts first") {
		t.Fatalf("in use: %v", err)
	}
	if _, err := DeleteHelmRegistryCred(ctx, creds, repos, "quay"); err != nil {
		t.Fatal(err)
	}
	// Deleting the repository does not delete the credential.
	_ = repos.Remove(ctx, "org-charts")
	if _, err := creds.get(ctx, "ghcr"); err != nil {
		t.Fatalf("credential went with the repository: %v", err)
	}
	if _, err := DeleteHelmRegistryCred(ctx, creds, repos, "ghcr"); err != nil {
		t.Fatal(err)
	}
}

func TestHelmCredAge(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]string{
		"": "-", "garbage": "-", fmtTime(now.Add(-10 * time.Second)): "now", fmtTime(now.Add(-5 * time.Minute)): "5m",
		fmtTime(now.Add(-3 * time.Hour)): "3h", fmtTime(now.Add(-50 * time.Hour)): "2d",
	} {
		if got := helmCredAge(in, now); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestHelmOCIMatchRepoLongestPrefix(t *testing.T) {
	repos := []HelmRepo{
		{Name: "root", URL: "oci://ghcr.io", Kind: "oci"},
		{Name: "org", URL: "oci://ghcr.io/org", Kind: "oci"},
		{Name: "org-charts", URL: "oci://ghcr.io/org/charts", Kind: "oci"},
		{Name: "other-port", URL: "oci://ghcr.io:5000/org/charts", Kind: "oci"},
		{Name: "index", URL: "https://ghcr.io/org/charts/deep", Kind: "index"},
	}
	for ref, want := range map[string]string{
		"oci://ghcr.io/org/charts/nginx":      "org-charts",
		"oci://ghcr.io/org/charts/deep/nginx": "org-charts",
		"oci://ghcr.io/org/chartsx/nginx":     "org", // component boundary, not a string prefix
		"oci://ghcr.io/org/nginx":             "org",
		"oci://ghcr.io/nginx":                 "root",
		"oci://GHCR.io/org/charts/nginx":      "org-charts",
		"oci://ghcr.io:5000/org/charts/nginx": "other-port",
		"oci://quay.io/org/charts/nginx":      "",
		"oci://ghcr.io:5000/nginx":            "",
		"not a ref":                           "",
	} {
		r, ok := helmOCIMatchRepo(repos, ref)
		if (want == "") == ok || r.Name != want {
			t.Errorf("%s: %q %v, want %q", ref, r.Name, ok, want)
		}
	}
}

func TestResolveHelmOCIAccess(t *testing.T) {
	ctx := context.Background()
	creds := NewMemoryHelmRegistryCredStore()
	repos := NewMemoryHelmRepoStore()
	addTestCred(t, creds, "ghcr", "ghcr.io", "bot", testRegistryPassword)
	addTestCred(t, creds, "quay", "quay.io", "bot", "pw")
	_ = repos.Add(ctx, HelmRepo{Name: "org", URL: "oci://ghcr.io/org", Kind: "oci"})
	_ = repos.Add(ctx, HelmRepo{Name: "private", URL: "oci://ghcr.io/org/private", Kind: "oci", CredentialsSecret: "ghcr"})
	_ = repos.Add(ctx, HelmRepo{Name: "lab", URL: "oci://registry.lab:5000/charts", Kind: "oci", PlainHTTP: true})
	_ = repos.Add(ctx, HelmRepo{Name: "mismatch", URL: "oci://ghcr.io/mismatch", Kind: "oci", CredentialsSecret: "quay"})
	_ = repos.Add(ctx, HelmRepo{Name: "dangling", URL: "oci://ghcr.io/dangling", Kind: "oci", CredentialsSecret: "gone"})

	a, err := ResolveHelmOCIAccess(ctx, repos, creds, "oci://ghcr.io/org/nginx")
	if err != nil || a != nil {
		t.Fatalf("anonymous https repository must resolve to nil: %v %v", a, err)
	}
	if a, err := ResolveHelmOCIAccess(ctx, repos, creds, "oci://docker.io/x/y"); err != nil || a != nil {
		t.Fatalf("no repository: %v %v", a, err)
	}
	a, err = ResolveHelmOCIAccess(ctx, repos, creds, "oci://ghcr.io/org/private/nginx")
	if err != nil || a == nil || !a.authenticated() || a.PlainHTTP() || a.Repo() != "private" || a.CredentialName() != "ghcr" || a.registry != "ghcr.io" {
		t.Fatalf("private: %v %v", a, err)
	}
	if s := fmt.Sprintf("%v %+v %#v %s", a, a, a, a); strings.Contains(s, testRegistryPassword) {
		t.Fatalf("access formatting leaks: %s", s)
	}
	if b, _ := json.Marshal(InstallHelmRequest{OCIRef: "oci://ghcr.io/org/private/nginx", OCIAccess: a}); strings.Contains(string(b), testRegistryPassword) || strings.Contains(string(b), "ociAccess") {
		t.Fatalf("request JSON carries the access: %s", b)
	}
	var decoded InstallHelmRequest
	_ = json.Unmarshal([]byte(`{"ociRef":"oci://x/y/z","OCIAccess":{"password":"x"}}`), &decoded)
	if decoded.OCIAccess != nil {
		t.Fatal("OCIAccess must not be decodable from a request")
	}
	cfg, _ := a.dockerConfig()
	var shape struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(cfg, &shape); err != nil || len(shape.Auths) != 1 {
		t.Fatalf("config.json: %v", err)
	}
	if got, _ := base64.StdEncoding.DecodeString(shape.Auths["ghcr.io"].Auth); string(got) != "bot:"+testRegistryPassword {
		t.Fatalf("config.json auth: %q", got)
	}

	a, err = ResolveHelmOCIAccess(ctx, repos, creds, "oci://registry.lab:5000/charts/app")
	if err != nil || a == nil || !a.PlainHTTP() || a.authenticated() {
		t.Fatalf("plain http: %v %v", a, err)
	}
	if _, err := ResolveHelmOCIAccess(ctx, repos, creds, "oci://ghcr.io/mismatch/x"); HTTPStatus(err) != 409 || !strings.Contains(err.Error(), "is for registry quay.io") {
		t.Fatalf("mismatch: %v", err)
	}
	if _, err := ResolveHelmOCIAccess(ctx, repos, creds, "oci://ghcr.io/dangling/x"); HTTPStatus(err) != 409 || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("dangling: %v", err)
	}
	var nilAccess *HelmOCIAccess
	if nilAccess.PlainHTTP() || nilAccess.authenticated() || nilAccess.CredentialName() != "" || nilAccess.String() == "" {
		t.Fatal("nil access helpers")
	}
}

func TestHelmCredentialRealmAllowed(t *testing.T) {
	for _, c := range []struct {
		realm, registry string
		want            bool
	}{
		{"https://ghcr.io/token", "ghcr.io", true},
		{"https://GHCR.io/token", "ghcr.io", true},
		{"https://auth.docker.io/token", "registry-1.docker.io", true},
		{"https://gitlab.com/jwt/auth", "registry.gitlab.com", true},
		{"https://registry.example.com:8443/token", "registry.example.com:5000", true},
		{"http://127.0.0.1:5000/token", "127.0.0.1:5000", true},
		{"https://evil.example.org/token", "registry.example.com", false},
		{"https://example.com.evil.org/token", "example.com", false},
		{"https://a.co.uk/token", "b.co.uk", false},
		{"https://foo.github.io/token", "bar.github.io", false}, // a private suffix in the PSL
		{"https://127.0.0.2/token", "127.0.0.1", false},
		{"https://localhost/token", "registry.localhost", false},
	} {
		u, _ := url.Parse(c.realm)
		if got := helmCredentialRealmAllowed(u, c.registry); got != c.want {
			t.Errorf("%s for %s: %v, want %v", c.realm, c.registry, got, c.want)
		}
	}
}

func TestHelmTokenTTL(t *testing.T) {
	for in, want := range map[int64]time.Duration{0: time.Minute, -5: time.Minute, 30: 30 * time.Second, 300: 5 * time.Minute, 3600: 5 * time.Minute} {
		if got := helmTokenTTL(in); got != want {
			t.Errorf("%d: %v", in, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Registry client with credentials

func credCatalog(t *testing.T, f *HelmFetcher) (*HelmCatalog, *MemoryHelmRegistryCredStore) {
	t.Helper()
	c := newOCICatalog(t, f)
	creds := NewMemoryHelmRegistryCredStore()
	c.SetRegistryCredStore(creds)
	return c, creds
}

func regHost(rg *ociReg) string {
	u, _ := url.Parse(rg.URL)
	return u.Host
}

func TestHelmOCIBearerWithCredentials(t *testing.T) {
	rg := standardOCIReg(t, false)
	rg.set(func() {
		rg.requireToken, rg.tokenUser, rg.tokenPass, rg.tokenExpiresIn = true, "bot", testRegistryPassword, 120
	})
	clk := &testClock{t: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)}
	f := &HelmFetcher{Policy: openPolicy, now: clk.Now}
	c, creds := credCatalog(t, f)
	ctx := context.Background()

	// Without credentials: a clear pointer to the fix.
	_, err := c.AddHelmRepo(ctx, HelmRepoRequest{Name: "anon", URL: rg.ociURL("/charts"), Charts: []string{"nginx"}})
	if HTTPStatus(err) != 400 || !strings.Contains(err.Error(), helmOCINeedsCredentials) {
		t.Fatalf("anonymous: %v", err)
	}
	addTestCred(t, creds, "reg", regHost(rg), "bot", testRegistryPassword)
	addTestCred(t, creds, "wrong", regHost(rg), "bot", "not-the-password")
	addTestCred(t, creds, "elsewhere", "ghcr.io", "bot", testRegistryPassword)

	if _, err := c.AddHelmRepo(ctx, HelmRepoRequest{Name: "x1", URL: rg.ociURL("/charts"), Charts: []string{"nginx"}, CredentialsSecret: "elsewhere"}); HTTPStatus(err) != 400 || !strings.Contains(err.Error(), "credentials are for registry ghcr.io") {
		t.Fatalf("other registry: %v", err)
	}
	if _, err := c.AddHelmRepo(ctx, HelmRepoRequest{Name: "x2", URL: rg.ociURL("/charts"), Charts: []string{"nginx"}, CredentialsSecret: "missing"}); HTTPStatus(err) != 400 || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing credential: %v", err)
	}
	if _, err := c.AddHelmRepo(ctx, HelmRepoRequest{Name: "x3", URL: "https://charts.example.com", CredentialsSecret: "reg"}); HTTPStatus(err) != 400 || !strings.Contains(err.Error(), "only to OCI") {
		t.Fatalf("index repository with credentials: %v", err)
	}
	_, err = c.AddHelmRepo(ctx, HelmRepoRequest{Name: "bad", URL: rg.ociURL("/charts"), Charts: []string{"nginx"}, CredentialsSecret: "wrong"})
	if HTTPStatus(err) != 400 || !strings.Contains(err.Error(), "rejected the registry credentials") {
		t.Fatalf("wrong password: %v", err)
	}
	noLeak(t, err)
	if strings.Contains(err.Error(), "not-the-password") {
		t.Fatalf("error leaks the password: %v", err)
	}

	rg.set(func() { rg.scopes, rg.tokenAuth = nil, nil })
	v, err := c.AddHelmRepo(ctx, HelmRepoRequest{Name: "priv", URL: rg.ociURL("/charts"), Charts: []string{"nginx"}, CredentialsSecret: "reg"})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Authenticated || v.CredentialsSecret != "reg" || !v.PlainHTTP || v.ChartCount != 1 {
		t.Fatalf("view: %+v", v)
	}
	rg.set(func() {
		if len(rg.tokenAuth) != 1 || rg.tokenAuth[0] != basicOf("bot", testRegistryPassword) {
			t.Errorf("token requests: %q", rg.tokenAuth)
		}
		for _, a := range rg.v2Auth {
			if strings.HasPrefix(a, "Basic") {
				t.Errorf("the registry itself only gets the bearer token, saw %q", a)
			}
		}
		rg.tokenAuth = nil
	})
	// Cached: the next fetch within expires_in uses the same token.
	if _, err := c.ChartValues(ctx, "priv", "nginx", "1.2.0"); err != nil {
		t.Fatal(err)
	}
	rg.set(func() {
		if len(rg.tokenAuth) != 0 {
			t.Errorf("token was not reused: %d requests", len(rg.tokenAuth))
		}
	})
	// After expiry it is fetched again.
	clk.Add(121 * time.Second)
	if _, err := c.ChartValues(ctx, "priv", "nginx", "1.2.0"); err != nil {
		t.Fatal(err)
	}
	rg.set(func() {
		if len(rg.tokenAuth) != 1 {
			t.Errorf("token was not refetched after expiry: %d requests", len(rg.tokenAuth))
		}
	})
	// Cache keys hold a hash, never the password or the token.
	f.tokMu.Lock()
	for k, tok := range f.tokCache {
		if strings.Contains(k, testRegistryPassword) || strings.Contains(k, tok.token) || strings.Contains(k, "bot") {
			t.Errorf("cache key carries secrets: %q", k)
		}
	}
	f.tokMu.Unlock()
	// The credential disappeared (out of band): a clear error, nothing leaks.
	creds.mu.Lock()
	delete(creds.creds, "reg")
	creds.mu.Unlock()
	_, err = c.ChartValues(ctx, "priv", "nginx", "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("deleted credential: %v", err)
	}
}

func TestHelmOCIBasicChallengeWithCredentials(t *testing.T) {
	rg := standardOCIReg(t, false)
	rg.set(func() { rg.basicUser, rg.basicPass, rg.blobRedirect = "bot", testRegistryPassword, true })
	c, creds := credCatalog(t, &HelmFetcher{Policy: openPolicy})
	ctx := context.Background()
	if _, err := c.AddHelmRepo(ctx, HelmRepoRequest{Name: "anon", URL: rg.ociURL("/charts"), Charts: []string{"nginx"}}); HTTPStatus(err) != 400 || !strings.Contains(err.Error(), helmOCINeedsCredentials) {
		t.Fatalf("anonymous: %v", err)
	}
	addTestCred(t, creds, "reg", regHost(rg), "bot", testRegistryPassword)
	addTestCred(t, creds, "wrong", regHost(rg), "bot", "nope-nope")
	_, err := c.AddHelmRepo(ctx, HelmRepoRequest{Name: "bad", URL: rg.ociURL("/charts"), Charts: []string{"nginx"}, CredentialsSecret: "wrong"})
	if HTTPStatus(err) != 400 || !strings.Contains(err.Error(), "rejected the registry credentials") || strings.Contains(err.Error(), "nope-nope") {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err := c.AddHelmRepo(ctx, HelmRepoRequest{Name: "priv", URL: rg.ociURL("/charts"), Charts: []string{"nginx"}, CredentialsSecret: "reg"}); err != nil {
		t.Fatal(err)
	}
	vals, err := c.ChartValues(ctx, "priv", "nginx", "1.0.0")
	if err != nil || vals.Values != "replicaCount: 3\n" {
		t.Fatalf("values: %+v %v", vals, err)
	}
	rg.set(func() {
		if len(rg.storageAuth) == 0 {
			t.Error("blob redirect was not followed")
		}
		for _, a := range rg.storageAuth {
			if a != "" {
				t.Errorf("credentials followed a blob redirect: %q", a)
			}
		}
		if len(rg.tokenAuth) != 0 {
			t.Error("a Basic registry must not see token requests")
		}
	})
}

// TestHelmOCICredentialsStayOnTheRegistryHost: a token realm on another site
// and blob redirects to another host never see the credential.
func TestHelmOCICredentialsStayOnTheRegistryHost(t *testing.T) {
	ctx := context.Background()
	rg := standardOCIReg(t, true)
	target := rg.Listener.Addr().String()
	res := fakeResolver{
		"registry.example.com": {"93.184.216.34"}, "auth.example.com": {"93.184.216.35"},
		"auth.example.org": {"93.184.216.36"}, "cdn.example.net": {"93.184.216.37"},
	}
	fetcher := func() *HelmFetcher {
		f := routedFetcher(HelmFetchPolicy{}, res, target, nil)
		// One test certificate stands in for every host name.
		f.TLSConfig = &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12} //nolint:gosec // test server
		return f
	}
	creds := NewMemoryHelmRegistryCredStore()
	addTestCred(t, creds, "reg", "registry.example.com", "bot", testRegistryPassword)
	cred, _ := creds.get(ctx, "reg")
	repo := HelmRepo{URL: "oci://registry.example.com/charts", Kind: "oci", Charts: []string{"nginx"}}
	rg.set(func() { rg.requireToken, rg.tokenUser, rg.tokenPass = true, "bot", testRegistryPassword })

	t.Run("realm on another site is refused", func(t *testing.T) {
		rg.set(func() { rg.realm, rg.tokenAuth = "https://auth.example.org/token", nil })
		_, _, err := fetcher().fetchOCIIndex(ctx, repo, &cred, true)
		if err == nil || !strings.Contains(err.Error(), "refusing to send the registry credentials there") {
			t.Fatalf("want refusal, got %v", err)
		}
		rg.set(func() {
			if len(rg.tokenAuth) != 0 {
				t.Errorf("the other site was contacted: %q", rg.tokenAuth)
			}
		})
	})
	t.Run("realm on the same registrable domain gets the credential", func(t *testing.T) {
		rg.set(func() { rg.realm, rg.tokenAuth = "https://auth.example.com/token", nil })
		if _, _, err := fetcher().fetchOCIIndex(ctx, repo, &cred, true); err != nil {
			t.Fatal(err)
		}
		rg.set(func() {
			if len(rg.tokenAuth) == 0 || rg.tokenAuth[0] != basicOf("bot", testRegistryPassword) {
				t.Errorf("token request: %q", rg.tokenAuth)
			}
		})
	})
	t.Run("blob redirect to another host carries nothing", func(t *testing.T) {
		rg.set(func() { rg.realm, rg.redirectTo, rg.storageAuth = "", "https://cdn.example.net", nil })
		defer rg.set(func() { rg.redirectTo = "" })
		if _, _, err := fetcher().fetchOCIChartValues(ctx, repo.URL, &cred, "nginx", "1.2.0"); err != nil {
			t.Fatal(err)
		}
		rg.set(func() {
			if len(rg.storageAuth) == 0 {
				t.Fatal("redirect not followed")
			}
			for _, a := range rg.storageAuth {
				if a != "" {
					t.Errorf("Authorization carried to the redirect host: %q", a)
				}
			}
		})
	})
	t.Run("credential for another registry is refused by the client", func(t *testing.T) {
		other := cred
		other.Registry = "ghcr.io"
		if _, _, err := fetcher().fetchOCIIndex(ctx, repo, &other, true); err == nil || !strings.Contains(err.Error(), "is for registry ghcr.io") {
			t.Fatalf("want refusal, got %v", err)
		}
	})
}

func TestHelmOCIPlainHTTPRecorded(t *testing.T) {
	ctx := context.Background()
	plain := standardOCIReg(t, false)
	c := newOCICatalog(t, &HelmFetcher{Policy: openPolicy})
	v, err := c.AddHelmRepo(ctx, HelmRepoRequest{Name: "lab", URL: plain.ociURL("/charts"), Charts: []string{"nginx"}})
	if err != nil || !v.PlainHTTP || v.Authenticated || v.CredentialsSecret != "" {
		t.Fatalf("plain http: %+v %v", v, err)
	}
	stored, _ := c.store.List(ctx)
	if len(stored) != 1 || !stored[0].PlainHTTP {
		t.Fatalf("stored: %+v", stored)
	}
	secure := standardOCIReg(t, true)
	f := &HelmFetcher{Policy: openPolicy}
	trustServer(f, secure.Server)
	c2 := newOCICatalog(t, f)
	v, err = c2.AddHelmRepo(ctx, HelmRepoRequest{Name: "tls", URL: secure.ociURL("/charts"), Charts: []string{"nginx"}})
	if err != nil || v.PlainHTTP {
		t.Fatalf("https: %+v %v", v, err)
	}
	b, _ := json.Marshal(HelmRepo{Name: "x", URL: "oci://a/b", Kind: "oci"})
	if strings.Contains(string(b), "plainHttp") {
		t.Fatalf("plainHttp must be omitempty: %s", b)
	}
}
