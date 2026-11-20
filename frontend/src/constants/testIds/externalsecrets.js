// External Secrets Operator sync-health view (Secrets page).
export const EXTERNAL_SECRETS = {
    root: "external-secrets-root",
    unavailable: "external-secrets-unavailable",
    rollup: "external-secrets-rollup",
    count: (bucket) => `external-secrets-count-${bucket}`,
    row: (ns, name) => `external-secrets-row-${ns}-${name}`,
    health: (ns, name) => `external-secrets-health-${ns}-${name}`,
    storesRoot: "external-secrets-stores",
    storeRow: (kind, name) => `external-secrets-store-${kind}-${name}`,
};
