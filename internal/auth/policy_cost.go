package auth

import (
	"net/http"
	"strings"
)

// isCostBudgetOperatorForbidden reports a write to the cost budgets
// (PUT / DELETE /cost/budgets/{namespace}). Budgets are an administrative
// setting, so they are admin-only; reading them (GET /cost/budgets) and the
// rightsizing report stay open to operators and viewers. Namespace-restricted
// accounts are kept off both via restrictedSensitivePrefixes.
func isCostBudgetOperatorForbidden(r *http.Request) bool {
	p := r.URL.Path
	if p != "/api/v1/cost/budgets" && !strings.HasPrefix(p, "/api/v1/cost/budgets/") {
		return false
	}
	return isMutatingMethod(r.Method)
}
