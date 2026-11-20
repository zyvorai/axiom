package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ConnectionTestResult is returned by Settings → Test connection actions.
type ConnectionTestResult struct {
	OK        bool   `json:"ok"`
	Service   string `json:"service"`
	LatencyMS int64  `json:"latencyMs,omitempty"`
	Detail    any    `json:"detail,omitempty"`
	Error     string `json:"error,omitempty"`
	Hint      string `json:"hint,omitempty"`
}

func TestAtlasConnection(ctx context.Context, atlas *AtlasClient) ConnectionTestResult {
	start := time.Now()
	if atlas == nil || !atlas.Enabled() {
		return ConnectionTestResult{
			Service: "atlas",
			Error:   "Atlas gateway URL is not configured",
			Hint:    "Set Gateway URL under Atlas storage, save, then test again.",
		}
	}
	raw, code, err := atlas.Get(ctx, "version")
	ms := time.Since(start).Milliseconds()
	if err != nil {
		return ConnectionTestResult{
			Service:   "atlas",
			LatencyMS: ms,
			Error:     err.Error(),
			Hint:      "Check URL, token, and that the gateway listens on /version (not the SPA).",
		}
	}
	var detail any
	_ = json.Unmarshal(raw, &detail)
	return ConnectionTestResult{
		OK: true, Service: "atlas", LatencyMS: ms,
		Detail: map[string]any{"status": code, "version": detail},
	}
}

func TestOIDCConnection(ctx context.Context, issuer string) ConnectionTestResult {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	if issuer == "" {
		return ConnectionTestResult{
			Service: "oidc",
			Error:   "OIDC issuer URL is not configured",
			Hint:    "Set Issuer URL under OIDC / Haven, save, then test.",
		}
	}
	wellKnown := issuer + "/.well-known/openid-configuration"
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return ConnectionTestResult{Service: "oidc", Error: err.Error()}
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		return ConnectionTestResult{
			Service: "oidc", LatencyMS: ms, Error: err.Error(),
			Hint: "Issuer must be reachable from the Axiom pod (cluster DNS or NodePort).",
		}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode >= 400 {
		return ConnectionTestResult{
			Service: "oidc", LatencyMS: ms,
			Error: fmt.Sprintf("HTTP %d from %s", resp.StatusCode, wellKnown),
			Hint:  "Confirm realm path, e.g. …/realms/platform",
		}
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return ConnectionTestResult{
			Service: "oidc", LatencyMS: ms,
			Error: "Issuer responded but not with OpenID discovery JSON",
		}
	}
	return ConnectionTestResult{
		OK: true, Service: "oidc", LatencyMS: ms,
		Detail: map[string]any{
			"issuer":           doc["issuer"],
			"authorization_endpoint": doc["authorization_endpoint"],
			"token_endpoint":       doc["token_endpoint"],
		},
	}
}

func TestPostgresConnection(ctx context.Context, in Integrations) ConnectionTestResult {
	host := strings.TrimSpace(in.PostgresHost)
	if host == "" {
		return ConnectionTestResult{
			Service: "postgres",
			Error:   "Postgres host is not configured",
			Hint:    "Use the CNPG read-write service, e.g. platform-db-rw.identity.svc",
		}
	}
	port := in.PostgresPort
	if port <= 0 {
		port = 5432
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	d := net.Dialer{Timeout: 8 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		return ConnectionTestResult{
			Service: "postgres", LatencyMS: ms, Error: err.Error(),
			Hint: "Verify service name, namespace, and that CNPG pods are Ready.",
		}
	}
	_ = conn.Close()

	user := strings.TrimSpace(in.PostgresUser)
	pass := in.PostgresPassword
	db := strings.TrimSpace(in.PostgresDatabase)
	if db == "" {
		db = "postgres"
	}
	if user == "" {
		return ConnectionTestResult{
			OK: true, Service: "postgres", LatencyMS: ms,
			Detail: map[string]any{"tcp": addr, "auth": "skipped (no user set)"},
			Hint:   "TCP OK. Add user/password to verify login.",
		}
	}
	auth, err := postgresSelect1(ctx, addr, user, pass, db, in.PostgresSSLMode)
	ms = time.Since(start).Milliseconds()
	if err != nil {
		return ConnectionTestResult{
			Service: "postgres", LatencyMS: ms,
			Detail: map[string]any{"tcp": addr},
			Error:  err.Error(),
			Hint:   "Check credentials secret (CNPG app user) and database name.",
		}
	}
	return ConnectionTestResult{
		OK: true, Service: "postgres", LatencyMS: ms,
		Detail: map[string]any{"tcp": addr, "auth": auth},
	}
}

func postgresSelect1(ctx context.Context, addr, user, pass, db, sslMode string) (string, error) {
	if sslMode == "" {
		sslMode = "disable"
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, pass),
		Host:   addr,
		Path:   db,
	}
	q := u.Query()
	q.Set("sslmode", sslMode)
	q.Set("connect_timeout", "5")
	u.RawQuery = q.Encode()

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, u.String())
	if err != nil {
		return "", err
	}
	defer conn.Close(ctx)
	var one int
	if err := conn.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return "", err
	}
	return fmt.Sprintf("SELECT 1 → %d", one), nil
}

// TestPodProbe curls/wgets a URL from inside a pod (or from Axiom when pod unset).
func TestPodProbe(ctx context.Context, provider Provider, in Integrations) ConnectionTestResult {
	target := strings.TrimSpace(in.ProbeURL)
	if target == "" {
		return ConnectionTestResult{
			Service: "probe",
			Error:   "Probe URL is not configured",
			Hint:    "Set a URL to fetch — Atlas /version, Keycloak health, or any HTTP endpoint.",
		}
	}
	ns := strings.TrimSpace(in.ProbePodNamespace)
	pod := strings.TrimSpace(in.ProbePodName)
	start := time.Now()
	if ns != "" && pod != "" {
		out, err := provider.ExecCommand(ctx, ns, pod, in.ProbePodContainer, probeShellCmd(target))
		ms := time.Since(start).Milliseconds()
		if err != nil {
			return ConnectionTestResult{
				Service: "probe", LatencyMS: ms,
				Error: err.Error(),
				Hint:  "Pod needs curl or wget. Try a debug pod (curlimages/curl) in the target namespace.",
			}
		}
		return parseProbeOutput(out, ms, map[string]any{"from": ns + "/" + pod, "url": target})
	}
	return testHTTPFromAxiom(ctx, target, start)
}

func testHTTPFromAxiom(ctx context.Context, target string, start time.Time) ConnectionTestResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ConnectionTestResult{Service: "probe", Error: err.Error()}
	}
	req.Header.Set("Accept", "application/json, text/html, */*")
	resp, err := http.DefaultClient.Do(req)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		return ConnectionTestResult{
			Service: "probe", LatencyMS: ms, Error: err.Error(),
			Hint: "URL must resolve from the Axiom pod (cluster DNS).",
		}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	preview := strings.TrimSpace(string(body))
	if len(preview) > 120 {
		preview = preview[:120] + "…"
	}
	ok := resp.StatusCode >= 200 && resp.StatusCode < 400
	res := ConnectionTestResult{
		OK: ok, Service: "probe", LatencyMS: ms,
		Detail: map[string]any{
			"from": "axiom-pod", "url": target,
			"status": resp.StatusCode, "preview": preview,
		},
	}
	if !ok {
		res.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return res
}

func probeShellCmd(url string) []string {
	escaped := strings.ReplaceAll(url, "'", "'\\''")
	script := fmt.Sprintf(
		"if command -v curl >/dev/null 2>&1; then curl -fsS -m 8 -w '\\n__CODE__:%%{http_code}' '%s'; "+
			"elif command -v wget >/dev/null 2>&1; then wget -qO- -T 8 '%s' && echo '\\n__CODE__:200'; "+
			"else echo '__ERR__:no curl or wget' >&2; exit 127; fi",
		escaped, escaped,
	)
	return []string{"/bin/sh", "-c", script}
}

func parseProbeOutput(out string, ms int64, meta map[string]any) ConnectionTestResult {
	code := 0
	if i := strings.LastIndex(out, "__CODE__:"); i >= 0 {
		fmt.Sscanf(out[i+9:], "%d", &code)
		out = strings.TrimSpace(out[:i])
	}
	if strings.Contains(out, "__ERR__:") {
		return ConnectionTestResult{
			Service: "probe", LatencyMS: ms, Error: strings.TrimPrefix(out, "__ERR__:"),
			Detail: meta,
		}
	}
	preview := out
	if len(preview) > 120 {
		preview = preview[:120] + "…"
	}
	meta["status"] = code
	meta["preview"] = preview
	ok := code >= 200 && code < 400
	res := ConnectionTestResult{Service: "probe", LatencyMS: ms, Detail: meta, OK: ok}
	if !ok && code > 0 {
		res.Error = fmt.Sprintf("HTTP %d from pod", code)
	} else if !ok {
		res.OK = true // body returned without code marker
	}
	return res
}

// IntegrationExample is a copy-paste template for Settings.
type IntegrationExample struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Values      map[string]string `json:"values"`
}

func IntegrationExamples() []IntegrationExample {
	return []IntegrationExample{
		{
			ID: "atlas-incluster", Title: "Atlas — in-cluster gateway",
			Description: "Axiom reaches Atlas via cluster DNS. Use your Atlas Service host and port from the install.",
			Values: map[string]string{
				"atlasUrl":        "http://atlas-gateway.zyvor-system.svc:<atlas-port>",
				"atlasConsoleUrl": "https://atlas.example.com",
			},
		},
		{
			ID: "atlas-ingress", Title: "Atlas — Ingress / DNS",
			Description: "Browser console and API via your Ingress or load balancer (no NodePort).",
			Values: map[string]string{
				"atlasUrl":        "https://atlas-api.example.com",
				"atlasConsoleUrl": "https://atlas.example.com",
				"probeUrl":        "https://atlas-api.example.com/version",
			},
		},
		{
			ID: "haven-postgres", Title: "Haven — CNPG Postgres",
			Description: "CloudNativePG read-write service for platform-db (identity ns).",
			Values: map[string]string{
				"postgresHost":     "platform-db-rw.identity.svc",
				"postgresPort":     "5432",
				"postgresDatabase": "platform",
				"postgresUser":     "platform",
				"postgresSslMode":  "disable",
			},
		},
		{
			ID: "haven-oidc", Title: "Haven — Keycloak OIDC",
			Description: "Keycloak realm platform; use your auth Ingress hostname (HTTPS, no lab NodePort).",
			Values: map[string]string{
				"oidcIssuer":   "https://auth.example.com/realms/platform",
				"oidcLoginUrl": "https://auth.example.com/realms/platform/protocol/openid-connect/auth",
				"oidcClientId": "axiom",
			},
		},
		{
			ID: "probe-from-pod", Title: "Pod probe — curl from a debug pod",
			Description: "Run wget/curl inside an existing pod (e.g. curlimages/curl) to test routing.",
			Values: map[string]string{
				"probePodNamespace": "axiom-system",
				"probePodName":      "curl-debug",
				"probeUrl":          "http://atlas-gateway.zyvor-system.svc:<atlas-port>/version",
			},
		},
		{
			ID: "probe-from-axiom", Title: "Cluster probe — from Axiom pod",
			Description: "HTTP GET from the Axiom pod itself (leave pod fields empty).",
			Values: map[string]string{
				"probeUrl": "http://atlas-gateway.zyvor-system.svc:<atlas-port>/version",
			},
		},
	}
}
