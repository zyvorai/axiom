package api

import (
	"context"
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) listAccelerators(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListAccelerators(r.Context())
	respond(w, v, err)
}

type gpuRightsizer interface {
	ApplyGPURightsizing(context.Context, platform.GPURightsizingApplyRequest) (platform.GPURightsizingApplyResult, error)
}

func (s *Server) applyGPURightsizing(w http.ResponseWriter, r *http.Request) {
	var req platform.GPURightsizingApplyRequest
	if err := decode(r, &req); err != nil {
		fail(w, 400, err)
		return
	}
	gr, ok := s.provider.(gpuRightsizer)
	if !ok {
		fail(w, 501, errSSO("GPU rightsizing is not available on this provider"))
		return
	}
	res, err := gr.ApplyGPURightsizing(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	if !req.DryRun && res.OK {
		platform.RecordAxiomAudit("accelerators.gpu_rightsizing", req.Namespace+"/"+req.Kind+"/"+req.Name, res.From+"->"+res.To, s.auditUser(r))
	}
	writeJSON(w, 200, res)
}
