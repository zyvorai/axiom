package api

import (
	"context"
	"fmt"
	"net/http"
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
	// Brief wait so PacketWolf / NetworkPolicy can settle; best-effort.
	time.Sleep(200 * time.Millisecond)
	after := s.networkGuardFlows(r.Context(), ns, kind, name)
	platform.RecordAxiomAudit("workload.seal.receipt", ns+"/"+kind+"/"+name,
		fmt.Sprintf("via=%s flowsAfter=%d", via, after), s.auditUser(r))
}
