export const NAV = {
    sidebar: "sidebar-nav",
    sidebarCollapseToggle: "sidebar-collapse-toggle",
    themeToggle: "theme-toggle",
    clusterSwitcher: "cluster-switcher",
    clusterOption: (id) => `cluster-option-${id}`,
    link: (key) => `nav-link-${key}`,
    search: "topbar-search",
    connect: "topbar-connect",
};

export const PALETTE = {
    dialog: "command-palette",
    input: "command-palette-input",
    item: (k) => `palette-item-${k}`,
};

export const OVERVIEW = {
    root: "overview-page",
    metric: (k) => `metric-${k}`,
    alert: (i) => `alert-item-${i}`,
    activity: (i) => `activity-item-${i}`,
};

export const NODES = {
    root: "nodes-page",
    row: (name) => `node-row-${name}`,
};

export const WORKLOADS = {
    root: "workloads-page",
    row: (name) => `workload-row-${name}`,
    filterNs: "workload-filter-namespace",
    filterKind: "workload-filter-kind",
    search: "workload-search",
};

export const NET = {
    root: "networking-page",
    policyRow: (name) => `policy-row-${name}`,
    flowRow: (id) => `flow-row-${id}`,
};

export const VMS = {
    root: "vms-page",
    card: (name) => `vm-card-${name}`,
    consoleBtn: (name) => `vm-console-${name}`,
    consoleModal: "vm-console-modal",
    consoleCanvas: "vm-console-canvas",
    consoleClose: "vm-console-close",
};

export const LOGS = {
    root: "logs-page",
    podSelect: "logs-pod-select",
    startBtn: "logs-start",
    stopBtn: "logs-stop",
    stream: "logs-stream",
    clearBtn: "logs-clear",
};

export const STORAGE = {
    root: "storage-page",
    classRow: (name) => `storage-class-${name}`,
    pvcRow: (name) => `pvc-row-${name}`,
};

export const EVENTS = {
    root: "events-page",
    row: (i) => `event-row-${i}`,
};
