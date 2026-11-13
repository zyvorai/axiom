// constants/testIds/ — central registry of data-testid values used by the
// end-to-end testing agent (qabot) to locate and interact with UI elements
// during automated tests. UI without testids cannot be automatically verified.
//
// Structure: each feature lives in its own file (auth.js, cart.js, ...) and
// is re-exported from here, so consumers can do a single import like
// `import { LOGIN, CART } from '@/constants/testIds'` (or relative).
//
// Adding a new feature:
//   1. Create constants/testIds/<feature>.js
//   2. Export named objects (e.g. `export const PROFILE = { ... }`)
//   3. Re-export here: `export * from './<feature>';`

export * from './auth';
export * from './home';
export * from './dashboard';
export * from './ux';
export * from './troubleshoot';
export * from './security';
export * from './ai';
export * from './identity';
export * from './fleet';
export * from './gitops';
export * from './autoscaling';
export * from './deferred';
export * from './billing';
export * from './podresize';
export * from './costidle';
export * from './velero';
export * from './rollouts';
export * from './nodedrain';
