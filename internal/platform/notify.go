package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	neturl "net/url"
	"strings"
	"time"
)

func postJSON(ctx context.Context, url string, payload any) (int, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid webhook URL (%s)", scrubURLError(err, url))
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// *url.Error embeds the full request URL; for a Slack incoming
		// webhook that URL is the credential, so never surface it.
		return 0, fmt.Errorf("request to %s failed: %s", redactedHost(url), scrubURLError(err, url))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("HTTP %d from %s", resp.StatusCode, redactedHost(url))
	}
	return resp.StatusCode, nil
}

// SendSlackMessage posts a plain-text message to a Slack incoming webhook URL.
func SendSlackMessage(ctx context.Context, webhookURL, text string) ConnectionTestResult {
	start := time.Now()
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return ConnectionTestResult{Service: "slack", Error: "Slack webhook URL is not configured"}
	}
	code, err := postJSON(ctx, webhookURL, map[string]string{"text": text})
	ms := time.Since(start).Milliseconds()
	if err != nil {
		return ConnectionTestResult{Service: "slack", LatencyMS: ms, Error: err.Error(), Hint: "Verify the incoming webhook URL is still active in Slack."}
	}
	return ConnectionTestResult{OK: true, Service: "slack", LatencyMS: ms, Detail: map[string]any{"status": code}}
}

// SendAlertWebhook posts a structured JSON alert payload to a generic webhook URL.
func SendAlertWebhook(ctx context.Context, webhookURL string, payload map[string]any) ConnectionTestResult {
	start := time.Now()
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return ConnectionTestResult{Service: "webhook", Error: "Alert webhook URL is not configured"}
	}
	code, err := postJSON(ctx, webhookURL, payload)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		return ConnectionTestResult{Service: "webhook", LatencyMS: ms, Error: err.Error(), Hint: "Confirm the endpoint accepts POST with a JSON body."}
	}
	return ConnectionTestResult{OK: true, Service: "webhook", LatencyMS: ms, Detail: map[string]any{"status": code}}
}

func dispatchEvent(ctx context.Context, in Integrations, ev Event) {
	if strings.TrimSpace(in.SlackWebhookURL) != "" {
		text := fmt.Sprintf(":warning: *%s* in `%s`\n%s (%s) — %s", ev.Reason, ev.Namespace, ev.Object, ev.Type, ev.Message)
		if res := SendSlackMessage(ctx, in.SlackWebhookURL, text); res.Error != "" {
			log.Printf("notify: slack dispatch failed: %s", res.Error)
		}
	}
	if strings.TrimSpace(in.AlertWebhookURL) != "" {
		payload := map[string]any{
			"source":    "zyvor-axiom",
			"namespace": ev.Namespace,
			"object":    ev.Object,
			"type":      ev.Type,
			"reason":    ev.Reason,
			"message":   ev.Message,
			"count":     ev.Count,
			"time":      ev.Time,
		}
		if res := SendAlertWebhook(ctx, in.AlertWebhookURL, payload); res.Error != "" {
			log.Printf("notify: webhook dispatch failed: %s", res.Error)
		}
	}
}

// RunAlertDispatcher polls for new Warning events and forwards them to
// configured Slack/webhook channels. It runs until ctx is cancelled and
// never panics the caller — dispatch errors are logged, not fatal.
func RunAlertDispatcher(ctx context.Context, provider Provider, hub *IntegrationHub) {
	runAlertDispatcher(ctx, provider, hub, 60*time.Second)
}

func runAlertDispatcher(ctx context.Context, provider Provider, hub *IntegrationHub, interval time.Duration) {
	if provider == nil || hub == nil {
		return
	}
	watermark := time.Now()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		in := hub.Get()
		if !in.NotifyOnWarnings || (strings.TrimSpace(in.SlackWebhookURL) == "" && strings.TrimSpace(in.AlertWebhookURL) == "") {
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("notify: dispatcher recovered from panic: %v", r)
				}
			}()
			alerts, err := provider.ListAlerts(ctx, "all")
			if err != nil {
				log.Printf("notify: list alerts failed: %v", err)
				return
			}
			next := watermark
			for _, ev := range alerts.Warnings {
				if !ev.Time.After(watermark) {
					continue
				}
				dispatchEvent(ctx, in, ev)
				if ev.Time.After(next) {
					next = ev.Time
				}
			}
			watermark = next
		}()
	}
}

// redactedHost reduces a webhook URL to scheme://host so errors and logs never
// carry the secret path/query that authenticates it.
func redactedHost(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil || u.Host == "" {
		return "the configured endpoint"
	}
	return u.Scheme + "://" + u.Host
}

// scrubURLError returns err's text with every occurrence of the URL removed.
func scrubURLError(err error, raw string) string {
	msg := err.Error()
	if raw != "" {
		msg = strings.ReplaceAll(msg, raw, redactedHost(raw))
	}
	if ue, ok := err.(*neturl.Error); ok {
		msg = strings.ReplaceAll(msg, ue.URL, redactedHost(raw))
	}
	return msg
}
