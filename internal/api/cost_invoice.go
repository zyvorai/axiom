package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

// Actual-bill reconciliation (EXPERIMENTAL; see internal/platform/billing.go).
// GET /cost/invoice follows the cost-read policy: every role except
// namespace-restricted accounts (it is the cloud bill of the whole account).
// Configuring and testing it lives under /integrations/ (admin-only).

func (s *Server) billingService() *platform.BillingService {
	if s.billing == nil {
		return platform.NewBillingService() // a Server built without New* (tests): uncached
	}
	return s.billing
}

func (s *Server) billingSettings() platform.Integrations {
	if s.hub == nil {
		return platform.Integrations{}
	}
	return s.hub.Get()
}

func (s *Server) costInvoice(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	writeJSON(w, http.StatusOK, s.billingService().Report(r.Context(), s.billingSettings(), s.provider, s.mode == "demo"))
}

func (s *Server) testBilling(w http.ResponseWriter, r *http.Request) {
	res := s.billingService().Test(r.Context(), s.billingSettings(), s.mode == "demo")
	platform.RecordAxiomAudit("integrations.billing_test", "settings", "", s.auditUser(r))
	s.writeConnectionTest(w, res)
}
