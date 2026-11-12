// Test IDs for the experimental actual-bill reconciliation: the invoice card on
// Ops -> Cost and its Settings -> Integrations form. Naming follows ./auth.js.

export const BILLING = {
    card: 'cost-invoice-card',
    badge: 'cost-invoice-experimental',
    provider: 'cost-invoice-provider',
    error: 'cost-invoice-error',
    note: 'cost-invoice-note',
    row: (month) => `cost-invoice-row-${month}`,
    invoice: (month) => `cost-invoice-total-${month}`,
    allocated: (month) => `cost-invoice-allocated-${month}`,
    unallocated: (month) => `cost-invoice-unallocated-${month}`,
};

export const BILLING_SETTINGS = {
    card: 'billing-settings-card',
    provider: 'billing-settings-provider',
    tagKey: 'billing-settings-tag-key',
    tagValue: 'billing-settings-tag-value',
    awsRegion: 'billing-settings-aws-region',
    awsAccessKeyId: 'billing-settings-aws-access-key-id',
    awsSecretAccessKey: 'billing-settings-aws-secret-access-key',
    awsSessionToken: 'billing-settings-aws-session-token',
    azureTenantId: 'billing-settings-azure-tenant-id',
    azureClientId: 'billing-settings-azure-client-id',
    azureSubscriptionId: 'billing-settings-azure-subscription-id',
    azureClientSecret: 'billing-settings-azure-client-secret',
    gcpProjectId: 'billing-settings-gcp-project-id',
    gcpTable: 'billing-settings-gcp-table',
    gcpServiceAccountKey: 'billing-settings-gcp-service-account-key',
    save: 'billing-settings-save',
    test: 'billing-settings-test',
};
