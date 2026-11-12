package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) networkGuardFlows(ctx context.Context, ns, kind, name string) int {
	g, err := s.provider.WorkloadNetworkGuard(ctx, ns, kind, name)
	if err != nil {
		return 0
	}
	return g.FlowsCount
}

// recordSealReceipt audits PacketWolf flow delta after seal (containment theatre).
func (s *Server) recordSealReceipt(r *http.Request, ns, kind, name, via string) {
	before := s.networkGuardFlows(r.Context(), ns, kind, name)
	// Brief wait so PacketWolf / NetworkPolicy can settle; best-effort.
	time.Sleep(200 * time.Millisecond)
	after := s.networkGuardFlows(r.Context(), ns, kind, name)
	delta := before - after
	witnesses := platform.SealWitnesses(ns, kind, name)
	detail := fmt.Sprintf("via=%s flowsBefore=%d flowsAfter=%d delta=%d", via, before, after, delta)
	if len(witnesses) > 0 {
		detail += " witnesses=" + strings.Join(witnesses, ",")
	}
	platform.RecordAxiomAudit("workload.seal.receipt", ns+"/"+kind+"/"+name, detail, s.auditUser(r))
}
