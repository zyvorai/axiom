package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestSendSlackMessage(t *testing.T) {
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res := SendSlackMessage(context.Background(), srv.URL, "hello from axiom")
	if !res.OK || res.Error != "" {
		t.Fatalf("expected ok result, got %+v", res)
	}
	if received["text"] != "hello from axiom" {
		t.Fatalf("expected slack payload text, got %v", received)
	}
}

func TestSendSlackMessageNotConfigured(t *testing.T) {
	res := SendSlackMessage(context.Background(), "", "hi")
	if res.OK || res.Error == "" {
		t.Fatalf("expected error result for empty webhook, got %+v", res)
	}
}

func TestSendAlertWebhook(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res := SendAlertWebhook(context.Background(), srv.URL, map[string]any{"reason": "BackOff"})
	if !res.OK || res.Error != "" {
		t.Fatalf("expected ok result, got %+v", res)
	}
	if received["reason"] != "BackOff" {
		t.Fatalf("expected webhook payload reason, got %v", received)
	}
}

func TestDispatchEventSendsToBothChannels(t *testing.T) {
	var mu sync.Mutex
	var slackHits, webhookHits int
	slack := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		slackHits++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer slack.Close()
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		webhookHits++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer webhook.Close()

	in := Integrations{SlackWebhookURL: slack.URL, AlertWebhookURL: webhook.URL, NotifyOnWarnings: true}
	ev := Event{Namespace: "default", Type: "Warning", Reason: "BackOff", Object: "Pod/x", Message: "boom", Time: time.Now()}
	dispatchEvent(context.Background(), in, ev)

	mu.Lock()
	defer mu.Unlock()
	if slackHits != 1 || webhookHits != 1 {
		t.Fatalf("expected 1 hit each, got slack=%d webhook=%d", slackHits, webhookHits)
	}
}

type fakeAlertProvider struct {
	Provider
	reason string
}

func (f *fakeAlertProvider) ListAlerts(ctx context.Context, ns string) (AlertsResponse, error) {
	return AlertsResponse{Warnings: []Event{{Namespace: "default", Type: "Warning", Reason: f.reason, Object: "Pod/x", Message: "boom", Time: time.Now()}}}, nil
}

func TestRunAlertDispatcherSkipsWhenDisabled(t *testing.T) {
	var hits int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hub := NewIntegrationHub(Integrations{AlertWebhookURL: srv.URL, NotifyOnWarnings: false}, nil)
	provider := &fakeAlertProvider{reason: "BackOff"}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		runAlertDispatcher(ctx, provider, hub, 20*time.Millisecond)
		close(done)
	}()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if hits != 0 {
		t.Fatalf("expected no dispatch while notifyOnWarnings is false, got %d hits", hits)
	}
}

func TestRunAlertDispatcherFiresWhenEnabled(t *testing.T) {
	var hits int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hub := NewIntegrationHub(Integrations{AlertWebhookURL: srv.URL, NotifyOnWarnings: true}, nil)
	provider := &fakeAlertProvider{reason: "BackOff"}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		runAlertDispatcher(ctx, provider, hub, 20*time.Millisecond)
		close(done)
	}()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if hits == 0 {
		t.Fatalf("expected at least one dispatch while notifyOnWarnings is true, got 0 hits")
	}
}
