// Test IDs for the troubleshooting features: change timeline, cluster health
// check, workload log aggregation, debug containers, node shell and the
// port-forward helper.
export const TIMELINE = {
    root: "timeline-page",
    namespace: "timeline-namespace",
    search: "timeline-search",
    source: (s) => `timeline-source-${s}`,
    since: "timeline-since",
    objectChip: "timeline-object-chip",
    row: (i) => `timeline-row-${i}`,
    empty: "timeline-empty",
    refresh: "timeline-refresh",
    workloadRoot: "workload-timeline",
    workloadRow: (i) => `workload-timeline-row-${i}`,
    workloadTab: "workload-tab-changes",
    eventsLink: "events-open-timeline",
};

export const HEALTH = {
    root: "health-check-page",
    namespace: "health-namespace",
    status: "health-status",
    count: (sev) => `health-count-${sev}`,
    finding: (i) => `health-finding-${i}`,
    refresh: "health-refresh",
    errors: "health-errors",
    clean: "health-clean",
    panel: "overview-health-panel",
    panelFinding: (i) => `overview-health-finding-${i}`,
    panelLink: "overview-health-link",
};

export const WLOGS = {
    modePod: "logs-mode-pod",
    modeWorkload: "logs-mode-workload",
    workloadSelect: "logs-workload-select",
    filter: "logs-filter",
    filterError: "logs-filter-error",
    timestamps: "logs-timestamps",
    previous: "logs-previous",
    download: "logs-download",
    count: "logs-line-count",
    line: (i) => `logs-line-${i}`,
};

export const DEBUG = {
    panel: "debug-container-panel",
    target: "debug-container-target",
    start: "debug-container-start",
    result: "debug-container-result",
    message: "debug-container-message",
    error: "debug-container-error",
};

export const NODESHELL = {
    panel: "node-shell-panel",
    disabled: "node-shell-disabled",
    warning: "node-shell-warning",
};

export const PORTFWD = {
    copy: (port) => `pod-port-forward-copy-${port}`,
    open: (port) => `pod-port-open-${port}`,
};
