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
    search: 'fleet-search',
    searchKind: 'fleet-search-kind',
    searchResults: 'fleet-search-results',
    searchRow: (cluster, kind, ns, name) => `fleet-hit-${cluster}-${kind}-${ns || '_'}-${name}`,
    searchOpen: (cluster, kind, ns, name) => `fleet-hit-open-${cluster}-${kind}-${ns || '_'}-${name}`,
    searchSkipped: 'fleet-search-skipped',
    searchNone: 'fleet-search-none',
};
