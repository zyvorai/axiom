// Test IDs for the progressive-delivery view (Argo Rollouts + Flagger) on Ops -> GitOps.
// Naming follows ./auth.js.

export const ROLLOUTS = {
    root: 'rollouts-panel',
    notInstalled: 'rollouts-not-installed',
    argoRow: (ns, name) => `rollout-row-${ns}-${name}`,
    argoPhase: (ns, name) => `rollout-phase-${ns}-${name}`,
    argoAnalysis: (ns, name) => `rollout-analysis-${ns}-${name}`,
    canaryRow: (ns, name) => `flagger-row-${ns}-${name}`,
    canaryPhase: (ns, name) => `flagger-phase-${ns}-${name}`,
};
