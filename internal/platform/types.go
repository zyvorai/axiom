package platform

import "time"

type Overview struct {
	Cluster              string             `json:"cluster"`
	Health               string             `json:"health"`
	Workloads            int                `json:"workloads"`
	Pods                 int                `json:"pods"`
	RunningPods          int                `json:"runningPods"`
	Nodes                int                `json:"nodes"`
	ReadyNodes           int                `json:"readyNodes"`
	VMs                  int                `json:"vms"`
	ReadyVMs             int                `json:"readyVms"`
	CPUPercent           float64            `json:"cpuPercent"`
	MemoryPercent        float64            `json:"memoryPercent"`
	StorageUsedBytes     int64              `json:"storageUsedBytes"`
	StorageCapacityBytes int64              `json:"storageCapacityBytes"`
	StorageUsedTB        float64            `json:"storageUsedTb"`
	StorageCapacityTB    float64            `json:"storageCapacityTb"`
	Region               string             `json:"region"`
	Regions              []Region           `json:"regions,omitempty"`
	Utilization          []UtilizationPoint `json:"utilization,omitempty"`
	Warnings             int                `json:"warnings"`
}

type Region struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Active bool   `json:"active"`
}

type UtilizationPoint struct {
	At     string  `json:"at"`
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
}

type UtilizationSeries struct {
	CPUPercent    float64            `json:"cpuPercent"`
	MemoryPercent float64            `json:"memoryPercent"`
	PeakCPU       float64            `json:"peakCpu"`
	PeakMemory    float64            `json:"peakMemory"`
	Points        []UtilizationPoint `json:"points"`
	Source        string             `json:"source"`
}

type VirtualMachine struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Ready     bool   `json:"ready"`
	Running   bool   `json:"running"`
	CPU       string `json:"cpu"`
	Memory    string `json:"memory"`
	Node      string `json:"node,omitempty"`
	OS        string `json:"os,omitempty"`
	Age       string `json:"age"`
}

type Volume struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Capacity     string `json:"capacity"`
	StorageClass string `json:"storageClass"`
	AccessModes  string `json:"accessModes,omitempty"`
	VolumeName   string `json:"volumeName,omitempty"`
	Age          string `json:"age"`
}

type Workload struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Kind      string            `json:"kind"`
	Image     string            `json:"image"`
	Desired   int32             `json:"desired"`
	Ready     int32             `json:"ready"`
	Available int32             `json:"available"`
	Status    string            `json:"status"`
	Age       string            `json:"age"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type PodContainerStatus struct {
	Name  string `json:"name"`
	Init  bool   `json:"init"`
	Ready bool   `json:"ready"`
	State string `json:"state"`
}

type Pod struct {
	Namespace     string               `json:"namespace"`
	Name          string               `json:"name"`
	Node          string               `json:"node"`
	Status        string               `json:"status"`
	Ready         string               `json:"ready"`
	Restarts      int32                `json:"restarts"`
	IP            string               `json:"ip"`
	Age           string               `json:"age"`
	Containers    []string             `json:"containers"`
	ContainerInfo []PodContainerStatus `json:"containerInfo,omitempty"`
	Ports         []PodPort            `json:"ports,omitempty"`
	Labels        map[string]string    `json:"labels,omitempty"`
	WorkloadID    string               `json:"workloadId,omitempty"`
	Replica       int32                `json:"replica,omitempty"`
	Replicas      int32                `json:"replicas,omitempty"`
	Floor         string               `json:"floor,omitempty"`
	CPULimit      string               `json:"cpuLimit,omitempty"`
	MemLimit      string               `json:"memLimit,omitempty"`
	QOS           string               `json:"qos,omitempty"`
	Image         string               `json:"image,omitempty"`
	Intent        string               `json:"intent,omitempty"`
	Sealed        bool                 `json:"sealed,omitempty"`
}

type PodPort struct {
	Name          string `json:"name,omitempty"`
	Container     string `json:"container"`
	ContainerPort int32  `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

type Namespace struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Pods     int    `json:"pods"`
	Services int    `json:"services"`
	Age      string `json:"age"`
}

type ServicePort struct {
	Name       string `json:"name,omitempty"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort"`
	Protocol   string `json:"protocol"`
	NodePort   int32  `json:"nodePort,omitempty"`
}

type Service struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	ClusterIP string            `json:"clusterIP"`
	Ports     []ServicePort     `json:"ports"`
	Selector  map[string]string `json:"selector,omitempty"`
	Endpoints []string          `json:"endpoints"`
	Age       string            `json:"age"`
}

type NetworkNode struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Label     string   `json:"label"`
	IP        string   `json:"ip,omitempty"`
	Ports     []string `json:"ports,omitempty"`
}

type NetworkEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Port     int32  `json:"port"`
	Protocol string `json:"protocol"`
	Label    string `json:"label"`
}

type NetworkMap struct {
	Namespace string        `json:"namespace"`
	Nodes     []NetworkNode `json:"nodes"`
	Edges     []NetworkEdge `json:"edges"`
}

type NodeCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type TaintSummary struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Effect string `json:"effect"`
}

type Node struct {
	Name              string          `json:"name"`
	Status            string          `json:"status"`
	Role              string          `json:"role"`
	Version           string          `json:"version"`
	InternalIP        string          `json:"internalIp"`
	Pods              int             `json:"pods"`
	CPU               string          `json:"cpu"`
	Memory            string          `json:"memory"`
	AllocatableCPU    string          `json:"allocatableCpu,omitempty"`
	AllocatableMemory string          `json:"allocatableMemory,omitempty"`
	AllocatablePods   string          `json:"allocatablePods,omitempty"`
	OSImage           string          `json:"osImage,omitempty"`
	KernelVersion     string          `json:"kernelVersion,omitempty"`
	ContainerRuntime  string          `json:"containerRuntime,omitempty"`
	Conditions        []NodeCondition `json:"conditions,omitempty"`
	Taints            []TaintSummary  `json:"taints,omitempty"`
	Unschedulable     bool            `json:"unschedulable,omitempty"`
	Age               string          `json:"age"`
}

type RolloutRevision struct {
	Revision int    `json:"revision"`
	Status   string `json:"status"`
	Age      string `json:"age"`
	// Time is the revision's creation time when the provider knows it exactly
	// (used by the change timeline); Age is the display form.
	Time time.Time `json:"time,omitempty"`
}

type RolloutStatus struct {
	Status    string            `json:"status"`
	Paused    bool              `json:"paused"`
	Replicas  int32             `json:"replicas"`
	Ready     int32             `json:"ready"`
	Updated   int32             `json:"updated"`
	Available int32             `json:"available"`
	History   []RolloutRevision `json:"history"`
}

type ResourceDetail struct {
	Namespace      string            `json:"namespace"`
	Kind           string            `json:"kind"`
	Name           string            `json:"name"`
	Manifest       map[string]any    `json:"manifest"`
	Labels         map[string]string `json:"labels,omitempty"`
	Annotations    map[string]string `json:"annotations,omitempty"`
	ContainerPorts []map[string]any  `json:"containerPorts,omitempty"`
	Pods           []Pod             `json:"pods,omitempty"`
	RelatedEvents  []Event           `json:"relatedEvents,omitempty"`
}

type NetworkGuardSummary struct {
	Configured bool   `json:"configured"`
	Reachable  bool   `json:"reachable"`
	Message    string `json:"message,omitempty"`
	FlowsCount int    `json:"flowsCount,omitempty"`
	PolicyHint string `json:"policyHint,omitempty"`
}

type PacketWolfStatus struct {
	Configured bool              `json:"configured"`
	Reachable  bool              `json:"reachable"`
	BaseURL    string            `json:"baseUrl,omitempty"`
	Message    string            `json:"message,omitempty"`
	Subsystems map[string]string `json:"subsystems,omitempty"`
}

type Event struct {
	Namespace string    `json:"namespace"`
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	Object    string    `json:"object"`
	Message   string    `json:"message"`
	Count     int32     `json:"count"`
	Time      time.Time `json:"time"`
}

type LaunchRequest struct {
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Replicas    int32             `json:"replicas"`
	Port        int32             `json:"port"`
	CPU         string            `json:"cpu"`
	Memory      string            `json:"memory"`
	Environment map[string]string `json:"environment"`
	Expose      bool              `json:"expose"`
	Intent      string            `json:"intent,omitempty"`
}

type ScaleRequest struct {
	Replicas int32 `json:"replicas"`
}

type StillAction string

const (
	StillHush  StillAction = "hush"
	StillSleep StillAction = "sleep"
	StillSeal  StillAction = "seal"
	StillEnd   StillAction = "end"
)

type BlastPreview struct {
	Kind       string   `json:"kind"`
	WorkloadID string   `json:"workloadId"`
	Namespace  string   `json:"namespace"`
	Replicas   int      `json:"replicas"`
	Sessions   int      `json:"sessions"`
	Windows    int      `json:"windows"`
	Services   int      `json:"services"`
	Rps        int      `json:"rps"`
	Names      []string `json:"names"`
	Ports      []string `json:"ports"`
	Atlas      []string `json:"atlas"`
}

type IntentParseResponse struct {
	Draft         PlaceDraft    `json:"draft"`
	LaunchRequest LaunchRequest `json:"launchRequest"`
	Receipt       string        `json:"receipt"`
}

// --- Ops / FinOps / GitOps types ---

type SecretKeyInfo struct {
	Key  string `json:"key"`
	Size int    `json:"size"`
}

type SecretSummary struct {
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Keys      []string `json:"keys"`
	KeyCount  int      `json:"keyCount"`
	Age       string   `json:"age"`
}

type SecretDetail struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Keys      []SecretKeyInfo   `json:"keys"`
	Values    map[string]string `json:"values,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Age       string            `json:"age"`
}

type CreateSecretRequest struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Data      map[string]string `json:"data"`
	Keys      map[string]string `json:"keys"` // alias accepted from Aether-style clients
	Labels    map[string]string `json:"labels,omitempty"`
}

type ConfigMapSummary struct {
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Keys      []string `json:"keys"`
	KeyCount  int      `json:"keyCount"`
	Age       string   `json:"age"`
}

type ConfigMapDetail struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Data      map[string]string `json:"data,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Age       string            `json:"age"`
}

type CreateConfigMapRequest struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Data      map[string]string `json:"data"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type CreateResourceQuotaRequest struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Hard      map[string]string `json:"hard"`
}

type CreateIngressRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Service   string `json:"service"`
	Port      int32  `json:"port"`
	Class     string `json:"class,omitempty"`
	TLSSecret string `json:"tlsSecret,omitempty"`
}

type CreateServiceRequest struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Port      int32             `json:"port"`
	TargetPort int32            `json:"targetPort"`
	Selector  map[string]string `json:"selector"`
}

type CreateVolumeRequest struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	Size         string `json:"size"`
	StorageClass string `json:"storageClass,omitempty"`
	AccessMode   string `json:"accessMode,omitempty"`
}

type CreateVMRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	CPU       string `json:"cpu"`
	Memory    string `json:"memory"`
	Image     string `json:"image"`
	Running   bool   `json:"running"`
}

type CreateVeleroBackupRequest struct {
	Namespace          string   `json:"namespace"`
	Name               string   `json:"name"`
	IncludedNamespaces []string `json:"includedNamespaces,omitempty"`
	TTL                string   `json:"ttl,omitempty"`
}

type ResourceQuotaSummary struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Hard      map[string]string `json:"hard"`
	Used      map[string]string `json:"used"`
	Age       string            `json:"age"`
}

type LimitRangeItem struct {
	Type           string `json:"type"`
	Max            string `json:"max,omitempty"`
	Min            string `json:"min,omitempty"`
	Default        string `json:"default,omitempty"`
	DefaultRequest string `json:"defaultRequest,omitempty"`
}

type LimitRangeSummary struct {
	Namespace string           `json:"namespace"`
	Name      string           `json:"name"`
	Limits    []LimitRangeItem `json:"limits"`
	Age       string           `json:"age"`
}

type IngressSummary struct {
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Class     string   `json:"class,omitempty"`
	Hosts     []string `json:"hosts"`
	Address   string   `json:"address,omitempty"`
	TLS       bool     `json:"tls"`
	Age       string   `json:"age"`
}

type BackupSnapshot struct {
	Namespace           string `json:"namespace"`
	Name                string `json:"name"`
	PVC                 string `json:"pvc"`
	Ready               bool   `json:"ready"`
	Status              string `json:"status"`
	VolumeSnapshotClass string `json:"volumeSnapshotClass,omitempty"`
	Age                 string `json:"age"`
}

type CreateBackupRequest struct {
	Namespace           string `json:"namespace"`
	Name                string `json:"name"`
	PVC                 string `json:"pvc"`
	VolumeSnapshotClass string `json:"volumeSnapshotClass,omitempty"`
}

type RestoreBackupRequest struct {
	Namespace    string   `json:"namespace"`
	SnapshotName string   `json:"snapshotName"`
	PVCName      string   `json:"pvcName"`
	StorageClass string   `json:"storageClass,omitempty"`
	AccessModes  []string `json:"accessModes,omitempty"`
	Size         string   `json:"size"`
}

type HelmRelease struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Chart     string `json:"chart"`
	Revision  int    `json:"revision"`
	Age       string `json:"age"`
}

type HelmActionResult struct {
	OK      bool   `json:"ok"`
	Action  string `json:"action"`
	JobName string `json:"jobName,omitempty"`
	Message string `json:"message"`
}

type ExternalSecretSummary struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Store     string `json:"store"`
	Refresh   string `json:"refresh,omitempty"`
	Status    string `json:"status"`
	Ready     bool   `json:"ready"`
	Age       string `json:"age"`
}

type VMSnapshot struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	VM        string `json:"vm"`
	Ready     bool   `json:"ready"`
	Status    string `json:"status"`
	Age       string `json:"age"`
}

type CreateVMSnapshotRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	VM        string `json:"vm"`
}

type VMClone struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Source     string `json:"source"`
	SourceKind string `json:"sourceKind,omitempty"`
	Target     string `json:"target"`
	Phase      string `json:"phase"`
	Age        string `json:"age"`
}

type CreateVMCloneRequest struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	SourceVM   string `json:"sourceVM"`
	TargetVM   string `json:"targetVM"`
	SourceKind string `json:"sourceKind,omitempty"` // VirtualMachine (default) or VirtualMachineSnapshot
}

type VMExport struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	Phase     string `json:"phase"`
	Links     string `json:"links,omitempty"`
	Age       string `json:"age"`
}

type CreateVMExportRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	VM        string `json:"vm"`
}

type VMRestore struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Snapshot  string `json:"snapshot"`
	Target    string `json:"target"`
	Complete  bool   `json:"complete"`
	Phase     string `json:"phase"`
	Age       string `json:"age"`
}

type CreateVMRestoreRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Snapshot  string `json:"snapshot"`
	TargetVM  string `json:"targetVM"`
}

type VeleroBackup struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Phase      string `json:"phase"`
	Namespaces string `json:"namespaces,omitempty"`
	Errors     int    `json:"errors,omitempty"`
	Age        string `json:"age"`
}

type VeleroSchedule struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Schedule   string `json:"schedule"`
	Paused     bool   `json:"paused"`
	LastBackup string `json:"lastBackup,omitempty"`
	Age        string `json:"age"`
}

type CreateVeleroScheduleRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Schedule  string `json:"schedule"`
	TTL       string `json:"ttl,omitempty"`
}

type AlertmanagerSummary struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Version   string `json:"version,omitempty"`
	Replicas  int64  `json:"replicas"`
	Ready     bool   `json:"ready"`
	Age       string `json:"age"`
}

type GitOpsItem struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Source    string `json:"source,omitempty"`
	Age       string `json:"age"`
}

type PrometheusRuleSummary struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Groups    int    `json:"groups"`
	Age       string `json:"age"`
}

type AlertsResponse struct {
	Warnings      []Event                 `json:"warnings"`
	Rules         []PrometheusRuleSummary `json:"rules"`
	Alertmanagers []AlertmanagerSummary   `json:"alertmanagers"`
}

type AuditEntry struct {
	Time   time.Time `json:"time"`
	Action string    `json:"action"`
	Target string    `json:"target"`
	Detail string    `json:"detail,omitempty"`
	User   string    `json:"user,omitempty"`
}

type RBACRoleSummary struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Rules     int    `json:"rules"`
	Age       string `json:"age"`
}

type RBACBindingSummary struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	RoleRef   string `json:"roleRef"`
	Subjects  int    `json:"subjects"`
	Age       string `json:"age"`
}

type RBACSummary struct {
	ClusterRoles        []RBACRoleSummary    `json:"clusterRoles"`
	ClusterRoleBindings []RBACBindingSummary `json:"clusterRoleBindings"`
	Roles               []RBACRoleSummary    `json:"roles"`
	RoleBindings        []RBACBindingSummary `json:"roleBindings"`
}

type NamespaceCost struct {
	Namespace  string  `json:"namespace"`
	CPUCores   float64 `json:"cpuCores"`
	MemoryGi   float64 `json:"memoryGi"`
	MonthlyUSD float64 `json:"monthlyUsd"`
}

type WorkloadCost struct {
	Namespace  string  `json:"namespace"`
	Workload   string  `json:"workload"`
	Kind       string  `json:"kind"`
	CPUCores   float64 `json:"cpuCores"`
	MemoryGi   float64 `json:"memoryGi"`
	MonthlyUSD float64 `json:"monthlyUsd"`
}

type CostEstimate struct {
	TotalMonthlyUSD float64         `json:"totalMonthlyUsd"`
	CPURatePerCore  float64         `json:"cpuRatePerCore"`
	MemoryRatePerGi float64         `json:"memoryRatePerGi"`
	Namespaces      []NamespaceCost `json:"namespaces"`
	Workloads       []WorkloadCost  `json:"workloads"`
}

type ValidateResult struct {
	Valid      bool     `json:"valid"`
	Kind       string   `json:"kind,omitempty"`
	APIVersion string   `json:"apiVersion,omitempty"`
	Namespace  string   `json:"namespace,omitempty"`
	Name       string   `json:"name,omitempty"`
	Message    string   `json:"message,omitempty"`
	Errors     []string `json:"errors,omitempty"`
}

type VMConsoleInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Ready     bool   `json:"ready"`
	VNCPath   string `json:"vncPath"`
	Virtctl   string `json:"virtctl"`
	// WsPath is the Axiom-proxied WebSocket path for in-browser VNC.
	WsPath string `json:"wsPath,omitempty"`
}

type JobSummary struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	OwnerCronJob string `json:"ownerCronJob,omitempty"`
	Status       string `json:"status"`
	Completions  string `json:"completions"`
	Duration     string `json:"duration,omitempty"`
	Age          string `json:"age"`
}

type CronJobSummary struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	Schedule     string `json:"schedule"`
	Suspend      bool   `json:"suspend"`
	Active       int    `json:"active"`
	LastSchedule string `json:"lastSchedule,omitempty"`
	Age          string `json:"age"`
}

type HPASummary struct {
	Namespace         string `json:"namespace"`
	Name              string `json:"name"`
	TargetKind        string `json:"targetKind"`
	TargetName        string `json:"targetName"`
	MinReplicas       int32  `json:"minReplicas"`
	MaxReplicas       int32  `json:"maxReplicas"`
	CurrentReplicas   int32  `json:"currentReplicas"`
	TargetCPUPercent  int32  `json:"targetCpuPercent,omitempty"`
	CurrentCPUPercent int32  `json:"currentCpuPercent,omitempty"`
	Age               string `json:"age"`
}

type CreateHPARequest struct {
	Namespace        string `json:"namespace"`
	Name             string `json:"name"`
	TargetKind       string `json:"targetKind"`
	TargetName       string `json:"targetName"`
	MinReplicas      int32  `json:"minReplicas"`
	MaxReplicas      int32  `json:"maxReplicas"`
	TargetCPUPercent int32  `json:"targetCpuPercent"`
}

type PDBSummary struct {
	Namespace      string `json:"namespace"`
	Name           string `json:"name"`
	MinAvailable   string `json:"minAvailable,omitempty"`
	MaxUnavailable string `json:"maxUnavailable,omitempty"`
	CurrentHealthy int32  `json:"currentHealthy"`
	DesiredHealthy int32  `json:"desiredHealthy"`
	ExpectedPods   int32  `json:"expectedPods"`
	Age            string `json:"age"`
}

type NetworkPolicySummary struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	PodSelector string   `json:"podSelector"`
	PolicyTypes []string `json:"policyTypes"`
	Age         string   `json:"age"`
}

type ApplyResult struct {
	Valid      bool     `json:"valid"`
	Kind       string   `json:"kind,omitempty"`
	APIVersion string   `json:"apiVersion,omitempty"`
	Namespace  string   `json:"namespace,omitempty"`
	Name       string   `json:"name,omitempty"`
	Message    string   `json:"message,omitempty"`
	Errors     []string `json:"errors,omitempty"`
}

// DiffObject is one object's live vs proposed state for POST /apply/diff. Live and Proposed
// are rendered text (YAML on a real cluster, JSON in demo mode) with server-managed noise
// (managedFields, status, resourceVersion, uid, ...) removed, and Secret data values masked,
// so a line-based diff of the two is meaningful and never leaks a secret.
type DiffObject struct {
	Kind            string   `json:"kind"`
	APIVersion      string   `json:"apiVersion"`
	Namespace       string   `json:"namespace,omitempty"`
	Name            string   `json:"name"`
	Exists          bool     `json:"exists"`
	Changed         bool     `json:"changed"`
	Live            string   `json:"live"`
	Proposed        string   `json:"proposed"`
	ForeignManagers []string `json:"foreignManagers,omitempty"`
}

// DiffResult is the response of a manifest diff (a server-side apply dry-run on a real cluster).
type DiffResult struct {
	Valid    bool         `json:"valid"`
	Objects  []DiffObject `json:"objects,omitempty"`
	Warnings []string     `json:"warnings,omitempty"`
	Message  string       `json:"message,omitempty"`
	Errors   []string     `json:"errors,omitempty"`
}

// GraphNode is one object in a resource relationship graph. Only identity and a short status
// are carried — never Secret/ConfigMap contents.
type GraphNode struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Status    string `json:"status,omitempty"`
}

// GraphEdge is a directed relationship between two GraphNode IDs (From -> To), e.g. "owns",
// "selects", "routes", "scales", "mounts".
type GraphEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
}

// ResourceGraph is the relationship graph around one resource.
type ResourceGraph struct {
	Root      string      `json:"root"`
	Nodes     []GraphNode `json:"nodes"`
	Edges     []GraphEdge `json:"edges"`
	Truncated bool        `json:"truncated,omitempty"`
}

type PodMetricPoint struct {
	At     string `json:"at"`
	CPU    int64  `json:"cpuMillicores"`
	Memory int64  `json:"memoryBytes"`
}

type PodMetricSeries struct {
	Namespace     string            `json:"namespace"`
	Name          string            `json:"name"`
	CPUMillicores int64             `json:"cpuMillicores"`
	MemoryBytes   int64             `json:"memoryBytes"`
	Containers    []ContainerMetric `json:"containers"`
	Points        []PodMetricPoint  `json:"points"`
	Source        string            `json:"source"`
}

type ContainerMetric struct {
	Name          string `json:"name"`
	CPUMillicores int64  `json:"cpuMillicores"`
	MemoryBytes   int64  `json:"memoryBytes"`
}

type CRDSummary struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
	Plural  string `json:"plural"`
	Scope   string `json:"scope"`
	Age     string `json:"age"`
}

// PluginTarget/PluginColumn/PluginAction/PluginManifest are the Tier 1
// "declarative CRD-view plugin" schema — see docs/ARCHITECTURE.md. A
// plugin is pure data: it tells the frontend how to render an existing,
// already-RBAC-authorized CRD (which columns, which actions), it never
// carries or executes code.
type PluginTarget struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
	Plural  string `json:"plural"`
	Scope   string `json:"scope"`
}

type PluginColumn struct {
	Path  string `json:"path"`
	Label string `json:"label"`
	Type  string `json:"type"` // name|string|badge|status|age|count
}

type PluginAction struct {
	Name string `json:"name"`
	Type string `json:"type"` // delete (v1 only)
}

type PluginManifest struct {
	Name        string         `json:"name"`
	DisplayName string         `json:"displayName"`
	Icon        string         `json:"icon,omitempty"`
	Target      PluginTarget   `json:"target"`
	Columns     []PluginColumn `json:"columns,omitempty"`
	Actions     []PluginAction `json:"actions,omitempty"`
}

type InstallHelmRequest struct {
	Namespace    string `json:"namespace"`
	ReleaseName  string `json:"releaseName"`
	RepoURL      string `json:"repoUrl"`
	ChartName    string `json:"chartName"`
	ChartVersion string `json:"chartVersion,omitempty"`
	Values       string `json:"values,omitempty"`
}

type RBACPolicyRule struct {
	APIGroups []string `json:"apiGroups"`
	Resources []string `json:"resources"`
	Verbs     []string `json:"verbs"`
}

type CreateRoleRequest struct {
	Namespace string           `json:"namespace"`
	Name      string           `json:"name"`
	Rules     []RBACPolicyRule `json:"rules"`
}

type RBACSubject struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type CreateRoleBindingRequest struct {
	Namespace string        `json:"namespace"`
	Name      string        `json:"name"`
	RoleName  string        `json:"roleName"`
	Subjects  []RBACSubject `json:"subjects"`
}

type PodDiagnosis struct {
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Phase     string   `json:"phase"`
	Scheduled bool     `json:"scheduled"`
	Reason    string   `json:"reason,omitempty"`
	Message   string   `json:"message,omitempty"`
	Events    []string `json:"events,omitempty"`
	Diagnosis string   `json:"diagnosis"`
}

type GatewaySummary struct {
	Kind      string   `json:"kind"`
	Namespace string   `json:"namespace,omitempty"`
	Name      string   `json:"name"`
	Class     string   `json:"class,omitempty"`
	Hosts     []string `json:"hosts,omitempty"`
	Status    string   `json:"status,omitempty"`
	Age       string   `json:"age"`
}

type NodeMetricPoint struct {
	At     string `json:"at"`
	CPU    int64  `json:"cpuMillicores"`
	Memory int64  `json:"memoryBytes"`
}

type NodeMetricSeries struct {
	Name          string            `json:"name"`
	CPUMillicores int64             `json:"cpuMillicores"`
	MemoryBytes   int64             `json:"memoryBytes"`
	Points        []NodeMetricPoint `json:"points"`
	Source        string            `json:"source"`
}

type PriorityClassSummary struct {
	Name          string `json:"name"`
	Value         int32  `json:"value"`
	GlobalDefault bool   `json:"globalDefault"`
	Description   string `json:"description,omitempty"`
	Age           string `json:"age"`
}

type StorageClassSummary struct {
	Name              string `json:"name"`
	Provisioner       string `json:"provisioner"`
	ReclaimPolicy     string `json:"reclaimPolicy"`
	VolumeBindingMode string `json:"volumeBindingMode"`
	IsDefault         bool   `json:"isDefault"`
	Age               string `json:"age"`
}

type PersistentVolumeSummary struct {
	Name          string `json:"name"`
	Capacity      string `json:"capacity"`
	AccessModes   string `json:"accessModes"`
	ReclaimPolicy string `json:"reclaimPolicy"`
	Status        string `json:"status"`
	Claim         string `json:"claim,omitempty"`
	StorageClass  string `json:"storageClass,omitempty"`
	Age           string `json:"age"`
}

type AdmissionWebhookSummary struct {
	ConfigName    string   `json:"configName"`
	WebhookName   string   `json:"webhookName"`
	Type          string   `json:"type"`
	Rules         []string `json:"rules"`
	FailurePolicy string   `json:"failurePolicy"`
	Age           string   `json:"age"`
}
