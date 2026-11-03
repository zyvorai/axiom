// Test IDs for the cross-cluster Fleet page. Naming follows ./auth.js.

export const FLEET = {
    root: 'fleet-page',
    refresh: 'fleet-refresh',
    summary: (k) => `fleet-summary-${k}`,
    card: (name) => `fleet-cluster-${name}`,
    state: (name) => `fleet-state-${name}`,
    detail: (name) => `fleet-detail-${name}`,
    switch: (name) => `fleet-switch-${name}`,
    checked: 'fleet-checked',
    empty: 'fleet-empty',
};
