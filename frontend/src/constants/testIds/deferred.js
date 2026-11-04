// Test IDs for the v1.0 follow-ups: fleet history, OpenCost, OCI Helm repos,
// LDAP two-factor listing, GitOps edit + credentials, VPA/KEDA creation.
// Naming follows ./auth.js.

export const FLEET_HISTORY = {
    window: 'fleet-history-window',
    uptime: (c) => `fleet-uptime-${c}`,
    strip: (c) => `fleet-strip-${c}`,
    incidents: (c) => `fleet-incidents-${c}`,
    incident: (c, i) => `fleet-incident-${c}-${i}`,
    none: (c) => `fleet-no-incidents-${c}`,
};

export const OPENCOST = {
    source: 'cost-source',
    window: 'cost-window',
    idle: 'cost-idle',
    note: 'cost-note',
    breakdown: (ns) => `cost-breakdown-${ns}`,
    budgetSource: 'cost-budget-source',
};

export const OCI = {
    kind: 'helm-repo-kind',
    kindOci: 'helm-repo-kind-oci',
    charts: 'helm-repo-oci-charts',
    badge: (repo, chart) => `helm-chart-oci-${repo}-${chart}`,
    ref: 'helm-chart-oci-ref',
};

export const LDAP_MFA = {
    section: 'ldap-mfa-section',
    row: (u) => `ldap-mfa-row-${u}`,
    reset: (u) => `ldap-mfa-reset-${u}`,
    empty: 'ldap-mfa-empty',
};

export const GITOPS_EDIT = {
    open: (kind, ns, name) => `gitops-edit-${kind}-${ns}-${name}`,
    dialog: 'gitops-edit-dialog',
    notEditable: 'gitops-edit-not-editable',
    reasons: 'gitops-edit-reasons',
    revision: 'gitops-edit-revision',
    path: 'gitops-edit-path',
    chartVersion: 'gitops-edit-chart-version',
    destNamespace: 'gitops-edit-dest-namespace',
    interval: 'gitops-edit-interval',
    values: 'gitops-edit-values',
    preview: 'gitops-edit-preview',
    changes: 'gitops-edit-changes',
    noop: 'gitops-edit-noop',
    save: 'gitops-edit-save',
    error: 'gitops-edit-error',
    stale: 'gitops-edit-stale',
};

export const CREDS = {
    open: 'gitops-creds-open',
    dialog: 'gitops-creds-dialog',
    row: (n) => `gitops-cred-${n}`,
    remove: (n) => `gitops-cred-remove-${n}`,
    engine: 'gitops-cred-engine',
    name: 'gitops-cred-name',
    namespace: 'gitops-cred-namespace',
    url: 'gitops-cred-url',
    authType: 'gitops-cred-authtype',
    username: 'gitops-cred-username',
    password: 'gitops-cred-password',
    token: 'gitops-cred-token',
    sshKey: 'gitops-cred-sshkey',
    knownHosts: 'gitops-cred-knownhosts',
    save: 'gitops-cred-save',
    error: 'gitops-cred-error',
    writeOnly: 'gitops-cred-writeonly',
};

export const VPA_CREATE = {
    open: 'vpa-new',
    dialog: 'vpa-create-dialog',
    namespace: 'vpa-namespace',
    name: 'vpa-name',
    target: 'vpa-target',
    mode: 'vpa-mode',
    cpu: 'vpa-res-cpu',
    memory: 'vpa-res-memory',
    preview: 'vpa-preview',
    yaml: 'vpa-preview-yaml',
    warnings: 'vpa-warnings',
    create: 'vpa-create',
    error: 'vpa-error',
    remove: (ns, n) => `vpa-remove-${ns}-${n}`,
};

export const KEDA_CREATE = {
    open: 'keda-new',
    dialog: 'keda-create-dialog',
    namespace: 'keda-namespace',
    name: 'keda-name',
    target: 'keda-target',
    min: 'keda-min',
    max: 'keda-max',
    addTrigger: 'keda-add-trigger',
    triggerType: (i) => `keda-trigger-type-${i}`,
    triggerField: (i, k) => `keda-trigger-${i}-${k}`,
    triggerAuth: (i) => `keda-trigger-auth-${i}`,
    removeTrigger: (i) => `keda-trigger-remove-${i}`,
    preview: 'keda-preview',
    yaml: 'keda-preview-yaml',
    warnings: 'keda-warnings',
    create: 'keda-create',
    error: 'keda-error',
    remove: (ns, n) => `keda-remove-${ns}-${n}`,
};

export const REGCRED = {
    manage: 'helm-regcred-manage',
    dialog: 'helm-regcred-dialog',
    row: (n) => `helm-regcred-${n}`,
    remove: (n) => `helm-regcred-remove-${n}`,
    name: 'helm-regcred-name',
    registry: 'helm-regcred-registry',
    username: 'helm-regcred-username',
    password: 'helm-regcred-password',
    save: 'helm-regcred-save',
    error: 'helm-regcred-error',
    writeOnly: 'helm-regcred-writeonly',
    select: 'helm-repo-credentials-select',
    locked: (repo) => `helm-repo-locked-${repo}`,
};

export const AUTOSCALE_EDIT = {
    vpaOpen: (ns, n) => `vpa-edit-${ns}-${n}`,
    kedaOpen: (ns, n) => `keda-edit-${ns}-${n}`,
    dialog: 'autoscale-edit-dialog',
    notEditable: 'autoscale-edit-not-editable',
    reasons: 'autoscale-edit-reasons',
    mode: 'vpa-edit-mode',
    cpu: 'vpa-edit-cpu',
    memory: 'vpa-edit-memory',
    min: 'keda-edit-min',
    max: 'keda-edit-max',
    polling: 'keda-edit-polling',
    cooldown: 'keda-edit-cooldown',
    triggerField: (i, k) => `keda-edit-trigger-${i}-${k}`,
    triggerRemove: (i) => `keda-edit-trigger-remove-${i}`,
    addTrigger: 'keda-edit-add-trigger',
    preview: 'autoscale-edit-preview',
    changes: 'autoscale-edit-changes',
    noop: 'autoscale-edit-noop',
    save: 'autoscale-edit-save',
    error: 'autoscale-edit-error',
};
