package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

// auditUserFromRequest attributes an action to the authenticated identity
// (session cookie, API key or Basic), falling back to raw Basic credentials.
func auditUserFromRequest(r *http.Request) string {
	if id, ok := auth.IdentityFromContext(r.Context()); ok && id.User != "" {
		return id.User
	}
	if u, _, ok := r.BasicAuth(); ok && u != "" {
		return u
	}
	return "anonymous"
}

func (s *Server) auditUser(r *http.Request) string {
	if id, ok := auth.IdentityFromContext(r.Context()); ok && id.User != "" {
		return id.User
	}
	if u, _, ok := r.BasicAuth(); ok && u != "" {
		return u
	}
	if s.username != "" {
		return s.username
	}
	return "anonymous"
}

func (s *Server) listSecrets(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListSecrets(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) createSecret(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateSecretRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateSecret(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("secret.create", req.Namespace+"/"+req.Name, req.Type, s.auditUser(r))
	EmitStateChanged("secret", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) getSecret(w http.ResponseWriter, r *http.Request) {
	includeValues := r.URL.Query().Get("values") == "true" || r.URL.Query().Get("decode") == "true"
	v, err := s.provider.GetSecret(r.Context(), r.PathValue("namespace"), r.PathValue("name"), includeValues)
	respond(w, v, err)
}

func (s *Server) deleteSecret(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeleteSecret(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("secret.delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("secret", "delete")
	}
	respondAction(w, err)
}

func (s *Server) listConfigMaps(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListConfigMaps(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) createConfigMap(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateConfigMapRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateConfigMap(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("configmap.create", req.Namespace+"/"+req.Name, "", s.auditUser(r))
	EmitStateChanged("configmap", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) getConfigMap(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.GetConfigMap(r.Context(), r.PathValue("namespace"), r.PathValue("name"))
	respond(w, v, err)
}

func (s *Server) deleteConfigMap(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeleteConfigMap(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("configmap.delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("configmap", "delete")
	}
	respondAction(w, err)
}

func (s *Server) listResourceQuotas(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListResourceQuotas(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) listLimitRanges(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListLimitRanges(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) listIngresses(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListIngresses(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListJobs(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeleteJob(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("job.delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("job", "delete")
	}
	respondAction(w, err)
}

func (s *Server) listCronJobs(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListCronJobs(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) triggerCronJob(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	v, err := s.provider.TriggerCronJob(r.Context(), ns, name)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("cronjob.trigger", ns+"/"+name, "", s.auditUser(r))
	EmitStateChanged("cronjob", "trigger")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) suspendCronJob(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	var req struct {
		Suspend bool `json:"suspend"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	err := s.provider.SetCronJobSuspend(r.Context(), ns, name, req.Suspend)
	if err == nil {
		action := "cronjob.resume"
		if req.Suspend {
			action = "cronjob.suspend"
		}
		platform.RecordAxiomAudit(action, ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("cronjob", action)
	}
	respondAction(w, err)
}

func (s *Server) deleteCronJob(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeleteCronJob(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("cronjob.delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("cronjob", "delete")
	}
	respondAction(w, err)
}

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListBackups(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateBackupRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateBackup(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("backup.create", req.Namespace+"/"+req.Name, "pvc="+req.PVC, s.auditUser(r))
	EmitStateChanged("backup", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	var req platform.RestoreBackupRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.RestoreBackup(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("backup.restore", req.Namespace+"/"+req.PVCName, "snapshot="+req.SnapshotName, s.auditUser(r))
	EmitStateChanged("backup", "restore")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) helmReleases(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListHelmReleases(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) gitops(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListGitOps(r.Context())
	respond(w, v, err)
}

func (s *Server) triggerGitOpsSync(w http.ResponseWriter, r *http.Request) {
	kind, ns, name := r.PathValue("kind"), r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.TriggerGitOpsSync(r.Context(), kind, ns, name)
	if err == nil {
		platform.RecordAxiomAudit("gitops.sync", ns+"/"+name, kind, s.auditUser(r))
		EmitStateChanged("gitops", "sync")
	}
	respondAction(w, err)
}

func (s *Server) alerts(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListAlerts(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	respond(w, platform.ListAxiomAudit(), nil)
}

func (s *Server) rbac(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListRBAC(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) createRole(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateRoleRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateRole(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("rbac.role.create", req.Namespace+"/"+req.Name, "", s.auditUser(r))
	EmitStateChanged("rbac", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) deleteRole(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeleteRole(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("rbac.role.delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("rbac", "delete")
	}
	respondAction(w, err)
}

func (s *Server) createRoleBinding(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateRoleBindingRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateRoleBinding(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("rbac.rolebinding.create", req.Namespace+"/"+req.Name, req.RoleName, s.auditUser(r))
	EmitStateChanged("rbac", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) deleteRoleBinding(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeleteRoleBinding(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("rbac.rolebinding.delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("rbac", "delete")
	}
	respondAction(w, err)
}

func (s *Server) cost(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.EstimateCost(r.Context())
	respond(w, v, err)
}

func (s *Server) validateManifest(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	manifest := strings.TrimSpace(string(body))
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") || strings.HasPrefix(manifest, "{") {
		var envelope struct {
			Manifest string `json:"manifest"`
			YAML     string `json:"yaml"`
		}
		if err := json.Unmarshal(body, &envelope); err == nil {
			if envelope.Manifest != "" {
				manifest = envelope.Manifest
			} else if envelope.YAML != "" {
				manifest = envelope.YAML
			} else {
				// Body itself may be a K8s object JSON.
				var probe map[string]any
				if json.Unmarshal(body, &probe) == nil {
					if _, ok := probe["kind"]; ok {
						manifest = string(body)
					}
				}
			}
		}
	}
	v, err := s.provider.ValidateManifest(r.Context(), manifest)
	respond(w, v, err)
}

func (s *Server) vmConsole(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.VMConsole(r.Context(), r.PathValue("namespace"), r.PathValue("name"))
	respond(w, v, err)
}

func (s *Server) helmHistory(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListHelmHistory(r.Context(), r.PathValue("namespace"), r.PathValue("name"))
	respond(w, v, err)
}

func (s *Server) helmUninstall(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	v, err := s.provider.HelmUninstall(r.Context(), ns, name)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("helm.uninstall", ns+"/"+name, v.JobName, s.auditUser(r))
	EmitStateChanged("helm", "uninstall")
	writeJSON(w, http.StatusAccepted, v)
}

func (s *Server) helmRollback(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	rev := 0
	var body struct {
		Revision int `json:"revision"`
	}
	_ = decode(r, &body)
	rev = body.Revision
	v, err := s.provider.HelmRollback(r.Context(), ns, name, rev)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("helm.rollback", ns+"/"+name, v.Message, s.auditUser(r))
	EmitStateChanged("helm", "rollback")
	writeJSON(w, http.StatusAccepted, v)
}

func (s *Server) helmInstall(w http.ResponseWriter, r *http.Request) {
	var req platform.InstallHelmRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.InstallHelmChart(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("helm.install", req.Namespace+"/"+req.ReleaseName, req.RepoURL+" "+req.ChartName, s.auditUser(r))
	EmitStateChanged("helm", "install")
	writeJSON(w, http.StatusAccepted, v)
}

func (s *Server) externalSecrets(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListExternalSecrets(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) vmSnapshots(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListVMSnapshots(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) createVMSnapshot(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateVMSnapshotRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateVMSnapshot(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("vm.snapshot", req.Namespace+"/"+req.VM, v.Name, s.auditUser(r))
	EmitStateChanged("vmsnapshot", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) migrateVM(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	v, err := s.provider.LiveMigrateVM(r.Context(), ns, name)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("vm.migrate", ns+"/"+name, v.JobName, s.auditUser(r))
	EmitStateChanged("vm", "migrate")
	writeJSON(w, http.StatusAccepted, v)
}

func (s *Server) vmClones(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListVMClones(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) createVMClone(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateVMCloneRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateVMClone(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("vm.clone", req.Namespace+"/"+req.SourceVM, v.Name+" → "+req.TargetVM, s.auditUser(r))
	EmitStateChanged("vmclone", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) vmExports(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListVMExports(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) createVMExport(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateVMExportRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateVMExport(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("vm.export", req.Namespace+"/"+req.VM, v.Name, s.auditUser(r))
	EmitStateChanged("vmexport", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) vmRestores(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListVMRestores(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) createVMRestore(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateVMRestoreRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateVMRestore(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("vm.restore", req.Namespace+"/"+req.Snapshot, v.Name+" → "+req.TargetVM, s.auditUser(r))
	EmitStateChanged("vmrestore", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) veleroBackups(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListVeleroBackups(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) veleroSchedules(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListVeleroSchedules(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) createVeleroSchedule(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateVeleroScheduleRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateVeleroSchedule(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("velero-schedule.create", req.Namespace+"/"+req.Name, req.Schedule, s.auditUser(r))
	EmitStateChanged("velero-schedule", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) updateSecret(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateSecretRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	req.Namespace, req.Name = r.PathValue("namespace"), r.PathValue("name")
	v, err := s.provider.UpdateSecret(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("secret.update", req.Namespace+"/"+req.Name, "", s.auditUser(r))
	EmitStateChanged("secret", "update")
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) updateConfigMap(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateConfigMapRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	req.Namespace, req.Name = r.PathValue("namespace"), r.PathValue("name")
	v, err := s.provider.UpdateConfigMap(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("configmap.update", req.Namespace+"/"+req.Name, "", s.auditUser(r))
	EmitStateChanged("configmap", "update")
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) createResourceQuota(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateResourceQuotaRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateResourceQuota(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("resourcequota.create", req.Namespace+"/"+req.Name, "", s.auditUser(r))
	EmitStateChanged("resourcequota", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) deleteResourceQuota(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeleteResourceQuota(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("resourcequota.delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("resourcequota", "delete")
	}
	respondAction(w, err)
}

func (s *Server) createIngress(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateIngressRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateIngress(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("ingress.create", req.Namespace+"/"+req.Name, req.Host, s.auditUser(r))
	EmitStateChanged("ingress", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) deleteIngress(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeleteIngress(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("ingress.delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("ingress", "delete")
	}
	respondAction(w, err)
}

func (s *Server) createService(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateServiceRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateService(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("service.create", req.Namespace+"/"+req.Name, "", s.auditUser(r))
	EmitStateChanged("service", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) deleteService(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeleteService(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("service.delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("service", "delete")
	}
	respondAction(w, err)
}

func (s *Server) createVolume(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateVolumeRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateVolume(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("volume.create", req.Namespace+"/"+req.Name, req.Size, s.auditUser(r))
	EmitStateChanged("volume", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) deleteVolume(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeleteVolume(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("volume.delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("volume", "delete")
	}
	respondAction(w, err)
}

func (s *Server) createVM(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateVMRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateVirtualMachine(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("vm.create", req.Namespace+"/"+req.Name, "", s.auditUser(r))
	EmitStateChanged("vm", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) deleteVM(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeleteVirtualMachine(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("vm.delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("vm", "delete")
	}
	respondAction(w, err)
}

func (s *Server) createVeleroBackup(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateVeleroBackupRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateVeleroBackup(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("velero-backup.create", req.Namespace+"/"+req.Name, "", s.auditUser(r))
	EmitStateChanged("velero-backup", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) helmUpgrade(w http.ResponseWriter, r *http.Request) {
	var req platform.InstallHelmRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.UpgradeHelmChart(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("helm.upgrade", req.Namespace+"/"+req.ReleaseName, req.RepoURL+" "+req.ChartName, s.auditUser(r))
	EmitStateChanged("helm", "upgrade")
	writeJSON(w, http.StatusAccepted, v)
}
