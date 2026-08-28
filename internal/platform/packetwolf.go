package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	pwStatusCache     *PacketWolfStatus
	pwStatusCacheTime time.Time
	pwStatusMu        sync.Mutex
)

func packetWolfBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("AXIOM_PACKETWOLF_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return ""
}

func packetWolfAPIKey() string {
	return strings.TrimSpace(os.Getenv("AXIOM_PACKETWOLF_API_KEY"))
}

func FetchPacketWolfStatus(ctx context.Context) PacketWolfStatus {
	pwStatusMu.Lock()
	defer pwStatusMu.Unlock()
	if pwStatusCache != nil && time.Since(pwStatusCacheTime) < 10*time.Second {
		return *pwStatusCache
	}
	base := packetWolfBaseURL()
	if base == "" {
		st := PacketWolfStatus{Configured: false, Message: "Set AXIOM_PACKETWOLF_URL to enable PacketWolf integration"}
		pwStatusCache = &st
		pwStatusCacheTime = time.Now()
		return st
	}
	st := PacketWolfStatus{Configured: true, BaseURL: base}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/health", nil)
	if err != nil {
		st.Message = err.Error()
		pwStatusCache = &st
		pwStatusCacheTime = time.Now()
		return st
	}
	if key := packetWolfAPIKey(); key != "" {
		req.Header.Set("X-API-Key", key)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		st.Message = err.Error()
		pwStatusCache = &st
		pwStatusCacheTime = time.Now()
		return st
	}
	defer resp.Body.Close()
	st.Reachable = resp.StatusCode >= 200 && resp.StatusCode < 300
	if st.Reachable {
		st.Message = "Connected"
		var body map[string]any
		if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body) == nil {
			if subs, ok := body["subsystems"].(map[string]any); ok {
				st.Subsystems = map[string]string{}
				for k, v := range subs {
					st.Subsystems[k] = fmt.Sprint(v)
				}
			}
		}
	} else {
		st.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	pwStatusCache = &st
	pwStatusCacheTime = time.Now()
	return st
}

func PacketWolfProxy(ctx context.Context, path string, query string) ([]byte, int, error) {
	base := packetWolfBaseURL()
	if base == "" {
		return nil, 503, fmt.Errorf("PacketWolf not configured")
	}
	url := base + "/" + strings.TrimPrefix(path, "/")
	if query != "" {
		url += "?" + query
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 500, err
	}
	if key := packetWolfAPIKey(); key != "" {
		req.Header.Set("X-API-Key", key)
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, 502, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, 502, err
	}
	return body, resp.StatusCode, nil
}

func workloadNetworkGuardFromPW(ctx context.Context, ns, name string) (NetworkGuardSummary, error) {
	st := FetchPacketWolfStatus(ctx)
	if !st.Configured {
		return NetworkGuardSummary{Configured: false, Message: st.Message}, nil
	}
	summary := NetworkGuardSummary{
		Configured: true,
		Reachable:  st.Reachable,
		Message:    st.Message,
		PolicyHint: "Use Isolate (NetworkPolicy) for immediate containment; PacketWolf enriches flow visibility when connected.",
	}
	if !st.Reachable {
		return summary, nil
	}
	path := fmt.Sprintf("api/v1/network-brain/workloads/%s/%s", ns, name)
	body, code, err := PacketWolfProxy(ctx, path, "")
	if err != nil || code >= 400 {
		return summary, nil
	}
	var parsed map[string]any
	if json.Unmarshal(body, &parsed) == nil {
		if flows, ok := parsed["flows"].(float64); ok {
			summary.FlowsCount = int(flows)
		}
	}
	return summary, nil
}
