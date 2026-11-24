package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/zyvorai/axiom/internal/platform"
)

// Read-only cluster hygiene audits: quota headroom, image tags, HPA
// saturation and Service wiring. None of them changes the cluster.

func (s *Server) registerAudits(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/audits/quota", s.auditQuota)
	mux.HandleFunc("GET /api/v1/audits/images", s.auditImages)
	mux.HandleFunc("GET /api/v1/audits/autoscaling", s.auditAutoscaling)
	mux.HandleFunc("GET /api/v1/audits/services", s.auditServices)
}

// auditNamespace validates the optional namespace query parameter.
func auditNamespace(w http.ResponseWriter, r *http.Request) (string, bool) {
	noStore(w)
	ns := r.URL.Query().Get("namespace")
	if ns != "" && ns != "all" {
		if err := platform.ValidateCostNamespace(ns); err != nil {
			fail(w, http.StatusBadRequest, err)
			return "", false
		}
	}
	return ns, true
}

func parseQuotaThreshold(v string) (int, error) {
	if v == "" {
		return platform.QuotaDefaultThreshold, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 100 {
		return 0, fmt.Errorf("threshold must be a whole number from 1 to 100")
	}
	return n, nil
}

func (s *Server) auditQuota(w http.ResponseWriter, r *http.Request) {
	ns, ok := auditNamespace(w, r)
	if !ok {
		return
	}
	th, err := parseQuotaThreshold(r.URL.Query().Get("threshold"))
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := platform.QuotaHeadroom(r.Context(), s.provider, ns, th)
	respond(w, v, err)
}

func (s *Server) auditImages(w http.ResponseWriter, r *http.Request) {
	ns, ok := auditNamespace(w, r)
	if !ok {
		return
	}
	v, err := platform.ImageHygiene(r.Context(), s.provider, ns)
	respond(w, v, err)
}

func (s *Server) auditAutoscaling(w http.ResponseWriter, r *http.Request) {
	ns, ok := auditNamespace(w, r)
	if !ok {
		return
	}
	v, err := platform.HPASaturation(r.Context(), s.provider, ns)
	respond(w, v, err)
}

func (s *Server) auditServices(w http.ResponseWriter, r *http.Request) {
	ns, ok := auditNamespace(w, r)
	if !ok {
		return
	}
	v, err := platform.ServiceWiring(r.Context(), s.provider, ns)
	respond(w, v, err)
}
