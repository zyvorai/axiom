// Test IDs for the Autoscaling page (HPA / VPA / KEDA) and the Cost tab's
// budgets and rightsizing. Naming follows ./auth.js.

export const AUTOSCALE = {
    tab: (k) => `autoscale-tab-${k}`,
    hpaRow: (ns, name) => `hpa-row-${ns}-${name}`,
    hpaMetrics: (ns, name) => `hpa-metrics-${ns}-${name}`,
    hpaState: (ns, name) => `hpa-state-${ns}-${name}`,
    createMemory: 'hpa-create-memory',
    createStabilization: 'hpa-create-stabilization',
    vpaInstalled: 'vpa-installed',
    vpaNotInstalled: 'vpa-not-installed',
    vpaRow: (ns, name) => `vpa-row-${ns}-${name}`,
    vpaRecommendation: (ns, name) => `vpa-rec-${ns}-${name}`,
    kedaNotInstalled: 'keda-not-installed',
    kedaRow: (ns, name) => `keda-row-${ns}-${name}`,
    kedaTriggers: (ns, name) => `keda-triggers-${ns}-${name}`,
};

export const COST = {
    budgets: 'cost-budgets',
    budgetRow: (ns) => `cost-budget-${ns}`,
    budgetState: (ns) => `cost-budget-state-${ns}`,
    budgetBar: (ns) => `cost-budget-bar-${ns}`,
    budgetEdit: (ns) => `cost-budget-edit-${ns}`,
    budgetRemove: (ns) => `cost-budget-remove-${ns}`,
    unbudgeted: (ns) => `cost-unbudgeted-${ns}`,
    setBudget: (ns) => `cost-set-budget-${ns}`,
    addBudget: 'cost-add-budget',
    budgetDialog: 'cost-budget-dialog',
    budgetNamespace: 'cost-budget-namespace',
    budgetAmount: 'cost-budget-amount',
    budgetWarn: 'cost-budget-warn',
    budgetSave: 'cost-budget-save',
    totals: 'cost-budget-totals',
    rightsizing: 'cost-rightsizing',
    window: 'cost-rightsizing-window',
    source: 'cost-rightsizing-source',
    summary: (k) => `cost-rightsizing-summary-${k}`,
    rsRow: (ns, w, c) => `cost-rs-${ns}-${w}-${c}`,
    rsVerdict: (ns, w, c) => `cost-rs-verdict-${ns}-${w}-${c}`,
    rsEmpty: 'cost-rightsizing-empty',
};
