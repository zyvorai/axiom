package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// AlertmanagerClient talks to Prometheus Alertmanager API v2
// (AXIOM_ALERTMANAGER_URL). Same hardened HTTP rules as Prometheus.
type AlertmanagerClient struct {
	base    *url.URL
	token   string
	http    *http.Client
	timeout time.Duration
}

var (
	globalAMMu sync.Mutex
	globalAM   *AlertmanagerClient
)

// ConfigureAlertmanager stores the process-wide AM client (nil clears).
func ConfigureAlertmanager(c *AlertmanagerClient) {
	globalAMMu.Lock()
	defer globalAMMu.Unlock()
	globalAM = c
}

// AlertmanagerFromEnv returns the configured client, or nil.
func AlertmanagerFromEnv() *AlertmanagerClient {
	globalAMMu.Lock()
	defer globalAMMu.Unlock()
	return globalAM
}

// NewAlertmanagerClient validates rawURL. Empty URL → nil,nil.
func NewAlertmanagerClient(rawURL, token string) (*AlertmanagerClient, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("AXIOM_ALERTMANAGER_URL is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("AXIOM_ALERTMANAGER_URL must use http or https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("AXIOM_ALERTMANAGER_URL must include a host")
	}
	if u.User != nil {
		return nil, fmt.Errorf("AXIOM_ALERTMANAGER_URL must not embed credentials; use AXIOM_ALERTMANAGER_TOKEN")
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimRight(u.Path, "/")
	c := &AlertmanagerClient{base: u, token: strings.TrimSpace(token), timeout: 10 * time.Second}
	c.http = newOperatorHTTPClient(u, c.timeout)
	return c, nil
}

// FiringAlert is one Alertmanager alert.
type FiringAlert struct {
	Fingerprint string            `json:"fingerprint"`
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt,omitempty"`
	EndsAt      string            `json:"endsAt,omitempty"`
	GeneratorURL string           `json:"generatorURL,omitempty"`
	Summary     string            `json:"summary,omitempty"`
	Severity    string            `json:"severity,omitempty"`
	AlertName   string            `json:"alertname,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Pod         string            `json:"pod,omitempty"`
}

// AlertSilence is an Alertmanager silence.
type AlertSilence struct {
	ID        string              `json:"id,omitempty"`
	Status    string              `json:"status,omitempty"`
	CreatedBy string              `json:"createdBy,omitempty"`
	Comment   string              `json:"comment,omitempty"`
	StartsAt  string              `json:"startsAt,omitempty"`
	EndsAt    string              `json:"endsAt,omitempty"`
	Matchers  []SilenceMatcher    `json:"matchers,omitempty"`
}

type SilenceMatcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	IsEqual *bool  `json:"isEqual,omitempty"`
}

type CreateSilenceRequest struct {
	Matchers  []SilenceMatcher `json:"matchers"`
	StartsAt  string           `json:"startsAt,omitempty"`
	EndsAt    string           `json:"endsAt,omitempty"`
	CreatedBy string           `json:"createdBy,omitempty"`
	Comment   string           `json:"comment,omitempty"`
	Hours     int              `json:"hours,omitempty"` // convenience: duration from now
}

func (c *AlertmanagerClient) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	if c == nil {
		return nil, 0, fmt.Errorf("Alertmanager is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	u := *c.base
	u.Path = c.base.Path + path
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rdr)
	if err != nil {
		return nil, 0, fmt.Errorf("could not build the Alertmanager request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "zyvor-axiom")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("Alertmanager is unreachable")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading Alertmanager response failed")
	}
	return raw, resp.StatusCode, nil
}

func (c *AlertmanagerClient) ListFiring(ctx context.Context) ([]FiringAlert, error) {
	raw, code, err := c.do(ctx, http.MethodGet, "/api/v2/alerts?active=true&silenced=false&inhibited=false", nil)
	if err != nil {
		return nil, err
	}
	if code < 200 || code > 299 {
		return nil, fmt.Errorf("Alertmanager returned HTTP %d", code)
	}
	var parsed []struct {
		Fingerprint string            `json:"fingerprint"`
		Status      struct{ State string `json:"state"` } `json:"status"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
		StartsAt    time.Time         `json:"startsAt"`
		EndsAt      time.Time         `json:"endsAt"`
		GeneratorURL string           `json:"generatorURL"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("Alertmanager returned a malformed response")
	}
	out := make([]FiringAlert, 0, len(parsed))
	for _, a := range parsed {
		fa := FiringAlert{
			Fingerprint:  a.Fingerprint,
			Status:       a.Status.State,
			Labels:       a.Labels,
			Annotations:  a.Annotations,
			StartsAt:     a.StartsAt.UTC().Format(time.RFC3339),
			EndsAt:       a.EndsAt.UTC().Format(time.RFC3339),
			GeneratorURL: a.GeneratorURL,
		}
		if a.Labels != nil {
			fa.AlertName = a.Labels["alertname"]
			fa.Severity = a.Labels["severity"]
			fa.Namespace = a.Labels["namespace"]
			fa.Pod = a.Labels["pod"]
		}
		if a.Annotations != nil {
			fa.Summary = firstNonEmpty(a.Annotations["summary"], a.Annotations["description"], a.Annotations["message"])
		}
		out = append(out, fa)
	}
	return out, nil
}

func (c *AlertmanagerClient) ListSilences(ctx context.Context) ([]AlertSilence, error) {
	raw, code, err := c.do(ctx, http.MethodGet, "/api/v2/silences", nil)
	if err != nil {
		return nil, err
	}
	if code < 200 || code > 299 {
		return nil, fmt.Errorf("Alertmanager returned HTTP %d", code)
	}
	var parsed []struct {
		ID     string `json:"id"`
		Status struct{ State string `json:"state"` } `json:"status"`
		CreatedBy string `json:"createdBy"`
		Comment   string `json:"comment"`
		StartsAt  time.Time `json:"startsAt"`
		EndsAt    time.Time `json:"endsAt"`
		Matchers  []SilenceMatcher `json:"matchers"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("Alertmanager returned a malformed response")
	}
	out := make([]AlertSilence, 0, len(parsed))
	for _, s := range parsed {
		out = append(out, AlertSilence{
			ID: s.ID, Status: s.Status.State, CreatedBy: s.CreatedBy, Comment: s.Comment,
			StartsAt: s.StartsAt.UTC().Format(time.RFC3339),
			EndsAt:   s.EndsAt.UTC().Format(time.RFC3339),
			Matchers: s.Matchers,
		})
	}
	return out, nil
}

func (c *AlertmanagerClient) CreateSilence(ctx context.Context, req CreateSilenceRequest) (AlertSilence, error) {
	if len(req.Matchers) == 0 {
		return AlertSilence{}, NewStatusError(http.StatusBadRequest, fmt.Errorf("at least one matcher is required"))
	}
	now := time.Now().UTC()
	starts := now
	ends := now.Add(2 * time.Hour)
	if req.Hours > 0 && req.Hours <= 168 {
		ends = now.Add(time.Duration(req.Hours) * time.Hour)
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(req.StartsAt)); err == nil {
		starts = t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(req.EndsAt)); err == nil {
		ends = t.UTC()
	}
	if !ends.After(starts) {
		return AlertSilence{}, NewStatusError(http.StatusBadRequest, fmt.Errorf("endsAt must be after startsAt"))
	}
	createdBy := strings.TrimSpace(req.CreatedBy)
	if createdBy == "" {
		createdBy = "zyvor-axiom"
	}
	comment := strings.TrimSpace(req.Comment)
	if comment == "" {
		comment = "silenced from Axiom"
	}
	body := map[string]any{
		"matchers":  req.Matchers,
		"startsAt":  starts.Format(time.RFC3339Nano),
		"endsAt":    ends.Format(time.RFC3339Nano),
		"createdBy": createdBy,
		"comment":   comment,
	}
	raw, code, err := c.do(ctx, http.MethodPost, "/api/v2/silences", body)
	if err != nil {
		return AlertSilence{}, err
	}
	if code < 200 || code > 299 {
		return AlertSilence{}, fmt.Errorf("Alertmanager returned HTTP %d: %s", code, truncateStr(string(raw), 200))
	}
	var resp struct {
		SilenceID string `json:"silenceID"`
	}
	_ = json.Unmarshal(raw, &resp)
	return AlertSilence{
		ID: resp.SilenceID, Status: "pending", CreatedBy: createdBy, Comment: comment,
		StartsAt: starts.Format(time.RFC3339), EndsAt: ends.Format(time.RFC3339), Matchers: req.Matchers,
	}, nil
}

func (c *AlertmanagerClient) ExpireSilence(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return NewStatusError(http.StatusBadRequest, fmt.Errorf("silence id required"))
	}
	_, code, err := c.do(ctx, http.MethodDelete, "/api/v2/silence/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	if code == 404 {
		return NewStatusError(http.StatusNotFound, fmt.Errorf("silence %s not found", id))
	}
	if code < 200 || code > 299 {
		return fmt.Errorf("Alertmanager returned HTTP %d", code)
	}
	return nil
}

// InitAlertmanagerFromEnv is called from main.
func InitAlertmanagerFromEnv() error {
	c, err := NewAlertmanagerClient(os.Getenv("AXIOM_ALERTMANAGER_URL"), os.Getenv("AXIOM_ALERTMANAGER_TOKEN"))
	if err != nil {
		return err
	}
	ConfigureAlertmanager(c)
	return nil
}
