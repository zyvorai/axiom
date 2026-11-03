// Test IDs for the cross-page UX features: global namespace selector, saved views, keyboard
// shortcuts help, change review (YAML diff) before apply, relationship graph, notification inbox.

export const NS_SELECT = {
    global: "topbar-namespace",
};

export const VIEWS = {
    trigger: "saved-views-trigger",
    panel: "saved-views-panel",
    nameInput: "saved-views-name",
    save: "saved-views-save",
    item: (name) => `saved-view-${name}`,
    remove: (name) => `saved-view-delete-${name}`,
};

export const SHORTCUT_UI = {
    dialog: "shortcuts-dialog",
};

export const DIFF = {
    dialog: "diff-review-dialog",
    review: "diff-review",
    view: "diff-view",
    stats: "diff-stats",
    warning: "diff-warning",
    reviewButton: "diff-review-button",
    applyButton: "diff-apply-button",
};

export const GRAPH = {
    root: "resource-graph",
    node: (id) => `graph-node-${id}`,
    list: "resource-graph-list",
};

export const INBOX = {
    bell: "inbox-bell",
    panel: "inbox-panel",
    item: (i) => `inbox-item-${i}`,
    markAllRead: "inbox-mark-all-read",
    dismiss: (i) => `inbox-dismiss-${i}`,
    empty: "inbox-empty",
};
