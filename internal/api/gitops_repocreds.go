package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

// registerGitOpsCredentials wires write-only GitOps repository credentials.
// All three routes are admin-only, the listing included
// (internal/auth/policy_gitops.go isGitOpsCredentials). There is deliberately
// no update route: rotating a credential is delete + create.
//
// SECURITY: the POST body carries passwords, tokens and private keys. It is
// read once into a bounded buffer that is zeroed after decoding; decode
// errors are reported generically (never echoing the body); the response,
// audit entry and any error carry only name/namespace/engine/authType.
func (s *Server) registerGitOpsCredentials(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/gitops/credentials", s.listGitOpsCredentials)
	mux.HandleFunc("POST /api/v1/gitops/credentials", s.createGitOpsCredential)
	mux.HandleFunc("DELETE /api/v1/gitops/credentials/{engine}/{namespace}/{name}", s.deleteGitOpsCredential)
}

func (s *Server) listGitOpsCredentials(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v, err := s.provider.ListGitOpsCredentials(r.Context(), q.Get("engine"), q.Get("namespace"))
	respond(w, v, err)
}

func (s *Server) createGitOpsCredential(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, platform.GitOpsCredentialBodyLimit))
	_ = r.Body.Close()
	defer func() {
		for i := range body {
			body[i] = 0
		}
	}()
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			fail(w, http.StatusRequestEntityTooLarge, errors.New("request body too large"))
			return
		}
		fail(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	var req platform.GitOpsCredentialRequest
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || dec.More() {
		// Generic on purpose: a JSON error could quote part of the body.
		fail(w, http.StatusBadRequest, errors.New("invalid request body: expected a JSON object with engine, name, namespace, url, authType, kind and the credential fields"))
		return
	}
	v, err := s.provider.CreateGitOpsCredential(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("gitops.credential_create", v.Engine+"/"+v.Namespace+"/"+v.Name, "authType="+req.AuthType, s.auditUser(r))
	EmitStateChanged("gitops", "credential_create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) deleteGitOpsCredential(w http.ResponseWriter, r *http.Request) {
	engine, ns, name := r.PathValue("engine"), r.PathValue("namespace"), r.PathValue("name")
	if err := s.provider.DeleteGitOpsCredential(r.Context(), engine, ns, name); err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("gitops.credential_delete", engine+"/"+ns+"/"+name, "", s.auditUser(r))
	EmitStateChanged("gitops", "credential_delete")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "engine": engine, "namespace": ns, "name": name})
}
