package platform

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	agentRunConfigMapName = "zyvor-axiom-agent-runs"
	agentRunConfigMapKey  = "runs.json"
	agentRunMaxStored     = 200
)

// MemoryAgentRunStore is the demo / non-kubernetes persistence for agent runs.
type MemoryAgentRunStore struct {
	mu   sync.Mutex
	runs []AgentRun
}

func NewMemoryAgentRunStore() *MemoryAgentRunStore {
	return &MemoryAgentRunStore{runs: []AgentRun{}}
}

func (s *MemoryAgentRunStore) List(_ context.Context, ns string) ([]AgentRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AgentRun, 0, len(s.runs))
	for _, r := range s.runs {
		if ns == "" || ns == "all" || r.Namespace == ns {
			cp := r
			agentRunDecideFlags(&cp)
			out = append(out, cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, nil
}

func (s *MemoryAgentRunStore) Get(_ context.Context, ns, name string) (AgentRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.runs {
		if r.Namespace == ns && r.Name == name {
			cp := r
			agentRunDecideFlags(&cp)
			return cp, nil
		}
	}
	return AgentRun{}, NewStatusError(http.StatusNotFound, fmt.Errorf("agent run %s/%s not found", ns, name))
}

func (s *MemoryAgentRunStore) Create(_ context.Context, run AgentRun) (AgentRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.runs {
		if existing.Namespace == run.Namespace && existing.Name == run.Name {
			return AgentRun{}, NewStatusError(http.StatusConflict, fmt.Errorf("agent run %s/%s already exists", run.Namespace, run.Name))
		}
	}
	agentRunDecideFlags(&run)
	s.runs = append(s.runs, run)
	if len(s.runs) > agentRunMaxStored {
		s.runs = s.runs[len(s.runs)-agentRunMaxStored:]
	}
	return run, nil
}

func (s *MemoryAgentRunStore) Update(_ context.Context, run AgentRun) (AgentRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.runs {
		if existing.Namespace == run.Namespace && existing.Name == run.Name {
			agentRunDecideFlags(&run)
			s.runs[i] = run
			return run, nil
		}
	}
	return AgentRun{}, NewStatusError(http.StatusNotFound, fmt.Errorf("agent run %s/%s not found", run.Namespace, run.Name))
}

func newStoredAgentRun(req CreateAgentRunRequest) AgentRun {
	ns := strings.TrimSpace(req.Namespace)
	if ns == "" {
		ns = "default"
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fmt.Sprintf("run-%d", time.Now().UnixNano()%1_000_000_000)
	}
	dryRun := true
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}
	src := strings.TrimSpace(req.Source)
	if src == "" {
		src = AgentSourceManual
	}
	now := time.Now().UTC()
	run := AgentRun{
		Namespace:    ns,
		Name:         name,
		Goal:         strings.TrimSpace(req.Goal),
		Autonomy:     "ProposeOnly",
		DryRun:       dryRun,
		Target:       req.Target,
		Phase:        "Proposed",
		Approval:     "Pending",
		Risk:         "low",
		Age:          "now",
		OwnedByAxiom: true,
		Source:       src,
		CreatedAt:    now.Format(time.RFC3339),
		Message:      strings.TrimSpace(req.ExtraContext),
	}
	agentRunDecideFlags(&run)
	return run
}
