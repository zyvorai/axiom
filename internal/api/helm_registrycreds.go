package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/zyvorai/axiom/internal/platform"
)

// registerHelmRegistryCreds wires write-only OCI registry credentials for the
// Helm chart catalog. All three routes are admin-only, the listing included
// (internal/auth/policy_helm.go isHelmRegistryCreds). There is deliberately no
// update route: rotating a credential is delete + create.
//
// SECURITY: the POST body carries a password or access token. It is read once
// into a bounded buffer that is zeroed after decoding; decode errors are
// reported generically (never echoing the body); the response, audit entry
// and any error carry only name/registry/username. No handler here can read a
// stored password: the store's secret-bearing accessor is unexported.
func (s *Server) registerHelmRegistryCreds(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/helm/registry-credentials", s.listHelmRegistryCreds)
	mux.HandleFunc("POST /api/v1/helm/registry-credentials", s.createHelmRegistryCred)
	mux.HandleFunc("DELETE /api/v1/helm/registry-credentials/{name}", s.deleteHelmRegistryCred)
}

func (s *Server) listHelmRegistryCreds(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	v, err := platform.ListHelmRegistryCreds(r.Context(), s.auth.HelmRegistryCredStore, s.auth.HelmRepoStore)
	respond(w, v, err)
}

func (s *Server) createHelmRegistryCred(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, platform.HelmRegistryCredBodyLimit))
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
	var req platform.HelmRegistryCredRequest
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || dec.More() {
		// Generic on purpose: a JSON error could quote part of the body.
		fail(w, http.StatusBadRequest, errors.New("invalid request body: expected a JSON object with name, registry, username and password"))
		return
	}
	info, err := s.auth.HelmRegistryCredStore.Add(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("helm.registry_credential_create", info.Name, "registry="+info.Registry, s.auditUser(r))
	EmitStateChanged("helm", "registry_credential_create")
	writeJSON(w, http.StatusCreated, platform.HelmRegistryCredResult{OK: true, Name: info.Name, Registry: info.Registry, Username: info.Username})
}

func (s *Server) deleteHelmRegistryCred(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	info, err := platform.DeleteHelmRegistryCred(r.Context(), s.auth.HelmRegistryCredStore, s.auth.HelmRepoStore, r.PathValue("name"))
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("helm.registry_credential_delete", info.Name, "registry="+info.Registry, s.auditUser(r))
	EmitStateChanged("helm", "registry_credential_delete")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": info.Name, "registry": info.Registry})
}

// resolveHelmOCIAccess attaches the plain-http flag and registry credentials
// of the stored OCI repository an install/upgrade ociRef belongs to. The
// handler only carries the result (its fields are unexported): it cannot
// read the password.
func (s *Server) resolveHelmOCIAccess(r *http.Request, req *platform.InstallHelmRequest) error {
	req.OCIAccess = nil
	if strings.TrimSpace(req.OCIRef) == "" {
		return nil
	}
	a, err := platform.ResolveHelmOCIAccess(r.Context(), s.auth.HelmRepoStore, s.auth.HelmRegistryCredStore, req.OCIRef)
	if err != nil {
		return err
	}
	req.OCIAccess = a
	return nil
}

// helmInstallAuditDetail is the audit detail of an install/upgrade: the chart,
// and for an OCI chart pulled with stored credentials the credential's name.
func helmInstallAuditDetail(req platform.InstallHelmRequest) string {
	if req.OCIRef == "" {
		return req.RepoURL + " " + req.ChartName
	}
	if n := req.OCIAccess.CredentialName(); n != "" {
		return req.OCIRef + " registry-credential=" + n
	}
	return req.OCIRef
}
