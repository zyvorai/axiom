// Test IDs for the Workstream C security views: the read-only RBAC viewer
// (Ops -> Access), the NetworkPolicy analysis (Networking) and /security.
// Naming follows ./auth.js (keys camelCase, values kebab-case).

export const RBAC = {
    viewSwitch: "rbac-view-switch",
    viewTab: (k) => `rbac-view-${k}`,
    subjectsRoot: "rbac-subjects",
    subjectSearch: "rbac-subject-search",
    subjectKind: "rbac-subject-kind",
    subjectRow: (kind, name) => `rbac-subject-row-${kind}-${name}`,
    subjectDetail: "rbac-subject-detail",
    grant: (binding) => `rbac-grant-${binding}`,
    whoRoot: "rbac-who-can",
    whoVerb: "rbac-who-verb",
    whoResource: "rbac-who-resource",
    whoNamespace: "rbac-who-namespace",
    whoName: "rbac-who-name",
    whoRun: "rbac-who-run",
    whoRow: (kind, name, scope) => `rbac-who-row-${kind}-${name}-${scope}`,
    denied: "rbac-viewer-denied",
};

export const NETPOL = {
    root: "netpol-analysis",
    namespace: "netpol-analysis-namespace",
    summary: "netpol-analysis-summary",
    uncovered: "netpol-analysis-uncovered",
    podRow: (name) => `netpol-pod-${name}`,
    podDetail: (name) => `netpol-pod-detail-${name}`,
};

export const SECURITY = {
    root: "security-page",
    namespace: "security-namespace",
    empty: "security-empty",
    installHint: "security-install-hint",
    source: (k) => `security-source-${k}`,
    count: (k) => `security-count-${k}`,
    workloadRow: (name) => `security-workload-${name}`,
    detail: "security-detail",
    detailTab: (k) => `security-detail-tab-${k}`,
    vulnRow: (id) => `security-vuln-${id}`,
};
