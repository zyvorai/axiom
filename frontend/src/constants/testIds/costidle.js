// data-testid values for the Idle resources section of Ops -> Cost.
export const COST_IDLE = {
    root: 'cost-idle',
    minAge: 'cost-idle-min-age',
    total: 'cost-idle-total',
    savings: 'cost-idle-savings',
    empty: 'cost-idle-empty',
    error: 'cost-idle-error',
    row: (kind, ns, name) => `cost-idle-row-${kind}-${ns || '_'}-${name}`,
    confidence: (kind, ns, name) => `cost-idle-confidence-${kind}-${ns || '_'}-${name}`,
};
