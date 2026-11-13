package platform

import (
	"context"
	"io"
)

type Provider interface {
	Overview(context.Context) (Overview, error)
	ListWorkloads(context.Context, string) ([]Workload, error)
	Launch(context.Context, LaunchRequest) (Workload, error)
	Scale(context.Context, string, string, string, int32) error
	Start(context.Context, string, string, string) error
	Stop(context.Context, string, string, string) error
	Restart(context.Context, string, string, string) error
	DeleteWorkload(context.Context, string, string, string) error
	SealWorkload(context.Context, string, string, string) error
	UnsealWorkload(context.Context, string, string, string) error
	ListPods(context.Context, string) ([]Pod, error)
	DeletePod(context.Context, string, string) error
	EvictPod(context.Context, string, string) error
	ListNodes(context.Context) ([]Node, error)
	ListVirtualMachines(context.Context, string) ([]VirtualMachine, error)
	CreateVirtualMachine(context.Context, CreateVMRequest) (VirtualMachine, error)
	DeleteVirtualMachine(context.Context, string, string) error
	StartVirtualMachine(context.Context, string, string) error
	StopVirtualMachine(context.Context, string, string) error
	ListVolumes(context.Context, string) ([]Volume, error)
	CreateVolume(context.Context, CreateVolumeRequest) (Volume, error)
	DeleteVolume(context.Context, string, string) error
	Utilization(context.Context) (UtilizationSeries, error)
	Cordon(context.Context, string, bool) error
	Drain(context.Context, string) error
	// PreviewNodeDrain reports, read-only, what draining the node would do.
	PreviewNodeDrain(context.Context, string) (DrainPreview, error)
	ListEvents(context.Context, string) ([]Event, error)
	PodLogs(context.Context, string, string, string, bool, int64, bool) (io.ReadCloser, error)
	ExecCommand(context.Context, string, string, string, []string) (string, error)
	ListNamespaces(context.Context) ([]Namespace, error)
	CreateNamespace(context.Context, string) (Namespace, error)
	DeleteNamespace(context.Context, string) error
	ListServices(context.Context, string) ([]Service, error)
	CreateService(context.Context, CreateServiceRequest) (Service, error)
	DeleteService(context.Context, string, string) error
	NetworkMap(context.Context, string) (NetworkMap, error)
	PodProxyTarget(context.Context, string, string, int32) (string, error)
	GetNode(context.Context, string) (Node, error)
	GetRolloutStatus(context.Context, string, string, string) (RolloutStatus, error)
	RolloutAction(context.Context, string, string, string, string) error
	GetResource(context.Context, string, string, string) (ResourceDetail, error)
	WorkloadNetworkGuard(context.Context, string, string, string) (NetworkGuardSummary, error)

	ListSecrets(context.Context, string) ([]SecretSummary, error)
	GetSecret(context.Context, string, string, bool) (SecretDetail, error)
	CreateSecret(context.Context, CreateSecretRequest) (SecretDetail, error)
	UpdateSecret(context.Context, CreateSecretRequest) (SecretDetail, error)
	DeleteSecret(context.Context, string, string) error
	ListBackups(context.Context, string) ([]BackupSnapshot, error)
	CreateBackup(context.Context, CreateBackupRequest) (BackupSnapshot, error)
	RestoreBackup(context.Context, RestoreBackupRequest) (Volume, error)
	ListHelmReleases(context.Context, string) ([]HelmRelease, error)
	HelmUninstall(context.Context, string, string) (HelmActionResult, error)
	HelmRollback(context.Context, string, string, int) (HelmActionResult, error)
	ListHelmHistory(context.Context, string, string) ([]HelmRelease, error)
	ListExternalSecrets(context.Context, string) ([]ExternalSecretSummary, error)
	ListVMSnapshots(context.Context, string) ([]VMSnapshot, error)
	CreateVMSnapshot(context.Context, CreateVMSnapshotRequest) (VMSnapshot, error)
	LiveMigrateVM(context.Context, string, string) (HelmActionResult, error)
	ListVMClones(context.Context, string) ([]VMClone, error)
	CreateVMClone(context.Context, CreateVMCloneRequest) (VMClone, error)
	ListVMExports(context.Context, string) ([]VMExport, error)
	CreateVMExport(context.Context, CreateVMExportRequest) (VMExport, error)
	ListVMRestores(context.Context, string) ([]VMRestore, error)
	CreateVMRestore(context.Context, CreateVMRestoreRequest) (VMRestore, error)
	ListVeleroBackups(context.Context, string) ([]VeleroBackup, error)
	ListVeleroSchedules(context.Context, string) ([]VeleroSchedule, error)
	ListVeleroRestores(context.Context, string) ([]VeleroRestore, error)
	ListGitOps(context.Context) ([]GitOpsItem, error)
	ListAlerts(context.Context, string) (AlertsResponse, error)
	ListRBAC(context.Context, string) (RBACSummary, error)
	EstimateCost(context.Context) (CostEstimate, error)
	ValidateManifest(context.Context, string) (ValidateResult, error)
	VMConsole(context.Context, string, string) (VMConsoleInfo, error)

	ListAgentRuns(context.Context, string) ([]AgentRun, error)
	GetAgentRun(context.Context, string, string) (AgentRun, error)
	CreateAgentRun(context.Context, CreateAgentRunRequest) (AgentRun, error)
	UpdateAgentRun(context.Context, AgentRun) (AgentRun, error)
	ApproveAgentRun(context.Context, string, string) (AgentRun, error)
	DenyAgentRun(context.Context, string, string) (AgentRun, error)
	ListPlatformIntents(context.Context, string) ([]PlatformIntent, error)

	GetHavenIdentity(context.Context, string) (HavenIdentitySummary, error)
	ListIdentityPlanes(context.Context, string) ([]IdentityPlane, error)
	ListHavenKeycloaks(context.Context, string) ([]HavenKeycloak, error)
	ListHavenDatabases(context.Context, string) ([]HavenDatabase, error)
	GetHavenAdminBootstrap(context.Context, string, string) (HavenAdminBootstrap, error)

	ListConfigMaps(context.Context, string) ([]ConfigMapSummary, error)
	GetConfigMap(context.Context, string, string) (ConfigMapDetail, error)
	CreateConfigMap(context.Context, CreateConfigMapRequest) (ConfigMapDetail, error)
	UpdateConfigMap(context.Context, CreateConfigMapRequest) (ConfigMapDetail, error)
	DeleteConfigMap(context.Context, string, string) error
	ListResourceQuotas(context.Context, string) ([]ResourceQuotaSummary, error)
	CreateResourceQuota(context.Context, CreateResourceQuotaRequest) (ResourceQuotaSummary, error)
	DeleteResourceQuota(context.Context, string, string) error
	ListLimitRanges(context.Context, string) ([]LimitRangeSummary, error)
	ListIngresses(context.Context, string) ([]IngressSummary, error)
	CreateIngress(context.Context, CreateIngressRequest) (IngressSummary, error)
	DeleteIngress(context.Context, string, string) error

	ListJobs(context.Context, string) ([]JobSummary, error)
	DeleteJob(context.Context, string, string) error
	ListCronJobs(context.Context, string) ([]CronJobSummary, error)
	TriggerCronJob(context.Context, string, string) (JobSummary, error)
	SetCronJobSuspend(context.Context, string, string, bool) error
	DeleteCronJob(context.Context, string, string) error

	ListHPAs(context.Context, string) ([]HPASummary, error)
	CreateHPA(context.Context, CreateHPARequest) (HPASummary, error)
	DeleteHPA(context.Context, string, string) error
	// Read-only VPA and KEDA views (autoscaling*.go); a missing CRD is
	// Installed:false, not an error.
	ListVPAs(context.Context, string) (VPAList, error)
	ListScaledObjects(context.Context, string) (ScaledObjectList, error)
	// VPA and KEDA create/delete (autoscaling_build.go validates and renders;
	// a missing CRD is a 422). ListTriggerAuthentications returns names only.
	CreateVPA(context.Context, VPACreateRequest) (AutoscalerCreateResult, error)
	DeleteVPA(context.Context, string, string) error
	CreateScaledObject(context.Context, ScaledObjectCreateRequest) (AutoscalerCreateResult, error)
	DeleteScaledObject(context.Context, string, string) error
	ListTriggerAuthentications(context.Context, string) (TriggerAuthenticationList, error)
	// VPA and KEDA edit in place (autoscaling_edit.go): the typed fields, a
	// managed-field diff, and a resourceVersion-guarded JSON Patch.
	GetVPAEditable(context.Context, string, string) (VPAEditable, error)
	PreviewVPAEdit(context.Context, string, string, VPAEditRequest) (AutoscalerEditPreview, error)
	UpdateVPA(context.Context, string, string, VPAEditRequest) (AutoscalerEditResult, error)
	GetScaledObjectEditable(context.Context, string, string) (ScaledObjectEditable, error)
	PreviewScaledObjectEdit(context.Context, string, string, ScaledObjectEditRequest) (AutoscalerEditPreview, error)
	UpdateScaledObject(context.Context, string, string, ScaledObjectEditRequest) (AutoscalerEditResult, error)

	ListPodDisruptionBudgets(context.Context, string) ([]PDBSummary, error)

	ListNetworkPolicies(context.Context, string) ([]NetworkPolicySummary, error)
	DeleteNetworkPolicy(context.Context, string, string) error

	ApplyManifest(context.Context, string) (ApplyResult, error)
	// DiffManifest previews ApplyManifest without changing anything: live vs proposed per object.
	DiffManifest(context.Context, string) (DiffResult, error)
	// ResourceGraph returns the relationship graph (owners, selecting Services, Ingress routes,
	// HPA, mounted PVC/ConfigMap/Secret names) around one resource.
	ResourceGraph(context.Context, string, string, string) (ResourceGraph, error)

	GetPodMetrics(context.Context, string, string) (PodMetricSeries, error)

	ListCRDs(context.Context) ([]CRDSummary, error)
	ListCRDInstances(context.Context, string, string, string, string) ([]map[string]any, error)
	GetCRDInstance(context.Context, string, string, string, string, string) (map[string]any, error)
	DeleteCRDInstance(context.Context, string, string, string, string, string) error

	ListPlugins(context.Context) ([]PluginManifest, error)

	ListAdmissionWebhooks(context.Context) ([]AdmissionWebhookSummary, error)

	ListStorageClasses(context.Context) ([]StorageClassSummary, error)
	ListPriorityClasses(context.Context) ([]PriorityClassSummary, error)
	ListPersistentVolumes(context.Context) ([]PersistentVolumeSummary, error)

	GetNodeMetrics(context.Context, string) (NodeMetricSeries, error)
	AddTaint(context.Context, string, string, string, string) error
	RemoveTaint(context.Context, string, string, string) error

	ListGateways(context.Context, string) ([]GatewaySummary, error)
	// ListAccelerators returns NVIDIA GPU inventory (nodes, GPU pods, operator signals).
	ListAccelerators(context.Context) (AcceleratorsReport, error)

	CreateVeleroSchedule(context.Context, CreateVeleroScheduleRequest) (VeleroSchedule, error)
	CreateVeleroBackup(context.Context, CreateVeleroBackupRequest) (VeleroBackup, error)
	PreviewVeleroRestore(context.Context, VeleroRestoreRequest) (VeleroRestorePreview, error)
	CreateVeleroRestore(context.Context, VeleroRestoreRequest) (VeleroRestore, error)

	DiagnosePod(context.Context, string, string) (PodDiagnosis, error)

	CreateRole(context.Context, CreateRoleRequest) (RBACRoleSummary, error)
	DeleteRole(context.Context, string, string) error
	CreateRoleBinding(context.Context, CreateRoleBindingRequest) (RBACBindingSummary, error)
	DeleteRoleBinding(context.Context, string, string) error

	TriggerGitOpsSync(context.Context, string, string, string) error
	// CreateGitOpsApp creates an Argo CD Application or a Flux source plus
	// Kustomization/HelmRelease from a typed request (see gitops_build.go).
	CreateGitOpsApp(context.Context, GitOpsCreateRequest) (GitOpsCreateResult, error)
	// PreviewGitOpsDelete reports whether deleting an Application /
	// Kustomization / HelmRelease cascades to its resources (gitops_delete.go);
	// DeleteGitOpsApp recomputes that, enforces the typed confirmation and
	// optionally removes an unshared, Axiom-created Flux source.
	PreviewGitOpsDelete(ctx context.Context, kind, namespace, name string) (GitOpsDeletePreview, error)
	DeleteGitOpsApp(context.Context, GitOpsDeleteRequest) (GitOpsDeleteResult, error)
	// GitOps edit in place (gitops_edit.go): the current typed fields, a
	// managed-field diff, and a resourceVersion-guarded patch of only those fields.
	GetGitOpsEditable(context.Context, GitOpsRef) (GitOpsEditable, error)
	PreviewGitOpsEdit(context.Context, GitOpsRef, GitOpsEditRequest) (GitOpsEditPreview, error)
	UpdateGitOpsApp(context.Context, GitOpsRef, GitOpsEditRequest) (GitOpsEditResult, error)
	// Write-only GitOps repository credentials (gitops_repocreds.go).
	ListGitOpsCredentials(ctx context.Context, engine, namespace string) ([]GitOpsCredential, error)
	CreateGitOpsCredential(context.Context, GitOpsCredentialRequest) (GitOpsCredentialResult, error)
	DeleteGitOpsCredential(ctx context.Context, engine, namespace, name string) error

	InstallHelmChart(context.Context, InstallHelmRequest) (HelmActionResult, error)
	UpgradeHelmChart(context.Context, InstallHelmRequest) (HelmActionResult, error)

	// Troubleshooting (Workstream B): cluster health check, multi-pod workload
	// logs and ephemeral debug containers.
	ClusterHealth(context.Context, string) (HealthReport, error)
	WorkloadLogs(context.Context, string, string, string, WorkloadLogOptions) (io.ReadCloser, error)
	CreateDebugContainer(context.Context, string, string, DebugRequest) (DebugContainer, error)
	// --- Workstream C: security views (read-only) ---
	// Implemented in rbac_view*.go, netpol_analysis*.go, security_reports*.go;
	// SwitchableProvider delegates in switchable_provider_security.go.
	ListRBACSubjects(context.Context) ([]RBACSubjectSummary, error)
	GetRBACSubject(ctx context.Context, kind, name, namespace string) (RBACSubjectDetail, error)
	RBACWhoCan(context.Context, RBACWhoCanQuery) (RBACWhoCanResult, error)
	ListServiceAccounts(context.Context, string) ([]ServiceAccountSummary, error)
	AnalyzeNetworkPolicies(context.Context, string) (NetworkPolicyAnalysis, error)
	ListSecurityReports(context.Context, string) (SecurityReportSummary, error)
	GetWorkloadSecurity(ctx context.Context, namespace, kind, name string) (WorkloadSecurityDetail, error)
	// ListRightsizing recommends container requests from observed usage
	// (Prometheus history or a metrics.k8s.io snapshot); see cost_rightsizing.go.
	ListRightsizing(ctx context.Context, namespace, window string) (RightsizingReport, error)
	// ApplyRightsizing recomputes one container's recommendation and writes
	// its requests to the workload (or only reports the changes on a dry run);
	// see cost_rightsizing_apply.go.
	ApplyRightsizing(ctx context.Context, req RightsizingApplyRequest) (RightsizingApplyResult, error)
}
