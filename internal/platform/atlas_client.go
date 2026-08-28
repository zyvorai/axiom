package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AtlasClient talks to the Zyvor Atlas storage control plane (../atlas).
type AtlasClient struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func NewAtlasClient(baseURL, token string) *AtlasClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	return &AtlasClient{
		BaseURL: baseURL,
		Token:   strings.TrimSpace(token),
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *AtlasClient) Enabled() bool { return c != nil && c.BaseURL != "" }

// AtlasSummary is the Volumes page fabric deck.
type AtlasSummary struct {
	Available      bool            `json:"available"`
	Reason         string          `json:"reason,omitempty"`
	BaseURL        string          `json:"baseURL,omitempty"`
	ConsoleURL     string          `json:"consoleURL,omitempty"`
	Version        json.RawMessage `json:"version,omitempty"`
	Backends       json.RawMessage `json:"backends,omitempty"`
	Pools          json.RawMessage `json:"pools,omitempty"`
	Volumes        json.RawMessage `json:"volumes,omitempty"`
	Policies       json.RawMessage `json:"policies,omitempty"`
	Buckets        json.RawMessage `json:"buckets,omitempty"`
	StorageClasses json.RawMessage `json:"storageClasses,omitempty"`
	Metrics        json.RawMessage `json:"metrics,omitempty"`
}

// atlasURL builds the request URL. Meta endpoints (health/version/livez/readyz)
// live at the gateway root; everything else is under /api/atlas/v1.
func (c *AtlasClient) atlasURL(path string) string {
	path = strings.Trim(path, "/")
	switch path {
	case "health", "version", "livez", "readyz", "metrics":
		return c.BaseURL + "/" + path
	default:
		return c.BaseURL + "/api/atlas/v1/" + path
	}
}

func (c *AtlasClient) Get(ctx context.Context, path string) (json.RawMessage, int, error) {
	if !c.Enabled() {
		return nil, 0, fmt.Errorf("atlas not configured")
	}
	url := c.atlasURL(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		return body, resp.StatusCode, fmt.Errorf("atlas %s: %s", resp.Status, truncateBytes(body, 240))
	}
	if len(body) == 0 {
		body = []byte("{}")
	}
	if !json.Valid(body) {
		return nil, resp.StatusCode, fmt.Errorf("atlas %s: non-JSON response (SPA fallback?)", path)
	}
	return json.RawMessage(body), resp.StatusCode, nil
}

func (c *AtlasClient) Summary(ctx context.Context) AtlasSummary {
	out := AtlasSummary{BaseURL: c.BaseURL}
	if !c.Enabled() {
		out.Reason = "Configure Atlas in Settings → Integrations"
		return out
	}
	paths := []struct {
		path string
		dest *json.RawMessage
	}{
		{"version", &out.Version},
		{"backends/summary", &out.Backends},
		{"pools", &out.Pools},
		{"volumes", &out.Volumes},
		{"policies", &out.Policies},
		{"buckets", &out.Buckets},
		{"storage-classes", &out.StorageClasses},
		{"metrics/summary", &out.Metrics},
	}
	var firstErr string
	ok := 0
	for _, p := range paths {
		raw, _, err := c.Get(ctx, p.path)
		if err != nil {
			if firstErr == "" {
				firstErr = err.Error()
			}
			continue
		}
		*p.dest = raw
		ok++
	}
	if ok == 0 {
		out.Reason = firstErr
		if out.Reason == "" {
			out.Reason = "Atlas unreachable"
		}
		return out
	}
	out.Available = true
	if firstErr != "" {
		out.Reason = "partial: " + firstErr
	}
	return out
}

// WithConsole attaches a browser-reachable Atlas console URL (from Settings).
func (s AtlasSummary) WithConsole(url string) AtlasSummary {
	s.ConsoleURL = strings.TrimRight(strings.TrimSpace(url), "/")
	return s
}

// CreateAtlasVolumeRequest mirrors Atlas POST /volumes (intent-based).
type CreateAtlasVolumeRequest struct {
	Name      string `json:"name"`
	Policy    string `json:"policy"` // production|database|development|shared|ai
	SizeGi    int    `json:"size_gi"`
	Tenant    string `json:"tenant,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	CreatePVC bool   `json:"create_pvc"`
}

func (c *AtlasClient) CreateVolume(ctx context.Context, req CreateAtlasVolumeRequest) (json.RawMessage, int, error) {
	if !c.Enabled() {
		return nil, 0, fmt.Errorf("atlas not configured")
	}
	if req.Name == "" || req.Policy == "" {
		return nil, 0, fmt.Errorf("name and policy are required")
	}
	if req.SizeGi <= 0 {
		req.SizeGi = 10
	}
	ns := strings.TrimSpace(req.Namespace)
	if ns == "" {
		ns = "default"
	}
	payload := map[string]any{
		"name":       req.Name,
		"policy":     req.Policy,
		"size_bytes": int64(req.SizeGi) * 1024 * 1024 * 1024,
		"kind":       "block",
		"owner": map[string]string{
			"product": "axiom",
		},
		"kubernetes": map[string]any{
			"create_pvc": req.CreatePVC,
			"namespace":  ns,
		},
	}
	if req.Policy == "shared" {
		payload["kind"] = "filesystem"
	}
	if req.Tenant != "" {
		payload["tenant_id"] = req.Tenant
	}
	body, _ := json.Marshal(payload)
	url := c.BaseURL + "/api/atlas/v1/volumes"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if c.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		return raw, resp.StatusCode, fmt.Errorf("atlas create volume: %s", truncateBytes(raw, 240))
	}
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if !json.Valid(raw) {
		return nil, resp.StatusCode, fmt.Errorf("atlas create volume: non-JSON response")
	}
	return json.RawMessage(raw), resp.StatusCode, nil
}

func truncateBytes(b []byte, n int) string {
	s := string(b)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func DemoAtlasSummary() AtlasSummary {
	return AtlasSummary{
		Available: true,
		Reason:    "demo Atlas fabric",
		BaseURL:   "demo://atlas",
		Backends:  json.RawMessage(`[{"id":"ceph-lab","kind":"ceph","healthy":true}]`),
		Pools:     json.RawMessage(`[{"name":"rbd-prod","backend":"ceph-lab","type":"rbd"}]`),
		Policies:  json.RawMessage(`[{"intent":"production","storage_class":"zyvor-rbd-prod"},{"intent":"shared","storage_class":"zyvor-cephfs-shared"}]`),
		Volumes:   json.RawMessage(`[]`),
		Buckets:   json.RawMessage(`[]`),
		StorageClasses: json.RawMessage(`[{"name":"zyvor-rbd-prod"},{"name":"zyvor-cephfs-shared"}]`),
	}
}
