// Test IDs for the Helm release history / diff / rollback dialog (Ops > Helm).
// Naming follows ./auth.js.

export const HELM_HISTORY = {
    open: (name) => `helm-history-open-${name}`,
    dialog: 'helm-history-dialog',
    error: 'helm-history-error',
    row: (rev) => `helm-history-rev-${rev}`,
    pickFrom: 'helm-history-from',
    pickTo: 'helm-history-to',
    compare: 'helm-history-compare',
    diffValues: 'helm-history-diff-values',
    diffManifest: 'helm-history-diff-manifest',
    reviewRollback: (rev) => `helm-history-review-${rev}`,
    preview: 'helm-rollback-preview',
    warning: (code) => `helm-rollback-warning-${code}`,
    confirmInput: 'helm-rollback-confirm-input',
    confirm: 'helm-rollback-confirm',
    back: 'helm-history-back',
};
