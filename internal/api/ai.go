package api

// Opt-in "Explain with AI" endpoints. See internal/platform/ai.go for the
// privacy model. Summary: disabled unless an admin enables it in Settings,
// context is built only for the one namespace the auth guard authorized via
// ?namespace=, secrets/env values are never read and the payload is redacted,
// and the audit trail records the target only (never the prompt or answer).

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zyvorai/axiom/internal/platform"
)

const (
	aiSendPerMinute    = 10 // real sends (data leaves the cluster)
	aiPreviewPerMinute = 30 // previews (cluster reads only)
)

// aiRateLimiter is a small per-user sliding-window limiter. The zero value is
// ready to use, so Server needs no constructor change.
type aiRateLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
	now  func() time.Time // injectable for tests
}

// allow records a hit for key and reports whether it is within limit per
// window. When refused it returns how long to wait.
func (l *aiRateLimiter) allow(key string, limit int, window time.Duration) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = map[string][]time.Time{}
	}
	now := time.Now()
	if l.now != nil {
		now = l.now()
	}
	cutoff := now.Add(-window)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		l.hits[key] = kept
		return false, kept[0].Add(window).Sub(now)
	}
	l.hits[key] = append(kept, now)
	// Bound memory: drop idle keys opportunistically.
	if len(l.hits) > 5000 {
		for k, v := range l.hits {
			if len(v) == 0 || !v[len(v)-1].After(cutoff) {
				delete(l.hits, k)
			}
		}
	}
	return true, 0
}

func (s *Server) aiSettings() platform.Integrations {
	if s.hub == nil {
		return platform.Integrations{}
	}
	return s.hub.Get()
}

func (s *Server) aiStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, platform.AIStatusOf(s.aiSettings(), s.mode == "demo"))
}

func (s *Server) testAI(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		s.writeConnectionTest(w, platform.ConnectionTestResult{Service: "ai", Error: "integrations not available"})
		return
	}
	s.writeConnectionTest(w, platform.TestAIConnection(r.Context(), s.hub.Get(), s.mode == "demo"))
}

// aiExplain handles POST /api/v1/ai/explain?namespace=<ns>.
//
// The namespace comes from the query only (the JSON body has no namespace
// field and unknown fields are rejected), so the guard — which authorizes the
// ?namespace= value for namespace-restricted identities — and this handler can
// never disagree about which namespace is being read.
func (s *Server) aiExplain(w http.ResponseWriter, r *http.Request) {
	in := s.aiSettings()
	st := platform.AIStatusOf(in, s.mode == "demo")
	if !st.Enabled {
		msg := "the AI assistant is disabled; an admin can enable it under Settings → AI assistant"
		if in.AIEnabled && !st.Configured {
			msg = "the AI assistant is enabled but has no model configured; an admin must finish Settings → AI assistant"
		}
		fail(w, http.StatusConflict, errors.New(msg))
		return
	}
	var req platform.AIExplainRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	req.Namespace = strings.TrimSpace(r.URL.Query().Get("namespace"))
	if req.Namespace == "" || strings.EqualFold(req.Namespace, "all") {
		fail(w, http.StatusBadRequest, errors.New("a specific ?namespace= is required"))
		return
	}

	user := s.auditUser(r)
	limit := aiSendPerMinute
	if req.Preview {
		limit = aiPreviewPerMinute
	}
	// Separate buckets so previews cannot starve real sends and vice versa.
	key := user + "|send"
	if req.Preview {
		key = user + "|preview"
	}
	if ok, wait := s.aiRL.allow(key, limit, time.Minute); !ok {
		secs := int(wait.Seconds()) + 1
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		fail(w, http.StatusTooManyRequests, errors.New("too many AI requests; try again shortly"))
		return
	}

	asst := platform.NewAIAssistant(s.provider, in, s.mode == "demo")
	if !req.Preview {
		// Audit before the data leaves: the target only, never prompt or answer.
		platform.RecordAxiomAudit("ai.explain", req.Namespace+"/"+strings.ToLower(req.Kind)+"/"+req.Name, "", user)
	}
	res, err := asst.Explain(r.Context(), req)
	respond(w, res, err)
}
