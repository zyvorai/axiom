package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/zyvorai/axiom/internal/platform"
)

// helmCatalogAPI serves the Helm chart catalog: chart repositories an
// administrator added, search over their indexes, and a chart's versions and
// default values. Reads are open to every role except namespace-restricted
// accounts; repository writes are admin-only (see auth/policy_helm.go).
type helmCatalogAPI struct {
	s *Server
	c *platform.HelmCatalog
}

// registerHelmCatalog wires the chart catalog routes.
func (s *Server) registerHelmCatalog(mux *http.ServeMux) {
	store := s.auth.HelmRepoStore
	if store == nil {
		store = platform.NewMemoryHelmRepoStore()
	}
	fetcher := platform.NewHelmFetcher(platform.HelmFetchPolicyFromEnv(), "zyvor-axiom/"+buildVersion)
	h := &helmCatalogAPI{s: s, c: platform.NewHelmCatalog(store, fetcher)}
	mux.HandleFunc("GET /api/v1/helm/repos", h.listRepos)
	mux.HandleFunc("POST /api/v1/helm/repos", h.addRepo)
	mux.HandleFunc("DELETE /api/v1/helm/repos/{name}", h.removeRepo)
	mux.HandleFunc("GET /api/v1/helm/charts", h.searchCharts)
	mux.HandleFunc("GET /api/v1/helm/charts/{repo}/{chart}", h.chart)
	mux.HandleFunc("GET /api/v1/helm/charts/{repo}/{chart}/values", h.chartValues)
}

func noStore(w http.ResponseWriter) { w.Header().Set("Cache-Control", "no-store") }

func (h *helmCatalogAPI) listRepos(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	v, err := h.c.Repos(r.Context())
	respond(w, v, err)
}

func (h *helmCatalogAPI) addRepo(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	var req struct {
		Name              string   `json:"name"`
		URL               string   `json:"url"`
		Kind              string   `json:"kind"`
		Charts            []string `json:"charts"`
		CredentialsSecret string   `json:"credentialsSecret"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := h.c.AddHelmRepo(r.Context(), platform.HelmRepoRequest{
		Name: req.Name, URL: req.URL, Kind: req.Kind, Charts: req.Charts, CredentialsSecret: req.CredentialsSecret,
	})
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("helm.repo_add", v.Name, v.URL, h.s.auditUser(r))
	writeJSON(w, http.StatusCreated, v)
}

func (h *helmCatalogAPI) removeRepo(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	repo, err := h.c.RemoveRepo(r.Context(), r.PathValue("name"))
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("helm.repo_remove", repo.Name, repo.URL, h.s.auditUser(r))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *helmCatalogAPI) searchCharts(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	q := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive number"})
			return
		}
		limit = n
	}
	v, err := h.c.Search(r.Context(), q.Get("repo"), q.Get("q"), limit, q.Get("refresh") == "true")
	respond(w, v, err)
}

func (h *helmCatalogAPI) chart(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	v, err := h.c.Chart(r.Context(), r.PathValue("repo"), r.PathValue("chart"))
	respond(w, v, err)
}

func (h *helmCatalogAPI) chartValues(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	v, err := h.c.ChartValues(r.Context(), r.PathValue("repo"), r.PathValue("chart"), r.URL.Query().Get("version"))
	respond(w, v, err)
}
