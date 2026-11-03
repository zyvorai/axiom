package platform

// Types for the Workstream C security views: the read-only RBAC viewer, the
// NetworkPolicy analyzer and the vulnerability / policy report aggregation.
// They live apart from types.go to keep that shared file's diff small.

// --- RBAC viewer ----------------------------------------------------------

// RBACRuleView is one PolicyRule as shown in the viewer.
type RBACRuleView struct {
	APIGroups       []string `json:"apiGroups,omitempty"`
	Resources       []string `json:"resources,omitempty"`
	ResourceNames   []string `json:"resourceNames,omitempty"`
	NonResourceURLs []string `json:"nonResourceURLs,omitempty"`
	Verbs           []string `json:"verbs"`
}

// RBACSubjectSummary is a user, group or ServiceAccount that appears in at
// least one binding (or, for ServiceAccounts, exists in the cluster).
type RBACSubjectSummary struct {
	Kind      string `json:"kind"` // User | Group | ServiceAccount
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"` // ServiceAccounts only
	Bindings  int    `json:"bindings"`
	// ClusterWide is true when at least one ClusterRoleBinding grants this
	// subject permissions in every namespace.
	ClusterWide bool `json:"clusterWide"`
	// ClusterAdmin is true when the effective rules include */*/* verbs.
	ClusterAdmin bool     `json:"clusterAdmin,omitempty"`
	Roles        []string `json:"roles"` // distinct "ClusterRole/x" or "Role/x" refs
}

// RBACGrant is one binding that gives a subject permissions, with the rules of
// the role it references resolved.
type RBACGrant struct {
	Scope       string         `json:"scope"`               // cluster | namespace
	Namespace   string         `json:"namespace,omitempty"` // namespace the rules apply in
	Binding     string         `json:"binding"`             // "ClusterRoleBinding/x" or "RoleBinding/ns/x"
	RoleRef     string         `json:"roleRef"`             // "ClusterRole/view"
	Via         string         `json:"via,omitempty"`       // implicit group that matched (system:authenticated, ...)
	Dangling    bool           `json:"dangling,omitempty"`  // the referenced role does not exist
	Rules       []RBACRuleView `json:"rules"`
	RuleSummary string         `json:"ruleSummary,omitempty"`
}

// RBACSubjectDetail is the effective permission set of one subject.
type RBACSubjectDetail struct {
	Subject RBACSubjectSummary `json:"subject"`
	// Cluster grants apply in every namespace (and to cluster-scoped objects).
	Cluster []RBACGrant `json:"cluster"`
	// Namespaced grants apply only within RBACGrant.Namespace.
	Namespaced []RBACGrant `json:"namespaced"`
	// Notes explain what the view cannot see (e.g. IdP group membership).
	Notes []string `json:"notes,omitempty"`
}

// RBACWhoCanQuery is the question "who can <verb> <resource> [in <namespace>]".
type RBACWhoCanQuery struct {
	Verb      string `json:"verb"`
	Resource  string `json:"resource"`
	Group     string `json:"group,omitempty"`     // API group; empty matches any group
	Name      string `json:"name,omitempty"`      // object name (matches rules restricted by resourceNames)
	Namespace string `json:"namespace,omitempty"` // empty = across every namespace
}

// RBACWhoCanEntry is one subject allowed by one binding.
type RBACWhoCanEntry struct {
	Subject   RBACSubject `json:"subject"`
	Scope     string      `json:"scope"`
	Namespace string      `json:"namespace,omitempty"`
	Binding   string      `json:"binding"`
	RoleRef   string      `json:"roleRef"`
	Via       string      `json:"via,omitempty"`
}

// RBACWhoCanResult answers an RBACWhoCanQuery.
type RBACWhoCanResult struct {
	Query     RBACWhoCanQuery   `json:"query"`
	Allowed   []RBACWhoCanEntry `json:"allowed"`
	Truncated bool              `json:"truncated,omitempty"`
}

// ServiceAccountSummary is the read-only ServiceAccount list row.
type ServiceAccountSummary struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Secrets   int    `json:"secrets"`
	Bindings  int    `json:"bindings"`
	Age       string `json:"age"`
}

// --- NetworkPolicy analysis ----------------------------------------------

// NetworkPolicyAllow is one allowed ingress source or egress destination.
type NetworkPolicyAllow struct {
	Policy string `json:"policy"`
	// Kind is any | pods | namespace | cidr.
	Kind string `json:"kind"`
	// Peer is a human-readable description of the peer selector.
	Peer       string   `json:"peer"`
	Namespaces []string `json:"namespaces,omitempty"` // namespaces matched by a namespaceSelector
	Pods       []string `json:"pods,omitempty"`       // pods resolved in the analysed namespace
	PodCount   int      `json:"podCount,omitempty"`
	CIDR       string   `json:"cidr,omitempty"`
	Except     []string `json:"except,omitempty"`
	Ports      []string `json:"ports"` // "TCP/8080", "UDP/53", or "all ports"
}

// PodNetworkAnalysis is the effective NetworkPolicy position of one pod.
type PodNetworkAnalysis struct {
	Pod             string               `json:"pod"`
	Labels          map[string]string    `json:"labels,omitempty"`
	Policies        []string             `json:"policies"` // policies whose podSelector selects this pod
	IngressIsolated bool                 `json:"ingressIsolated"`
	EgressIsolated  bool                 `json:"egressIsolated"`
	Ingress         []NetworkPolicyAllow `json:"ingress"`
	Egress          []NetworkPolicyAllow `json:"egress"`
}

// NetworkPolicyAnalysis is the per-namespace result.
type NetworkPolicyAnalysis struct {
	Namespace string               `json:"namespace"`
	Policies  int                  `json:"policies"`
	Pods      []PodNetworkAnalysis `json:"pods"`
	// Uncovered lists pods selected by no policy at all: fully open both ways.
	Uncovered []string `json:"uncovered"`
	// DefaultDenyIngress/Egress are true when a policy selecting every pod
	// declares that direction with no allow rules.
	DefaultDenyIngress bool     `json:"defaultDenyIngress"`
	DefaultDenyEgress  bool     `json:"defaultDenyEgress"`
	Warnings           []string `json:"warnings"`
	Notes              []string `json:"notes"`
}

// --- Security reports ------------------------------------------------------

// SecuritySeverityCounts counts findings by severity.
type SecuritySeverityCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Unknown  int `json:"unknown"`
}

// SecurityPolicyCounts counts wgpolicyk8s.io PolicyReport results.
type SecurityPolicyCounts struct {
	Pass  int `json:"pass"`
	Fail  int `json:"fail"`
	Warn  int `json:"warn"`
	Error int `json:"error"`
	Skip  int `json:"skip"`
}

// SecuritySource says whether one report CRD family is installed.
type SecuritySource struct {
	Name      string `json:"name"`      // "Trivy VulnerabilityReports"
	Group     string `json:"group"`     // CRD group
	Available bool   `json:"available"` // the CRD exists and could be listed
	Reports   int    `json:"reports"`
	Message   string `json:"message,omitempty"`
}

// SecurityWorkloadRow aggregates all reports that belong to one workload.
type SecurityWorkloadRow struct {
	Namespace         string                 `json:"namespace"`
	Kind              string                 `json:"kind"`
	Name              string                 `json:"name"`
	Images            []string               `json:"images,omitempty"`
	Vulnerabilities   SecuritySeverityCounts `json:"vulnerabilities"`
	Misconfigurations SecuritySeverityCounts `json:"misconfigurations"`
	Policy            SecurityPolicyCounts   `json:"policy"`
}

// SecurityReportSummary is GET /api/v1/security/reports.
type SecurityReportSummary struct {
	// Available is true when at least one report CRD family is installed.
	Available         bool                   `json:"available"`
	Sources           []SecuritySource       `json:"sources"`
	Vulnerabilities   SecuritySeverityCounts `json:"vulnerabilities"`
	Misconfigurations SecuritySeverityCounts `json:"misconfigurations"`
	Policy            SecurityPolicyCounts   `json:"policy"`
	Workloads         []SecurityWorkloadRow  `json:"workloads"`
	InstallHint       string                 `json:"installHint,omitempty"`
	Truncated         bool                   `json:"truncated,omitempty"`
}

// SecurityVulnerability is one CVE finding in a workload drill-down.
type SecurityVulnerability struct {
	ID        string  `json:"id"`
	Severity  string  `json:"severity"`
	Package   string  `json:"package"`
	Installed string  `json:"installed"`
	Fixed     string  `json:"fixed,omitempty"`
	Title     string  `json:"title,omitempty"`
	Image     string  `json:"image,omitempty"`
	Score     float64 `json:"score,omitempty"`
	Link      string  `json:"link,omitempty"`
}

// SecurityCheck is one failed config-audit check.
type SecurityCheck struct {
	ID       string   `json:"id"`
	Severity string   `json:"severity"`
	Title    string   `json:"title"`
	Messages []string `json:"messages,omitempty"`
	Category string   `json:"category,omitempty"`
}

// SecurityPolicyResult is one PolicyReport result.
type SecurityPolicyResult struct {
	Policy   string `json:"policy"`
	Rule     string `json:"rule,omitempty"`
	Result   string `json:"result"` // pass | fail | warn | error | skip
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message,omitempty"`
	Source   string `json:"source,omitempty"`
}

// WorkloadSecurityDetail is GET /api/v1/security/reports/{ns}/{kind}/{name}.
type WorkloadSecurityDetail struct {
	Namespace         string                  `json:"namespace"`
	Kind              string                  `json:"kind"`
	Name              string                  `json:"name"`
	Images            []string                `json:"images,omitempty"`
	Vulnerabilities   []SecurityVulnerability `json:"vulnerabilities"`
	VulnerabilityCnt  SecuritySeverityCounts  `json:"vulnerabilityCounts"`
	Misconfigurations []SecurityCheck         `json:"misconfigurations"`
	MisconfigCnt      SecuritySeverityCounts  `json:"misconfigurationCounts"`
	PolicyResults     []SecurityPolicyResult  `json:"policyResults"`
	PolicyCnt         SecurityPolicyCounts    `json:"policyCounts"`
	Truncated         bool                    `json:"truncated,omitempty"`
}
