package platform

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// AgentAutoRunner watches Warning/CrashLoop signals and creates ProposeOnly runs.
type AgentAutoRunner struct {
	Provider Provider
	Hub      *IntegrationHub
	Demo     bool
	Interval time.Duration

	mu       sync.Mutex
	seen     map[string]time.Time
	stop     chan struct{}
	running  bool
}

func (a *AgentAutoRunner) Start() {
	if a == nil || a.Provider == nil || a.Hub == nil {
		return
	}
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return
	}
	a.running = true
	a.stop = make(chan struct{})
	if a.seen == nil {
		a.seen = map[string]time.Time{}
	}
	if a.Interval <= 0 {
		a.Interval = 60 * time.Second
	}
	a.mu.Unlock()
	go a.loop()
}

func (a *AgentAutoRunner) Stop() {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running && a.stop != nil {
		close(a.stop)
		a.running = false
	}
}

func (a *AgentAutoRunner) loop() {
	t := time.NewTicker(a.Interval)
	defer t.Stop()
	for {
		select {
		case <-a.stop:
			return
		case <-t.C:
			a.tick()
		}
	}
}

func (a *AgentAutoRunner) tick() {
	in := a.Hub.Get()
	if !in.AgentAutoRuns {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	evs, err := a.Provider.ListEvents(ctx, "")
	if err != nil {
		return
	}
	now := time.Now()
	a.mu.Lock()
	for k, t := range a.seen {
		if now.Sub(t) > 2*time.Hour {
			delete(a.seen, k)
		}
	}
	a.mu.Unlock()

	for _, e := range evs {
		if e.Type != "Warning" {
			continue
		}
		low := strings.ToLower(e.Reason + " " + e.Message)
		if !(strings.Contains(low, "crashloop") || strings.Contains(low, "backoff") ||
			strings.Contains(low, "unhealthy") || strings.Contains(low, "failed") ||
			strings.Contains(low, "oomkilled")) {
			continue
		}
		key := e.Namespace + "|" + e.Object + "|" + e.Reason
		a.mu.Lock()
		if _, ok := a.seen[key]; ok {
			a.mu.Unlock()
			continue
		}
		a.seen[key] = now
		a.mu.Unlock()

		name := strings.ToLower(strings.ReplaceAll(e.Reason, " ", "-"))
		if len(name) > 40 {
			name = name[:40]
		}
		req := CreateAgentRunRequest{
			Namespace:    e.Namespace,
			Name:         fmt.Sprintf("auto-%s-%d", name, now.Unix()%100000),
			Goal:         fmt.Sprintf("Investigate Warning %s on %s: %s", e.Reason, e.Object, truncateStr(e.Message, 160)),
			ExtraContext: e.Message,
			Source:       AgentSourceAlert,
		}
		dry := true
		req.DryRun = &dry
		// Best-effort parse "Pod/foo" style object refs.
		if parts := strings.SplitN(e.Object, "/", 2); len(parts) == 2 {
			req.Target = &AgentObjectRef{Kind: parts[0], Name: parts[1], Namespace: e.Namespace, APIVersion: "v1"}
		}
		run, err := a.Provider.CreateAgentRun(ctx, req)
		if err != nil {
			continue
		}
		planned, _ := PlanAgentRun(ctx, a.Provider, in, a.Demo, run, e.Message)
		_, _ = a.Provider.UpdateAgentRun(ctx, planned)
		RecordAxiomAudit("agentrun.create", run.Namespace+"/"+run.Name, "auto:"+e.Reason, "agent-auto")
	}
}
