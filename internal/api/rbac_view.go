package api

import (
	"errors"
	"net/http"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

// registerSecurityRoutes mounts the Workstream C routes. Access control lives in
// internal/auth (policy.go / rbac_view.go): the RBAC viewer is operator+ and
// off-limits to namespace-restricted identities; the NetworkPolicy analysis and
// security reports are namespaced and scoped per identity.
func (s *Server) registerSecurityRoutes(mux *http.ServeMux) {
	// Read-only RBAC viewer. Deliberately no write counterpart (see docs/GAPS.md).
	mux.HandleFunc("GET /api/v1/rbac/subjects", s.rbacSubjects)
	mux.HandleFunc("GET /api/v1/rbac/subjects/{kind}/{name}", s.rbacSubject)
	mux.HandleFunc("GET /api/v1/rbac/who-can", s.rbacWhoCan)
	mux.HandleFunc("GET /api/v1/serviceaccounts", s.listServiceAccounts)

	mux.HandleFunc("GET /api/v1/networking/policies/{namespace}/analysis", s.networkPolicyAnalysis)

	mux.HandleFunc("GET /api/v1/security/reports", s.securityReports)
	mux.HandleFunc("GET /api/v1/security/reports/{namespace}/{kind}/{name}", s.workloadSecurity)
	mux.HandleFunc("GET /api/v1/security/policy-coverage", s.policyCoverage)
	mux.HandleFunc("GET /api/v1/security/vulnerabilities", s.securityVulnerabilities)

	mux.HandleFunc("POST /api/v1/ws-ticket", s.issueWSTicket)
}

func (s *Server) rbacSubjects(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListRBACSubjects(r.Context())
	respond(w, v, err)
}

func (s *Server) rbacSubject(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.GetRBACSubject(r.Context(), r.PathValue("kind"), r.PathValue("name"), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) rbacWhoCan(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v, err := s.provider.RBACWhoCan(r.Context(), platform.RBACWhoCanQuery{
		Verb: q.Get("verb"), Resource: q.Get("resource"), Group: q.Get("group"),
		Name: q.Get("name"), Namespace: q.Get("namespace"),
	})
	respond(w, v, err)
}

func (s *Server) listServiceAccounts(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListServiceAccounts(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) networkPolicyAnalysis(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.AnalyzeNetworkPolicies(r.Context(), r.PathValue("namespace"))
	respond(w, v, err)
}

func (s *Server) securityReports(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListSecurityReports(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) workloadSecurity(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.GetWorkloadSecurity(r.Context(), r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name"))
	respond(w, v, err)
}

func (s *Server) securityVulnerabilities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v, err := platform.ListVulnerabilities(r.Context(), s.provider, q.Get("namespace"), q.Get("severity"))
	respond(w, v, err)
}

func (s *Server) policyCoverage(w http.ResponseWriter, r *http.Request) {
	v, err := platform.ListPolicyCoverage(r.Context(), s.provider, r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

// issueWSTicket mints a single-use, 30-second ticket for one streaming GET path
// so browsers can open WebSocket / EventSource / log connections without
// putting credentials in the URL. The ticket carries the caller's own identity;
// every role and namespace check is re-applied when it is redeemed.
func (s *Server) issueWSTicket(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	id, ok := auth.IdentityFromContext(r.Context())
	if !ok || id.User == "" {
		fail(w, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	if !auth.TicketablePath(req.Path) {
		fail(w, http.StatusBadRequest, errors.New("path is not eligible for a stream ticket"))
		return
	}
	ticket, exp, err := s.tickets.Issue(id, req.Path)
	if err != nil {
		if errors.Is(err, auth.ErrTicketTableFull) {
			fail(w, http.StatusTooManyRequests, err)
			return
		}
		fail(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket": ticket, "path": req.Path, "expiresIn": int(auth.TicketTTL.Seconds()), "expiresAt": exp.UTC(),
	})
}
