import { lucideSvg } from './lucide-icons.js';

const $ = (q, root = document) => root.querySelector(q);
const $$ = (q, root = document) => [...root.querySelectorAll(q)];

const navPrimary = [
  ['overview', 'Overview'],
  ['nodes', 'Nodes'],
  ['workloads', 'Workloads'],
  ['network', 'Networking · Cilium'],
  ['vms', 'Virtual Machines'],
  ['volumes', 'Storage'],
  ['logs', 'Pod Logs'],
  ['events', 'Events'],
];

const navSystem = [
  ['pods', 'Pods'],
  ['secrets', 'Secrets'],
  ['backups', 'Backups'],
  ['alerts', 'Alerts'],
  ['ops', 'Ops'],
  ['settings', 'Settings'],
];

const OPS_HUB_TABS = [
  ['helm', 'Helm'],
  ['gitops', 'GitOps'],
  ['agents', 'Agents'],
  ['identity', 'Identity'],
  ['audit', 'Audit'],
  ['rbac', 'Access'],
  ['cost', 'Cost'],
];

const OPS_HUB_IDS = new Set(OPS_HUB_TABS.map(([id]) => id));

const mobileNav = [
  ['overview', 'Overview'],
  ['pods', 'Pods'],
  ['vms', 'VMs'],
  ['more', 'More'],
];

const state = {
  credentials: null,
  session: null,
  page: 'overview',
  overview: null,
  workloads: [],
  pods: [],
  nodes: [],
  vms: [],
  atlas: null,
  atlasVolumeModal: false,
  integrations: null,
  integrationExamples: [],
  integrationDraft: null,
  volumes: [],
  utilization: null,
  regions: [],
  region: localStorage.getItem('axiom.region') || '',
  regionMenu: false,
  namespaceScope: localStorage.getItem('axiom.ns') || 'all',
  events: [],
  namespaces: [],
  services: [],
  network: null,
  networkTab: 'map',
  networkNs: 'all',
  namespaceModal: false,
  selected: null,
  selectedType: null,
  drawerTab: 'overview',
  palette: false,
  paletteHighlight: 0,
  launch: false,
  placeStep: 1,
  placeIntent: '',
  placeDraft: null,
  placeReceipt: '',
  still: null,
  presenceKnocked: false,
  estateQuery: '',
  estateFloor: 'all',
  placePreview: null,
  paletteQuery: '',
  rolloutData: null,
  resourceDetail: null,
  networkGuard: null,
  packetwolfStatus: null,
  packetwolfFlows: null,
  packetwolfFlowsLoading: false,
  nodeDrawerTab: 'overview',
  loginShowPassword: false,
  authProviders: null,
  loginShowLdap: false,
  loginShowApiKey: false,
  loginShowSso: false,
  clusterMenu: false,
  userMenu: false,
  loginPending: null,
  loginShowDemo: false,
  activeClusterId: localStorage.getItem('axiom.activeCluster') || 'default',
  logsPodIndex: 0,
  mobileSidebarOpen: false,
  placeMode: 'intent',
  placeForm: { name: 'hello-web', image: 'nginx:1.27', replicas: 1, memory: '256Mi', namespace: 'default', port: 80 },
  placeYaml: '',
  loading: false,
  theme: localStorage.getItem('axiom.theme') || 'system',
  shellPalette: (() => {
    const p = localStorage.getItem('axiom.palette') || 'carbon';
    return p === 'ember' ? 'carbon' : p;
  })(),
  density: localStorage.getItem('axiom.density') || 'comfortable',
  railExpanded: localStorage.getItem('axiom.rail') === 'expanded',
  dockAutoHide: localStorage.getItem('axiom.dockAutoHide') === '1',
  dockVisible: true,
  controlCenter: false,
  clock: '',
  workloadFilter: 'all',
  podFilter: 'all',
  vmFilter: 'all',
  volumeFilter: 'all',
  vmLifecycleTab: 'snapshots',
  vmMenu: null,
  vmWizard: null,
  vmExportSupported: localStorage.getItem('axiom.vmExport') !== '0',
  opsTab: localStorage.getItem('axiom.opsTab') || 'helm',
  jobs: [],
  jobsExpanded: true,
  sseConnected: false,
  sseDisconnectedAt: null,
  realtimeCapabilities: null,
  scaleDraft: null,
  secrets: [],
  backups: [],
  helmReleases: [],
  externalSecrets: [],
  vmSnapshots: [],
  vmClones: [],
  vmExports: [],
  vmRestores: [],
  veleroBackups: [],
  veleroSchedules: [],
  alertmanagers: [],
  prometheusRules: [],
  gitops: { applications: [], kustomizations: [], helmReleases: [] },
  alerts: [],
  audit: [],
  rbac: { clusterRoles: [], clusterRoleBindings: [], roles: [], roleBindings: [] },
  cost: null,
  opsLoaded: {},
  agentRuns: [],
  platformIntents: [],
  agentRunModal: false,
  agentRunDetail: null,
  havenIdentity: null,
  secretModal: false,
  secretExpand: null,
  secretDetail: null,
  rbacTab: 'bindings',
  vmConsole: null,
  helmRollback: null,
  validateResult: null,
};
let logPollTimer = null;
let terminalSocket = null;
let sseAbort = null;
let refreshDebounceTimer = null;
let shellMenuCloser = null;

function floorForNamespace(ns) {
  const low = String(ns || '').toLowerCase();
  if (ns === 'kube-system' || ns === 'network' || ns === 'axiom-system') return 'plant';
  if (low.includes('staging') || ns === 'migrate') return 'staging';
  return 'prod';
}

const FLOOR_LABELS = { plant: 'System', staging: 'Staging', prod: 'Production' };
const SSO_PROVIDERS = [
  { id: 'google', label: 'Continue with Google', authKey: 'oidc' },
  { id: 'github', label: 'Continue with GitHub', authKey: 'oidc' },
  { id: 'okta', label: 'Continue with Okta', authKey: 'saml' },
];

function ssoConfigured(providers) {
  if (!providers) return false;
  return !!(providers.oidc?.enabled || providers.saml?.enabled || providers.ldap?.enabled);
}

function oidcOrSamlConfigured(providers) {
  if (!providers) return false;
  return !!(providers.oidc?.enabled || providers.saml?.enabled);
}

function isLocalDevHost() {
  const h = location.hostname;
  return h === 'localhost' || h === '127.0.0.1' || h === '[::1]';
}

function resolveLoginShowDemo() {
  return !!state.loginShowDemo;
}

function defaultLoginUsername() {
  return state.authProviders?.local?.default_username || savedLoginUsername();
}

function demoCredentialHint() {
  const user = defaultLoginUsername();
  if (user === 'demo' || user === 'demo@zyvor.dev') return 'demo / demo';
  return `${user} / (your AXIOM_PASSWORD)`;
}

function workloadIsSealed(workload) {
  if (!workload) return false;
  if (workload.status === 'Sealed') return true;
  return workloadPods(workload).some(p => p.sealed || p.status === 'Sealed');
}

function zyvorMark(size = 'lg') {
  const px = size === 'sm' ? 40 : 48;
  return `<button type="button" class="z-mark" data-login-mark style="width:${px}px;height:${px}px;border:0;border-radius:${size === 'sm' ? 12 : 16}px;display:grid;place-items:center;background:linear-gradient(135deg,#ff7550,#c8452a);color:#fff" aria-label="Zyvor">
    <svg width="${Math.round(px / 2)}" height="${Math.round(px / 2)}" viewBox="0 0 18 18" aria-hidden="true"><path d="M2 2h14L6.6 16H16" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linejoin="round" stroke-linecap="round"/></svg>
  </button>`;
}

function ssoIcon(name) {
  if (name === 'google') return `<svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true"><path fill="#4285F4" d="M23.5 12.27c0-.85-.08-1.66-.22-2.45H12v4.64h6.45a5.52 5.52 0 0 1-2.39 3.62v3h3.87c2.26-2.09 3.57-5.16 3.57-8.81Z"/><path fill="#34A853" d="M12 24c3.24 0 5.96-1.07 7.93-2.91l-3.87-3c-1.07.72-2.44 1.14-4.06 1.14-3.12 0-5.77-2.11-6.72-4.95H1.3v3.1A12 12 0 0 0 12 24Z"/><path fill="#FBBC05" d="M5.28 14.28a7.2 7.2 0 0 1 0-4.56v-3.1H1.3a12 12 0 0 0 0 10.76l3.98-3.1Z"/><path fill="#EA4335" d="M12 4.77c1.76 0 3.34.6 4.58 1.79l3.44-3.44A11.98 11.98 0 0 0 1.3 6.62l3.98 3.1C6.23 6.88 8.88 4.77 12 4.77Z"/></svg>`;
  if (name === 'github') return `<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 .5A11.5 11.5 0 0 0 .5 12c0 5.08 3.29 9.39 7.86 10.91.58.11.79-.25.79-.56v-2.17c-3.2.7-3.87-1.37-3.87-1.37-.52-1.33-1.28-1.68-1.28-1.68-1.04-.71.08-.7.08-.7 1.15.08 1.76 1.19 1.76 1.19 1.03 1.75 2.69 1.25 3.35.95.1-.74.4-1.25.72-1.54-2.55-.29-5.24-1.28-5.24-5.68 0-1.26.45-2.28 1.19-3.09-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.18 1.18a11.05 11.05 0 0 1 5.8 0C17.85 4.9 18.82 5.2 18.82 5.2c.63 1.6.23 2.77.11 3.06.74.8 1.18 1.83 1.18 3.09 0 4.41-2.69 5.38-5.26 5.66.41.36.78 1.05.78 2.13v3.16c0 .31.2.68.8.56A11.5 11.5 0 0 0 23.5 12 11.5 11.5 0 0 0 12 .5Z"/></svg>`;
  return `<svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="10" fill="none" stroke="#007DC1" stroke-width="4.5"/></svg>`;
}

function metricRing(value, label, color, sublabel) {
  const size = 108, stroke = 10, v = Math.max(0, Math.min(100, Number(value) || 0)), r = (size - stroke) / 2, c = 2 * Math.PI * r, dash = (v / 100) * c;
  const id = `metric-${String(label).replace(/\W/g, '')}`;
  return `<div class="metric-ring" style="display:flex;align-items:center;gap:12px"><svg width="${size}" height="${size}" viewBox="0 0 ${size} ${size}" style="transform:rotate(-90deg)"><defs><linearGradient id="${id}" x1="0" y1="0" x2="1" y2="1"><stop offset="0%" stop-color="${enc(color)}"/><stop offset="100%" stop-color="${enc(color)}" stop-opacity=".4"/></linearGradient></defs><circle cx="54" cy="54" r="${r}" stroke="currentColor" opacity=".12" stroke-width="${stroke}" fill="none"/><circle cx="54" cy="54" r="${r}" stroke="url(#${id})" stroke-linecap="round" stroke-width="${stroke}" fill="none" stroke-dasharray="${dash} ${c - dash}"/></svg><div><div class="eyebrow">${enc(label)}</div><strong style="font-size:28px">${v}%</strong>${sublabel ? `<small class="muted" style="display:block">${enc(sublabel)}</small>` : ''}</div></div>`;
}

function sparklineSvg(values, color, height = 80) {
  const data = Array.isArray(values) && values.length > 1 ? values : [42, 50, 45, 62, 57, 70];
  const points = data.map((v, i) => `${(i / (data.length - 1) * 100).toFixed(1)},${(100 - Math.max(0, Math.min(100, Number(v) || 0))).toFixed(1)}`).join(' ');
  const id = `spark-${String(color).replace(/\W/g, '')}`;
  return `<svg width="100%" height="${height}" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true"><defs><linearGradient id="${id}" x1="0" y1="0" x2="0" y2="1"><stop stop-color="${enc(color)}" stop-opacity=".5"/><stop offset="1" stop-color="${enc(color)}" stop-opacity="0"/></linearGradient></defs><polygon points="${points} 100,100 0,100" fill="url(#${id})"/><polyline points="${points}" fill="none" stroke="${enc(color)}" stroke-width="2.5" vector-effect="non-scaling-stroke"/></svg>`;
}

function glassCard(content, className = '') { return `<section class="glass specular ${className}">${content}</section>`; }
function statusBadgeGlass(status) {
  const s = String(status || '');
  const key = /healthy|ready|running|bound|normal|forwarded/i.test(s) ? 'ok'
    : /degraded|warning|pending|paused/i.test(s) ? 'warn'
    : /fail|notready|error|dropped/i.test(s) ? 'err' : 'info';
  return `<span class="status-badge-glass"><span class="status-dot ${key}"></span>${enc(s || 'Unknown')}</span>`;
}

function flowVerdictBadge(verdict) {
  const colors = { forwarded: 'ok', dropped: 'err', audit: 'info' };
  const key = colors[verdict] || 'info';
  return `<span class="status-badge-glass"><span class="status-dot ${key}"></span>${enc(verdict)}</span>`;
}

function demoNetworkingData() {
  const policies = [
    { name: 'default-deny-all', kind: 'CiliumClusterwideNetworkPolicy', endpoints: 142, hits_1h: 0 },
    { name: 'allow-dns', kind: 'CiliumNetworkPolicy', endpoints: 142, hits_1h: 5058 },
    { name: 'tenant-a-egress', kind: 'CiliumNetworkPolicy', endpoints: 34, hits_1h: 1210 },
    { name: 'tenant-b-isolation', kind: 'CiliumNetworkPolicy', endpoints: 21, hits_1h: 798 },
    { name: 'payments-strict', kind: 'CiliumNetworkPolicy', endpoints: 8, hits_1h: 98 },
    { name: 'monitoring-scrape', kind: 'CiliumNetworkPolicy', endpoints: 62, hits_1h: 762 },
  ];
  const flows = [
    { id: 'f1', source: 'kafka-3', source_ns: 'data', destination: 'vault-7', destination_ns: 'platform', l7: 'DNS query api.stripe.com', verdict: 'forwarded', bytes: 30106 },
    { id: 'f2', source: 'catalog-9', source_ns: 'production', destination: 'otel-1', destination_ns: 'external', l7: 'gRPC catalog.v1/Search', verdict: 'dropped', bytes: 11264 },
    { id: 'f3', source: 'checkout-3', source_ns: 'production', destination: 'kafka-5', destination_ns: 'data', l7: 'HTTP GET /health', verdict: 'dropped', bytes: 26624 },
    { id: 'f4', source: 'redis-6', source_ns: 'production', destination: 'stripe-6', destination_ns: 'data', l7: 'DNS query api.stripe.com', verdict: 'dropped', bytes: 36454 },
    { id: 'f5', source: 'auth-2', source_ns: 'data', destination: 'redis-9', destination_ns: 'data', l7: 'DNS query api.stripe.com', verdict: 'audit', bytes: 5427 },
    { id: 'f6', source: 'api-gateway-1', source_ns: 'production', destination: 'external-9', destination_ns: 'data', l7: 'HTTP POST /api/orders', verdict: 'forwarded', bytes: 23757 },
    { id: 'f7', source: 'kafka-5', source_ns: 'commerce', destination: 'redis-6', destination_ns: 'platform', l7: 'Kafka fetch', verdict: 'forwarded', bytes: 17613 },
    { id: 'f8', source: 'api-gateway-1', source_ns: 'platform', destination: 'kafka-8', destination_ns: 'platform', l7: 'gRPC catalog.v1/Search', verdict: 'audit', bytes: 46182 },
    { id: 'f9', source: 'kafka-3', source_ns: 'data', destination: 'external-1', destination_ns: 'platform', l7: 'TCP :5432', verdict: 'forwarded', bytes: 25088 },
    { id: 'f10', source: 'kafka-3', source_ns: 'data', destination: 'stripe-4', destination_ns: 'data', l7: 'HTTP GET /health', verdict: 'forwarded', bytes: 4403 },
    { id: 'f11', source: 'checkout-2', source_ns: 'commerce', destination: 'postgres-1', destination_ns: 'platform', l7: 'HTTP GET /health', verdict: 'forwarded', bytes: 6042 },
    { id: 'f12', source: 'checkout-3', source_ns: 'commerce', destination: 'vault-6', destination_ns: 'platform', l7: 'HTTP GET /health', verdict: 'forwarded', bytes: 22221 },
    { id: 'f13', source: 'checkout-3', source_ns: 'platform', destination: 'otel-5', destination_ns: 'platform', l7: 'HTTP POST /v1/tokens', verdict: 'forwarded', bytes: 6144 },
    { id: 'f14', source: 'auth-9', source_ns: 'platform', destination: 'stripe-3', destination_ns: 'platform', l7: 'HTTP GET /metrics', verdict: 'forwarded', bytes: 36966 },
  ];
  return {
    cni: state.overview?.cni || 'Cilium 1.15.3',
    encryption: 'WireGuard',
    hubble_relay: 'healthy',
    identities: 200,
    endpoints: 184,
    throughput_mbps: 883,
    policies,
    flows,
    services_mesh: { engine: 'Cilium Service Mesh (Envoy-less)', mTLS: true },
  };
}
function clusterRecords() {
  const session = state.session || {}, overview = state.overview || {};
  const list = state.regions?.length ? state.regions : (overview.regions || []);
  const records = list.map(r => ({ id: r.id || r.name || r.label, name: r.name || r.label || r.id, status: r.status || 'Healthy', provider: r.provider || overview.provider || session.provider || 'Kubernetes', region: r.region || r.id || overview.region || 'local', version: r.version || overview.version || session.version || '—' })).filter(r => r.id);
  if (records.length) return records;
  return [{ id: 'default', name: cloudName(), status: overview.status || 'Healthy', provider: overview.provider || 'Kubernetes', region: overview.region || 'local', version: overview.version || '—' }];
}
function activeCluster() { return clusterRecords().find(c => c.id === state.activeClusterId) || clusterRecords()[0]; }
function ambientBackdrop() { return `<div class="ambient-backdrop" aria-hidden="true" style="position:fixed;inset:0;z-index:-1;overflow:hidden"><div class="grid-bg" style="position:absolute;inset:0;opacity:.6"></div><div class="orb" style="width:560px;height:560px;left:-8%;top:-10%;background:radial-gradient(circle at 30% 30%,rgba(240,88,58,.55),transparent 60%)"></div><div class="orb" style="width:480px;height:480px;right:-6%;top:18%;background:radial-gradient(circle at 60% 40%,rgba(34,211,238,.4),transparent 60%);animation-delay:-6s"></div><div class="orb" style="width:620px;height:620px;left:35%;bottom:-20%;background:radial-gradient(circle at 40% 60%,rgba(255,143,63,.28),transparent 60%);animation-delay:-11s"></div></div>`; }
function themeToggleGlass() {
  const dark = resolvedTheme() === 'dark';
  return `<button type="button" class="glass-inset specular theme-toggle-glass" id="theme-toggle-glass" aria-label="Toggle theme"><span style="transform:translateX(${dark ? '28px' : '0'});">${dark ? lucideSvg('moon', 16) : lucideSvg('sun', 16)}</span></button>`;
}

const REMEMBER_USER_KEY = 'axiom.login-remember-username';
const REMEMBER_FLAG_KEY = 'axiom.login-remember-me';

function savedLoginUsername() {
  if (localStorage.getItem(REMEMBER_FLAG_KEY) === '1') {
    return localStorage.getItem(REMEMBER_USER_KEY) || 'demo@zyvor.dev';
  }
  return 'demo@zyvor.dev';
}

function inferFixRecommendations(subject) {
  const status = String(subject?.status || '').toLowerCase();
  const name = subject?.name || 'workload';
  if (status.includes('crashloop') || status.includes('crash')) {
    return { title: `${name} is repeatedly crashing`, summary: 'Pods are exiting and restarting. Check logs for missing env vars, bad image tags, or failing readiness probes.', actions: [{ label: 'Open Logs', action: 'logs' }, { label: 'Restart', action: 'restart' }, { label: 'View Events', action: 'events' }] };
  }
  if (status.includes('imagepull') || status.includes('errimagepull')) {
    return { title: 'Cannot pull container image', summary: 'The registry may be unreachable, the image name may be wrong, or pull secrets may be missing.', actions: [{ label: 'View Events', action: 'events' }] };
  }
  if (status.includes('pending')) {
    return { title: `${name} cannot be scheduled`, summary: 'No node may have enough CPU/memory, or taints/tolerations may block placement.', actions: [{ label: 'View Nodes', action: 'nodes' }] };
  }
  if (status.includes('fail') || status.includes('error') || status.includes('unhealthy') || status.includes('degraded')) {
    return { title: `${name} needs attention`, summary: 'Health checks or deployment conditions report a problem.', actions: [{ label: 'Open Logs', action: 'logs' }, { label: 'Restart', action: 'restart' }] };
  }
  return null;
}

function renderFixItBanner(subject) {
  const fix = inferFixRecommendations(subject);
  if (!fix) return '';
  return `<div class="fix-it-banner"><div class="fix-it-icon">!</div><div><h4>${enc(fix.title)}</h4><p>${enc(fix.summary)}</p><div class="fix-it-actions">${fix.actions.map(a => `<button class="ghost mini-btn fix-it-btn" data-fix-action="${enc(a.action)}">${enc(a.label)}</button>`).join('')}</div></div></div>`;
}

function greetingPeriod() {
  const h = new Date().getHours();
  if (h < 12) return 'morning';
  if (h < 17) return 'afternoon';
  return 'evening';
}

function phaseGlowClass(status) {
  const s = String(status || '');
  if (s === 'Running') return 'phase-running';
  if (s.includes('CrashLoop') || s === 'Failed' || s === 'Error') return 'phase-crash pulse-fault';
  if (s === 'ScaledToZero') return 'phase-hushed';
  if (s === 'Sealed') return 'phase-sealed';
  if (s === 'Pending') return 'phase-pending';
  return 'phase-idle';
}

function workloadForPod(pod) {
  const wl = pod.workloadId || pod.labels?.['app.kubernetes.io/name'] || pod.labels?.app;
  if (!wl) return null;
  return state.workloads.find(w => w.namespace === pod.namespace && w.name === wl) || null;
}

function openWindow(pod) {
  const port = pod.ports?.[0]?.containerPort || 80;
  const qs = streamAuthQuery();
  const url = `/api/v1/pods/${encodeURIComponent(pod.namespace)}/${encodeURIComponent(pod.name)}/window/${port}/?${qs}`;
  window.open(url, '_blank', 'noopener');
  toast(`Port forward opened · ${port}`);
}

function placeSibling(pod) {
  state.launch = true;
  state.placeStep = 1;
  state.placeIntent = `Run ${pod.workloadId || pod.name}, ${pod.replicas || 1} copies, ${pod.memLimit || '256Mi'}`;
  state.placeDraft = null;
  render();
}

function waveformSparkline(seed) {
  const n = Number(seed) || 0;
  const bars = [];
  for (let i = 0; i < 12; i++) {
    const h = 20 + ((n * (i + 3) * 17) % 70);
    bars.push(`<span style="height:${h}%"></span>`);
  }
  return `<div class="waveform" aria-hidden="true">${bars.join('')}</div>`;
}

function honestBreath(pod) {
  const status = String(pod?.status || '');
  const info = pod.containerInfo || [];
  let failed = info.find(c => c.init && !c.ready);
  const crash = status.includes('CrashLoop') || status === 'Failed' || failed;
  if (!crash) return null;
  let name = failed?.name;
  if (!name) {
    const app = info.find(c => !c.init);
    name = app?.name || (pod.containers || [])[0] || 'app';
  }
  const reason = failed
    ? `Showing previous container output from ${name} — the pod is stuck in init, so the current container has not written logs yet.`
    : 'Pod is failing — showing previous container output when available.';
  return { previous: true, container: name, banner: reason };
}

function filterEstatePods(pods) {
  let list = pods;
  if (state.estateFloor !== 'all') {
    list = list.filter(p => floorForNamespace(p.namespace) === state.estateFloor);
  }
  const q = state.estateQuery.trim().toLowerCase();
  if (!q) return list;
  return list.filter(p => {
    const hay = `${p.name} ${p.workloadId || ''} ${p.namespace} ${p.node} ${p.status} ${p.image || ''} ${p.floor || floorForNamespace(p.namespace)} ${p.restarts}`.toLowerCase();
    return q.split(/\s+/).every(token => hay.includes(token));
  });
}

function estateFloors() {
  const pods = filterEstatePods(state.pods);
  const floors = { plant: [], staging: [], prod: [] };
  pods.forEach(p => floors[floorForNamespace(p.namespace)].push(p));
  return floors;
}

function isPodHushed(pod) {
  return state.workloads.some(w => w.namespace === pod.namespace && w.desired === 0 && (pod.name.startsWith(`${w.name}-`) || pod.name.includes(w.name)));
}

function podDisplayPhase(pod) {
  if (pod.sealed || pod.status === 'Sealed') return 'Sealed';
  if (isPodHushed(pod)) return 'ScaledToZero';
  return pod.status;
}

async function openStillRing(action) {
  let w = state.selected;
  let podName = '';
  if (state.selectedType === 'pod') {
    podName = state.selected.name;
    w = workloadForPod(state.selected);
  }
  if (!w) {
    if (state.selectedType === 'workload') w = state.selected;
    else { toast('No workload found for this pod', 'error'); return; }
  }
  try {
    const qs = `action=${encodeURIComponent(action)}${podName ? `&pod=${encodeURIComponent(podName)}` : ''}`;
    const preview = await api(`/api/v1/workloads/${encodeURIComponent(w.namespace)}/${encodeURIComponent(w.kind)}/${encodeURIComponent(w.name)}/still?${qs}`);
    state.still = { action, preview, workload: w, podName };
    render();
  } catch (error) { toast(error.message, 'error'); }
}

function followWires(ns) {
  cleanupPanels();
  state.selected = null;
  state.selectedType = null;
  state.drawerTab = 'overview';
  state.page = 'network';
  state.networkNs = ns || 'all';
  state.networkTab = 'map';
  refreshAll(false).then(() => render());
}

function enc(value) {
  return String(value ?? '').replace(/[&<>'"]/g, c => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;'
  }[c]));
}

const PALETTES = [
  { id: 'carbon', label: 'Carbon', swatch: '#2997ff' },
  { id: 'tahoe', label: 'Tahoe', swatch: '#38bdf8' },
  { id: 'steel', label: 'Dark Steel', swatch: '#7aa2ff' },
  { id: 'day', label: 'Day', swatch: '#e2e8f0' },
];

function resolvedTheme() {
  if (state.theme === 'system') {
    return matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
  }
  return state.theme === 'light' ? 'light' : 'dark';
}

function resolvedPalette() {
  if (resolvedTheme() === 'light') return 'day';
  const p = state.shellPalette || 'carbon';
  if (p === 'ember') return 'carbon';
  return ['carbon', 'tahoe', 'steel'].includes(p) ? p : 'carbon';
}

function applyTheme() {
  const theme = resolvedTheme();
  const palette = resolvedPalette();
  const root = document.documentElement;
  root.classList.toggle('dark', theme === 'dark');
  root.classList.toggle('light', theme === 'light');
  root.dataset.theme = theme;
  root.dataset.palette = palette;
  root.dataset.density = state.density || 'comfortable';
  const meta = { carbon: '#0f0f12', tahoe: '#0a0b0d', steel: '#0c1016', day: '#f5f5f7' };
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', meta[palette] || '#0a0b0d');
}

function setTheme(value) {
  state.theme = value;
  localStorage.setItem('axiom.theme', value);
  applyTheme();
  render();
}

function setShellPalette(id) {
  state.shellPalette = id;
  localStorage.setItem('axiom.palette', id);
  if (resolvedTheme() === 'light') {
    state.theme = 'dark';
    localStorage.setItem('axiom.theme', 'dark');
  }
  applyTheme();
  render();
}

function setDensity(value) {
  state.density = value;
  localStorage.setItem('axiom.density', value);
  applyTheme();
  render();
}

function toggleRail() {
  state.railExpanded = !state.railExpanded;
  localStorage.setItem('axiom.rail', state.railExpanded ? 'expanded' : 'collapsed');
  render();
}

function setDockAutoHide(on) {
  state.dockAutoHide = !!on;
  localStorage.setItem('axiom.dockAutoHide', state.dockAutoHide ? '1' : '0');
  state.dockVisible = !state.dockAutoHide;
  render();
}

function formatClock() {
  return new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

const ICON_ALIASES = {
  cloud: 'cloud',
  cloudLg: 'cloud',
  gauge: 'gauge',
  box: 'box',
  server: 'server',
  network: 'network',
  database: 'database',
  activity: 'activity',
  settings: 'settings',
  help: 'circle-help',
  logout: 'log-out',
  chevron: 'chevron-right',
  chevronR: 'chevron-right',
  search: 'search',
  menu: 'menu',
  arrow: 'arrow-up-right',
  shield: 'shield-check',
  cpu: 'cpu',
  drive: 'hard-drive',
  more: 'more-horizontal',
  check: 'check',
  refresh: 'refresh-cw',
  zap: 'zap',
  shieldPlain: 'shield',
  user: 'user',
  lock: 'lock',
  globe: 'globe',
  sun: 'sun',
  moon: 'moon',
};

const ICON_SIZES = { cloud: 20, cloudLg: 34, chevronR: 15, menu: 19, search: 16, help: 16, logout: 16, arrow: 16, chevron: 14 };

function icon(name, size) {
  const key = ICON_ALIASES[name] || name;
  return lucideSvg(key, size || ICON_SIZES[name] || 18);
}

const NAV_ICON_MAP = {
  overview: 'layout-dashboard',
  pods: 'package',
  nodes: 'hard-drive',
  vms: 'monitor-smartphone',
  network: 'waypoints',
  volumes: 'database',
  logs: 'scroll-text',
  events: 'bell-ring',
  settings: 'settings',
  workloads: 'boxes',
  secrets: 'key',
  backups: 'archive',
  helm: 'package',
  gitops: 'git-branch',
  alerts: 'alert-circle',
  audit: 'clipboard-list',
  rbac: 'shield-check',
  cost: 'dollar-sign',
  ops: 'wrench',
  agents: 'plug-zap',
  identity: 'fingerprint',
  support: 'circle-help',
  launch: 'box',
  logout: 'log-out',
  more: 'more-horizontal',
};

function navIcon(id, size = 18) {
  const key = NAV_ICON_MAP[id] || 'layout-dashboard';
  return `<span class="nav-icon">${lucideSvg(key, size, 'lucide-icon nav-glyph')}</span>`;
}

function asList(data, keys = []) {
  if (Array.isArray(data)) return data;
  if (!data || typeof data !== 'object') return [];
  for (const k of keys) {
    if (Array.isArray(data[k])) return data[k];
  }
  return [];
}

const OPS_PAGES = new Set(['secrets', 'backups', 'helm', 'gitops', 'agents', 'identity', 'alerts', 'audit', 'rbac', 'cost', 'ops']);

function nsParam() {
  return state.namespaceScope || 'all';
}

function nsQuery() {
  return `namespace=${encodeURIComponent(nsParam())}`;
}

function setNamespaceScope(ns) {
  state.namespaceScope = ns || 'all';
  localStorage.setItem('axiom.ns', state.namespaceScope);
  state.opsLoaded = {};
  refreshAll(false);
}

function trackJob({ kind, label, namespace, name, jobName, phase = 'Pending' }) {
  const id = `${kind}:${namespace || ''}:${name || jobName || Date.now()}`;
  state.jobs = [
    { id, kind, label, namespace: namespace || '', name: name || '', jobName: jobName || '', phase, updatedAt: Date.now() },
    ...state.jobs.filter(j => j.id !== id),
  ].slice(0, 10);
  state.jobsExpanded = true;
  startJobsPoll();
  render();
}

let jobsPollTimer = null;
function startJobsPoll() {
  if (jobsPollTimer) return;
  jobsPollTimer = setInterval(() => { pollJobs().catch(() => {}); }, 5000);
  pollJobs().catch(() => {});
}

function stopJobsPollIfIdle() {
  const active = state.jobs.some(j => !/succeeded|complete|failed|error|ready/i.test(j.phase || ''));
  if (!active && jobsPollTimer) {
    clearInterval(jobsPollTimer);
    jobsPollTimer = null;
  }
}

async function pollJobs() {
  if (!state.jobs.length || !state.credentials) return;
  let changed = false;
  for (const job of state.jobs) {
    try {
      if (job.kind === 'clone') {
        const list = asList(await api(`/api/v1/vm-clones?${nsQuery()}`), ['items']);
        const hit = list.find(c => c.name === job.name && (!job.namespace || c.namespace === job.namespace));
        if (hit && hit.phase !== job.phase) { job.phase = hit.phase; job.updatedAt = Date.now(); changed = true; }
      } else if (job.kind === 'restore') {
        const list = asList(await api(`/api/v1/vm-restores?${nsQuery()}`), ['items']);
        const hit = list.find(c => c.name === job.name && (!job.namespace || c.namespace === job.namespace));
        if (hit) {
          const phase = hit.phase || (hit.complete ? 'Complete' : 'InProgress');
          if (phase !== job.phase) { job.phase = phase; job.updatedAt = Date.now(); changed = true; }
        }
      } else if (job.kind === 'migrate' || job.kind === 'snapshot' || job.kind === 'export') {
        // Keep Pending briefly then mark Accepted if still open after poll
        if (job.phase === 'Pending' && Date.now() - job.updatedAt > 8000) {
          job.phase = 'Accepted';
          job.updatedAt = Date.now();
          changed = true;
        }
      } else if (job.kind === 'helm' && job.jobName) {
        if (job.phase === 'Pending' && Date.now() - job.updatedAt > 12000) {
          job.phase = 'Running';
          job.updatedAt = Date.now();
          changed = true;
        } else if (job.phase === 'Running' && Date.now() - job.updatedAt > 45000) {
          job.phase = 'Finished';
          job.updatedAt = Date.now();
          changed = true;
        }
      }
    } catch {}
  }
  stopJobsPollIfIdle();
  if (changed) render();
}

async function ensureOpsPage(page, force = false) {
  const target = page === 'ops' ? (state.opsTab || 'helm') : page;
  if (!OPS_PAGES.has(target) && target !== 'ops') return;
  if (target === 'ops') return ensureOpsPage(state.opsTab || 'helm', force);
  if (!force && state.opsLoaded[target]) return;
  try {
    if (target === 'secrets') {
      const [data, eso] = await Promise.all([
        api(`/api/v1/secrets?${nsQuery()}`),
        api(`/api/v1/external-secrets?${nsQuery()}`).catch(() => []),
      ]);
      state.secrets = asList(data, ['secrets', 'items']);
      state.externalSecrets = asList(eso, ['items', 'externalSecrets']);
    } else if (target === 'backups') {
      const [data, velero, schedules] = await Promise.all([
        api(`/api/v1/backups?${nsQuery()}`),
        api(`/api/v1/velero/backups?namespace=all`).catch(() => []),
        api(`/api/v1/velero/schedules?namespace=all`).catch(() => []),
      ]);
      state.backups = asList(data, ['backups', 'snapshots', 'items']);
      state.veleroBackups = asList(velero, ['items', 'backups']);
      state.veleroSchedules = asList(schedules, ['items', 'schedules']);
    } else if (target === 'helm') {
      const data = await api('/api/v1/helm/releases');
      state.helmReleases = asList(data, ['releases', 'items']);
    } else if (target === 'gitops') {
      const data = await api('/api/v1/gitops');
      if (Array.isArray(data)) {
        state.gitops = { applications: data, kustomizations: [], helmReleases: [] };
      } else {
        state.gitops = {
          applications: asList(data, ['applications', 'apps']),
          kustomizations: asList(data, ['kustomizations']),
          helmReleases: asList(data, ['helmReleases', 'helmreleases']),
        };
      }
    } else if (target === 'alerts') {
      const data = await api(`/api/v1/alerts?${nsQuery()}`);
      state.alerts = asList(data, ['alerts', 'warnings', 'items']);
      state.prometheusRules = asList(data?.rules, ['rules', 'items']);
      state.alertmanagers = asList(data?.alertmanagers, ['alertmanagers', 'items']);
    } else if (target === 'audit') {
      const data = await api('/api/v1/audit');
      state.audit = asList(data, ['events', 'entries', 'items', 'audit']);
    } else if (target === 'rbac') {
      const data = await api(`/api/v1/rbac?${nsQuery()}`);
      state.rbac = {
        clusterRoles: asList(data?.clusterRoles ?? data?.cluster_roles, ['clusterRoles', 'cluster_roles']),
        clusterRoleBindings: asList(data?.clusterRoleBindings ?? data?.cluster_role_bindings, ['clusterRoleBindings', 'cluster_role_bindings']),
        roles: asList(data?.roles, ['roles']),
        roleBindings: asList(data?.roleBindings ?? data?.role_bindings, ['roleBindings', 'role_bindings']),
      };
    } else if (target === 'cost') {
      state.cost = await api('/api/v1/cost');
    } else if (target === 'agents') {
      const [runs, intents] = await Promise.all([
        api(`/api/v1/agent-runs?${nsQuery()}`),
        api(`/api/v1/platform-intents?${nsQuery()}`).catch(() => []),
      ]);
      state.agentRuns = asList(runs, ['items', 'agentRuns']);
      state.platformIntents = asList(intents, ['items', 'intents']);
    } else if (target === 'identity') {
      state.havenIdentity = await api('/api/v1/haven/identity?namespace=identity');
    }
    state.opsLoaded[target] = true;
  } catch (error) {
    toast(error.message || `Failed to load ${target}`, 'error');
  }
}

function navigateTo(page) {
  state.mobileSidebarOpen = false;
  state.regionMenu = false;
  state.vmMenu = null;
  if (OPS_HUB_IDS.has(page)) {
    state.page = 'ops';
    state.opsTab = page;
    localStorage.setItem('axiom.opsTab', page);
    ensureOpsPage(page).then(() => render());
    render();
    return;
  }
  state.page = page;
  if (page === 'ops') {
    ensureOpsPage(state.opsTab || 'helm').then(() => render());
  } else if (OPS_PAGES.has(page)) {
    ensureOpsPage(page).then(() => render());
  }
  if (page === 'vms') {
    loadVMExtras().then(() => render());
  }
  if (page === 'volumes') {
    loadAtlas().then(() => render());
  }
  if (page === 'settings') {
    loadIntegrations().then(() => render());
  }
  render();
}

async function loadAtlas() {
  try {
    state.atlas = await api('/api/v1/atlas');
  } catch (err) {
    state.atlas = { available: false, reason: err.message || 'Atlas unavailable' };
  }
}

async function loadIntegrations() {
  try {
    const [cfg, ex] = await Promise.all([
      api('/api/v1/integrations'),
      api('/api/v1/integrations/examples').catch(() => ({ examples: [] })),
    ]);
    state.integrations = cfg;
    state.integrationExamples = ex.examples || [];
  } catch (err) {
    state.integrations = null;
    state.integrationExamples = [];
    toast(err.message || 'Failed to load integrations', 'error');
  }
}

function integrationTestToast(res) {
  const svc = (res?.service || 'connection').toUpperCase();
  const ms = res?.latencyMs != null ? ` (${res.latencyMs}ms)` : '';
  if (res?.ok) {
    const extra = res.detail?.version?.version || res.detail?.auth || res.detail?.preview || '';
    toast(`${svc} OK${ms}${extra ? ' · ' + String(extra).slice(0, 60) : ''}`);
    return;
  }
  toast(`${svc} failed${ms}: ${res?.error || 'unknown'}${res?.hint ? ' — ' + res.hint : ''}`, 'error');
}

function applyIntegrationExample(ex) {
  if (!ex?.values) return;
  state.integrationDraft = { ...(state.integrationDraft || {}), ...ex.values };
  toast(`Example loaded: ${ex.title} — review fields and Save`);
  render();
}

async function loadVMSnapshots() {
  try {
    const data = await api(`/api/v1/vm-snapshots?${nsQuery()}`);
    state.vmSnapshots = asList(data, ['items', 'snapshots']);
  } catch (err) {
    state.vmSnapshots = state.vmSnapshots || [];
  }
}

async function loadVMExtras() {
  const q = nsQuery();
  const settled = await Promise.allSettled([
    api(`/api/v1/vm-snapshots?${q}`),
    api(`/api/v1/vm-clones?${q}`),
    api(`/api/v1/vm-exports?${q}`),
    api(`/api/v1/vm-restores?${q}`),
  ]);
  const val = (i) => settled[i].status === 'fulfilled' ? settled[i].value : [];
  state.vmSnapshots = asList(val(0), ['items', 'snapshots']);
  state.vmClones = asList(val(1), ['items', 'clones']);
  state.vmExports = asList(val(2), ['items', 'exports']);
  state.vmRestores = asList(val(3), ['items', 'restores']);
}

matchMedia('(prefers-color-scheme: light)').addEventListener?.('change', () => {
  if (state.theme === 'system') {
    applyTheme();
    render();
  }
});

function authHeader() {
  if (!state.credentials || state.credentials.oidc) return '';
  return `Basic ${btoa(`${state.credentials.user}:${state.credentials.pass}`)}`;
}

function streamAuthQuery() {
  if (!state.credentials || state.credentials.oidc) return '';
  return `access_token=${encodeURIComponent(btoa(`${state.credentials.user}:${state.credentials.pass}`))}`;
}

async function api(path, opts = {}) {
  const headers = { ...opts.headers };
  const basic = authHeader();
  if (basic) headers.Authorization = basic;
  if (opts.body && !(opts.body instanceof FormData)) headers['Content-Type'] = 'application/json';
  const res = await fetch(path, { ...opts, headers, credentials: 'same-origin' });
  if (res.status === 401) {
    logout();
    throw new Error('Authentication failed');
  }
  const ct = res.headers.get('content-type') || '';
  const body = ct.includes('application/json') ? await res.json() : await res.text();
  if (!res.ok) throw new Error(body?.error || body || `HTTP ${res.status}`);
  return body;
}

function toast(message, type = 'ok') {
  const root = $('#toast-root');
  if (!root) return;
  const el = document.createElement('div');
  el.className = `toast ${type}`;
  el.textContent = message;
  root.append(el);
  setTimeout(() => el.remove(), 3400);
}

function setCreds(user, pass, opts = {}) {
  state.credentials = { user, pass, oidc: !!opts.oidc };
  sessionStorage.setItem('axiom.credentials', btoa(JSON.stringify(state.credentials)));
}

function restoreCreds() {
  try {
    const raw = sessionStorage.getItem('axiom.credentials');
    if (raw) state.credentials = JSON.parse(atob(raw));
  } catch { state.credentials = null; }
}

function cleanupPanels() {
  logPanelAbort?.abort();
  logPanelAbort = null;
  if (logPollTimer) clearInterval(logPollTimer);
  logPollTimer = null;
  if (terminalSocket) {
    terminalSocket.close();
    terminalSocket = null;
  }
}

function disconnectEventStream() {
  sseAbort?.abort();
  sseAbort = null;
  state.sseConnected = false;
  state.sseDisconnectedAt = null;
}

function scheduleRefresh() {
  if (refreshDebounceTimer) clearTimeout(refreshDebounceTimer);
  refreshDebounceTimer = setTimeout(async () => {
    refreshDebounceTimer = null;
    if (state.session) await refreshAll(false);
    render();
  }, 500);
}

async function connectEventStream() {
  disconnectEventStream();
  if (!state.credentials) return;
  const controller = new AbortController();
  sseAbort = controller;
  try {
    const res = await fetch(`/api/v1/events/stream?${streamAuthQuery()}`, {
      headers: authHeader() ? { Authorization: authHeader() } : {},
      credentials: 'same-origin',
      signal: controller.signal,
    });
    if (!res.ok || !res.body) throw new Error('stream unavailable');
    state.sseConnected = true;
    state.sseDisconnectedAt = null;
    render();
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const chunks = buffer.split('\n\n');
      buffer = chunks.pop() || '';
      for (const chunk of chunks) {
        const line = chunk.split('\n').find(l => l.startsWith('data: '));
        if (!line) continue;
        try {
          const data = JSON.parse(line.slice(6));
          if (data.type === 'stateChanged') scheduleRefresh();
        } catch {}
      }
    }
  } catch (error) {
    if (error.name === 'AbortError') return;
  } finally {
    if (sseAbort === controller) {
      state.sseConnected = false;
      if (!state.sseDisconnectedAt) state.sseDisconnectedAt = Date.now();
      sseAbort = null;
      render();
      setTimeout(() => {
        if (state.session && !sseAbort) connectEventStream();
      }, 3000);
    }
  }
}

function logout() {
  cleanupPanels();
  disconnectEventStream();
  sessionStorage.removeItem('axiom.credentials');
  state.credentials = null;
  state.session = null;
  state.selected = null;
  fetch('/api/v1/auth/logout', { method: 'POST', credentials: 'same-origin' }).catch(() => {});
  render();
}

async function login(user, pass) {
  setCreds(user, pass);
  try {
    state.session = await api('/api/v1/session');
    state.realtimeCapabilities = await api('/api/v1/realtime/capabilities').catch(() => null);
    // Enter the shell immediately — do not block the UI on cluster list APIs.
    render();
    connectEventStream();
    refreshAll(true).catch(err => toast(err.message, 'error'));
  } catch (error) {
    state.credentials = null;
    sessionStorage.removeItem('axiom.credentials');
    render();
    toast(error.message || 'Sign-in failed', 'error');
  }
}

async function loadPacketwolfFlows() {
  if (state.packetwolfFlowsLoading) return;
  state.packetwolfFlowsLoading = true;
  render();
  try {
    const data = await api('/api/v1/packetwolf/proxy/api/v1/flows?limit=25');
    state.packetwolfFlows = data;
  } catch (error) {
    state.packetwolfFlows = null;
    toast(error.message || 'Failed to load PacketWolf flows', 'error');
  } finally {
    state.packetwolfFlowsLoading = false;
    render();
  }
}

async function refreshAll(showLoading = true) {
  state.loading = showLoading;
  if (showLoading) render();
  try {
    const nsq = nsQuery();
    const settled = await Promise.allSettled([
      api('/api/v1/overview'),
      api(`/api/v1/workloads?${nsq}`),
      api(`/api/v1/pods?${nsq}`),
      api('/api/v1/nodes'),
      api(`/api/v1/events?${nsq}`),
      api('/api/v1/namespaces'),
      api(`/api/v1/services?${nsq}`),
      api(`/api/v1/network?namespace=${encodeURIComponent(state.networkNs || nsParam())}`),
      api('/api/v1/packetwolf/status'),
      api(`/api/v1/vms?${nsq}`),
      api(`/api/v1/volumes?${nsq}`),
      api('/api/v1/metrics/utilization'),
      api(`/api/v1/vm-snapshots?${nsq}`).catch(() => []),
    ]);
    const val = (i, fallback) => settled[i].status === 'fulfilled' ? settled[i].value : fallback;
    const overview = val(0, state.overview);
    const workloads = val(1, state.workloads || []);
    const pods = val(2, state.pods || []);
    const nodes = val(3, state.nodes || []);
    const events = val(4, state.events || []);
    const namespaces = val(5, state.namespaces || []);
    const services = val(6, state.services || []);
    const network = val(7, state.network);
    const packetwolfStatus = val(8, state.packetwolfStatus);
    const vms = val(9, state.vms || []);
    const volumes = val(10, state.volumes || []);
    const utilization = val(11, state.utilization);
    const vmSnapshots = asList(val(12, state.vmSnapshots || []), ['items', 'snapshots']);
    const failed = settled.filter(s => s.status === 'rejected');
    if (failed.length && failed.length === settled.length) {
      throw failed[0].reason;
    }
    if (failed.length) {
      toast(`${failed.length} cluster queries timed out — showing partial state`, 'error');
    }
    const regions = overview?.regions || state.session?.regions || state.regions || [];
    if (!state.region) {
      state.region = overview?.region || state.session?.region || regions.find(r => r.active)?.id || regions[0]?.id || '';
    }
    Object.assign(state, { overview, workloads, pods, nodes, events, namespaces, services, network, packetwolfStatus, vms, volumes, utilization, regions, vmSnapshots });
    if (state.selected) {
      if (state.selectedType === 'workload') {
        state.selected = workloads.find(w => w.namespace === state.selected.namespace && w.kind === state.selected.kind && w.name === state.selected.name) || null;
      } else if (state.selectedType === 'pod') {
        state.selected = pods.find(p => p.namespace === state.selected.namespace && p.name === state.selected.name) || null;
      } else if (state.selectedType === 'node') {
        state.selected = nodes.find(n => n.name === state.selected.name) || null;
      }
      if (!state.selected) state.selectedType = null;
    }
  } catch (error) {
    toast(error.message, 'error');
  } finally {
    state.loading = false;
    if (OPS_PAGES.has(state.page) || state.page === 'ops') {
      const target = state.page === 'ops' ? (state.opsTab || 'helm') : state.page;
      state.opsLoaded[target] = false;
      await ensureOpsPage(target, true);
    }
    if (state.page === 'vms') await loadVMExtras();
    render();
  }
}

function openDrawer(item, type, tab = 'overview') {
  cleanupPanels();
  state.selected = item;
  state.selectedType = type;
  state.drawerTab = tab;
  state.scaleDraft = type === 'workload' ? item.desired : null;
  state.rolloutData = null;
  state.resourceDetail = null;
  state.networkGuard = null;
  if (type === 'pod') state.presenceKnocked = false;
  if (type === 'node') {
    state.nodeDrawerTab = tab;
    state.drawerTab = tab;
  }
  render();
  loadDrawerExtras();
}

async function loadDrawerExtras() {
  const w = state.selected;
  const type = state.selectedType;
  if (type === 'workload' && w) {
    try {
      const [rollout, resource, guard] = await Promise.allSettled([
        api(`/api/v1/workloads/${encodeURIComponent(w.namespace)}/${encodeURIComponent(w.kind)}/${encodeURIComponent(w.name)}/rollout`),
        api(`/api/v1/resources/${encodeURIComponent(w.namespace)}/${encodeURIComponent(w.kind)}/${encodeURIComponent(w.name)}`),
        api(`/api/v1/workloads/${encodeURIComponent(w.namespace)}/${encodeURIComponent(w.kind)}/${encodeURIComponent(w.name)}/network/guard`),
      ]);
      if (rollout.status === 'fulfilled') state.rolloutData = rollout.value;
      if (resource.status === 'fulfilled') state.resourceDetail = resource.value;
      if (guard.status === 'fulfilled') state.networkGuard = guard.value;
      if (state.selected === w) render();
    } catch {}
  }
}

function render() {
  applyTheme();
  const root = $('#app');
  if (!state.credentials || !state.session) {
    root.innerHTML = loginView();
    wireLogin();
    wireTheme();
    return;
  }
  root.innerHTML = shellView();
  wireShell();
  wireTheme();
}

function themeSwitch() {
  return `<div class="theme-switch" aria-label="Appearance">
    <button title="Dark" class="${state.theme === 'dark' ? 'active' : ''}" data-theme-value="dark">●</button>
    <button title="System" class="${state.theme === 'system' ? 'active' : ''}" data-theme-value="system">◐</button>
    <button title="Light" class="${state.theme === 'light' ? 'active' : ''}" data-theme-value="light">☼</button>
  </div>`;
}

function paletteSwitch() {
  return `<div class="palette-switch" aria-label="Color theme">
    ${PALETTES.filter(p => p.id !== 'day').map(p => `
      <button type="button" class="palette-swatch ${resolvedPalette() === p.id ? 'active' : ''}"
        data-palette-value="${enc(p.id)}" title="${enc(p.label)}"
        style="--swatch:${enc(p.swatch)}"><span></span>${enc(p.label)}</button>`).join('')}
  </div>`;
}

function wireTheme() {
  $$('[data-theme-value]').forEach(btn => {
    btn.onclick = () => setTheme(btn.dataset.themeValue);
  });
  $$('[data-palette-value]').forEach(btn => {
    btn.onclick = () => setShellPalette(btn.dataset.paletteValue);
  });
}

function loginView() {
  const showDemo = resolveLoginShowDemo();
  const demoForm = showDemo ? `<form class="login-form" id="login-form" style="margin-top:20px">
    <label>Username<span class="login-field-icon">${icon('user', 16)}</span><input name="user" type="text" value="${enc(defaultLoginUsername())}" autocomplete="username" required /></label>
    <label>Password<span class="login-field-icon">${icon('lock', 16)}</span><input name="pass" type="password" autocomplete="current-password" required /></label>
    <button class="primary-button" type="submit" id="login-submit">Enter console ${icon('arrow')}</button>
  </form>` : '';
  const ssoButtons = SSO_PROVIDERS.map((provider, i) => {
    const pending = state.loginPending === provider.id;
    return `<button type="button" class="sso-btn fade-up group" data-sso-id="${provider.id}" ${state.loginPending ? 'disabled' : ''}" style="animation-delay:${260 + i * 80}ms">
      <span class="sso-btn-icon">${pending ? '<span style="color:#ff7550">◌</span>' : ssoIcon(provider.id)}</span>
      <span>${pending ? 'Authenticating…' : provider.label}</span>
      <span class="sso-btn-tag">SSO</span>
    </button>`;
  }).join('');
  return `<main class="login-shell" data-testid="login-page">
    <div style="position:absolute;inset:0;pointer-events:none;overflow:hidden;z-index:0">
      <div class="grid-bg" style="position:absolute;inset:0;opacity:.4"></div>
      <div class="aurora aurora-a"></div><div class="aurora aurora-b"></div><div class="aurora aurora-c"></div>
    </div>
    <div class="login-grid">
      <section class="login-hero fade-up">
        <div class="login-brand-row">${zyvorMark()}<b>zyvor</b></div>
        <h1>Axiom.<br><span class="login-hero-tagline">Every cluster, one truth.</span></h1>
        <p class="muted" style="max-width:500px;margin-top:24px;font-size:16px">The Zyvor control plane for Kubernetes, Cilium, KubeVirt and multi-tenant SaaS — rendered in liquid glass.</p>
        <div style="display:flex;gap:12px;margin-top:40px;flex-wrap:wrap">${[
          [lucideSvg('globe', 16), '6 clusters', '4 regions'],
          [lucideSvg('cpu', 16), '1.2k pods', 'orchestrated'],
          [lucideSvg('shield-check', 16), 'mTLS', 'everywhere'],
        ].map(s => `<div class="glass specular login-stat-chip"><span style="color:#ff7550;display:flex">${s[0]}</span><div><b>${s[1]}</b><small>${s[2]}</small></div></div>`).join('')}</div>
      </section>
      <section class="fade-up glass specular noise login-card" style="animation-delay:140ms">
        <div class="login-mobile-brand" style="display:none">${zyvorMark('sm')}<b style="text-transform:lowercase">zyvor · Axiom</b></div>
        <div class="login-eyebrow">Axiom Console</div>
        <h2>Sign in to continue</h2>
        <p class="muted" style="margin-top:6px;font-size:14px">Single sign-on enforced by your Zyvor organization.</p>
        <div class="sso-btn-list">${ssoButtons}</div>
        ${demoForm}
        <div class="login-footer-note">${lucideSvg('lock', 14)} Sessions are scoped per device and expire automatically.</div>
      </section>
    </div>
    <a href="https://zyvor.dev" target="_blank" rel="noopener noreferrer" style="position:absolute;bottom:20px;left:0;right:0;border:0;background:none;color:var(--muted);font-size:11px;text-align:center;text-decoration:none;opacity:.6">Axiom by Zyvor · zyvor.dev</a>
  </main>`;
}

function wireLogin() {
  $$('[data-sso-id]').forEach(btn => {
    btn.onclick = async () => {
      if (state.loginPending) return;
      const selected = SSO_PROVIDERS.find(p => p.id === btn.dataset.ssoId);
      const provider = state.authProviders?.[selected.authKey];
      state.loginPending = selected.id;
      render();
      await new Promise(r => setTimeout(r, 900));
      if (provider?.enabled) {
        window.location.assign(provider.login_url || `/api/v1/auth/${selected.authKey}/login`);
        return;
      }
      try {
        const user = defaultLoginUsername();
        state.loginPending = null;
        await login(user, 'demo');
        toast('Welcome back, Ari');
      } catch (error) {
        state.loginPending = null;
        render();
        toast(error.message || 'Sign-in failed', 'error');
      }
    };
  });
  const revealDemo = () => { state.loginShowDemo = true; render(); };
  $$('[data-login-mark]').forEach(mark => mark.addEventListener('click', e => { if (e.shiftKey) revealDemo(); }));
  $('#login-form')?.addEventListener('submit', event => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const user = String(form.get('user') || '').trim();
    const pass = String(form.get('pass') || '');
    if (!user || !pass) {
      toast('Enter username and password', 'error');
      return;
    }
    localStorage.setItem(REMEMBER_FLAG_KEY, '1');
    localStorage.setItem(REMEMBER_USER_KEY, user);
    const btn = $('#login-submit');
    if (btn) { btn.disabled = true; btn.innerHTML = 'Entering…'; }
    login(user, pass);
  });
}

function sseBanner() {
  if (state.sseConnected) return '';
  if (!state.sseDisconnectedAt || Date.now() - state.sseDisconnectedAt < 5000) return '';
  return `<div class="sse-banner">Reconnecting to control plane…</div>`;
}

function shellView() {
  return `${ambientBackdrop()}<div class="console app-frame" style="background:transparent;position:relative">
    ${sidebar()}
    <div class="main app-main">
      ${topbar()}
      ${jobsStrip()}
      ${sseBanner()}
      <div class="content scrollbar-thin">${pageView()}</div>
    </div>
    ${state.selected ? drawerView() : ''}
    ${state.launch ? placeModal() : ''}
    ${state.namespaceModal ? namespaceModal() : ''}
    ${state.secretModal ? secretCreateModal() : ''}
    ${state.agentRunModal ? agentRunCreateModal() : ''}
    ${state.agentRunDetail ? agentRunDetailModal() : ''}
    ${state.vmWizard ? vmWizardModal() : ''}
    ${state.vmConsole ? vmConsoleModal() : ''}
    ${state.palette ? paletteView() : ''}
    ${state.helmRollback ? helmRollbackModal() : ''}
    ${state.confirm ? confirmModal() : ''}
    ${state.still ? stillRingModal() : ''}
  </div>`;
}

function navButton([id, label], badge = '') {
  const active = state.page === id || (id === 'ops' && OPS_HUB_IDS.has(state.page));
  return `<button type="button" data-testid="nav-${id}" class="${active ? 'active' : ''}" data-page="${id}">${navIcon(id)}<span>${enc(label)}</span>${badge}${active ? '<span class="nav-dot"></span>' : ''}</button>`;
}

function cloudName() {
  const raw = (state.session?.cluster || state.overview?.cluster || 'zyvor').toString();
  if (/^(kubernetes|k8s|default)$/i.test(raw)) return 'Zyvor';
  if (/^zyvor(\.dev)?$/i.test(raw)) return 'Zyvor';
  if (/^axiom-demo$/i.test(raw) || /^zyvor-demo$/i.test(raw)) return 'Zyvor Demo';
  return raw.replace(/\b\w/g, c => c.toUpperCase());
}

function sidebar() {
  return `<aside class="sidebar glass specular ${state.mobileSidebarOpen ? 'mobile-open' : ''}" data-testid="nav-sidebar" aria-label="Primary navigation">
    <div class="brand-mark" style="padding:4px 8px 12px">
      <div style="display:flex;align-items:center;gap:10px">${zyvorMark('sm')}
        <div style="line-height:1.2">
          <div style="display:flex;align-items:baseline;gap:6px"><b style="font-size:15px;font-weight:600;text-transform:lowercase;letter-spacing:-0.02em">zyvor</b><span class="sidebar-brand-axiom">Axiom</span></div>
          <div class="sidebar-brand-sub">Multi-cluster Console</div>
        </div>
      </div>
    </div>
    ${clusterSwitcher()}
    <nav style="margin-top:16px;flex:1;display:grid;gap:4px">
      ${navPrimary.map(item => navButton(item)).join('')}
    </nav>
    <div class="sidebar-appearance"><span>Appearance</span>${themeToggleGlass()}</div>
  </aside>`;
}

function clusterSwitcher() {
  const active = activeCluster();
  return `<div class="cluster-switcher"><button type="button" class="workspace glass-inset specular" id="cluster-toggle" data-testid="nav-cluster-switcher">
    <div style="width:36px;height:36px;border-radius:12px;display:grid;place-items:center;background:linear-gradient(135deg,rgb(255 117 80 / 0.3),rgb(106 182 240 / 0.2));border:1px solid rgb(255 255 255 / 0.1)">${lucideSvg('cloud', 16)}</div>
    <div style="min-width:0;flex:1"><b>${enc(active.name)}</b><small>${enc(active.status)} · ${enc(active.provider)}</small></div>
    ${icon('chevron')}</button>
    ${state.clusterMenu ? `<div class="cluster-menu glass specular">${clusterRecords().map(cluster => `<button type="button" data-cluster-id="${enc(cluster.id)}" class="${cluster.id === active.id ? 'active' : ''}"><span class="status-dot ${/healthy|ready/i.test(cluster.status) ? 'ok' : 'warn'}"></span><span><b>${enc(cluster.name)}</b><small>${enc(cluster.provider)} · ${enc(cluster.region)} · ${enc(cluster.version)}</small></span></button>`).join('')}</div>` : ''}
  </div>`;
}

function pageTitle() {
  const titles = {
    overview: 'Overview', pods: 'Pods', nodes: 'Nodes', vms: 'Virtual Machines', network: 'Networking · Cilium',
    volumes: 'Storage', logs: 'Pod Logs', events: 'Events', settings: 'Settings', workloads: 'Workloads',
    secrets: 'Secrets', backups: 'Backups', helm: 'Helm', gitops: 'GitOps', alerts: 'Alerts',
    audit: 'Audit', rbac: 'Access', cost: 'Cost', ops: 'Ops', agents: 'Agents', identity: 'Identity',
  };
  if (state.page === 'ops') {
    const tab = OPS_HUB_TABS.find(([id]) => id === state.opsTab);
    return tab ? `Ops · ${tab[1]}` : 'Ops';
  }
  return titles[state.page] || 'Overview';
}

function jobsStrip() {
  if (!state.jobs.length) return '';
  const open = state.jobsExpanded;
  return `<div class="jobs-strip ${open ? 'open' : ''}">
    <button type="button" class="jobs-toggle" id="jobs-toggle">
      <span class="live-dot"></span>
      <b>${state.jobs.length} action${state.jobs.length === 1 ? '' : 's'}</b>
      <span class="muted">${open ? 'Hide' : 'Show'} progress</span>
    </button>
    ${open ? `<div class="jobs-list">${state.jobs.map(j => `
      <div class="job-row">
        <span class="job-kind">${enc(j.kind)}</span>
        <span class="job-label">${enc(j.label || j.name || j.jobName)}</span>
        ${statusDot(j.phase || 'Pending')}
        <button type="button" class="ghost mini-btn" data-dismiss-job="${enc(j.id)}" title="Dismiss">×</button>
      </div>`).join('')}</div>` : ''}
  </div>`;
}

function topbar() {
  const user = state.session?.user || 'operator';
  const display = user.includes('@') ? user.split('@')[0] : user;
  const initials = (display === 'demo' || display === 'operator') ? 'AV' : display.slice(0, 2).toUpperCase();
  const cluster = activeCluster();
  return `<header class="topbar glass specular">
    <button data-testid="mobile-menu-button" class="mobile-menu icon-button" id="mobile-menu" aria-label="Menu">${icon('menu')}</button>
    <div class="breadcrumbs"><span style="color:hsl(var(--foreground)/0.7)">Clusters</span><span class="sep">/</span><b>${enc(cluster.name)}</b></div>
    <button type="button" class="topbar-search" id="palette-open" data-testid="nav-search">
      ${icon('search')}<span>Search pods, services, nodes…</span>
      <kbd>⌘ K</kbd>
    </button>
    <button type="button" class="kc-btn-outline glass-inset specular" id="connect-kubeconfig" data-testid="nav-connect">
      ${lucideSvg('zap', 16)} Connect kubeconfig
    </button>
    <div class="user-menu-wrap">
      <button type="button" class="user-avatar-btn" id="user-toggle" data-testid="user-menu-trigger">${enc(initials)}</button>
      ${state.userMenu ? `<div class="user-menu glass specular"><div><b>${enc(display === 'demo' ? 'Ari Vance' : display)}</b><small>${enc(user.includes('@') ? user : 'ari@zyvor.dev')}</small></div><div style="font-size:10px;text-transform:uppercase;letter-spacing:0.12em;color:var(--muted);padding:4px 0">${lucideSvg('shield-check', 12)} SSO · google</div><button type="button" id="logout">${icon('logout')}Sign out</button></div>` : ''}
    </div>
  </header>`;
}

function pageView() {
  if (state.loading && !state.overview) return `<div class="skeleton"></div><div style="height:18px"></div><div class="skeleton"></div>`;
  switch (state.page) {
    case 'workloads': return workloadsView();
    case 'pods': return podsView();
    case 'network': return networkView();
    case 'nodes': return hostsView();
    case 'vms': return vmsView();
    case 'events': return eventsView();
    case 'volumes': return volumesView();
    case 'logs': return logsView();
    case 'secrets': return secretsView();
    case 'backups': return backupsView();
    case 'ops': return opsHubView();
    case 'helm': return opsHubView();
    case 'gitops': return opsHubView();
    case 'alerts': return alertsView();
    case 'audit': return opsHubView();
    case 'rbac': return opsHubView();
    case 'cost': return opsHubView();
    case 'settings': return settingsView();
    default: return overviewView();
  }
}

function opsHubView() {
  const tab = state.opsTab || 'helm';
  const tabs = OPS_HUB_TABS.map(([id, label]) =>
    `<button type="button" class="ops-hub-tab ${tab === id ? 'active' : ''}" data-ops-tab="${id}">${enc(label)}</button>`
  ).join('');
  let body = '';
  if (tab === 'helm') body = helmView(true);
  else if (tab === 'gitops') body = gitopsView(true);
  else if (tab === 'agents') body = agentsView(true);
  else if (tab === 'identity') body = identityView(true);
  else if (tab === 'audit') body = auditView(true);
  else if (tab === 'rbac') body = rbacView(true);
  else body = costView(true);
  return `${pageHead('Ops', 'Helm, GitOps, agents, identity, audit, access, and cost — advanced control-plane tools.')}
    <div class="ops-hub-tabs">${tabs}</div>
    <div class="ops-hub-body">${body}</div>`;
}

function recommendedStrip() {
  const chips = [];
  const degraded = state.pods.filter(p => {
    const phase = podDisplayPhase(p);
    return phase !== 'Running' && phase !== 'Succeeded';
  });
  const warning = state.events.find(e => /warning|fail|error|backoff/i.test(`${e.type || ''} ${e.reason || ''} ${e.message || ''}`));
  if (degraded.length) {
    chips.push({
      label: `${degraded.length} pod${degraded.length === 1 ? '' : 's'} need attention`,
      hint: 'Open Pods',
      attrs: 'data-page="pods"',
    });
  }
  if (warning) {
    chips.push({
      label: 'Review latest warning',
      hint: 'Activity',
      attrs: 'data-page="events"',
    });
  }
  if (!(state.vms || []).length) {
    chips.push({
      label: 'Explore virtual machines',
      hint: 'VMs',
      attrs: 'data-page="vms"',
    });
  }
  chips.push({
    label: 'Launch a new pod',
    hint: 'Create',
    attrs: 'data-hero-launch="1"',
  });
  const shown = chips.slice(0, 3);
  return `<section class="recommend-strip" aria-label="Recommended actions">
    <div class="recommend-head"><p class="eyebrow">RECOMMENDED</p><p class="recommend-lead">What needs your attention next.</p></div>
    <div class="recommend-chips">${shown.map(c => `
      <button type="button" class="recommend-chip" ${c.attrs}>
        <span>${enc(c.label)}</span>
        <small>${enc(c.hint)} ${icon('arrow')}</small>
      </button>`).join('')}
    </div>
  </section>`;
}

function overviewView() {
  const o = state.overview || {};
  const util = state.utilization || {};
  const cluster = activeCluster();
  const cpu = Number(util.cpuPercent ?? o.cpuPercent ?? 0), memory = Number(util.memoryPercent ?? o.memoryPercent ?? 0), storage = Number(util.storagePercent ?? o.storagePercent ?? 0);
  const series = util.points || o.utilization || [];
  const values = key => series.map(p => Number(p[key]) || 0);
  const metrics = [
    ['nodes', 'Nodes', `${o.readyNodes ?? state.nodes.filter(n => /ready/i.test(n.status)).length}/${o.nodes ?? state.nodes.length}`, '#ff7550', 'cpu'],
    ['pods', 'Pods', `${o.runningPods ?? state.pods.filter(p => podDisplayPhase(p) === 'Running').length}/${o.pods ?? state.pods.length}`, '#93c88f', 'memory'],
    ['deployments', 'Deployments', o.deployments ?? state.workloads.filter(w => /deployment/i.test(w.kind)).length, '#e0c996', 'net_rx'],
    ['services', 'Services', o.services ?? state.services.length, '#6ab6f0', 'net_tx'],
  ];
  const warnings = state.events.filter(e => /warning|fail|error|backoff/i.test(`${e.type || ''} ${e.reason || ''} ${e.message || ''}`)).slice(0, 3);
  const health = o.health || { kubernetes: o.status || 'Healthy', cni: o.cni || 'Cilium', nodes: `${o.readyNodes ?? state.nodes.length} ready` };
  return `<div class="overview-kube space-y-4">
    ${glassCard(`<div style="display:flex;justify-content:space-between;gap:24px;align-items:end;flex-wrap:wrap"><div><div style="font-size:11px;text-transform:uppercase;letter-spacing:0.18em;color:rgb(255 117 80 / 0.9)">zyvor · Axiom · ${enc(cluster.provider)} · ${enc(cluster.region)}</div><h1 style="margin:4px 0 0;font-size:clamp(36px,4vw,48px);font-weight:600;letter-spacing:-0.025em">${enc(cluster.name)}</h1><div class="muted" style="display:flex;gap:10px;align-items:center;margin-top:8px;font-size:14px">${statusBadgeGlass(cluster.status)}<span>·</span><span>Kubernetes ${enc(cluster.version)}</span><span>·</span><span>${enc(o.cni || 'Cilium')}</span></div></div><div style="display:flex;gap:20px;flex-wrap:wrap">${metricRing(cpu, 'CPU', '#ff7550', 'of allocatable')}${metricRing(memory, 'Memory', '#93c88f', 'of allocatable')}${metricRing(storage, 'Storage', '#b49bd8', 'of provisioned')}</div></div>`, 'overview-hero')}
    <div class="stat-grid">${metrics.map(([key, label, value, color, seriesKey]) => glassCard(`<div class="eyebrow">${label}</div><strong style="display:block;font-size:30px;margin-top:6px">${enc(value)}</strong><div style="margin-top:8px">${sparklineSvg(values(seriesKey), color)}</div>`, 'stat')).join('')}</div>
    <div class="overview-grid">${glassCard(`<div class="panel-heading"><div><h2>Cluster CPU</h2><small class="muted">Last 30 minutes</small></div><b class="mono">${cpu}%</b></div>${sparklineSvg(values('cpu'), '#ff7550', 180)}`, 'usage-panel')}${glassCard(`<div class="panel-heading"><div><h2>Cluster Memory</h2><small class="muted">Last 30 minutes</small></div><b class="mono">${memory}%</b></div>${sparklineSvg(values('memory'), '#93c88f', 180)}`, 'usage-panel')}</div>
    <div class="overview-grid overview-bottom">${glassCard(`<div class="panel-heading"><h2>Control plane</h2></div><div class="activity-list">${Object.entries(health).map(([key, value]) => `<div class="rail-row"><span class="muted">${enc(key.replace(/_/g, ' '))}</span><b class="mono">${enc(value)}</b></div>`).join('')}</div>`, '')}${glassCard(`<div class="panel-heading"><h2>Alerts</h2></div><div class="activity-list">${warnings.length ? warnings.map(e => `<div class="rail-row"><span class="status-dot warn"></span><span>${enc(e.reason || e.message || 'Warning')}</span><small class="muted">${enc(relativeTime(e.lastTimestamp || e.time || e.timestamp))}</small></div>`).join('') : '<p class="muted">No active warnings.</p>'}</div>`, '')}${glassCard(`<div class="panel-heading"><h2>Activity</h2></div><div class="activity-list">${state.events.slice(0, 4).map(e => `<div><div><span class="status-dot info"></span> <b class="mono">${enc(e.source?.component || e.involvedObject?.name || 'cluster')}</b><small class="muted" style="float:right">${enc(relativeTime(e.lastTimestamp || e.time || e.timestamp))}</small></div><small class="muted">${enc(e.message || e.reason || 'Cluster activity')}</small></div>`).join('') || '<p class="muted">No recent activity.</p>'}</div>`, '')}</div>
  </div>`;
}

function logsView() {
  const pods = state.pods || [];
  const index = Math.max(0, Math.min(state.logsPodIndex, Math.max(0, pods.length - 1)));
  const pod = pods[index];
  if (!pod) return `${pageHead('Pod Logs', 'Select a pod to inspect its container logs.')}<p class="muted">No pods are available in the selected namespace.</p>`;
  return `${pageHead('Pod Logs', 'Inspect live and previous container logs.')}<section class="glass specular" style="padding:20px"><label class="eyebrow">POD <select id="logs-pod-select" style="margin-left:8px">${pods.map((p, i) => `<option value="${i}" ${i === index ? 'selected' : ''}>${enc(p.namespace)}/${enc(p.name)}</option>`).join('')}</select></label>${renderLogPanelHTML(pod)}</section>`;
}

function capacityChart(o, util = {}) {
  const points = util.points || o.utilization || [];
  if (points.length >= 2) {
    const toPath = (key) => points.map((p, i) => {
      const x = (i / Math.max(points.length - 1, 1)) * 100;
      const y = 100 - Math.max(0, Math.min(100, Number(p[key]) || 0));
      return `${i === 0 ? 'M' : 'L'}${x.toFixed(2)},${y.toFixed(2)}`;
    }).join(' ');
    const cpuPath = toPath('cpu');
    const memPath = toPath('memory');
    const area = `${cpuPath} L100,100 L0,100 Z`;
    const labels = ['00:00', '06:00', '12:00', '18:00', 'Now'];
    return `<div class="chart">
      <div class="chart-y"><span>100%</span><span>75%</span><span>50%</span><span>25%</span><span>0%</span></div>
      <div class="chart-main">
        <div class="grid-lines"></div>
        <svg viewBox="0 0 100 100" preserveAspectRatio="none">
          <defs><linearGradient id="chartFill" x1="0" y1="0" x2="0" y2="1"><stop stop-color="#2997ff" stop-opacity=".28"/><stop offset="1" stop-color="#2997ff" stop-opacity="0"/></linearGradient></defs>
          <path class="area" d="${area}"></path>
          <path class="line" d="${cpuPath}"></path>
          <path class="line blue-line" d="${memPath}"></path>
        </svg>
        <div class="chart-x">${labels.map(l => `<span>${l}</span>`).join('')}</div>
      </div>
    </div>`;
  }
  const pods = Math.max(1, Number(o.pods) || 1);
  const running = Math.min(pods, Number(o.runningPods) || 0);
  const nodes = Math.max(1, Number(o.nodes) || 1);
  const ready = Math.min(nodes, Number(o.readyNodes) || 0);
  const cpuH = 100 - Math.round((running / pods) * 55) - 10;
  const memH = 100 - Math.round((ready / nodes) * 45) - 18;
  const cpuPath = `M0,${cpuH + 8} C18,${cpuH - 6} 32,${memH + 14} 48,${cpuH - 2} S72,${memH} 100,${Math.max(12, cpuH - 10)}`;
  const memPath = `M0,${memH + 16} C22,${memH + 4} 38,${cpuH + 10} 55,${memH - 2} S78,${cpuH} 100,${Math.max(20, memH - 6)}`;
  const area = `${cpuPath} L100,100 L0,100 Z`;
  return `<div class="chart">
    <div class="chart-y"><span>100%</span><span>75%</span><span>50%</span><span>25%</span><span>0%</span></div>
    <div class="chart-main">
      <div class="grid-lines"></div>
      <svg viewBox="0 0 100 100" preserveAspectRatio="none">
        <defs><linearGradient id="chartFill" x1="0" y1="0" x2="0" y2="1"><stop stop-color="#ff7a1a" stop-opacity=".28"/><stop offset="1" stop-color="#ff7a1a" stop-opacity="0"/></linearGradient></defs>
        <path class="area" d="${area}"></path>
        <path class="line" d="${cpuPath}"></path>
        <path class="line blue-line" d="${memPath}"></path>
      </svg>
      <div class="chart-x"><span>00:00</span><span>06:00</span><span>12:00</span><span>18:00</span><span>Now</span></div>
    </div>
  </div>`;
}

function onboardingDismissed() {
  return localStorage.getItem('axiom.onboarding') === 'dismissed';
}

function onboardingComplete() {
  return state.workloads.length > 0 && state.pods.length > 0;
}

function onboardingStrip() {
  if (onboardingDismissed() || (onboardingComplete() && localStorage.getItem('axiom.onboarding') === 'done')) return '';
  const steps = [
    { id: 'launch', label: 'Launch first workload', done: state.workloads.length > 0 },
    { id: 'pod', label: 'Open a pod', done: state.selectedType === 'pod' || state.pods.length > 0 },
    { id: 'logs', label: 'View logs', done: state.drawerTab === 'logs' },
  ];
  const allDone = steps.every(s => s.done);
  if (allDone) localStorage.setItem('axiom.onboarding', 'done');
  return `<section class="onboarding-strip">
    <div class="onboarding-head"><div><div class="eyebrow">GETTING STARTED</div><strong>First steps</strong></div><button class="ghost mini-btn" id="onboarding-dismiss">Dismiss</button></div>
    <div class="onboarding-steps">${steps.map(s => `<div class="onboarding-step ${s.done ? 'done' : ''}"><span class="step-dot"></span>${enc(s.label)}</div>`).join('')}</div>
  </section>`;
}

function postureStrip() {
  const o = state.overview || {};
  const unhealthy = state.workloads.filter(w => w.status !== 'Healthy').length;
  const warning = state.events.find(e => e.type === 'Warning');
  return `<section class="posture-strip">
    <div class="posture-head"><div class="eyebrow">Fleet posture</div><h3>Cluster at a glance</h3></div>
    <div class="posture-grid">
      <button class="posture-tile" data-page="workloads"><div class="posture-value">${enc(o.workloads ?? '—')}</div><div class="posture-label">Workloads</div></button>
      <button class="posture-tile" data-page="pods"><div class="posture-value">${enc(o.runningPods ?? '—')}</div><div class="posture-label">Running pods</div></button>
      <button class="posture-tile" data-page="vms"><div class="posture-value">${enc(o.readyVms ?? state.vms.filter(v => v.ready).length)}/${enc(o.vms ?? state.vms.length)}</div><div class="posture-label">Ready VMs</div></button>
      <button class="posture-tile" data-page="events"><div class="posture-value warn">${enc(o.warnings ?? 0)}</div><div class="posture-label">Warnings</div></button>
      <button class="posture-tile" data-workload-filter="attention"><div class="posture-value warn">${unhealthy}</div><div class="posture-label">Need attention</div></button>
      <button class="posture-tile" data-page="events"><div class="posture-note">${warning ? enc(warning.message.slice(0, 48)) : 'No active warnings'}</div><div class="posture-label">Latest signal</div></button>
    </div>
  </section>`;
}

function metricStat(value, label, note) {
  return `<div class="metric-stat"><div class="label">${enc(label)}</div><div class="value">${enc(value ?? '—')}</div><div class="note">${enc(note)}</div></div>`;
}

function emptyState(title, description, action = '') {
  return `<div class="empty-state"><div class="empty-title">${enc(title)}</div><div class="empty-desc">${enc(description)}</div>${action ? `<div class="empty-action">${action}</div>` : ''}</div>`;
}

function canvas() {
  const workloads = state.workloads;
  const shown = workloads.slice(0, 5);
  const positions = [[8, 17], [58, 11], [34, 48], [68, 61], [9, 70]];
  const extra = workloads.length - shown.length;
  return `<div class="canvas">
    <svg class="canvas-lines" viewBox="0 0 100 100" preserveAspectRatio="none">
      <defs><linearGradient id="flow" x1="0" x2="1"><stop stop-color="#64d2ff" stop-opacity=".2"/><stop offset="1" stop-color="#7d7aff" stop-opacity=".55"/></linearGradient></defs>
      <path d="M18 28 C34 25,45 22,65 23 M64 24 C58 37,51 44,43 57 M43 60 C55 63,65 68,74 70 M42 60 C31 67,24 75,17 79" fill="none" stroke="url(#flow)" stroke-width=".5" stroke-dasharray="2 2"/>
    </svg>
    ${shown.map((w, i) => {
      const degraded = w.desired > 0 && w.ready < w.desired;
      return `<div class="app-node ${degraded ? 'degraded' : ''}" data-workload="${state.workloads.indexOf(w)}" style="left:${positions[i][0]}%;top:${positions[i][1]}%">
      <div class="name">${enc(w.name)}</div><div class="meta">${enc(w.namespace)} · ${w.ready}/${w.desired} ready</div><div class="health ${w.status === 'Healthy' && !degraded ? '' : 'degraded'}">${enc(w.status)}</div>
    </div>`;
    }).join('')}
    ${extra > 0 ? `<button class="canvas-more mini-btn" data-page="workloads">+${extra} more</button>` : ''}
    ${!workloads.length ? emptyState('No workloads on canvas', 'Launch an application to populate the cloud field.', '<button class="primary mini-btn" id="hero-launch">Launch workload</button>') : ''}
  </div>`;
}

function eventRow(event, index = 0) {
  const palette = [
    { tone: 'green', glyph: 'check' },
    { tone: 'blue', glyph: 'refresh' },
    { tone: 'amber', glyph: 'zap' },
    { tone: 'purple', glyph: 'shieldPlain' },
  ];
  const pick = event.type === 'Warning'
    ? { tone: 'amber', glyph: 'zap' }
    : palette[index % palette.length];
  return `<div class="activity-row">
    <div class="activity-icon ${pick.tone}">${icon(pick.glyph)}</div>
    <div style="flex:1;min-width:0"><b>${enc(event.message)}</b><small>${enc(event.namespace)} · ${enc(event.object)} · ${enc(event.reason)}</small></div>
    <small class="mono muted">${relativeTime(event.time)}</small>
  </div>`;
}

function pageHead(title, sub, action = '') {
  return `<div class="page-head"><div><div class="eyebrow">AXIOM / OPERATE</div><h1>${title}</h1><p class="subhead">${sub}</p></div>${action}</div>`;
}

function statusDot(label) {
  return `<span class="status ${enc(label)}"><i></i>${enc(label)}</span>`;
}

function filterWorkloads(items) {
  if (state.workloadFilter === 'healthy') return items.filter(w => w.status === 'Healthy');
  if (state.workloadFilter === 'attention') return items.filter(w => w.status !== 'Healthy' || w.ready < w.desired);
  return items;
}

function filterPods(items) {
  if (state.podFilter === 'running') return items.filter(p => p.status === 'Running');
  if (state.podFilter === 'pending') return items.filter(p => p.status === 'Pending');
  if (state.podFilter === 'failed') return items.filter(p => ['Failed', 'Error', 'CrashLoopBackOff'].includes(p.status));
  return items;
}

function workloadsView() {
  const items = filterWorkloads(state.workloads);
  return `${pageHead('Workloads', 'Operate applications without translating every intent into kubectl.', '<button class="primary-button" id="hero-launch">＋ New pod</button>')}
    <div class="toolbar"><input class="search-input" id="filter" placeholder="Search workloads, namespaces, images…" /><div class="segment">${[
      ['all', 'All'], ['healthy', 'Healthy'], ['attention', 'Attention'],
    ].map(([id, label]) => `<button class="${state.workloadFilter === id ? 'active' : ''}" data-workload-filter="${id}">${label}</button>`).join('')}</div></div>
    <section class="panel pod-panel"><div class="table-wrap"><table><thead><tr><th>Application</th><th>Kind</th><th>State</th><th>Ready</th><th>Image</th><th>Age</th></tr></thead><tbody id="workload-body">${workloadRows(items)}</tbody></table></div></section>`;
}

function workloadRows(items) {
  if (!items.length) return `<tr><td colspan="6">${emptyState('No workloads', 'Launch an application or change your filter.', '<button class="primary-button small" id="hero-launch">New pod</button>')}</td></tr>`;
  return items.map(w => {
    const index = state.workloads.indexOf(w);
    const pct = w.desired ? Math.min(100, (w.ready / w.desired) * 100) : 0;
    return `<tr data-workload-id="${index}"><td><div class="name-cell"><span class="pod-glyph">${navIcon('workloads')}</span><div><b>${enc(w.name)}</b><small>${enc(w.namespace)}</small></div></div></td><td>${enc(w.kind)}</td><td>${statusDot(w.status)}</td><td>${w.ready} / ${w.desired}<div class="progress"><span style="width:${pct}%"></span></div></td><td class="mono">${enc(w.image)}</td><td>${enc(w.age)}</td></tr>`;
  }).join('');
}

function podsView() {
  const items = filterPods(state.pods);
  return `${pageHead('Pods', 'Runtime state, logs, terminal, eviction and termination.', '<button class="primary-button" id="hero-launch">＋ New pod</button>')}
    <div class="toolbar"><input class="search-input" id="filter" placeholder="Search pods, namespaces, nodes…" /><div class="segment">${[
      ['all', 'All'], ['running', 'Running'], ['pending', 'Pending'], ['failed', 'Failed'],
    ].map(([id, label]) => `<button class="${state.podFilter === id ? 'active' : ''}" data-pod-filter="${id}">${label}</button>`).join('')}</div></div>
    <section class="panel pod-panel"><div class="table-wrap"><table><thead><tr><th>NAME</th><th>STATUS</th><th>REPLICAS</th><th>NODE</th><th>IP</th><th>RESTARTS</th><th>AGE</th></tr></thead><tbody id="pod-body">${podRows(items)}</tbody></table></div></section>`;
}

function podRows(items) {
  if (!items.length) return `<tr><td colspan="7">${emptyState('No pods', 'Pods appear when workloads are scheduled.', '<button class="secondary mini-btn" data-refresh>Refresh</button>')}</td></tr>`;
  return items.map(p => `<tr data-pod-id="${state.pods.indexOf(p)}"><td><div class="name-cell"><span class="pod-glyph">${navIcon('pods')}</span><div><b>${enc(p.name)}</b><small>${enc(p.namespace)}</small></div></div></td><td>${statusDot(p.status)}</td><td>${enc(p.ready)}</td><td>${enc(p.node)}</td><td class="mono">${enc(p.ip)}</td><td>${enc(p.restarts)}</td><td>${enc(p.age)}</td></tr>`).join('');
}

function filterVMs(items) {
  if (state.vmFilter === 'running') return items.filter(v => /running/i.test(v.status) || v.ready);
  if (state.vmFilter === 'stopped') return items.filter(v => /stop/i.test(v.status) || (!v.running && !v.ready));
  return items;
}

function filterVolumes(items) {
  if (state.volumeFilter === 'bound') return items.filter(v => /bound/i.test(v.status));
  if (state.volumeFilter === 'pending') return items.filter(v => /pending/i.test(v.status));
  return items;
}

function vmsView() {
  const items = filterVMs(state.vms);
  const life = state.vmLifecycleTab || 'snapshots';
  const lifeTabs = [
    ['snapshots', 'Snapshots', (state.vmSnapshots || []).length],
    ['clones', 'Clones', (state.vmClones || []).length],
    ['restores', 'Restores', (state.vmRestores || []).length],
    ...(state.vmExportSupported ? [['exports', 'Exports', (state.vmExports || []).length]] : []),
  ];
  let lifeBody = '';
  if (life === 'clones') {
    const rows = (state.vmClones || []).map(c => `<tr>
      <td><b>${enc(c.name)}</b><small class="muted"> · ${enc(c.namespace)}</small></td>
      <td class="mono">${enc(c.source)} → ${enc(c.target)}</td>
      <td>${statusDot(c.phase || 'Unknown')}</td>
      <td>${enc(c.age || '—')}</td>
    </tr>`).join('');
    lifeBody = `<table><thead><tr><th>Clone</th><th>Source → Target</th><th>Phase</th><th>Age</th></tr></thead><tbody>${rows || `<tr><td colspan="4">${emptyState('No clones', 'Use ⋯ → Clone on a VM.', '')}</td></tr>`}</tbody></table>`;
  } else if (life === 'restores') {
    const rows = (state.vmRestores || []).map(r => `<tr>
      <td><b>${enc(r.name)}</b><small class="muted"> · ${enc(r.namespace)}</small></td>
      <td class="mono">${enc(r.snapshot || '—')}</td>
      <td class="mono">${enc(r.target || '—')}</td>
      <td>${statusDot(r.phase || (r.complete ? 'Complete' : 'InProgress'))}</td>
      <td>${enc(r.age || '—')}</td>
    </tr>`).join('');
    lifeBody = `<table><thead><tr><th>Restore</th><th>Snapshot</th><th>Target</th><th>Phase</th><th>Age</th></tr></thead><tbody>${rows || `<tr><td colspan="5">${emptyState('No restores', 'Use Restore from the lifecycle actions.', '')}</td></tr>`}</tbody></table>`;
  } else if (life === 'exports') {
    const rows = (state.vmExports || []).map(e => `<tr>
      <td><b>${enc(e.name)}</b><small class="muted"> · ${enc(e.namespace)}</small></td>
      <td class="mono">${enc(e.source || '—')}</td>
      <td>${statusDot(e.phase || 'Unknown')}</td>
      <td class="mono">${enc(e.links || '—')}</td>
      <td>${enc(e.age || '—')}</td>
    </tr>`).join('');
    lifeBody = `<table><thead><tr><th>Export</th><th>Source</th><th>Phase</th><th>Links</th><th>Age</th></tr></thead><tbody>${rows || `<tr><td colspan="5">${emptyState('No exports', 'Export may be disabled by cluster feature gates.', '')}</td></tr>`}</tbody></table>`;
  } else {
    const rows = (state.vmSnapshots || []).map(s => `<tr>
      <td><div class="name-cell"><span class="pod-glyph">${navIcon('vms')}</span><div><b>${enc(s.name)}</b><small>${enc(s.namespace)}</small></div></div></td>
      <td class="mono">${enc(s.vm || '—')}</td>
      <td>${statusDot(s.ready ? 'Ready' : (s.status || 'Unknown'))}</td>
      <td>${enc(s.age || '—')}</td>
      <td class="row-actions"><button class="ghost mini-btn" data-vm-wizard="restore" data-vm-ns="${enc(s.namespace)}" data-vm-snap="${enc(s.name)}" data-vm-name="${enc(s.vm || '')}">Restore…</button></td>
    </tr>`).join('');
    lifeBody = `<table><thead><tr><th>Snapshot</th><th>VM</th><th>Status</th><th>Age</th><th></th></tr></thead><tbody>${rows || `<tr><td colspan="5">${emptyState('No VM snapshots', 'Use ⋯ → Snapshot on a VM.', '')}</td></tr>`}</tbody></table>`;
  }
  return `${pageHead('Virtual machines', 'KubeVirt guests across the private cloud.', '<button class="secondary" data-page="nodes">View hosts</button>')}
    <div class="toolbar"><input class="search-input" id="filter" placeholder="Search VMs, namespaces, OS…" /><div class="segment">${[
      ['all', 'All'], ['running', 'Running'], ['stopped', 'Stopped'],
    ].map(([id, label]) => `<button class="${state.vmFilter === id ? 'active' : ''}" data-vm-filter="${id}">${label}</button>`).join('')}</div></div>
    <section class="panel pod-panel"><div class="table-wrap"><table><thead><tr><th>NAME</th><th>STATUS</th><th>CPU</th><th>MEMORY</th><th>NODE</th><th>OS</th><th>AGE</th><th></th></tr></thead><tbody id="vm-body">${vmRows(items)}</tbody></table></div></section>
    <section class="panel pod-panel lifecycle-panel" style="margin-top:20px">
      <div class="panel-heading"><div><div class="eyebrow">LIFECYCLE</div><h2>Snapshots, clones & restores</h2></div>
        <button class="secondary mini-btn" data-refresh-vmsnaps>Refresh</button></div>
      <div class="segment lifecycle-tabs">${lifeTabs.map(([id, label, n]) =>
        `<button type="button" class="${life === id ? 'active' : ''}" data-vm-life="${id}">${enc(label)}${n ? ` <span class="nav-count">${n}</span>` : ''}</button>`
      ).join('')}</div>
      <div class="table-wrap">${lifeBody}</div>
    </section>`;
}

function vmRows(items) {
  if (!items.length) return `<tr><td colspan="8">${emptyState('No virtual machines', 'KubeVirt VMs in this cluster will appear here.', '<button class="secondary mini-btn" data-refresh>Refresh</button>')}</td></tr>`;
  return items.map((vm) => {
    const idx = state.vms.indexOf(vm);
    const running = vm.running || /running/i.test(vm.status);
    const menuOpen = state.vmMenu === `${vm.namespace}/${vm.name}`;
    const menuItems = [
      running ? ['migrate', 'Live migrate'] : null,
      ['snapshot', 'Snapshot…'],
      ['clone', 'Clone…'],
      state.vmExportSupported ? ['export', 'Export'] : null,
      (running || vm.ready) ? ['console', 'Console'] : null,
    ].filter(Boolean);
    return `<tr data-vm-id="${idx}" class="clickable-row">
      <td><div class="name-cell"><span class="pod-glyph">${navIcon('vms')}</span><div><b>${enc(vm.name)}</b><small>${enc(vm.namespace)}</small></div></div></td>
      <td>${statusDot(vm.status)}</td>
      <td class="mono">${enc(vm.cpu)}</td>
      <td class="mono">${enc(vm.memory)}</td>
      <td class="mono">${enc(vm.node || '—')}</td>
      <td>${enc(vm.os || '—')}</td>
      <td>${enc(vm.age)}</td>
      <td class="row-actions" onclick="event.stopPropagation()">
        <button class="ghost mini-btn" data-vm-action="${running ? 'stop' : 'start'}" data-vm-ns="${enc(vm.namespace)}" data-vm-name="${enc(vm.name)}">${running ? 'Stop' : 'Start'}</button>
        <div class="overflow-wrap">
          <button type="button" class="ghost mini-btn overflow-btn" data-vm-menu="${enc(vm.namespace)}/${enc(vm.name)}" aria-label="More actions">⋯</button>
          ${menuOpen ? `<div class="overflow-menu">
            ${menuItems.map(([act, label]) =>
              `<button type="button" data-vm-more="${act}" data-vm-ns="${enc(vm.namespace)}" data-vm-name="${enc(vm.name)}">${enc(label)}</button>`
            ).join('')}
          </div>` : ''}
        </div>
      </td>
    </tr>`;
  }).join('');
}

function openVmWizard(type, vm = {}) {
  state.vmMenu = null;
  state.vmWizard = {
    type,
    namespace: vm.namespace || (nsParam() === 'all' ? 'default' : nsParam()),
    name: vm.name || '',
    targetVM: vm.name ? `${vm.name}-clone` : '',
    snapshot: vm.snapshot || '',
  };
  render();
}

function vmWizardModal() {
  const w = state.vmWizard;
  if (!w) return '';
  const titles = {
    snapshot: ['Snapshot', 'Capture disk state for this VM'],
    clone: ['Clone', 'Create a new VM from this guest'],
    restore: ['Restore', 'Restore a VM from a snapshot'],
    export: ['Export', 'Create a VirtualMachineExport'],
  };
  const [title, sub] = titles[w.type] || ['Action', ''];
  let fields = '';
  if (w.type === 'snapshot') {
    fields = `
      <div class="field"><label>Namespace</label><input name="namespace" value="${enc(w.namespace)}" required /></div>
      <div class="field"><label>VM</label><input name="vm" value="${enc(w.name)}" required /></div>
      <div class="field full"><label>Snapshot name</label><input name="name" placeholder="${enc(w.name)}-snap" /></div>`;
  } else if (w.type === 'clone') {
    fields = `
      <div class="field"><label>Namespace</label><input name="namespace" value="${enc(w.namespace)}" required /></div>
      <div class="field"><label>Source VM</label><input name="sourceVM" value="${enc(w.name)}" required /></div>
      <div class="field full"><label>Target VM</label><input name="targetVM" value="${enc(w.targetVM || `${w.name}-clone`)}" required /></div>`;
  } else if (w.type === 'restore') {
    fields = `
      <div class="field"><label>Namespace</label><input name="namespace" value="${enc(w.namespace)}" required /></div>
      <div class="field"><label>Snapshot</label><input name="snapshot" value="${enc(w.snapshot)}" required /></div>
      <div class="field full"><label>Target VM</label><input name="targetVM" value="${enc(w.targetVM || w.name || '')}" required /></div>`;
  } else {
    fields = `
      <div class="field"><label>Namespace</label><input name="namespace" value="${enc(w.namespace)}" required /></div>
      <div class="field"><label>VM</label><input name="vm" value="${enc(w.name)}" required /></div>`;
  }
  return `<div class="overlay" id="vm-wizard-overlay"></div><section class="modal ops-modal">
    <div class="modal-head"><div class="eyebrow">Virtual machine</div><h3>${enc(title)}</h3><p>${enc(sub)}</p></div>
    <form id="vm-wizard-form">
      <div class="modal-body"><div class="form-grid">${fields}</div></div>
      <div class="modal-foot">
        <button class="secondary" type="button" id="vm-wizard-cancel">Cancel</button>
        <button class="primary" type="submit">${enc(title)} →</button>
      </div>
    </form>
  </section>`;
}

function hostsView() {
  return `${pageHead('Hosts', 'Kubernetes nodes underpinning the VM and pod estate.', '<button class="secondary" data-page="vms">Back to VMs</button>')}
    <section class="panel pod-panel"><div class="table-wrap"><table><thead><tr><th>Node</th><th>Status</th><th>Role</th><th>Version</th><th>Pods</th><th>Capacity</th><th>IP</th></tr></thead><tbody>${state.nodes.length ? state.nodes.map((n, i) => `<tr data-node-id="${i}"><td><div class="name-cell"><span class="pod-glyph">${navIcon('nodes')}</span><div><b>${enc(n.name)}</b><small>${enc(n.age)} old</small></div></div></td><td>${statusDot(n.status)}</td><td>${enc(n.role)}</td><td class="mono">${enc(n.version)}</td><td>${enc(n.pods)}</td><td>${enc(n.cpu)} CPU · ${enc(n.memory)}</td><td class="mono">${enc(n.internalIp)}</td></tr>`).join('') : `<tr><td colspan="7">${emptyState('No nodes', 'Connect Axiom to a Kubernetes cluster.', '')}</td></tr>`}</tbody></table></div></section>`;
}

function volumesView() {
  const items = filterVolumes(state.volumes);
  const atlas = state.atlas || {};
  const policies = asList(atlas.policies, ['items', 'policies']);
  const pools = asList(atlas.pools, ['items', 'pools']);
  const buckets = asList(atlas.buckets, ['items', 'buckets']);
  const atlasVols = asList(atlas.volumes, ['items', 'volumes']);
  const backendsRaw = atlas.backends;
  const backends = Array.isArray(backendsRaw)
    ? backendsRaw
    : asList(backendsRaw, ['items', 'backends']);
  const sc = asList(atlas.storageClasses, ['items', 'storageClasses', 'classes']);
  const consoleURL = atlas.consoleURL || '';
  const consoleLink = consoleURL
    ? `<a class="text-button" style="margin-left:12px" href="${enc(consoleURL)}" target="_blank" rel="noopener">Open Atlas console →</a>`
    : `<a class="text-button" style="margin-left:12px" data-page="settings">Set console URL in Settings →</a>`;
  const fabric = `
    <section class="panel" style="margin-bottom:20px">
      <div class="panel-heading"><div><div class="eyebrow">ATLAS</div><h2>Storage fabric</h2></div>
        <div style="display:flex;gap:8px">
          <button class="secondary mini-btn" data-page="settings">Configure</button>
          <button class="secondary mini-btn" id="atlas-refresh">Refresh Atlas</button>
        </div></div>
      ${atlas.available
        ? `<p class="muted" style="margin:0 0 12px">Control plane ${enc(atlas.baseURL || '')}${atlas.reason ? ' · ' + enc(atlas.reason) : ''}</p>
           <div class="stat-grid" style="margin:0">
             <div class="stat"><div><span>Backends</span><strong>${backends.length || '—'}</strong><small>Ceph / NFS fabric</small></div></div>
             <div class="stat"><div><span>Pools</span><strong>${pools.length}</strong><small>RBD / CephFS</small></div></div>
             <div class="stat"><div><span>Atlas volumes</span><strong>${atlasVols.length}</strong><small>Intent inventory</small></div></div>
             <div class="stat"><div><span>Buckets</span><strong>${buckets.length}</strong><small>RGW / S3</small></div></div>
           </div>
           <div class="table-wrap" style="margin-top:16px"><table><thead><tr><th>Policy</th><th>StorageClass</th><th>Notes</th></tr></thead>
           <tbody>${policies.map(p => `<tr><td class="mono">${enc(p.intent || p.id || p.name || p.policy || '—')}</td><td class="mono">${enc(p.storageClass || p.storage_class || '—')}</td><td class="muted">${enc(p.description || p.kind || p.access_mode || '')}</td></tr>`).join('') || `<tr><td colspan="3" class="muted">No policies returned</td></tr>`}</tbody></table></div>
           ${sc.length ? `<p class="muted" style="margin-top:12px">Classes: ${sc.map(c => enc(c.name || c)).join(', ')}</p>` : ''}
           ${buckets.length ? `<div class="table-wrap" style="margin-top:16px"><table><thead><tr><th>Bucket</th><th>Backend</th><th>Status</th></tr></thead>
           <tbody>${buckets.slice(0, 12).map(b => `<tr><td class="mono">${enc(b.name || b.id)}</td><td>${enc(b.backend || b.store || '—')}</td><td>${statusDot(b.status || b.phase || 'Ready')}</td></tr>`).join('')}</tbody></table></div>` : ''}
           <div style="margin-top:16px"><button class="primary-button small" id="atlas-volume-open">＋ Request Atlas volume</button>
             ${consoleLink}</div>`
        : `<p class="muted">${enc(atlas.reason || 'Atlas not configured.')}</p>
           <p style="margin-top:12px"><button class="primary-button small" data-page="settings">Open Settings → Integrations</button></p>`}
    </section>`;
  return `${pageHead('Volumes', 'Persistent volume claims and Atlas storage fabric.')}
    ${fabric}
    <div class="toolbar"><input class="search-input" id="filter" placeholder="Search volumes, classes, namespaces…" /><div class="segment">${[
      ['all', 'All'], ['bound', 'Bound'], ['pending', 'Pending'],
    ].map(([id, label]) => `<button class="${state.volumeFilter === id ? 'active' : ''}" data-volume-filter="${id}">${label}</button>`).join('')}</div></div>
    <section class="panel pod-panel"><div class="table-wrap"><table><thead><tr><th>NAME</th><th>STATUS</th><th>CAPACITY</th><th>STORAGE CLASS</th><th>ACCESS</th><th>VOLUME</th><th>AGE</th></tr></thead><tbody id="volume-body">${volumeRows(items)}</tbody></table></div></section>
    ${state.atlasVolumeModal ? atlasVolumeModal() : ''}`;
}

function atlasVolumeModal() {
  const ns = nsParam() === 'all' ? 'default' : (nsParam() || 'default');
  const policies = asList((state.atlas || {}).policies, ['items', 'policies']);
  const opts = policies.length
    ? policies.map(p => {
        const id = p.intent || p.id || p.name || p.policy;
        return id ? `<option value="${enc(id)}">${enc(id)}${p.storage_class || p.storageClass ? ' · ' + enc(p.storage_class || p.storageClass) : ''}</option>` : '';
      }).join('')
    : `<option value="production">production</option><option value="database">database</option><option value="development">development</option><option value="shared">shared</option><option value="ai">ai</option>`;
  return `<div class="overlay" id="atlas-vol-overlay"></div><section class="modal ops-modal">
    <div class="modal-head"><div class="eyebrow">Atlas</div><h3>Request volume</h3><p>Intent-based provision via Atlas (owner product=axiom).</p></div>
    <form id="atlas-vol-form">
      <div class="modal-body"><div class="form-grid">
        <div class="field"><label>Name</label><input name="name" required pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" placeholder="app-data" /></div>
        <div class="field"><label>Namespace</label><input name="namespace" value="${enc(ns)}" required /></div>
        <div class="field"><label>Policy</label><select name="policy">${opts}</select></div>
        <div class="field"><label>Size (Gi)</label><input name="size_gi" type="number" min="1" value="10" required /></div>
      </div></div>
      <div class="modal-foot"><button type="button" class="secondary" id="atlas-vol-cancel">Cancel</button><button class="primary" type="submit">Create →</button></div>
    </form>
  </section>`;
}

function volumeRows(items) {
  if (!items.length) return `<tr><td colspan="7">${emptyState('No volumes yet', 'Persistent volume claims will appear here once workloads request storage.', '<button class="primary-button small" id="hero-launch">New pod</button>')}</td></tr>`;
  return items.map(v => `<tr><td><div class="name-cell"><span class="pod-glyph">${navIcon('volumes', 14)}</span><div><b>${enc(v.name)}</b><small>${enc(v.namespace)}</small></div></div></td><td>${statusDot(v.status || 'Unknown')}</td><td class="mono">${enc(v.capacity)}</td><td class="mono">${enc(v.storageClass || '—')}</td><td class="mono">${enc(v.accessModes || '—')}</td><td class="mono">${enc(v.volumeName || '—')}</td><td>${enc(v.age)}</td></tr>`).join('');
}

function nodesView() {
  return hostsView();
}

function eventsView() {
  return `${pageHead('Activity', 'One chronological signal stream across the cluster.')}
    <section class="panel"><div class="activity-list">${state.events.map((e, i) => eventRow(e, i)).join('') || emptyState('No events', 'Kubernetes events will stream here.', '<button class="secondary mini-btn" data-refresh>Refresh</button>')}</div></section>`;
}

function settingsView() {
  const s = state.session || {};
  const i = { ...(state.integrations || {}), ...(state.integrationDraft || {}) };
  const tokenHint = i.atlasTokenSet ? 'Token saved — leave blank to keep' : 'Bearer token when Atlas auth is required';
  const pgHint = i.postgresPasswordSet ? 'Password saved — leave blank to keep' : 'CNPG app user password';
  const examples = state.integrationExamples || [];
  const exCards = examples.map(ex => `
    <article class="example-card" style="border:1px solid var(--border);border-radius:12px;padding:14px;margin-bottom:10px">
      <div style="display:flex;justify-content:space-between;gap:12px;align-items:flex-start">
        <div><b>${enc(ex.title)}</b><p class="muted" style="margin:6px 0 0">${enc(ex.description || '')}</p></div>
        <button type="button" class="secondary mini-btn" data-apply-example="${enc(ex.id)}">Use example</button>
      </div>
      <pre class="mono muted" style="margin:10px 0 0;font-size:11px;white-space:pre-wrap">${enc(Object.entries(ex.values || {}).map(([k,v]) => `${k}: ${v}`).join('\n'))}</pre>
    </article>`).join('');
  return `${pageHead('Settings', 'Appearance, session, and product integrations.')}
    <section class="panel settings-panel">
      <div class="panel-heading"><div><div class="eyebrow">APPEARANCE</div><h2>Theme</h2></div></div>
      ${themeSwitch()}
      <div class="panel-heading" style="margin-top:28px"><div><div class="eyebrow">COLOR</div><h2>Palette</h2>
        <p class="muted" style="margin:4px 0 0">Carbon default · Tahoe · Dark Steel (Day follows Light theme).</p></div></div>
      ${paletteSwitch()}
      <div class="panel-heading" style="margin-top:28px"><div><div class="eyebrow">SESSION</div><h2>Signed in</h2></div></div>
      <div class="rail-row"><span>User</span><span>${enc(s.user || '—')}</span></div>
      <div class="rail-row"><span>Role</span><span>${enc(s.role || '—')}</span></div>
      <div class="rail-row"><span>Cloud</span><span>Zyvor Private Cloud</span></div>
      <div class="rail-row"><span>Cluster</span><span>${enc(cloudName())}</span></div>
      <div class="rail-row"><span>Region</span><span>${enc(state.region || s.region || '—')}</span></div>
      <div class="rail-row"><span>Mode</span><span>${enc(s.mode || '—')}</span></div>
      <div style="margin-top:20px"><button class="secondary" id="logout-settings">Sign out</button></div>
    </section>
    <section class="panel settings-panel" style="margin-top:20px">
      <div class="panel-heading"><div><div class="eyebrow">GUIDE</div><h2>Configuration examples</h2>
        <p class="muted" style="margin:4px 0 0">Templates for Atlas, Haven Postgres, OIDC, and probes. Replace hosts and ports with values from your install (Ingress preferred; no lab NodePorts baked in).</p></div></div>
      ${exCards || '<p class="muted">Examples load when the API is reachable.</p>'}
    </section>
    <section class="panel settings-panel" style="margin-top:20px">
      <div class="panel-heading"><div><div class="eyebrow">INTEGRATIONS</div><h2>Atlas storage</h2>
        <p class="muted" style="margin:4px 0 0">Gateway URL for the control plane; console URL opens in the browser from Volumes.</p></div>
        <span class="pill ${i.atlasConfigured ? 'ok' : ''}">${i.atlasConfigured ? 'Configured' : 'Not set'}</span></div>
      <form id="integrations-atlas-form">
        <div class="form-grid">
          <div class="field full"><label>Gateway URL</label><input name="atlasUrl" value="${enc(i.atlasUrl || '')}" placeholder="http://atlas-gateway.zyvor-system.svc:&lt;port&gt; or https://atlas-api.example.com" /></div>
          <div class="field full"><label>Console URL</label><input name="atlasConsoleUrl" value="${enc(i.atlasConsoleUrl || '')}" placeholder="https://atlas.example.com" /></div>
          <div class="field full"><label>API token</label><input name="atlasToken" type="password" autocomplete="off" placeholder="${enc(tokenHint)}" /></div>
          <div class="field full" style="display:flex;align-items:center;gap:10px">
            <label style="display:flex;align-items:center;gap:8px;margin:0"><input type="checkbox" name="atlasTokenClear" /> Clear saved token</label>
          </div>
        </div>
        <div style="margin-top:16px;display:flex;gap:8px;flex-wrap:wrap">
          <button class="primary" type="submit">Save Atlas</button>
          <button class="secondary" type="button" id="integrations-atlas-test">Test connection</button>
        </div>
      </form>
    </section>
    <section class="panel settings-panel" style="margin-top:20px">
      <div class="panel-heading"><div><div class="eyebrow">INTEGRATIONS</div><h2>Postgres (Haven / CNPG)</h2>
        <p class="muted" style="margin:4px 0 0">TCP + SELECT 1 from the Axiom pod. Use the CNPG <code>-rw</code> service.</p></div>
        <span class="pill ${i.postgresConfigured ? 'ok' : ''}">${i.postgresConfigured ? 'Configured' : 'Not set'}</span></div>
      <form id="integrations-postgres-form">
        <div class="form-grid">
          <div class="field"><label>Host</label><input name="postgresHost" value="${enc(i.postgresHost || '')}" placeholder="platform-db-rw.identity.svc" /></div>
          <div class="field"><label>Port</label><input name="postgresPort" type="number" min="1" value="${enc(String(i.postgresPort || 5432))}" /></div>
          <div class="field"><label>Database</label><input name="postgresDatabase" value="${enc(i.postgresDatabase || '')}" placeholder="platform" /></div>
          <div class="field"><label>User</label><input name="postgresUser" value="${enc(i.postgresUser || '')}" placeholder="platform" /></div>
          <div class="field"><label>Password</label><input name="postgresPassword" type="password" autocomplete="off" placeholder="${enc(pgHint)}" /></div>
          <div class="field"><label>SSL mode</label><select name="postgresSslMode"><option value="disable" ${(i.postgresSslMode || 'disable') === 'disable' ? 'selected' : ''}>disable</option><option value="require" ${i.postgresSslMode === 'require' ? 'selected' : ''}>require</option></select></div>
          <div class="field full" style="display:flex;align-items:center;gap:10px">
            <label style="display:flex;align-items:center;gap:8px;margin:0"><input type="checkbox" name="postgresPasswordClear" /> Clear saved password</label>
          </div>
        </div>
        <div style="margin-top:16px;display:flex;gap:8px;flex-wrap:wrap">
          <button class="primary" type="submit">Save Postgres</button>
          <button class="secondary" type="button" id="integrations-postgres-test">Test connection</button>
        </div>
      </form>
    </section>
    <section class="panel settings-panel" style="margin-top:20px">
      <div class="panel-heading"><div><div class="eyebrow">INTEGRATIONS</div><h2>Network probe</h2>
        <p class="muted" style="margin:4px 0 0">HTTP GET from the Axiom pod, or curl/wget from a specific pod (debug sidecar).</p></div>
        <span class="pill ${i.probeConfigured ? 'ok' : ''}">${i.probeConfigured ? 'Configured' : 'Not set'}</span></div>
      <form id="integrations-probe-form">
        <div class="form-grid">
          <div class="field full"><label>URL to probe</label><input name="probeUrl" value="${enc(i.probeUrl || '')}" placeholder="https://atlas-api.example.com/version" /></div>
          <div class="field"><label>Pod namespace (optional)</label><input name="probePodNamespace" value="${enc(i.probePodNamespace || '')}" placeholder="axiom-system" /></div>
          <div class="field"><label>Pod name (optional)</label><input name="probePodName" value="${enc(i.probePodName || '')}" placeholder="curl-debug" /></div>
          <div class="field"><label>Container (optional)</label><input name="probePodContainer" value="${enc(i.probePodContainer || '')}" placeholder="default container" /></div>
        </div>
        <div style="margin-top:16px;display:flex;gap:8px;flex-wrap:wrap">
          <button class="primary" type="submit">Save probe</button>
          <button class="secondary" type="button" id="integrations-probe-test">Test connection</button>
        </div>
      </form>
    </section>
    <section class="panel settings-panel" style="margin-top:20px">
      <div class="panel-heading"><div><div class="eyebrow">INTEGRATIONS</div><h2>OIDC / Haven</h2>
        <p class="muted" style="margin:4px 0 0">Sign-in redirect and issuer for Haven Keycloak or any OIDC IdP.</p></div>
        <span class="pill ${i.oidcConfigured ? 'ok' : ''}">${i.oidcConfigured ? 'Configured' : 'Not set'}</span></div>
      <form id="integrations-oidc-form">
        <div class="form-grid">
          <div class="field full"><label>Issuer URL</label><input name="oidcIssuer" value="${enc(i.oidcIssuer || '')}" placeholder="https://auth.example.com/realms/platform" /></div>
          <div class="field full"><label>Authorize / login URL</label><input name="oidcLoginUrl" value="${enc(i.oidcLoginUrl || '')}" placeholder="https://auth.example.com/realms/platform/protocol/openid-connect/auth" /></div>
          <div class="field"><label>Client ID</label><input name="oidcClientId" value="${enc(i.oidcClientId || '')}" placeholder="axiom" /></div>
        </div>
        <div style="margin-top:16px;display:flex;gap:8px;flex-wrap:wrap">
          <button class="primary" type="submit">Save OIDC</button>
          <button class="secondary" type="button" id="integrations-oidc-test">Test connection</button>
        </div>
      </form>
    </section>`;
}

function secretsView() {
  const rows = state.secrets.map((s, i) => {
    const key = `${s.namespace}/${s.name}`;
    const expanded = state.secretExpand === key;
    const keys = (state.secretDetail && expanded)
      ? (state.secretDetail.keys || Object.keys(state.secretDetail.data || {}) || [])
      : (s.keys || []);
    const keyCount = s.keyCount ?? s.key_count ?? keys.length ?? 0;
    return `<tr data-secret-row="${i}">
      <td><button class="ghost mini-btn secret-expand" data-secret-expand="${enc(key)}" data-secret-ns="${enc(s.namespace)}" data-secret-name="${enc(s.name)}">${expanded ? '▾' : '▸'}</button>
        <div class="name-cell" style="display:inline-flex"><span class="pod-glyph">${navIcon('secrets')}</span><div><b>${enc(s.name)}</b><small>${enc(s.namespace)}</small></div></div>
        ${expanded ? `<div class="secret-keys mono">${keys.length ? keys.map(k => `<span>${enc(typeof k === 'string' ? k : k.name || k.key || '')}</span>`).join('') : '<span class="muted">No keys</span>'}</div>` : ''}
      </td>
      <td class="mono">${enc(s.type || 'Opaque')}</td>
      <td>${keyCount}</td>
      <td>${enc(s.age || '—')}</td>
      <td class="row-actions"><button class="ghost mini-btn danger-text" data-delete-secret data-secret-ns="${enc(s.namespace)}" data-secret-name="${enc(s.name)}">Delete</button></td>
    </tr>`;
  }).join('');
  return `${pageHead('Secrets', 'Kubernetes secrets across namespaces — values stay masked in the list.', '<button class="primary-button" id="create-secret">＋ Create secret</button>')}
    <section class="panel pod-panel"><div class="table-wrap"><table><thead><tr><th>Name</th><th>Type</th><th>Keys</th><th>Age</th><th></th></tr></thead><tbody>${rows || `<tr><td colspan="5">${emptyState('No secrets', 'Create a secret or wait for workloads to mount them.', '<button class="primary-button small" id="create-secret">Create secret</button>')}</td></tr>`}</tbody></table></div></section>
    <section class="panel pod-panel" style="margin-top:20px">
      <div class="panel-heading"><div><div class="eyebrow">EXTERNAL SECRETS</div><h2>Synced from stores</h2></div>
        <button class="secondary mini-btn" data-refresh-ops="secrets">Refresh</button></div>
      <div class="table-wrap"><table><thead><tr><th>Name</th><th>Store</th><th>Status</th><th>Refresh</th><th>Age</th></tr></thead><tbody>${
        (state.externalSecrets || []).map(e => `<tr>
          <td><div class="name-cell"><span class="pod-glyph">${navIcon('secrets')}</span><div><b>${enc(e.name)}</b><small>${enc(e.namespace)}</small></div></div></td>
          <td class="mono">${enc(e.store || '—')}</td>
          <td>${statusDot(e.ready ? 'Ready' : (e.status || 'Unknown'))}</td>
          <td class="mono">${enc(e.refresh || '—')}</td>
          <td>${enc(e.age || '—')}</td>
        </tr>`).join('') || `<tr><td colspan="5">${emptyState('No ExternalSecrets', 'Install External Secrets Operator CRDs to sync from vaults and cloud stores.', '')}</td></tr>`
      }</tbody></table></div>
    </section>`;
}

function secretCreateModal() {
  return `<div class="overlay" id="secret-overlay"></div><section class="modal ops-modal">
    <div class="modal-head"><div class="eyebrow">Secrets</div><h3>Create secret</h3><p>Provide name, namespace, and KEY=value lines.</p></div>
    <form id="secret-form">
      <div class="modal-body">
        <div class="form-grid">
          <div class="field"><label>Name</label><input name="name" required pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" placeholder="app-credentials" /></div>
          <div class="field"><label>Namespace</label><input name="namespace" value="default" required /></div>
          <div class="field full"><label>Data (KEY=value per line)</label><textarea name="data" rows="6" class="mono" placeholder="API_KEY=&#10;DATABASE_URL=" required></textarea></div>
        </div>
      </div>
      <div class="modal-foot"><button class="secondary" type="button" id="secret-cancel">Cancel</button><button class="primary" type="submit">Create secret →</button></div>
    </form>
  </section>`;
}

function backupsView() {
  const rows = state.backups.map(b => `<tr>
    <td><div class="name-cell"><span class="pod-glyph">${navIcon('backups')}</span><div><b>${enc(b.name)}</b><small>${enc(b.namespace)}</small></div></div></td>
    <td>${statusDot(b.status || (b.readyToUse === false ? 'Pending' : (b.readyToUse ? 'Ready' : 'Unknown')))}</td>
    <td class="mono">${enc(b.pvc || b.sourcePvc || b.source || '—')}</td>
    <td class="mono">${enc(b.volumeSnapshotClassName || b.class || '—')}</td>
    <td>${enc(b.age || b.createdAt || '—')}</td>
  </tr>`).join('');
  return `${pageHead('Backups', 'VolumeSnapshots and Velero cluster backups.')}
    <div class="ops-forms">
      <section class="panel ops-form-panel">
        <div class="panel-heading"><div><div class="eyebrow">CREATE</div><h2>Snapshot PVC</h2></div></div>
        <form id="backup-create-form" class="form-grid">
          <div class="field"><label>Namespace</label><input name="namespace" value="default" required /></div>
          <div class="field"><label>PVC</label><input name="pvc" placeholder="data-pvc" required /></div>
          <div class="field"><label>Snapshot name</label><input name="name" placeholder="data-snap-1" required /></div>
          <div class="field"><button class="primary-button" type="submit">Create snapshot</button></div>
        </form>
      </section>
      <section class="panel ops-form-panel">
        <div class="panel-heading"><div><div class="eyebrow">RESTORE</div><h2>New PVC from snapshot</h2></div></div>
        <form id="backup-restore-form" class="form-grid">
          <div class="field"><label>Namespace</label><input name="namespace" value="default" required /></div>
          <div class="field"><label>Snapshot</label><input name="snapshot" placeholder="data-snap-1" required /></div>
          <div class="field"><label>New PVC name</label><input name="pvc" placeholder="data-restored" required /></div>
          <div class="field"><button class="secondary" type="submit">Restore</button></div>
        </form>
      </section>
    </div>
    <section class="panel pod-panel"><div class="table-wrap"><table><thead><tr><th>Snapshot</th><th>Status</th><th>Source PVC</th><th>Class</th><th>Age</th></tr></thead><tbody>${rows || `<tr><td colspan="5">${emptyState('No volume snapshots', 'Create a snapshot from a PVC, or install the CSI snapshot CRDs.', '')}</td></tr>`}</tbody></table></div></section>
    <section class="panel pod-panel" style="margin-top:20px">
      <div class="panel-heading"><div><div class="eyebrow">VELERO</div><h2>Cluster backups</h2></div>
        <button class="secondary mini-btn" data-refresh-ops="backups">Refresh</button></div>
      <div class="table-wrap"><table><thead><tr><th>Backup</th><th>Phase</th><th>Namespaces</th><th>Errors</th><th>Age</th></tr></thead><tbody>${
        (state.veleroBackups || []).map(b => `<tr>
          <td><div class="name-cell"><span class="pod-glyph">${navIcon('backups')}</span><div><b>${enc(b.name)}</b><small>${enc(b.namespace)}</small></div></div></td>
          <td>${statusDot(b.phase || 'Unknown')}</td>
          <td class="mono">${enc(b.namespaces || '—')}</td>
          <td class="mono">${enc(b.errors ?? 0)}</td>
          <td>${enc(b.age || '—')}</td>
        </tr>`).join('') || `<tr><td colspan="5">${emptyState('No Velero backups', 'Install Velero or wait for a scheduled backup.', '')}</td></tr>`
      }</tbody></table></div>
    </section>
    <section class="panel pod-panel" style="margin-top:20px">
      <div class="panel-heading"><div><div class="eyebrow">SCHEDULES</div><h2>Velero schedules</h2></div></div>
      <div class="table-wrap"><table><thead><tr><th>Schedule</th><th>Cron</th><th>Paused</th><th>Last backup</th><th>Age</th></tr></thead><tbody>${
        (state.veleroSchedules || []).map(s => `<tr>
          <td><b>${enc(s.name)}</b><small class="muted"> · ${enc(s.namespace)}</small></td>
          <td class="mono">${enc(s.schedule || '—')}</td>
          <td>${s.paused ? 'yes' : 'no'}</td>
          <td class="mono">${enc(s.lastBackup || '—')}</td>
          <td>${enc(s.age || '—')}</td>
        </tr>`).join('') || `<tr><td colspan="5">${emptyState('No Velero schedules', '', '')}</td></tr>`
      }</tbody></table></div>
    </section>`;
}

function identityView(embedded = false) {
  const h = state.havenIdentity || {};
  const planes = h.planes || [];
  const kcs = h.keycloaks || [];
  const dbs = h.databases || [];
  const admin = h.admin || {};
  const oidc = h.oidc || {};
  const planeRows = planes.map(p => `<tr>
    <td><b>${enc(p.name)}</b><small class="muted"> ${enc(p.namespace)}</small></td>
    <td>${enc(p.profile || '—')}</td>
    <td>${statusDot(p.phase || 'Pending')}</td>
    <td class="mono">${enc(p.hostname || '—')}</td>
    <td>${enc(p.age || '—')}</td>
  </tr>`).join('');
  const kcRows = kcs.map(k => `<tr>
    <td><b>${enc(k.name)}</b><small class="muted"> ${enc(k.namespace)}</small></td>
    <td>${statusDot(k.ready ? 'Ready' : 'Pending')}</td>
    <td>${enc(String(k.instances ?? '—'))}</td>
    <td class="mono">${enc(k.url || k.hostname || '—')}</td>
    <td>${enc(k.age || '—')}</td>
  </tr>`).join('');
  const dbRows = dbs.map(d => `<tr>
    <td><b>${enc(d.name)}</b><small class="muted"> ${enc(d.namespace)}</small></td>
    <td>${statusDot(d.ready ? 'Ready' : (d.status || 'Pending'))}</td>
    <td>${enc(String(d.instances ?? '—'))}</td>
    <td>${enc(d.status || '—')}</td>
    <td>${enc(d.age || '—')}</td>
  </tr>`).join('');
  const body = `
    <section class="panel">
      <div class="panel-heading"><div><div class="eyebrow">HAVEN</div><h2>Identity plane</h2></div>
        <button class="secondary mini-btn" data-refresh-ops="identity">Refresh</button></div>
      <p class="muted" style="margin:0 0 12px">${enc(h.message || 'Keycloak + CloudNativePG as one product.')}
        ${h.ready ? ' · <span class="soft">Ready</span>' : ''}</p>
      <div class="rail-row"><span>Admin secret</span><span class="mono">${admin.exists ? enc(admin.namespace + '/' + admin.secretName) : '— pending —'}</span></div>
      ${admin.hint ? `<div class="rail-row"><span>Bootstrap</span><span class="mono" style="font-size:12px">${enc(admin.hint)}</span></div>` : ''}
      <div class="rail-row"><span>OIDC issuer</span><span class="mono">${enc(oidc.issuer || '—')}</span></div>
      <div class="rail-row"><span>Login URL</span><span class="mono">${enc(oidc.loginURL || '—')}</span></div>
      <div class="rail-row"><span>Client</span><span class="mono">${enc(oidc.clientID || 'axiom')}</span></div>
      ${oidc.note ? `<p class="muted" style="margin-top:8px">${enc(oidc.note)}</p>` : ''}
      ${oidc.issuer ? `<p style="margin-top:12px"><a class="text-button" href="${enc(oidc.issuer.replace(/\/realms\/.*/, '/admin'))}" target="_blank" rel="noopener">Open Keycloak admin →</a></p>` : ''}
    </section>
    <section class="panel pod-panel" style="margin-top:20px">
      <div class="panel-heading"><div><div class="eyebrow">IDENTITY PLANES</div><h2>CR intent</h2></div></div>
      <div class="table-wrap"><table><thead><tr><th>Name</th><th>Profile</th><th>Phase</th><th>Hostname</th><th>Age</th></tr></thead>
      <tbody>${planeRows || `<tr><td colspan="5">${emptyState('No IdentityPlane CRs', 'Apply Haven CRDs and samples — status fills when the Haven controller exists.', '')}</td></tr>`}</tbody></table></div>
    </section>
    <section class="panel pod-panel" style="margin-top:20px">
      <div class="panel-heading"><div><div class="eyebrow">KEYCLOAK</div><h2>Operator CRs</h2></div></div>
      <div class="table-wrap"><table><thead><tr><th>Name</th><th>Ready</th><th>Instances</th><th>URL</th><th>Age</th></tr></thead>
      <tbody>${kcRows || `<tr><td colspan="5">${emptyState('No Keycloak CRs', 'Install the Keycloak Operator and apply the Haven compose path.', '')}</td></tr>`}</tbody></table></div>
    </section>
    <section class="panel pod-panel" style="margin-top:20px">
      <div class="panel-heading"><div><div class="eyebrow">DATABASE</div><h2>CloudNativePG</h2></div></div>
      <div class="table-wrap"><table><thead><tr><th>Name</th><th>Ready</th><th>Instances</th><th>Status</th><th>Age</th></tr></thead>
      <tbody>${dbRows || `<tr><td colspan="5">${emptyState('No CNPG clusters', 'Haven compose creates platform-db in the identity namespace.', '')}</td></tr>`}</tbody></table></div>
    </section>`;
  if (embedded) return body;
  return `${pageHead('Identity', 'Haven — Keycloak + Postgres as one plane.')}${body}`;
}

function agentsView(embedded = false) {
  const rows = (state.agentRuns || []).map(r => {
    const target = r.target ? `${r.target.kind}/${r.target.name}` : '—';
    return `<tr data-agent-run="${enc(r.namespace)}/${enc(r.name)}" class="clickable-row">
      <td><div class="name-cell"><span class="pod-glyph">${navIcon('ops')}</span><div><b>${enc(r.name)}</b><small>${enc(r.namespace)}</small></div></div></td>
      <td>${enc((r.goal || '').slice(0, 72))}${(r.goal || '').length > 72 ? '…' : ''}</td>
      <td class="mono">${enc(target)}</td>
      <td>${statusDot(r.phase || 'Pending')}</td>
      <td>${enc(r.risk || '—')}</td>
      <td>${enc(r.approval || '—')}</td>
      <td>${enc(r.age || '—')}</td>
      <td class="row-actions">
        <button class="ghost mini-btn" data-agent-detail data-agent-ns="${enc(r.namespace)}" data-agent-name="${enc(r.name)}">Plan</button>
        ${r.canApprove ? `<button class="ghost mini-btn" data-agent-approve data-agent-ns="${enc(r.namespace)}" data-agent-name="${enc(r.name)}">Approve</button>` : ''}
        ${r.canDeny ? `<button class="ghost mini-btn danger-text" data-agent-deny data-agent-ns="${enc(r.namespace)}" data-agent-name="${enc(r.name)}">Deny</button>` : ''}
      </td>
    </tr>`;
  }).join('');
  const intentRows = (state.platformIntents || []).map(i => `<tr>
    <td><b>${enc(i.name)}</b><small class="muted"> ${enc(i.namespace)}</small></td>
    <td class="mono">${enc(i.workloadKind || '—')}</td>
    <td>${statusDot(i.phase || 'Pending')}</td>
    <td class="mono">${enc(i.appliedTo || '—')}</td>
    <td>${enc(i.age || '—')}</td>
  </tr>`).join('');
  const body = `
    <section class="panel pod-panel">
      <div class="panel-heading"><div><div class="eyebrow">AGENT RUNS</div><h2>Hermes proposals</h2></div>
        <div class="panel-actions">
          <button class="secondary mini-btn" data-refresh-ops="agents">Refresh</button>
          <button class="primary-button small" id="create-agent-run">＋ New run</button>
        </div>
      </div>
      <div class="table-wrap"><table><thead><tr><th>Name</th><th>Goal</th><th>Target</th><th>Phase</th><th>Risk</th><th>Approval</th><th>Age</th><th></th></tr></thead>
      <tbody>${rows || `<tr><td colspan="8">${emptyState('No agent runs', 'Create a run or install platform.example.com CRDs and the mock controller.', '<button class="primary-button small" id="create-agent-run">New run</button>')}</td></tr>`}</tbody></table></div>
    </section>
    <section class="panel pod-panel" style="margin-top:20px">
      <div class="panel-heading"><div><div class="eyebrow">PLATFORM INTENTS</div><h2>Read-only Intent CRs</h2></div>
        <button class="secondary mini-btn" data-refresh-ops="agents">Refresh</button></div>
      <p class="muted" style="margin:0 0 12px">These are Hermes platform Intent objects — not Axiom Place intents.</p>
      <div class="table-wrap"><table><thead><tr><th>Name</th><th>Kind</th><th>Phase</th><th>Applied to</th><th>Age</th></tr></thead>
      <tbody>${intentRows || `<tr><td colspan="5">${emptyState('No platform intents', 'Intents appear when Hermes proposes create_intent actions.', '')}</td></tr>`}</tbody></table></div>
    </section>`;
  if (embedded) return body;
  return `${pageHead('Agents', 'Hermes AgentRuns — propose, review, approve.')}${body}`;
}

function agentRunCreateModal() {
  const ns = nsParam() === 'all' ? 'default' : (nsParam() || 'default');
  return `<div class="overlay" id="agent-run-overlay"></div><section class="modal ops-modal">
    <div class="modal-head"><div class="eyebrow">Agents</div><h3>New agent run</h3><p>Autonomy is ProposeOnly. The controller plans; you approve.</p></div>
    <form id="agent-run-form">
      <div class="modal-body">
        <div class="form-grid">
          <div class="field"><label>Name</label><input name="name" placeholder="diagnose-my-pod" pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" /></div>
          <div class="field"><label>Namespace</label><input name="namespace" value="${enc(ns)}" required /></div>
          <div class="field full"><label>Goal</label><textarea name="goal" rows="3" required placeholder="Diagnose why this pod is unhealthy and propose the safest fix."></textarea></div>
          <div class="field"><label>Target kind</label><select name="targetKind"><option value="">(none)</option><option value="Pod">Pod</option><option value="VirtualMachine">VirtualMachine</option></select></div>
          <div class="field"><label>Target name</label><input name="targetName" placeholder="optional" /></div>
          <div class="field"><label>Target namespace</label><input name="targetNamespace" value="${enc(ns)}" /></div>
        </div>
      </div>
      <div class="modal-foot"><button class="secondary" type="button" id="agent-run-cancel">Cancel</button><button class="primary" type="submit">Create run →</button></div>
    </form>
  </section>`;
}

function agentRunDetailModal() {
  const r = state.agentRunDetail;
  if (!r) return '';
  const plan = r.plan || {};
  const actions = (plan.actions || []).map(a => {
    const tgt = a.target ? `${a.target.kind}/${a.target.namespace || ''}/${a.target.name}` : '—';
    return `<tr><td class="mono">${enc(a.id || '')}</td><td class="mono">${enc(a.type || '')}</td><td>${enc(a.risk || '—')}</td><td>${enc(a.description || '')}</td><td class="mono">${enc(tgt)}</td></tr>`;
  }).join('');
  return `<div class="overlay" id="agent-detail-overlay"></div><section class="modal ops-modal wide">
    <div class="modal-head"><div class="eyebrow">${enc(r.namespace)} / ${enc(r.name)}</div>
      <h3>${enc(r.goal || 'Agent run')}</h3>
      <p>${statusDot(r.phase)} · risk ${enc(r.risk || '—')} · approval ${enc(r.approval || '—')}</p></div>
    <div class="modal-body">
      <div class="rail-row"><span>Summary</span><span>${enc(plan.summary || r.message || '—')}</span></div>
      <div class="rail-row"><span>Root cause</span><span>${enc(plan.rootCause || '—')}</span></div>
      <div class="table-wrap" style="margin-top:16px"><table><thead><tr><th>ID</th><th>Type</th><th>Risk</th><th>Description</th><th>Target</th></tr></thead>
      <tbody>${actions || `<tr><td colspan="5" class="muted">No actions yet — waiting for planner.</td></tr>`}</tbody></table></div>
    </div>
    <div class="modal-foot">
      <button class="secondary" type="button" id="agent-detail-close">Close</button>
      ${r.canApprove ? `<button class="primary" type="button" id="agent-detail-approve" data-agent-ns="${enc(r.namespace)}" data-agent-name="${enc(r.name)}">Approve</button>` : ''}
      ${r.canDeny ? `<button class="secondary danger-text" type="button" id="agent-detail-deny" data-agent-ns="${enc(r.namespace)}" data-agent-name="${enc(r.name)}">Deny</button>` : ''}
    </div>
  </section>`;
}

function helmView(embedded = false) {
  const rows = state.helmReleases.map(r => `<tr>
    <td><div class="name-cell"><span class="pod-glyph">${navIcon('helm')}</span><div><b>${enc(r.name)}</b><small>${enc(r.namespace)}</small></div></div></td>
    <td>${statusDot(r.status || 'Unknown')}</td>
    <td class="mono">${enc(r.chart || '—')}</td>
    <td class="mono">${enc(r.revision ?? '—')}</td>
    <td>${enc(r.age || '—')}</td>
    <td class="row-actions">
      <button class="ghost mini-btn" data-helm-rollback data-helm-ns="${enc(r.namespace)}" data-helm-name="${enc(r.name)}" data-helm-rev="${enc(String(Math.max(0, (Number(r.revision) || 1) - 1)))}">Rollback</button>
      <button class="ghost mini-btn danger-text" data-helm-uninstall data-helm-ns="${enc(r.namespace)}" data-helm-name="${enc(r.name)}">Uninstall</button>
    </td>
  </tr>`).join('');
  const table = `<section class="panel pod-panel"><div class="table-wrap"><table><thead><tr><th>Release</th><th>Status</th><th>Chart</th><th>Revision</th><th>Age</th><th></th></tr></thead><tbody>${rows || `<tr><td colspan="6">${emptyState('No Helm releases', 'Install a chart or sync GitOps HelmReleases to populate this list.', '')}</td></tr>`}</tbody></table></div></section>`;
  if (embedded) return table;
  return `${pageHead('Helm', 'Releases discovered from helm.sh/release secrets.')}${table}`;
}

function gitopsView(embedded = false) {
  const g = state.gitops || {};
  const apps = g.applications || [];
  const kust = g.kustomizations || [];
  const hrs = g.helmReleases || [];
  const total = apps.length + kust.length + hrs.length;
  const head = embedded ? '' : pageHead('GitOps', 'Argo CD Applications and Flux Kustomizations / HelmReleases.');
  if (!total) {
    return `${head}<section class="panel">${emptyState('No GitOps resources', 'Install Argo CD or Flux CRDs, or connect a sync target. Empty when CRDs are absent.', '<button class="secondary mini-btn" data-refresh-ops="gitops">Refresh</button>')}</section>`;
  }
  const card = (kind, item) => `<div class="ops-card">
    <div class="eyebrow">${enc(kind)}</div>
    <b>${enc(item.name)}</b>
    <small class="muted">${enc(item.namespace || 'cluster')}</small>
    <div class="ops-card-meta">${statusDot(item.status || item.syncStatus || item.ready || 'Unknown')} <span class="mono muted">${enc(item.revision || item.path || item.source || '')}</span></div>
  </div>`;
  return `${head}<div class="ops-card-grid">
      ${apps.map(a => card('Application', a)).join('')}
      ${kust.map(k => card('Kustomization', k)).join('')}
      ${hrs.map(h => card('HelmRelease', h)).join('')}
    </div>`;
}

function alertsView() {
  const items = state.alerts;
  const rows = items.map(a => {
    const sev = a.severity || a.type || a.reason || 'Warning';
    return `<div class="activity-row alert-row">
      <div class="activity-icon warn">${icon('zap')}</div>
      <div style="flex:1;min-width:0"><b>${enc(a.message || a.summary || a.name || 'Alert')}</b>
        <small>${enc(a.namespace || '')}${a.namespace && a.object ? ' · ' : ''}${enc(a.object || a.name || '')}${a.rule ? ` · ${enc(a.rule)}` : ''}</small>
      </div>
      <span class="status ${enc(sev)}"><i></i>${enc(sev)}</span>
      <small class="mono muted">${enc(a.age || (a.time || a.timestamp ? relativeTime(a.time || a.timestamp) : '—'))}</small>
    </div>`;
  }).join('');
  const amRows = (state.alertmanagers || []).map(a => `<tr>
    <td><b>${enc(a.name)}</b><small class="muted"> · ${enc(a.namespace)}</small></td>
    <td class="mono">${enc(a.version || '—')}</td>
    <td>${enc(a.replicas ?? '—')}</td>
    <td>${statusDot(a.ready ? 'Ready' : 'NotReady')}</td>
    <td>${enc(a.age || '—')}</td>
  </tr>`).join('');
  const ruleRows = (state.prometheusRules || []).map(r => `<tr>
    <td><b>${enc(r.name)}</b><small class="muted"> · ${enc(r.namespace)}</small></td>
    <td class="mono">${enc(r.groups ?? 0)} groups</td>
    <td>${enc(r.age || '—')}</td>
  </tr>`).join('');
  return `${pageHead('Alerts', 'Warning events, Alertmanagers, and PrometheusRules.')}
    <section class="panel"><div class="activity-list">${rows || emptyState('No warning alerts', 'Cluster Warning events appear here.', '<button class="secondary mini-btn" data-refresh-ops="alerts">Refresh</button>')}</div></section>
    <section class="panel pod-panel" style="margin-top:20px">
      <div class="panel-heading"><div><div class="eyebrow">ALERTMANAGER</div><h2>Receivers</h2></div></div>
      <div class="table-wrap"><table><thead><tr><th>Name</th><th>Version</th><th>Replicas</th><th>Status</th><th>Age</th></tr></thead><tbody>${amRows || `<tr><td colspan="5">${emptyState('No Alertmanagers', 'Install kube-prometheus or Alertmanager CRDs.', '')}</td></tr>`}</tbody></table></div>
    </section>
    <section class="panel pod-panel" style="margin-top:20px">
      <div class="panel-heading"><div><div class="eyebrow">RULES</div><h2>PrometheusRules</h2></div></div>
      <div class="table-wrap"><table><thead><tr><th>Rule</th><th>Groups</th><th>Age</th></tr></thead><tbody>${ruleRows || `<tr><td colspan="3">${emptyState('No PrometheusRules', '', '')}</td></tr>`}</tbody></table></div>
    </section>`;
}

function auditView(embedded = false) {
  const rows = state.audit.map(e => `<tr>
    <td class="mono audit-time">${enc(e.time || e.timestamp || e.at || '—')}</td>
    <td>${enc(e.action || '—')}</td>
    <td class="mono">${enc(e.target || e.workload || '—')}</td>
    <td>${enc(e.detail || e.message || e.user || '—')}</td>
  </tr>`).join('');
  const table = `<section class="panel pod-panel"><div class="table-wrap"><table><thead><tr><th>Time</th><th>Action</th><th>Target</th><th>Detail</th></tr></thead><tbody>${rows || `<tr><td colspan="4">${emptyState('No audit entries', 'Mutations such as launch, secrets, and VM power will appear here.', '')}</td></tr>`}</tbody></table></div></section>`;
  if (embedded) return table;
  return `${pageHead('Audit', 'Operator actions recorded by the control plane.')}${table}`;
}

function rbacView(embedded = false) {
  const r = state.rbac || {};
  const tab = state.rbacTab || 'bindings';
  const tabs = [
    ['bindings', 'ClusterRoleBindings'],
    ['roles', 'Roles'],
    ['rolebindings', 'RoleBindings'],
    ['clusterroles', 'ClusterRoles'],
  ];
  let body = '';
  if (tab === 'bindings') {
    const items = r.clusterRoleBindings || [];
    body = `<div class="table-wrap"><table><thead><tr><th>Binding</th><th>Role</th><th>Subjects</th></tr></thead><tbody>${items.length ? items.map(b => `<tr><td><b>${enc(b.name)}</b></td><td class="mono">${enc(b.role || b.roleRef || b.clusterRole || '—')}</td><td class="mono">${enc((b.subjects || []).map(s => s.name || s).join(', ') || b.subject || '—')}</td></tr>`).join('') : `<tr><td colspan="3">${emptyState('No cluster role bindings', '', '')}</td></tr>`}</tbody></table></div>`;
  } else if (tab === 'roles') {
    const items = r.roles || [];
    body = `<div class="table-wrap"><table><thead><tr><th>Role</th><th>Namespace</th><th>Rules</th></tr></thead><tbody>${items.length ? items.map(x => `<tr><td><b>${enc(x.name)}</b></td><td>${enc(x.namespace || '—')}</td><td class="mono">${enc(x.rules ?? x.ruleCount ?? '—')}</td></tr>`).join('') : `<tr><td colspan="3">${emptyState('No roles', '', '')}</td></tr>`}</tbody></table></div>`;
  } else if (tab === 'rolebindings') {
    const items = r.roleBindings || [];
    body = `<div class="table-wrap"><table><thead><tr><th>Binding</th><th>Namespace</th><th>Role</th><th>Subjects</th></tr></thead><tbody>${items.length ? items.map(b => `<tr><td><b>${enc(b.name)}</b></td><td>${enc(b.namespace || '—')}</td><td class="mono">${enc(b.role || b.roleRef || '—')}</td><td class="mono">${enc((b.subjects || []).map(s => s.name || s).join(', ') || '—')}</td></tr>`).join('') : `<tr><td colspan="4">${emptyState('No role bindings', '', '')}</td></tr>`}</tbody></table></div>`;
  } else {
    const items = r.clusterRoles || [];
    body = `<div class="table-wrap"><table><thead><tr><th>ClusterRole</th><th>Rules</th></tr></thead><tbody>${items.length ? items.map(x => `<tr><td><b>${enc(x.name)}</b></td><td class="mono">${enc(x.rules ?? x.ruleCount ?? '—')}</td></tr>`).join('') : `<tr><td colspan="2">${emptyState('No cluster roles', '', '')}</td></tr>`}</tbody></table></div>`;
  }
  const inner = `<div class="toolbar"><div class="segment">${tabs.map(([id, label]) => `<button class="${tab === id ? 'active' : ''}" data-rbac-tab="${id}">${label}</button>`).join('')}</div></div>
    <section class="panel pod-panel">${body}</section>`;
  if (embedded) return inner;
  return `${pageHead('Access', 'RBAC bindings and roles across the cluster.')}${inner}`;
}

function costView(embedded = false) {
  const c = state.cost || {};
  const total = c.totalMonthlyUSD ?? c.total_monthly_usd ?? c.totalMonthly ?? c.total_monthly ?? 0;
  const ns = asList(c.namespaces ?? c.breakdown ?? c.byNamespace, ['namespaces', 'breakdown', 'items']);
  const max = Math.max(...ns.map(n => Number(n.monthlyUSD ?? n.monthly_usd ?? n.total ?? 0)), 1);
  const bars = ns.map(n => {
    const usd = Number(n.monthlyUSD ?? n.monthly_usd ?? n.total ?? 0);
    const pct = Math.min(100, (usd / max) * 100);
    return `<div class="cost-row">
      <div class="cost-label"><b>${enc(n.namespace || n.name || '—')}</b><span class="mono">$${usd.toFixed(2)}</span></div>
      <div class="cost-bar"><span style="width:${pct}%"></span></div>
    </div>`;
  }).join('');
  const inner = `<section class="panel cost-panel">
      <div class="cost-hero"><div class="eyebrow">ESTIMATED</div><strong>$${Number(total).toFixed(0)}<span>/mo</span></strong><p class="muted">Based on requests · not a bill</p></div>
      <div class="cost-breakdown">${bars || emptyState('No cost breakdown', 'Workload requests drive this estimate.', '')}</div>
    </section>`;
  if (embedded) return inner;
  return `${pageHead('Cost', 'FinOps estimate from container CPU/memory requests.')}${inner}`;
}

function vmConsoleModal() {
  const c = state.vmConsole || {};
  return `<div class="overlay" id="vm-console-overlay"></div><section class="modal ops-modal">
    <div class="modal-head"><div class="eyebrow">Virtual machine / Console</div><h3>${enc(c.name || '')}</h3><p>${enc(c.namespace || '')} · ${c.ready ? 'Ready' : 'Not ready'}</p></div>
    <div class="modal-body">
      <div class="field full"><label>virtctl</label><pre class="place-receipt mono" id="vm-virtctl">${enc(c.virtctl || `virtctl vnc ${c.name || ''}`)}</pre></div>
      ${c.vncPath ? `<div class="field full"><label>VNC path</label><p class="mono muted">${enc(c.vncPath)}</p></div>` : ''}
      <p class="subhead">Run virtctl from a machine with cluster access, or open the VNC subresource when proxied.</p>
    </div>
    <div class="modal-foot">
      <button class="secondary" type="button" id="vm-console-copy">Copy command</button>
      <button class="primary" type="button" id="vm-console-close">Close</button>
    </div>
  </section>`;
}


function networkView() {
  const d = state.networkingDemo || demoNetworkingData();
  const stats = [
    { label: 'Endpoints', value: d.endpoints, icon: 'network', tone: '#6ab6f0' },
    { label: 'Identities', value: d.identities, icon: 'key', tone: '#b49bd8' },
    { label: 'Throughput', value: `${d.throughput_mbps} Mbps`, icon: 'activity', tone: '#93c88f' },
    { label: 'Encryption', value: d.encryption, icon: 'shield-check', tone: '#ff7550' },
  ];
  return `<div class="kc-page" data-testid="networking-page">
    <div class="kc-page-head">
      <div><h1>Networking</h1><p>eBPF datapath, Hubble flows and network policies powered by Cilium.</p></div>
      <div class="mono muted" style="font-size:12px">${enc(d.cni)}</div>
    </div>
    <div class="kc-stat-grid">${stats.map(s => glassCard(`<div style="display:flex;justify-content:space-between;align-items:center"><div class="kc-stat-label">${enc(s.label)}</div><span style="color:${s.tone}">${lucideSvg(s.icon, 16)}</span></div><div class="kc-stat-value">${enc(String(s.value))}</div>`, 'kc-stat-card')).join('')}</div>
    ${glassCard(`<div class="kc-mesh-banner"><div class="kc-mesh-icon">${lucideSvg('shield', 20)}</div><div><div style="font-size:14px;font-weight:500">${enc(d.services_mesh.engine)}</div><div class="muted" style="font-size:12px;margin-top:2px">mTLS ${d.services_mesh.mTLS ? 'enforced' : 'off'} · Hubble Relay ${enc(d.hubble_relay)}</div></div><span class="kc-badge-ok">Active</span></div>`, '')}
    <div class="kc-split">
      ${glassCard(`<div class="kc-panel-head">Network policies</div><div class="kc-panel-body scrollbar-thin">${d.policies.map(p => `<div class="kc-policy-row"><span style="color:#93c88f">${lucideSvg('shield-check', 16)}</span><div style="min-width:0;flex:1"><div class="mono" style="font-size:14px">${enc(p.name)}</div><div class="muted" style="font-size:11px">${enc(p.kind)}</div></div><div style="text-align:right"><div class="mono" style="font-size:14px">${Number(p.hits_1h).toLocaleString()}</div><div class="muted" style="font-size:11px">hits/1h · ${p.endpoints} eps</div></div></div>`).join('')}</div>`, 'kc-panel')}
      ${glassCard(`<div class="kc-panel-head"><span>Live Hubble flows</span><span class="kc-streaming"><span class="status-dot ok"></span> streaming</span></div><div class="kc-panel-body scrollbar-thin">${d.flows.map(f => `<div class="kc-flow-row"><div><div class="mono">${enc(f.source)}</div><div class="muted" style="font-size:10px">${enc(f.source_ns)}</div></div><div class="kc-flow-arrow">→</div><div><div class="mono">${enc(f.destination)}</div><div class="muted" style="font-size:10px">${enc(f.destination_ns)}</div></div><div class="mono muted">${enc(f.l7)}</div><div>${flowVerdictBadge(f.verdict)}</div><div class="mono muted" style="text-align:right">${(f.bytes / 1024).toFixed(1)}K</div></div>`).join('')}</div>`, 'kc-panel')}
    </div>
  </div>`;
}

function namespacesPanel() {
  const rows = state.namespaces.map((n, i) => `<tr data-namespace-id="${i}"><td><div class="object-name">${enc(n.name)}</div></td><td><span class="status ${n.status === 'Active' ? 'Healthy' : 'Degraded'}">${enc(n.status)}</span></td><td>${n.pods}</td><td>${n.services}</td><td>${enc(n.age)}</td><td><button class="ghost mini-btn danger-text" data-delete-namespace="${enc(n.name)}">Delete</button></td></tr>`).join('');
  return `<div class="table-wrap"><table><thead><tr><th>Namespace</th><th>Status</th><th>Pods</th><th>Services</th><th>Age</th><th></th></tr></thead><tbody>${rows || `<tr><td colspan="6">${emptyState('No namespaces', 'Create a namespace to isolate workloads.', '<button class="primary mini-btn" id="create-namespace">Create namespace</button>')}</td></tr>`}</tbody></table></div>`;
}

function servicesPanel() {
  const items = filterNetworkServices();
  const rows = items.map((s, i) => {
    const ports = (s.ports || []).map(p => `${p.port}→${p.targetPort}${p.nodePort ? ` (node ${p.nodePort})` : ''}`).join(', ') || '—';
    const eps = (s.endpoints || []).join(', ') || '—';
    return `<tr data-service-id="${i}"><td><div class="object-name">${enc(s.name)}</div><div class="object-sub">${enc(s.namespace)}</div></td><td>${enc(s.type)}</td><td class="mono">${enc(s.clusterIP)}</td><td class="mono">${enc(ports)}</td><td class="mono">${enc(eps)}</td><td>${enc(s.age)}</td></tr>`;
  }).join('');
  return `<div class="table-wrap"><table><thead><tr><th>Service</th><th>Type</th><th>Cluster IP</th><th>Ports</th><th>Backend pods</th><th>Age</th></tr></thead><tbody>${rows || `<tr><td colspan="6">${emptyState('No services', 'Services expose pod ports inside the cluster.', '')}</td></tr>`}</tbody></table></div>`;
}

function filterNetworkServices() {
  if (state.networkNs === 'all') return state.services;
  return state.services.filter(s => s.namespace === state.networkNs);
}

function packetwolfFlowsPanel() {
  const flows = state.packetwolfFlows;
  if (!flows) return '';
  const items = Array.isArray(flows) ? flows : (flows.items || flows.flows || []);
  if (!Array.isArray(items) || !items.length) {
    return `<div class="panel glass" style="margin-bottom:16px"><div class="panel-title">PacketWolf flows</div><pre class="mono muted" style="margin:0;white-space:pre-wrap">${enc(typeof flows === 'string' ? flows : JSON.stringify(flows, null, 2))}</pre></div>`;
  }
  const rows = items.slice(0, 25).map(f => {
    const src = f.source || f.src || f.from || '—';
    const dst = f.destination || f.dst || f.to || '—';
    const proto = f.protocol || f.proto || '—';
    const verdict = f.verdict || f.action || f.disposition || '—';
    return `<tr><td class="mono">${enc(String(src))}</td><td class="mono">${enc(String(dst))}</td><td>${enc(String(proto))}</td><td>${enc(String(verdict))}</td></tr>`;
  }).join('');
  return `<div class="panel glass" style="margin-bottom:16px"><div class="panel-title">PacketWolf flows <small class="muted">(latest ${Math.min(items.length, 25)})</small></div>
    <div class="table-wrap"><table><thead><tr><th>Source</th><th>Destination</th><th>Proto</th><th>Verdict</th></tr></thead><tbody>${rows}</tbody></table></div></div>`;
}

function networkMapPanel() {
  const map = state.network || { nodes: [], edges: [] };
  if (!map.nodes?.length) return `<div class="panel">${emptyState('No network paths', 'Services and pods with open ports appear here.', '')}</div>`;
  const services = map.nodes.filter(n => n.kind === 'service');
  const pods = map.nodes.filter(n => n.kind === 'pod');
  const edges = map.edges || [];
  return `<div class="network-map panel">
    <div class="network-map-head"><div class="panel-title">Service → Pod topology</div><div class="panel-sub">Logical traffic paths from Services to selected Pods (declared ports, not live packet capture)</div></div>
    <div class="network-columns">
      <div class="network-col"><div class="network-col-title">Services</div>${services.map(n => networkNodeCard(n)).join('') || '<div class="empty">None</div>'}</div>
      <div class="network-col network-links">${edges.map(e => `<div class="network-link"><span class="mono">${enc(e.label)}</span><span class="network-arrow">→</span><span>${enc(e.to.split('/').pop())}</span></div>`).join('') || '<div class="empty">No edges</div>'}</div>
      <div class="network-col"><div class="network-col-title">Pods</div>${pods.map(n => networkNodeCard(n)).join('') || '<div class="empty">None</div>'}</div>
    </div>
    <div class="network-legend">Open container ports on unmatched pods are listed without a Service edge.</div>
  </div>`;
}

function networkNodeCard(node) {
  const ports = (node.ports || []).join(', ') || '—';
  return `<div class="network-node ${enc(node.kind)}" data-network-node="${enc(node.id)}">
    <div class="network-node-kind">${enc(node.kind)}</div>
    <div class="object-name">${enc(node.label)}</div>
    <div class="object-sub mono">${enc(node.namespace)}${node.ip ? ` · ${enc(node.ip)}` : ''}</div>
    <div class="network-node-ports">${enc(ports)}</div>
  </div>`;
}

function namespaceModal() {
  return `<div class="overlay" id="namespace-overlay"></div><section class="modal">
    <div class="modal-head"><div class="eyebrow">Network / Namespace</div><h3>Create namespace</h3><p>Isolate workloads and services in a dedicated network boundary.</p></div>
    <form id="namespace-form"><div class="modal-body"><div class="field full"><label>Name</label><input name="name" placeholder="payments" required pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" /></div></div>
    <div class="modal-foot"><button class="secondary" type="button" id="namespace-cancel">Cancel</button><button class="primary" type="submit">Create namespace →</button></div></form>
  </section>`;
}

function drawerView() {
  if (state.selectedType === 'pod') return podDrawer(state.selected);
  if (state.selectedType === 'node') return nodeDrawer(state.selected);
  if (state.selectedType === 'vm') return vmDrawer(state.selected);
  return workloadDrawer(state.selected);
}

function drawerTabs(tabs, active) {
  return `<div class="drawer-tabs">${tabs.map(([id, label]) => `<button class="drawer-tab ${active === id ? 'active' : ''}" data-drawer-tab="${id}">${label}</button>`).join('')}</div>`;
}

function vmDrawer(vm) {
  const running = vm.running || /running/i.test(vm.status);
  const tab = state.drawerTab || 'overview';
  const tabs = drawerTabs([['overview', 'Overview'], ['lifecycle', 'Lifecycle'], ['actions', 'Actions']], tab);
  let content = '';
  if (tab === 'lifecycle') {
    const snaps = (state.vmSnapshots || []).filter(s => s.namespace === vm.namespace && s.vm === vm.name);
    const clones = (state.vmClones || []).filter(c => c.namespace === vm.namespace && (c.source === vm.name || c.target === vm.name));
    content = `<div class="drawer-body">
      <div class="detail-section"><div class="detail-label">Snapshots</div>
        ${(snaps.map(s => `<div class="rail-row"><span>${enc(s.name)}</span>${statusDot(s.status || (s.ready ? 'Ready' : 'Pending'))}</div>`).join('') || '<p class="muted">None yet</p>')}
      </div>
      <div class="detail-section"><div class="detail-label">Clones</div>
        ${(clones.map(c => `<div class="rail-row"><span class="mono">${enc(c.source)} → ${enc(c.target)}</span>${statusDot(c.phase)}</div>`).join('') || '<p class="muted">None yet</p>')}
      </div>
    </div>`;
  } else if (tab === 'actions') {
    content = `<div class="drawer-body">
      <div class="actions">
        <button class="primary" data-vm-action="${running ? 'stop' : 'start'}" data-vm-ns="${enc(vm.namespace)}" data-vm-name="${enc(vm.name)}">${running ? 'Stop' : 'Start'}</button>
        ${running ? `<button class="ghost" data-vm-more="migrate" data-vm-ns="${enc(vm.namespace)}" data-vm-name="${enc(vm.name)}">Migrate</button>` : ''}
        <button class="ghost" data-vm-more="snapshot" data-vm-ns="${enc(vm.namespace)}" data-vm-name="${enc(vm.name)}">Snapshot…</button>
        <button class="ghost" data-vm-more="clone" data-vm-ns="${enc(vm.namespace)}" data-vm-name="${enc(vm.name)}">Clone…</button>
        ${state.vmExportSupported ? `<button class="ghost" data-vm-more="export" data-vm-ns="${enc(vm.namespace)}" data-vm-name="${enc(vm.name)}">Export</button>` : ''}
        ${(running || vm.ready) ? `<button class="ghost" data-vm-more="console" data-vm-ns="${enc(vm.namespace)}" data-vm-name="${enc(vm.name)}">Console</button>` : ''}
      </div>
    </div>`;
  } else {
    content = `<div class="drawer-body">
      <div class="metrics-grid">
        <div class="metric"><strong>${enc(vm.status || '—')}</strong><span>Status</span></div>
        <div class="metric"><strong class="mono">${enc(vm.cpu || '—')}</strong><span>CPU</span></div>
        <div class="metric"><strong class="mono">${enc(vm.memory || '—')}</strong><span>Memory</span></div>
      </div>
      <div class="rail-row"><span>Node</span><span class="mono">${enc(vm.node || '—')}</span></div>
      <div class="rail-row"><span>OS</span><span>${enc(vm.os || '—')}</span></div>
      <div class="rail-row"><span>Age</span><span>${enc(vm.age || '—')}</span></div>
      <div class="actions" style="margin-top:16px">
        <button class="primary" data-vm-action="${running ? 'stop' : 'start'}" data-vm-ns="${enc(vm.namespace)}" data-vm-name="${enc(vm.name)}">${running ? 'Stop' : 'Start'}</button>
        <button class="ghost" data-drawer-tab="actions">More actions</button>
      </div>
    </div>`;
  }
  return drawerShell(vm.name, `VirtualMachine · ${enc(vm.namespace)}`, tabs, content);
}

function drawerShell(title, kicker, tabs, body) {
  return `<div class="overlay" id="drawer-overlay"></div><aside class="drawer"><div class="drawer-head"><div><h3>${enc(title)}</h3><div class="drawer-kicker">${kicker}</div></div><button class="icon-btn" id="drawer-close">×</button></div>${tabs}${body}</aside>`;
}

function workloadPods(workload) {
  return state.pods.filter(p => p.namespace === workload.namespace && (p.name.startsWith(`${workload.name}-`) || p.name.includes(workload.name)));
}

function pickLogPod(workload) {
  const pods = workloadPods(workload);
  return pods.find(p => p.status === 'Running' && String(p.ready).includes('/')) || pods[0] || null;
}

function rolloutSection(workload) {
  const r = state.rolloutData;
  if (!r || !['Deployment', 'StatefulSet'].includes(workload.kind)) return '';
  const actions = r.paused ? ['resume', 'restart', 'undo'] : ['pause', 'restart', 'undo'];
  return `<div class="detail-section rollout-section">
    <div class="detail-label">Rollout · ${enc(r.status || 'unknown')}</div>
    <div class="rollout-stats mono">${r.ready}/${r.replicas} ready · ${r.updated} updated</div>
    <div class="actions">${actions.map(a => `<button class="ghost mini-btn" data-rollout="${a}">${a}</button>`).join('')}</div>
    ${(r.history || []).length ? `<div class="rollout-history">${r.history.map(h => `<div class="rollout-rev mono">rev ${h.revision} · ${enc(h.status)} · ${enc(h.age)}</div>`).join('')}</div>` : ''}
  </div>`;
}

function manifestSection() {
  const d = state.resourceDetail;
  if (!d) return `<div class="detail-section"><div class="detail-label">Manifest</div><p class="object-sub">Loading resource detail…</p></div>`;
  const labels = Object.entries(d.labels || {}).map(([k, v]) => `${k}=${v}`).join('\n') || '—';
  const yaml = JSON.stringify(d.manifest || {}, null, 2);
  return `<div class="detail-section">
    <div class="detail-label">Labels</div><pre class="manifest-block mono">${enc(labels)}</pre>
    <div class="detail-label">Manifest</div><pre class="manifest-block mono">${enc(yaml.slice(0, 8000))}</pre>
    <button class="ghost mini-btn" id="manifest-copy">Copy manifest</button>
  </div>`;
}

function networkGuardSection(workload) {
  const g = state.networkGuard;
  const svcs = state.services.filter(s => s.namespace === workload.namespace && (s.endpoints || []).some(ep => ep.includes(workload.name)));
  return `<div class="detail-section">
    <div class="detail-label">Network</div>
    ${g ? `<p class="object-sub">PacketWolf: ${g.configured ? (g.reachable ? 'Connected' : 'Unreachable') : 'Not configured'}${g.flowsCount ? ` · ${g.flowsCount} flows` : ''}</p>${g.policyHint ? `<p class="object-sub">${enc(g.policyHint)}</p>` : ''}` : ''}
    <div class="pod-list">${svcs.map(s => `<div class="pod-chip"><div><div class="object-name">${enc(s.name)}</div><div class="object-sub">${enc(s.type)} · ${enc(s.clusterIP)}</div></div></div>`).join('') || emptyState('No services', 'Services exposing this workload appear here.', '')}</div>
  </div>`;
}

function workloadDrawer(workload) {
  const pods = workloadPods(workload);
  const tabs = drawerTabs([['overview', 'Overview'], ['logs', 'Logs'], ['events', 'Events'], ['manifest', 'Manifest'], ['network', 'Network']], state.drawerTab);
  let content = '';
  if (state.drawerTab === 'overview') {
    const draft = state.scaleDraft ?? workload.desired;
    content = `<div class="drawer-body">
      ${renderFixItBanner(workload)}
      <div class="actions">
        ${workload.desired === 0 ? '<button class="primary" data-action="start">▶ Scale up</button>' : '<button class="secondary" data-still="hush">Scale to 0</button>'}
        <button class="ghost" data-action="restart">↻ Restart</button>
        ${workloadIsSealed(workload) ? '<button class="ghost" data-action="unseal">Remove isolation</button>' : '<button class="ghost" data-still="seal">⛨ Isolate</button>'}
        <button class="danger" data-still="end">Delete</button>
      </div>
      <div class="scale-row"><label>Replicas</label><div class="scale-controls"><button class="ghost" id="scale-down">−</button><input id="scale-input" type="number" min="0" value="${draft}" /><button class="ghost" id="scale-up">+</button><button class="secondary" id="scale-apply">Apply</button></div></div>
      <div class="metrics-grid"><div class="metric"><strong>${workload.ready}/${workload.desired}</strong><span>Ready replicas</span></div><div class="metric"><strong>${enc(workload.status)}</strong><span>State</span></div><div class="metric"><strong>${enc(workload.age)}</strong><span>Age</span></div></div>
      ${rolloutSection(workload)}
      <div class="detail-section"><div class="detail-label">Image</div><div class="mono">${enc(workload.image)}</div></div>
      <div class="detail-section"><div class="detail-label">Pods</div><div class="pod-list">${pods.map(p => `<div class="pod-chip" data-pod-name="${enc(p.name)}"><div><div class="object-name">${enc(p.name)}</div><div class="object-sub">${enc(p.node)}</div></div><span class="status ${enc(podDisplayPhase(p))} phase-pill ${phaseGlowClass(podDisplayPhase(p))}">${enc(podDisplayPhase(p))}</span></div>`).join('') || emptyState('No active pods', 'Pods appear when replicas are scheduled.', '')}</div></div>
    </div>`;
  } else if (state.drawerTab === 'logs') {
    const pod = pickLogPod(workload);
    content = `<div class="drawer-body">${pod ? renderLogPanelHTML(pod) : emptyState('No pod for logs', 'No running pod found for this workload.', '')}</div>`;
  } else if (state.drawerTab === 'manifest') {
    content = `<div class="drawer-body">${manifestSection()}</div>`;
  } else if (state.drawerTab === 'network') {
    content = `<div class="drawer-body">${networkGuardSection(workload)}</div>`;
  } else {
    const events = (state.resourceDetail?.relatedEvents || state.events.filter(e => e.object?.includes(workload.name) || e.namespace === workload.namespace)).slice(0, 20);
    content = `<div class="drawer-body"><div class="activity">${events.map(eventRow).join('') || emptyState('No events', 'No recent events for this workload.', '')}</div></div>`;
  }
  return drawerShell(workload.name, `${enc(workload.kind)} · ${enc(workload.namespace)}`, tabs, content);
}

function podDrawer(pod) {
  const tabs = drawerTabs([['overview', 'Overview'], ['logs', 'Logs'], ['terminal', 'Terminal'], ['events', 'Events']], state.drawerTab);
  const phase = podDisplayPhase(pod);
  let content = '';
  if (state.drawerTab === 'overview') {
    content = `<div class="drawer-body pod-detail">
      ${renderFixItBanner(pod)}
      <div class="pod-detail-head"><h3>${enc(pod.workloadId || pod.name)}</h3><span class="phase-pill ${phaseGlowClass(phase)}">${enc(phase)}</span></div>
      <p class="pod-detail-sub">${enc(pod.namespace)} · ${enc(pod.node || '—')} · ${enc(pod.age)} · Ready ${enc(pod.ready)}</p>
      <div class="metrics-grid"><div class="metric"><strong>${enc(pod.restarts)}</strong><span>Restarts</span></div><div class="metric"><strong>${enc(pod.qos || '—')}</strong><span>QoS</span></div><div class="metric"><strong class="mono">${enc(pod.ip || '—')}</strong><span>Pod IP</span></div></div>
      <div class="detail-section"><div class="detail-label">Image</div><div class="mono">${enc(pod.image || '—')}</div></div>
      <div class="detail-section"><div class="detail-label">Resources</div><div class="mono">${enc(pod.cpuLimit || '—')} CPU · ${enc(pod.memLimit || '—')} memory</div></div>
      <div class="detail-section"><div class="detail-label">Ports</div><div class="mono">${enc((pod.ports || []).map(p => p.containerPort).join(', ') || '—')}</div></div>
      <div class="actions">
        <button class="primary" data-drawer-tab="terminal">Terminal</button>
        <button class="ghost" data-drawer-tab="logs">Logs</button>
        <button class="ghost" id="open-window">Port forward</button>
        <button class="ghost" data-still="sleep">Restart pod</button>
        <button class="ghost" data-still="seal">Isolate</button>
        <button class="secondary" id="place-sibling">Launch similar</button>
        <button class="secondary" id="follow-wires">View network →</button>
      </div>
    </div>`;
  } else if (state.drawerTab === 'logs') {
    content = `<div class="drawer-body">${renderLogPanelHTML(pod)}</div>`;
  } else if (state.drawerTab === 'events') {
    const events = state.events.filter(e => e.object?.includes(pod.name) || e.namespace === pod.namespace).slice(0, 20);
    content = `<div class="drawer-body"><div class="activity">${events.map(eventRow).join('') || emptyState('No events', 'No recent events for this pod.', '')}</div></div>`;
  } else {
    content = `<div class="drawer-body"><div id="pod-console">${renderTerminalHTML(pod)}</div></div>`;
  }
  return drawerShell(pod.name, `${enc(pod.namespace)} · ${enc(pod.node)}`, tabs, content);
}

function nodeDrawer(node) {
  const tab = state.nodeDrawerTab || 'overview';
  const tabs = drawerTabs([['overview', 'Overview'], ['capacity', 'Capacity'], ['pods', 'Pods'], ['events', 'Events']], tab);
  const nodePods = state.pods.filter(p => p.node === node.name);
  const nodeEvents = state.events.filter(e => e.object?.includes(node.name) || e.message?.includes(node.name)).slice(0, 20);
  let body = '';
  if (tab === 'overview') {
    const conds = (node.conditions || []).map(c => `<div class="rail-row"><span>${enc(c.type)}</span><span class="${c.status === 'True' && c.type === 'Ready' ? 'status Healthy' : ''}">${enc(c.status)}</span></div>`).join('');
    body = `<div class="drawer-body">
      <div class="actions"><button class="secondary" data-action="cordon">Cordon</button><button class="ghost" data-action="uncordon">Uncordon</button><button class="danger" data-action="drain">Drain</button></div>
      ${node.unschedulable ? '<div class="fix-it-banner"><div class="fix-it-icon">!</div><div><h4>Node cordoned</h4><p>New pods will not be scheduled on this node.</p></div></div>' : ''}
      <div class="detail-section"><div class="detail-label">Conditions</div>${conds || '<p class="object-sub">—</p>'}</div>
      <div class="detail-section"><div class="detail-label">System</div>
        <div class="rail-row"><span>Kubelet</span><span class="mono">${enc(node.version)}</span></div>
        <div class="rail-row"><span>OS</span><span>${enc(node.osImage || '—')}</span></div>
        <div class="rail-row"><span>Kernel</span><span class="mono">${enc(node.kernelVersion || '—')}</span></div>
        <div class="rail-row"><span>Runtime</span><span class="mono">${enc(node.containerRuntime || '—')}</span></div>
        <div class="rail-row"><span>Internal IP</span><span class="mono">${enc(node.internalIp)}</span></div>
      </div>
    </div>`;
  } else if (tab === 'capacity') {
    body = `<div class="drawer-body">
      <div class="metrics-grid"><div class="metric"><strong>${enc(node.pods)}</strong><span>Pods scheduled</span></div><div class="metric"><strong>${enc(node.cpu)}</strong><span>CPU capacity</span></div><div class="metric"><strong>${enc(node.memory)}</strong><span>Memory capacity</span></div></div>
      <div class="detail-section"><div class="detail-label">Allocatable</div>
        <div class="rail-row"><span>CPU</span><span class="mono">${enc(node.allocatableCpu || node.cpu)}</span></div>
        <div class="rail-row"><span>Memory</span><span class="mono">${enc(node.allocatableMemory || node.memory)}</span></div>
        <div class="rail-row"><span>Max pods</span><span class="mono">${enc(node.allocatablePods || '—')}</span></div>
      </div>
    </div>`;
  } else if (tab === 'pods') {
    body = `<div class="drawer-body"><div class="pod-list">${nodePods.map(p => `<div class="pod-chip" data-pod-name="${enc(p.name)}" data-pod-ns="${enc(p.namespace)}"><div><div class="object-name">${enc(p.name)}</div><div class="object-sub">${enc(p.namespace)}</div></div><span class="status ${enc(p.status)}">${enc(p.status)}</span></div>`).join('') || emptyState('No pods', 'No pods scheduled on this node.', '')}</div></div>`;
  } else {
    body = `<div class="drawer-body"><div class="activity">${nodeEvents.map(eventRow).join('') || emptyState('No events', 'No recent events for this node.', '')}</div></div>`;
  }
  return drawerShell(node.name, `${enc(node.role)} · ${enc(node.status)}`, tabs, body);
}

function renderLogPanelHTML(pod) {
  const containers = pod.containers || [];
  const hint = honestBreath(pod);
  return `<div class="log-panel" data-log-ns="${enc(pod.namespace)}" data-log-pod="${enc(pod.name)}" data-honest-previous="${hint?.previous ? '1' : '0'}" data-honest-container="${enc(hint?.container || '')}">
    ${waveformSparkline(pod.restarts)}
    ${hint ? `<div class="breath-hint">${enc(hint.banner)}</div>` : ''}
    <div class="log-toolbar">
      <select id="log-tail"><option value="100">100</option><option value="200" selected>200</option><option value="500">500</option><option value="1000">1000</option><option value="5000">5000</option></select>
      ${containers.length ? `<select id="log-container">${containers.map(c => `<option value="${enc(c)}" ${hint?.container === c ? 'selected' : ''}>${enc(c)}</option>`).join('')}</select>` : ''}
      <label class="log-toggle"><input type="checkbox" id="log-follow" checked /> Follow</label>
      <label class="log-toggle"><input type="checkbox" id="log-wrap" /> Wrap</label>
      <label class="log-toggle"><input type="checkbox" id="log-previous" ${hint?.previous ? 'checked' : ''} /> Previous</label>
      <input id="log-filter" placeholder="Filter logs…" />
      <button class="ghost mini-btn" id="log-copy">Copy</button>
      <button class="ghost mini-btn" id="log-refresh">Refresh</button>
    </div>
    <pre class="log-view" id="log-view">Loading logs…</pre>
    <div class="log-meta" id="log-meta"></div>
  </div>`;
}

function renderTerminalHTML(pod) {
  const interactive = state.realtimeCapabilities?.interactiveExec;
  const ports = (pod.ports || []).map(p => `${p.containerPort}/${p.protocol}`).join(', ') || 'none declared';
  if (!state.presenceKnocked) {
    const phase = podDisplayPhase(pod);
    const canKnock = phase === 'Running' || phase === 'Sealed';
    if (!canKnock) {
      return `<div class="presence-gate"><div class="presence-gate-inner"><div class="eyebrow font-mono">Terminal</div><h3>Unavailable</h3><p>Pod must be Running or Isolated to open a terminal.</p></div></div>`;
    }
    return `<div class="presence-gate" data-term-ns="${enc(pod.namespace)}" data-term-pod="${enc(pod.name)}">
      <div class="presence-gate-inner">
        <div class="eyebrow font-mono">Terminal</div>
        <h3>Open shell session</h3>
        <p>Operator <strong>${enc(state.session?.user || 'admin')}</strong> · ${enc(pod.namespace)}/${enc(pod.name)} · ${enc(pod.node || '—')}</p>
        <div class="presence-meta mono">Ports: ${enc(ports)}</div>
        <button class="primary" id="presence-knock">Connect →</button>
      </div>
    </div>`;
  }
  return `<div class="terminal-shell" data-term-ns="${enc(pod.namespace)}" data-term-pod="${enc(pod.name)}">
    <div class="presence-quick">
      <span class="eyebrow font-mono">Quick</span>
      ${['whoami', 'ps', 'env'].map(c => `<button class="ghost mini-btn" data-quick-cmd="${enc(c)}">${c}</button>`).join('')}
      <button class="ghost mini-btn" id="presence-netstat" title="Open ports from spec">netstat</button>
      <span class="presence-timer mono" id="presence-timer">40:00</span>
    </div>
    <div class="terminal"><div class="terminal-top"><div class="lights"><i></i><i></i><i></i></div><span class="mono">${enc(pod.namespace)}/${enc(pod.name)}</span><span class="term-status" id="term-status">${interactive ? 'Connecting…' : 'Command mode'}</span></div>
    <pre class="term-screen" id="term-screen">${interactive ? '' : `Axiom terminal · command mode\nConnected to ${enc(pod.name)}\n\n`}</pre>
    ${interactive ? '' : `<div class="terminal-line"><span class="prompt">$</span><input class="terminal-input" id="terminal-input" placeholder="whoami" autofocus /></div>`}
    </div></div>`;
}

function stillActionLabels() {
  return { hush: 'Scale to 0', sleep: 'Restart pod', seal: 'Isolate', end: 'Delete' };
}

function stillActionDesc() {
  return {
    hush: 'Scale the workload to zero replicas. The spec remains; scale up to restore.',
    sleep: 'Delete this pod. The controller will create a replacement if replicas are desired.',
    seal: 'Apply deny-all NetworkPolicy except control plane traffic.',
    end: 'Delete the workload. Affected pods, ports, and volumes are listed below.',
  };
}

function helmRollbackModal() {
  const h = state.helmRollback;
  if (!h) return '';
  const options = (h.revisions || []).map(r => `<option value="${enc(String(r.revision))}">rev ${enc(String(r.revision))} · ${enc(r.chart || '—')} · ${enc(r.status || '—')} · ${enc(r.age || '')}</option>`).join('');
  return `<div class="overlay" id="helm-rollback-overlay"></div><section class="modal confirm-modal">
    <div class="modal-head"><h3>Rollback Helm release</h3><p class="object-sub">${enc(h.namespace)}/${enc(h.name)}</p></div>
    <div class="modal-body"><label>Target revision<select id="helm-rollback-rev" class="search-input" style="width:100%;margin-top:8px">${options}</select></label></div>
    <div class="modal-foot"><button class="secondary" id="helm-rollback-cancel">Cancel</button><button class="primary" id="helm-rollback-go">Rollback</button></div>
  </section>`;
}

function confirmModal() {
  const c = state.confirm;
  return `<div class="overlay" id="confirm-overlay"></div><section class="modal confirm-modal">
    <div class="modal-head"><h3>${enc(c.title)}</h3><p>Type <strong class="mono">${enc(c.name)}</strong> to confirm.</p></div>
    <div class="modal-body"><input id="confirm-input" placeholder="${enc(c.name)}" autofocus /></div>
    <div class="modal-foot"><button class="secondary" id="confirm-cancel">Cancel</button><button class="danger" id="confirm-go" disabled>${enc(c.confirmLabel || 'Confirm')}</button></div>
  </section>`;
}

function placeModal() {
  const step = state.placeStep;
  const draft = state.placeDraft;
  const mode = state.placeMode || 'intent';
  const form = state.placeForm || {};
  const modeTabs = [['intent', 'Intent'], ['form', 'Form'], ['yaml', 'YAML']].map(([id, label]) =>
    `<button type="button" class="${mode === id ? 'active' : ''}" data-place-mode="${id}">${label}</button>`).join('');
  const suggested = [
    'Run hello-web, 3 copies, 512Mi',
    'Run demo-api on staging, 2 copies, image nginx:1.27',
    'Run nginx on prod, 1 copy, 256Mi',
  ];
  let body = '';
  if (mode === 'form') {
    body = `<div class="place-step"><label class="eyebrow font-mono">Run workload — form</label>
      <div class="form-grid">
        <div class="field"><label>Name</label><input id="form-name" value="${enc(form.name || '')}" /></div>
        <div class="field"><label>Namespace</label><input id="form-namespace" value="${enc(form.namespace || 'default')}" /></div>
        <div class="field full"><label>Image</label><input id="form-image" value="${enc(form.image || 'nginx:1.27')}" class="mono" /></div>
        <div class="field"><label>Replicas</label><input id="form-replicas" type="number" min="1" max="32" value="${enc(form.replicas ?? 1)}" /></div>
        <div class="field"><label>Memory</label><input id="form-memory" value="${enc(form.memory || '256Mi')}" /></div>
        <div class="field"><label>Port</label><input id="form-port" type="number" min="1" value="${enc(form.port ?? 80)}" /></div>
      </div>
      <p class="subhead" style="margin-top:12px">Deploys a Kubernetes Deployment via <span class="mono">POST /api/v1/workloads</span> — same path Aether uses for Run.</p></div>`;
  } else if (mode === 'yaml') {
    body = `<div class="place-step"><label class="eyebrow font-mono">Run workload — YAML</label>
      <textarea id="place-yaml" rows="12" class="mono" placeholder="name: hello-web&#10;namespace: default&#10;image: nginx:1.27&#10;replicas: 2&#10;memory: 256Mi&#10;port: 80">${enc(state.placeYaml || '')}</textarea>
      <div class="place-yaml-actions"><button type="button" class="secondary mini-btn" id="place-validate">Validate</button>
        ${state.validateResult ? `<span class="validate-result ${state.validateResult.valid ? 'ok' : 'bad'} mono">${state.validateResult.valid ? 'Valid' : enc((state.validateResult.errors || []).join('; ') || state.validateResult.error || 'Invalid')}</span>` : ''}
      </div>
      <p class="subhead" style="margin-top:10px">Simple key:value fields (or paste Axiom Application receipt). Validate dry-runs via <span class="mono">POST /api/v1/validate</span>.</p></div>`;
  } else if (step === 1) {
    const prev = state.placePreview;
    body = `<div class="place-step"><label class="eyebrow font-mono">Step 1 — Intent</label>
      <textarea id="place-intent" rows="4" placeholder="Run hello-web, 3 copies, 512Mi">${enc(state.placeIntent)}</textarea>
      <div class="place-suggestions">${suggested.map(s => `<button type="button" class="ghost mini-btn place-suggest">${enc(s)}</button>`).join('')}</div>
      ${prev ? `<p class="place-parse-preview mono">Parsed ${enc(prev.name)} ×${prev.replicas} · ${enc(prev.image || '')} · ${enc(prev.memory)} · ${enc(prev.namespace)}</p>` : ''}</div>`;
  } else if (step === 2 && draft) {
    body = `<div class="place-step"><label class="eyebrow font-mono">Step 2 — Object</label>
      <div class="place-object-preview">
        <p class="font-mono">${enc(draft.floor)}</p>
        <h3>${enc(draft.name)}</h3>
        <p>${draft.replicas} replicas · ${enc(draft.image)} · ${enc(draft.memory)}</p>
        <div class="place-replica-bars">${Array.from({ length: Math.min(draft.replicas, 8) }).map(() => '<span></span>').join('')}</div>
      </div>
      <div class="form-grid">
        <div class="field"><label>Name</label><input id="place-name" value="${enc(draft.name)}" /></div>
        <div class="field"><label>Replicas</label><input id="place-replicas" type="number" min="1" value="${draft.replicas}" /></div>
        <div class="field"><label>Floor</label><input id="place-floor" value="${enc(draft.floor)}" readonly /></div>
        <div class="field"><label>Namespace</label><input id="place-namespace" value="${enc(draft.namespace)}" /></div>
        <div class="field"><label>Image</label><input id="place-image" value="${enc(draft.image)}" class="mono" /></div>
        <div class="field"><label>Memory</label><input id="place-memory" value="${enc(draft.memory)}" /></div>
      </div></div>`;
  } else if (step === 3 && draft) {
    const scores = draft.scores || {};
    const notes = {
      cost: 'Right-sizer: memory is honest.',
      fit: draft.floor === 'plant' ? 'Plant rooms are for the control plane.' : 'Floor has headroom.',
      latency: 'Pinned to kubernetes on k3s.',
      policy: draft.noRoot ? 'Restricted PSS will admit.' : 'Admin reason required.',
    };
    body = `<div class="place-step"><label class="eyebrow font-mono">Step 3 — Scores</label>
      <div class="score-bars">${['cost', 'fit', 'latency', 'policy'].map(k => `<div class="score-block"><div class="score-row"><span>${k}</span><div class="score-track"><span style="width:${scores[k] || 0}%"></span></div><span class="mono">${scores[k] || 0}</span></div><p class="score-note">${enc(notes[k])}</p></div>`).join('')}</div></div>`;
  } else if (step === 4) {
    body = `<div class="place-step"><label class="eyebrow font-mono">Step 4 — Receipt</label>
      <pre class="place-receipt mono">${enc(state.placeReceipt || '')}</pre></div>`;
  }
  const steps = mode === 'intent' ? [1, 2, 3, 4].map(n => `<span class="place-dot ${step >= n ? 'active' : ''}"></span>`).join('') : '';
  const primaryLabel = mode === 'intent'
    ? (step === 4 ? 'Deploy →' : 'Continue →')
    : 'Run workload →';
  return `<div class="overlay" id="modal-overlay"></div><section class="modal place-modal">
    <div class="modal-head">
      <div class="eyebrow font-mono">Launch / Run</div>
      <h3>Run a workload.</h3>
      <p>Intent scoring, form deploy, or YAML — Aether-style run from the console.</p>
      <div class="segment" style="margin-top:12px">${modeTabs}</div>
      ${steps ? `<div class="place-steps">${steps}</div>` : ''}
    </div>
    <div class="modal-body">${body}</div>
    <div class="modal-foot">
      <button class="secondary" type="button" id="launch-cancel">${mode === 'intent' && step > 1 ? 'Back' : 'Cancel'}</button>
      <button class="primary-button" type="button" id="place-next">${primaryLabel}</button>
    </div>
  </section>`;
}

function stillRingModal() {
  const s = state.still;
  if (!s) return '';
  const p = s.preview || {};
  const labels = stillActionLabels();
  const desc = stillActionDesc();
  return `<div class="overlay" id="still-overlay"></div><section class="modal still-ring">
    <div class="still-ring-head"><div class="eyebrow font-mono">Action preview / ${enc(labels[s.action] || s.action)}</div><h3>${enc(labels[s.action] || s.action)} ${enc(s.workload?.name || s.preview?.workloadId || '')}</h3><p>${enc(desc[s.action] || '')}</p></div>
    <div class="still-ring-body">
      <div class="still-stats">
        <div class="metric"><strong>${p.replicas ?? 0}</strong><span>Pods</span></div>
        <div class="metric"><strong>${p.sessions ?? 0}</strong><span>Terminal</span></div>
        <div class="metric"><strong>${p.windows ?? 0}</strong><span>Port forwards</span></div>
        <div class="metric"><strong>${p.services ?? 0}</strong><span>Services</span></div>
        <div class="metric"><strong>${p.rps ?? 0}</strong><span>Ports</span></div>
        <div class="metric"><strong>${(p.atlas || []).length}</strong><span>PVCs</span></div>
      </div>
      ${(p.names || []).length ? `<div class="detail-section"><div class="detail-label">Affected pods</div><div class="mono still-names">${(p.names || []).map(n => enc(n)).join('<br>')}</div></div>` : ''}
      ${(p.ports || []).length ? `<div class="detail-section"><div class="detail-label">Open ports</div><div class="mono">${(p.ports || []).map(n => enc(n)).join(', ')}</div></div>` : ''}
      ${(p.atlas || []).length ? `<div class="detail-section"><div class="detail-label">Persistent volumes</div><div class="mono">${(p.atlas || []).map(n => enc(n)).join(', ')}</div></div>` : ''}
    </div>
    <div class="modal-foot">
      <button class="secondary" id="still-cancel">Cancel</button>
      <button class="danger" id="still-confirm">Confirm ${enc(labels[s.action] || '')}</button>
    </div>
  </section>`;
}

function launchModal() {
  return placeModal();
}

function paletteItems(query = '') {
  const items = [
    ['overview', 'Open Overview', 'Cluster health and pods'],
    ['workloads', 'Open Workloads', 'Application lifecycle'],
    ['pods', 'Open Pods', 'Runtime instances'],
    ['network', 'Open Network', 'Services and topology'],
    ['nodes', 'Open Nodes', 'Compute fleet'],
    ['events', 'Open Events', 'Kubernetes events'],
    ['secrets', 'Open Secrets', 'Namespaced secret inventory'],
    ['backups', 'Open Backups', 'VolumeSnapshots'],
    ['ops', 'Open Ops', 'Helm · GitOps · Audit · Access · Cost'],
    ['helm', 'Open Helm', 'Helm releases (Ops)'],
    ['gitops', 'Open GitOps', 'Argo / Flux (Ops)'],
    ['alerts', 'Open Alerts', 'Warning signals'],
    ['audit', 'Open Audit', 'Operator action trail (Ops)'],
    ['rbac', 'Open Access', 'RBAC bindings (Ops)'],
    ['cost', 'Open Cost', 'FinOps estimate (Ops)'],
    ['vms', 'Open Virtual machines', 'KubeVirt guests'],
    ['launch', 'Launch', 'Intent → deploy a workload'],
    ['refresh', 'Refresh cluster state', 'Pull control-plane state'],
  ];
  const q = String(query || '').trim().toLowerCase();
  if (q) {
    state.pods.forEach((p, idx) => {
      const hay = `${p.name} ${p.workloadId || ''} ${p.namespace} ${p.node} ${p.status} ${p.image || ''}`.toLowerCase();
      if (!q.split(/\s+/).every(tok => hay.includes(tok))) return;
      items.unshift([`pod-enter:${idx}`, `Terminal · ${p.name}`, `${p.namespace} · ${podDisplayPhase(p)}`]);
      items.unshift([`pod-breath:${idx}`, `Logs · ${p.name}`, p.namespace]);
      if (p.workloadId) items.unshift([`pod-still:${idx}:hush`, `Scale to 0 · ${p.workloadId}`, p.name]);
    });
    state.workloads.forEach((w, idx) => {
      const hay = `${w.name} ${w.namespace} ${w.image} ${w.kind}`.toLowerCase();
      if (q.split(/\s+/).every(tok => hay.includes(tok))) {
        items.unshift([`workload:${idx}`, `Open workload ${w.name}`, w.namespace]);
      }
    });
  }
  if (state.selectedType === 'workload' && state.selected) {
    items.unshift(['drawer-logs', 'Open logs', state.selected.name], ['drawer-still-sleep', 'Restart pod', state.selected.name], ['drawer-still-hush', 'Scale to 0', state.selected.name], ['drawer-still-end', 'Delete workload', state.selected.name]);
  }
  if (state.selectedType === 'pod' && state.selected) {
    items.unshift(['drawer-terminal', 'Open terminal', state.selected.name], ['drawer-logs', 'Open logs', state.selected.name], ['drawer-wires', 'View network', state.selected.namespace], ['drawer-evict', 'Evict pod', state.selected.name]);
  }
  return items;
}

function paletteView() {
  const q = state.paletteQuery || '';
  const items = paletteItems(q);
  return `<div class="overlay" id="palette-overlay"></div><section class="palette"><input id="palette-input" placeholder="Launch, find pods, logs, scale, terminal…" value="${enc(q)}" autofocus /><div class="palette-list">${items.length ? items.map((i, idx) => `<div class="palette-item ${idx === state.paletteHighlight ? 'highlight' : ''}" data-command="${enc(i[0])}"><div class="palette-icon">${navIcon(i[0].startsWith('drawer') || i[0].startsWith('pod') ? 'pods' : i[0] === 'refresh' ? 'events' : i[0].split(':')[0] === 'workload' ? 'workloads' : i[0])}</div><div><div class="palette-label">${i[1]}</div><div class="palette-meta">${i[2]}</div></div></div>`).join('') : '<div class="palette-empty">No matching resources.</div>'}</div></section>`;
}

function relativeTime(value) {
  if (!value) return '—';
  const delta = (Date.now() - new Date(value).getTime()) / 1000;
  if (delta < 60) return `${Math.max(0, Math.round(delta))}s`;
  if (delta < 3600) return `${Math.round(delta / 60)}m`;
  if (delta < 86400) return `${Math.round(delta / 3600)}h`;
  return `${Math.round(delta / 86400)}d`;
}

function wireShell() {
  $$('[data-page]').forEach(btn => btn.onclick = () => navigateTo(btn.dataset.page));
  $('#cluster-toggle')?.addEventListener('click', e => {
    e.stopPropagation();
    state.clusterMenu = !state.clusterMenu;
    state.userMenu = false;
    render();
  });
  $$('[data-cluster-id]').forEach(btn => btn.onclick = e => {
    e.stopPropagation();
    state.activeClusterId = btn.dataset.clusterId;
    localStorage.setItem('axiom.activeCluster', state.activeClusterId);
    state.clusterMenu = false;
    render();
  });
  $('#user-toggle')?.addEventListener('click', e => {
    e.stopPropagation();
    state.userMenu = !state.userMenu;
    state.clusterMenu = false;
    render();
  });
  $('#theme-toggle-glass')?.addEventListener('click', () => setTheme(resolvedTheme() === 'dark' ? 'light' : 'dark'));
  $('#connect-kubeconfig')?.addEventListener('click', () => toast('Sample data mode — Point KUBECONFIG at this dashboard to switch to a live cluster.'));
  $('#logs-pod-select')?.addEventListener('change', e => {
    state.logsPodIndex = Number(e.target.value) || 0;
    cleanupPanels();
    render();
  });
  if (state.page === 'logs') wireLogPanel();
  if (shellMenuCloser) document.removeEventListener('click', shellMenuCloser);
  shellMenuCloser = event => {
    if ((state.clusterMenu && !event.target.closest('.cluster-switcher')) || (state.userMenu && !event.target.closest('.user-menu-wrap'))) {
      state.clusterMenu = false;
      state.userMenu = false;
      render();
    }
  };
  document.addEventListener('click', shellMenuCloser);
  $('#jobs-toggle')?.addEventListener('click', () => { state.jobsExpanded = !state.jobsExpanded; render(); });
  $$('[data-dismiss-job]').forEach(btn => btn.onclick = () => {
    state.jobs = state.jobs.filter(j => j.id !== btn.dataset.dismissJob);
    stopJobsPollIfIdle();
    render();
  });
  $$('[data-ops-tab]').forEach(btn => btn.onclick = () => {
    state.opsTab = btn.dataset.opsTab;
    localStorage.setItem('axiom.opsTab', state.opsTab);
    state.page = 'ops';
    state.opsLoaded[state.opsTab] = false;
    ensureOpsPage(state.opsTab, true).then(() => render());
    render();
  });
  $('#region-toggle')?.addEventListener('click', (e) => {
    e.stopPropagation();
    state.regionMenu = !state.regionMenu;
    render();
  });
  $$('[data-region]').forEach(btn => btn.onclick = (e) => {
    e.stopPropagation();
    state.region = btn.dataset.region;
    localStorage.setItem('axiom.region', state.region);
    state.regionMenu = false;
    toast(`Region set to ${state.region}`);
    render();
  });
  $$('[data-vm-filter]').forEach(btn => btn.onclick = () => { state.vmFilter = btn.dataset.vmFilter; render(); });
  $$('[data-vm-life]').forEach(btn => btn.onclick = () => { state.vmLifecycleTab = btn.dataset.vmLife; render(); });
  $$('[data-volume-filter]').forEach(btn => btn.onclick = () => { state.volumeFilter = btn.dataset.volumeFilter; render(); });
  $('#atlas-refresh')?.addEventListener('click', () => loadAtlas().then(() => render()));
  $('#atlas-volume-open')?.addEventListener('click', () => { state.atlasVolumeModal = true; render(); });
  $('#atlas-vol-overlay')?.addEventListener('click', () => { state.atlasVolumeModal = false; render(); });
  $('#atlas-vol-cancel')?.addEventListener('click', () => { state.atlasVolumeModal = false; render(); });
  $('#atlas-vol-form')?.addEventListener('submit', async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    try {
      await api('/api/v1/atlas/volumes', {
        method: 'POST',
        body: JSON.stringify({
          name: String(fd.get('name') || '').trim(),
          namespace: String(fd.get('namespace') || 'default').trim(),
          policy: String(fd.get('policy') || 'production'),
          size_gi: Number(fd.get('size_gi') || 10),
          create_pvc: true,
        }),
      });
      toast('Atlas volume requested');
      state.atlasVolumeModal = false;
      await Promise.all([refreshAll(false), loadAtlas()]);
      render();
    } catch (err) { toast(err.message || 'Atlas create failed', 'error'); }
  });
  $$('[data-vm-action]').forEach(btn => btn.onclick = async (e) => {
    e.stopPropagation();
    const action = btn.dataset.vmAction;
    const ns = btn.dataset.vmNs;
    const name = btn.dataset.vmName;
    btn.disabled = true;
    try {
      await api(`/api/v1/vms/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/${action}`, { method: 'POST' });
      toast(`${action === 'start' ? 'Starting' : 'Stopping'} ${name}`);
      await refreshAll(false);
    } catch (err) {
      toast(err.message || `VM ${action} failed`, 'error');
      btn.disabled = false;
    }
  });
  $$('[data-vm-menu]').forEach(btn => btn.onclick = (e) => {
    e.stopPropagation();
    const key = btn.dataset.vmMenu;
    state.vmMenu = state.vmMenu === key ? null : key;
    render();
  });
  $$('[data-vm-more]').forEach(btn => btn.onclick = (e) => {
    e.stopPropagation();
    handleVmMore(btn.dataset.vmMore, btn.dataset.vmNs, btn.dataset.vmName);
  });
  $$('[data-vm-wizard]').forEach(btn => btn.onclick = (e) => {
    e.stopPropagation();
    openVmWizard(btn.dataset.vmWizard, {
      namespace: btn.dataset.vmNs,
      name: btn.dataset.vmName,
      snapshot: btn.dataset.vmSnap,
      targetVM: btn.dataset.vmName ? `${btn.dataset.vmName}-restored` : '',
    });
  });
  document.addEventListener('click', function closeMenus(ev) {
    if (state.vmMenu && !ev.target.closest('.overflow-wrap')) {
      state.vmMenu = null;
      document.removeEventListener('click', closeMenus);
      render();
    }
  }, { once: true });
  $$('.quick-action').forEach(btn => btn.onclick = () => {
    if (btn.dataset.page === 'launch') { state.launch = true; state.placeStep = 1; state.placeDraft = null; render(); return; }
    navigateTo(btn.dataset.page);
  });
  $$('[data-workload-filter]').forEach(btn => btn.onclick = () => { state.workloadFilter = btn.dataset.workloadFilter; if (btn.closest('.posture-strip')) state.page = 'workloads'; render(); });
  $$('[data-pod-filter]').forEach(btn => btn.onclick = () => { state.podFilter = btn.dataset.podFilter; render(); });
  $$('[data-refresh]').forEach(btn => btn.onclick = () => refreshAll(false).then(() => render()));
  $$('[data-refresh-ops]').forEach(btn => btn.onclick = () => {
    const page = btn.dataset.refreshOps || (state.page === 'ops' ? state.opsTab : state.page);
    state.opsLoaded[page] = false;
    ensureOpsPage(page, true).then(() => render());
  });
  $('#onboarding-dismiss')?.addEventListener('click', () => { localStorage.setItem('axiom.onboarding', 'dismissed'); render(); });
  $('#logout')?.addEventListener('click', logout);
  $('#logout-settings')?.addEventListener('click', logout);
  $('#integrations-atlas-form')?.addEventListener('submit', async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const patch = {
      atlasUrl: String(fd.get('atlasUrl') || ''),
      atlasConsoleUrl: String(fd.get('atlasConsoleUrl') || ''),
    };
    if (fd.get('atlasTokenClear')) patch.atlasTokenClear = true;
    const tok = String(fd.get('atlasToken') || '');
    if (tok) patch.atlasToken = tok;
    try {
      state.integrations = await api('/api/v1/integrations', { method: 'PUT', body: JSON.stringify(patch) });
      state.integrationDraft = null;
      toast('Atlas settings saved');
      loadAtlas();
      render();
    } catch (err) {
      toast(err.message || 'Save failed', 'error');
    }
  });
  $('#integrations-atlas-test')?.addEventListener('click', async () => {
    try {
      integrationTestToast(await api('/api/v1/integrations/atlas/test', { method: 'POST', body: '{}' }));
    } catch (err) {
      toast(err.message || 'Atlas test failed', 'error');
    }
  });
  $('#integrations-postgres-form')?.addEventListener('submit', async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const patch = {
      postgresHost: String(fd.get('postgresHost') || ''),
      postgresPort: parseInt(String(fd.get('postgresPort') || '5432'), 10) || 5432,
      postgresDatabase: String(fd.get('postgresDatabase') || ''),
      postgresUser: String(fd.get('postgresUser') || ''),
      postgresSslMode: String(fd.get('postgresSslMode') || 'disable'),
    };
    if (fd.get('postgresPasswordClear')) patch.postgresPasswordClear = true;
    const pw = String(fd.get('postgresPassword') || '');
    if (pw) patch.postgresPassword = pw;
    try {
      state.integrations = await api('/api/v1/integrations', { method: 'PUT', body: JSON.stringify(patch) });
      state.integrationDraft = null;
      toast('Postgres settings saved');
      render();
    } catch (err) {
      toast(err.message || 'Save failed', 'error');
    }
  });
  $('#integrations-postgres-test')?.addEventListener('click', async () => {
    try {
      integrationTestToast(await api('/api/v1/integrations/postgres/test', { method: 'POST', body: '{}' }));
    } catch (err) {
      toast(err.message || 'Postgres test failed', 'error');
    }
  });
  $('#integrations-probe-form')?.addEventListener('submit', async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const patch = {
      probeUrl: String(fd.get('probeUrl') || ''),
      probePodNamespace: String(fd.get('probePodNamespace') || ''),
      probePodName: String(fd.get('probePodName') || ''),
      probePodContainer: String(fd.get('probePodContainer') || ''),
    };
    try {
      state.integrations = await api('/api/v1/integrations', { method: 'PUT', body: JSON.stringify(patch) });
      state.integrationDraft = null;
      toast('Probe settings saved');
      render();
    } catch (err) {
      toast(err.message || 'Save failed', 'error');
    }
  });
  $('#integrations-probe-test')?.addEventListener('click', async () => {
    try {
      integrationTestToast(await api('/api/v1/integrations/probe/test', { method: 'POST', body: '{}' }));
    } catch (err) {
      toast(err.message || 'Probe test failed', 'error');
    }
  });
  $('#integrations-oidc-form')?.addEventListener('submit', async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const patch = {
      oidcIssuer: String(fd.get('oidcIssuer') || ''),
      oidcLoginUrl: String(fd.get('oidcLoginUrl') || ''),
      oidcClientId: String(fd.get('oidcClientId') || ''),
    };
    try {
      state.integrations = await api('/api/v1/integrations', { method: 'PUT', body: JSON.stringify(patch) });
      state.integrationDraft = null;
      toast('OIDC settings saved');
      render();
    } catch (err) {
      toast(err.message || 'Save failed', 'error');
    }
  });
  $('#integrations-oidc-test')?.addEventListener('click', async () => {
    try {
      integrationTestToast(await api('/api/v1/integrations/oidc/test', { method: 'POST', body: '{}' }));
    } catch (err) {
      toast(err.message || 'OIDC test failed', 'error');
    }
  });
  document.querySelectorAll('[data-apply-example]').forEach(btn => {
    btn.addEventListener('click', () => {
      const id = btn.getAttribute('data-apply-example');
      const ex = (state.integrationExamples || []).find(x => x.id === id);
      if (ex) applyIntegrationExample(ex);
    });
  });
  $('#mobile-menu')?.addEventListener('click', () => { state.mobileSidebarOpen = !state.mobileSidebarOpen; render(); });
  $('.sidebar.mobile-open')?.addEventListener('click', e => { if (e.target === e.currentTarget || e.target.matches('.sidebar.mobile-open:after')) return; });
  if (state.mobileSidebarOpen) {
    document.addEventListener('click', function closeMobile(ev) {
      if (ev.target.closest('.sidebar') || ev.target.closest('#mobile-menu')) return;
      state.mobileSidebarOpen = false;
      document.removeEventListener('click', closeMobile);
      render();
    }, { once: true });
  }
  $('#palette-open')?.addEventListener('click', () => { state.palette = true; state.paletteHighlight = 0; render(); });
  $('#nav-launch')?.addEventListener('click', () => { state.launch = true; state.placeStep = 1; state.placeMode = 'form'; state.placeDraft = null; render(); });
  $$('#hero-launch, [data-hero-launch]').forEach(btn => btn.onclick = () => { state.launch = true; state.placeStep = 1; state.placeMode = 'form'; state.placeDraft = null; render(); });
  $('#estate-filter')?.addEventListener('input', e => { state.estateQuery = e.target.value; render(); });
  $$('[data-estate-floor]').forEach(btn => btn.onclick = () => { state.estateFloor = btn.dataset.estateFloor; render(); });
  $$('#create-namespace').forEach(btn => btn.onclick = () => { state.namespaceModal = true; render(); });
  $$('[data-network-tab]').forEach(btn => btn.onclick = () => { state.networkTab = btn.dataset.networkTab; render(); });
  $('#network-ns')?.addEventListener('change', async e => {
    state.networkNs = e.target.value;
    try {
      state.network = await api(`/api/v1/network?namespace=${encodeURIComponent(state.networkNs)}`);
      if (state.networkTab === 'services' && state.networkNs !== 'all') {
        state.services = await api(`/api/v1/services?namespace=${encodeURIComponent(state.networkNs)}`);
      } else if (state.networkNs === 'all') {
        state.services = await api('/api/v1/services?namespace=all');
      }
    } catch (error) { toast(error.message, 'error'); }
    render();
  });
  $$('[data-delete-namespace]').forEach(btn => btn.onclick = () => {
    const name = btn.dataset.deleteNamespace;
    requestConfirm({ title: 'Delete namespace', name, confirmLabel: 'Delete', onConfirm: async () => {
      await mutate(`/api/v1/namespaces/${encodeURIComponent(name)}`, { method: 'DELETE' }, `Namespace ${name} deleted`);
    }});
  });
  $('#pw-load-flows')?.addEventListener('click', () => loadPacketwolfFlows());

  $$('[data-mobile-nav]').forEach(btn => btn.onclick = () => {
    const id = btn.dataset.mobileNav;
    if (id === 'more') { state.palette = true; state.paletteHighlight = 0; render(); return; }
    navigateTo(id);
  });

  $$('[data-workload], [data-workload-id]').forEach(el => el.onclick = () => {
    openDrawer(state.workloads[Number(el.dataset.workload ?? el.dataset.workloadId)], 'workload');
  });
  $$('[data-pod-id]').forEach(el => el.onclick = () => openDrawer(state.pods[Number(el.dataset.podId)], 'pod'));
  $$('[data-node-id]').forEach(el => el.onclick = () => openDrawer(state.nodes[Number(el.dataset.nodeId)], 'node'));
  $$('[data-vm-id]').forEach(el => el.onclick = () => openDrawer(state.vms[Number(el.dataset.vmId)], 'vm'));

  wireOpsPages();

  const filter = $('#filter');
  if (filter) filter.oninput = () => {
    const q = filter.value.toLowerCase();
    if (state.page === 'workloads') {
      const items = filterWorkloads(state.workloads.filter(w => JSON.stringify(w).toLowerCase().includes(q)));
      $('#workload-body').innerHTML = workloadRows(items);
    }
    if (state.page === 'pods') {
      const items = filterPods(state.pods.filter(p => JSON.stringify(p).toLowerCase().includes(q)));
      $('#pod-body').innerHTML = podRows(items);
    }
    if (state.page === 'vms') {
      const items = filterVMs(state.vms.filter(v => JSON.stringify(v).toLowerCase().includes(q)));
      const body = $('#vm-body');
      if (body) body.innerHTML = vmRows(items);
      wireOpsPages();
      $$('[data-vm-id]').forEach(el => el.onclick = () => openDrawer(state.vms[Number(el.dataset.vmId)], 'vm'));
      $$('[data-vm-menu]').forEach(btn => btn.onclick = (e) => {
        e.stopPropagation();
        const key = btn.dataset.vmMenu;
        state.vmMenu = state.vmMenu === key ? null : key;
        render();
      });
      $$('[data-vm-more]').forEach(btn => btn.onclick = (e) => {
        e.stopPropagation();
        handleVmMore(btn.dataset.vmMore, btn.dataset.vmNs, btn.dataset.vmName);
      });
      $$('[data-vm-action]').forEach(btn => btn.onclick = async (e) => {
        e.stopPropagation();
        const action = btn.dataset.vmAction;
        const ns = btn.dataset.vmNs;
        const name = btn.dataset.vmName;
        try {
          await api(`/api/v1/vms/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/${action}`, { method: 'POST' });
          toast(`${action === 'start' ? 'Starting' : 'Stopping'} ${name}`);
          await refreshAll(false);
        } catch (err) { toast(err.message || `VM ${action} failed`, 'error'); }
      });
    }
    if (state.page === 'volumes') {
      const items = filterVolumes(state.volumes.filter(v => JSON.stringify(v).toLowerCase().includes(q)));
      const body = $('#volume-body');
      if (body) body.innerHTML = volumeRows(items);
    }
    wireRowsOnly();
  };

  if (state.selected) wireDrawer();
  if (state.launch) wirePlace();
  if (state.namespaceModal) wireNamespaceModal();
  if (state.secretModal) wireSecretModal();
  if (state.agentRunModal || state.agentRunDetail) wireOpsPages();
  if (state.vmWizard) wireVmWizard();
  if (state.vmConsole) wireVmConsoleModal();
  if (state.palette) wirePalette();
  if (state.confirm) wireConfirm();
  if (state.helmRollback) wireHelmRollback();
  if (state.still) wireStill();
}

async function handleVmMore(action, ns, name) {
  state.vmMenu = null;
  if (action === 'snapshot' || action === 'clone' || action === 'export') {
    openVmWizard(action, { namespace: ns, name });
    return;
  }
  if (action === 'console') {
    try {
      state.vmConsole = await api(`/api/v1/vms/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/console`);
      render();
    } catch (err) { toast(err.message || 'Console unavailable', 'error'); }
    return;
  }
  if (action === 'migrate') {
    try {
      const res = await api(`/api/v1/vms/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/migrate`, { method: 'POST' });
      trackJob({ kind: 'migrate', label: `Migrate ${name}`, namespace: ns, name, jobName: res.jobName, phase: 'Pending' });
      toast(res.message || `Migration requested for ${name}`);
    } catch (err) { toast(err.message || 'Migrate failed', 'error'); }
    return;
  }
  render();
}

function wireVmWizard() {
  $('#vm-wizard-overlay')?.addEventListener('click', () => { state.vmWizard = null; render(); });
  $('#vm-wizard-cancel')?.addEventListener('click', () => { state.vmWizard = null; render(); });
  $('#vm-wizard-form')?.addEventListener('submit', async (event) => {
    event.preventDefault();
    const fd = new FormData(event.currentTarget);
    const type = state.vmWizard?.type;
    try {
      if (type === 'snapshot') {
        const res = await api('/api/v1/vm-snapshots', {
          method: 'POST',
          body: JSON.stringify({ namespace: fd.get('namespace'), vm: fd.get('vm'), name: fd.get('name') || undefined }),
        });
        trackJob({ kind: 'snapshot', label: `Snapshot ${fd.get('vm')}`, namespace: fd.get('namespace'), name: res.name || fd.get('name'), phase: 'Pending' });
        toast(`Snapshot requested`);
      } else if (type === 'clone') {
        const res = await api('/api/v1/vm-clones', {
          method: 'POST',
          body: JSON.stringify({ namespace: fd.get('namespace'), sourceVM: fd.get('sourceVM'), targetVM: fd.get('targetVM') }),
        });
        trackJob({ kind: 'clone', label: `Clone → ${fd.get('targetVM')}`, namespace: fd.get('namespace'), name: res.name, phase: res.phase || 'InProgress' });
        toast(`Clone ${fd.get('targetVM')} requested`);
      } else if (type === 'restore') {
        const res = await api('/api/v1/vm-restores', {
          method: 'POST',
          body: JSON.stringify({ namespace: fd.get('namespace'), snapshot: fd.get('snapshot'), targetVM: fd.get('targetVM') }),
        });
        trackJob({ kind: 'restore', label: `Restore → ${fd.get('targetVM')}`, namespace: fd.get('namespace'), name: res.name, phase: 'InProgress' });
        toast(`Restore to ${fd.get('targetVM')} requested`);
      } else if (type === 'export') {
        try {
          const res = await api('/api/v1/vm-exports', {
            method: 'POST',
            body: JSON.stringify({ namespace: fd.get('namespace'), vm: fd.get('vm') }),
          });
          trackJob({ kind: 'export', label: `Export ${fd.get('vm')}`, namespace: fd.get('namespace'), name: res.name, phase: 'Pending' });
          toast(`Export requested`);
        } catch (err) {
          const msg = err.message || '';
          if (/feature gate/i.test(msg)) {
            state.vmExportSupported = false;
            localStorage.setItem('axiom.vmExport', '0');
            toast('VM Export is disabled on this cluster (feature gate)', 'error');
          } else throw err;
        }
      }
      state.vmWizard = null;
      await loadVMExtras();
      render();
    } catch (err) { toast(err.message, 'error'); }
  });
}

function parseKeyValueLines(raw) {
  const keys = {};
  String(raw || '').split('\n').forEach(line => {
    const trimmed = line.trim();
    if (!trimmed) return;
    const eq = trimmed.indexOf('=');
    if (eq <= 0) return;
    keys[trimmed.slice(0, eq).trim()] = trimmed.slice(eq + 1);
  });
  return keys;
}

async function approveAgentRun(ns, name) {
  try {
    await api(`/api/v1/agent-runs/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/approve`, { method: 'POST' });
    toast(`Approved ${name}`);
    state.agentRunDetail = null;
    state.opsLoaded.agents = false;
    await ensureOpsPage('agents', true);
    render();
  } catch (err) { toast(err.message || 'Approve failed', 'error'); }
}

async function denyAgentRun(ns, name) {
  try {
    await api(`/api/v1/agent-runs/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/deny`, { method: 'POST' });
    toast(`Denied ${name}`);
    state.agentRunDetail = null;
    state.opsLoaded.agents = false;
    await ensureOpsPage('agents', true);
    render();
  } catch (err) { toast(err.message || 'Deny failed', 'error'); }
}

function wireOpsPages() {
  $$('#create-secret').forEach(btn => btn.onclick = () => { state.secretModal = true; render(); });
  $$('#create-agent-run').forEach(btn => btn.onclick = () => { state.agentRunModal = true; render(); });
  $$('[data-agent-detail]').forEach(btn => btn.onclick = async (e) => {
    e.stopPropagation();
    try {
      state.agentRunDetail = await api(`/api/v1/agent-runs/${encodeURIComponent(btn.dataset.agentNs)}/${encodeURIComponent(btn.dataset.agentName)}`);
      render();
    } catch (err) { toast(err.message || 'Failed to load agent run', 'error'); }
  });
  $$('[data-agent-approve]').forEach(btn => btn.onclick = (e) => {
    e.stopPropagation();
    approveAgentRun(btn.dataset.agentNs, btn.dataset.agentName);
  });
  $$('[data-agent-deny]').forEach(btn => btn.onclick = (e) => {
    e.stopPropagation();
    denyAgentRun(btn.dataset.agentNs, btn.dataset.agentName);
  });
  $('#agent-run-overlay')?.addEventListener('click', () => { state.agentRunModal = false; render(); });
  $('#agent-run-cancel')?.addEventListener('click', () => { state.agentRunModal = false; render(); });
  $('#agent-run-form')?.addEventListener('submit', async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const targetKind = String(fd.get('targetKind') || '').trim();
    const targetName = String(fd.get('targetName') || '').trim();
    const body = {
      name: String(fd.get('name') || '').trim(),
      namespace: String(fd.get('namespace') || 'default').trim(),
      goal: String(fd.get('goal') || '').trim(),
      dryRun: false,
    };
    if (targetKind && targetName) {
      body.target = {
        kind: targetKind,
        name: targetName,
        namespace: String(fd.get('targetNamespace') || body.namespace).trim(),
        apiVersion: targetKind === 'VirtualMachine' ? 'kubevirt.io/v1' : 'v1',
      };
    }
    try {
      await api('/api/v1/agent-runs', { method: 'POST', body: JSON.stringify(body) });
      toast('Agent run created');
      state.agentRunModal = false;
      state.opsLoaded.agents = false;
      await ensureOpsPage('agents', true);
      render();
    } catch (err) { toast(err.message || 'Create failed', 'error'); }
  });
  $('#agent-detail-overlay')?.addEventListener('click', () => { state.agentRunDetail = null; render(); });
  $('#agent-detail-close')?.addEventListener('click', () => { state.agentRunDetail = null; render(); });
  $('#agent-detail-approve')?.addEventListener('click', () => {
    const btn = $('#agent-detail-approve');
    approveAgentRun(btn.dataset.agentNs, btn.dataset.agentName);
  });
  $('#agent-detail-deny')?.addEventListener('click', () => {
    const btn = $('#agent-detail-deny');
    denyAgentRun(btn.dataset.agentNs, btn.dataset.agentName);
  });
  $$('[data-secret-expand]').forEach(btn => btn.onclick = async (e) => {
    e.stopPropagation();
    const key = btn.dataset.secretExpand;
    const ns = btn.dataset.secretNs;
    const name = btn.dataset.secretName;
    if (state.secretExpand === key) {
      state.secretExpand = null;
      state.secretDetail = null;
      render();
      return;
    }
    try {
      state.secretDetail = await api(`/api/v1/secrets/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`);
      state.secretExpand = key;
      render();
    } catch (err) {
      const local = state.secrets.find(s => s.namespace === ns && s.name === name);
      state.secretDetail = local || { keys: [] };
      state.secretExpand = key;
      render();
      if (!local?.keys?.length) toast(err.message || 'Could not load secret keys', 'error');
    }
  });
  $$('[data-delete-secret]').forEach(btn => btn.onclick = (e) => {
    e.stopPropagation();
    const ns = btn.dataset.secretNs;
    const name = btn.dataset.secretName;
    requestConfirm({
      title: 'Delete secret',
      name,
      confirmLabel: 'Delete',
      onConfirm: async () => {
        await api(`/api/v1/secrets/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`, { method: 'DELETE' });
        toast(`Secret ${name} deleted`);
        state.opsLoaded.secrets = false;
        await ensureOpsPage('secrets', true);
        render();
      },
    });
  });
  $('#backup-create-form')?.addEventListener('submit', async (event) => {
    event.preventDefault();
    const fd = new FormData(event.currentTarget);
    try {
      await api('/api/v1/backups', {
        method: 'POST',
        body: JSON.stringify({
          namespace: fd.get('namespace'),
          pvc: fd.get('pvc'),
          name: fd.get('name'),
        }),
      });
      toast(`Snapshot ${fd.get('name')} requested`);
      state.opsLoaded.backups = false;
      await ensureOpsPage('backups', true);
      render();
    } catch (err) { toast(err.message, 'error'); }
  });
  $('#backup-restore-form')?.addEventListener('submit', async (event) => {
    event.preventDefault();
    const fd = new FormData(event.currentTarget);
    try {
      await api('/api/v1/backups/restore', {
        method: 'POST',
        body: JSON.stringify({
          namespace: fd.get('namespace'),
          snapshot: fd.get('snapshot'),
          pvc: fd.get('pvc'),
          name: fd.get('pvc'),
        }),
      });
      toast(`Restore to ${fd.get('pvc')} requested`);
      state.opsLoaded.backups = false;
      await ensureOpsPage('backups', true);
      render();
    } catch (err) { toast(err.message, 'error'); }
  });
  $$('[data-rbac-tab]').forEach(btn => btn.onclick = () => { state.rbacTab = btn.dataset.rbacTab; render(); });
  $$('[data-helm-uninstall]').forEach(btn => btn.onclick = (e) => {
    e.stopPropagation();
    const ns = btn.dataset.helmNs;
    const name = btn.dataset.helmName;
    requestConfirm({
      title: 'Uninstall Helm release',
      name: `${ns}/${name}`,
      confirmLabel: 'Uninstall',
      onConfirm: async () => {
        const res = await api(`/api/v1/helm/releases/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/uninstall`, { method: 'POST' });
        trackJob({ kind: 'helm', label: `Uninstall ${name}`, namespace: ns, name, jobName: res.jobName, phase: 'Pending' });
        toast(res.message || `Uninstall started for ${name}`);
        state.opsLoaded.helm = false;
        await ensureOpsPage('helm', true);
        render();
      },
    });
  });
  $$('[data-helm-rollback]').forEach(btn => btn.onclick = async (e) => {
    e.stopPropagation();
    const ns = btn.dataset.helmNs;
    const name = btn.dataset.helmName;
    try {
      const history = await api(`/api/v1/helm/releases/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/history`);
      const revisions = (Array.isArray(history) ? history : []).filter(r => r.revision);
      if (!revisions.length) {
        toast('No rollback revisions available', 'error');
        return;
      }
      state.helmRollback = { namespace: ns, name, revisions };
      render();
    } catch (error) {
      toast(error.message || 'Failed to load Helm history', 'error');
    }
  });
  $$('[data-refresh-vmsnaps]').forEach(btn => btn.onclick = () => loadVMExtras().then(() => render()));
}

function wireSecretModal() {
  $('#secret-overlay')?.addEventListener('click', () => { state.secretModal = false; render(); });
  $('#secret-cancel')?.addEventListener('click', () => { state.secretModal = false; render(); });
  $('#secret-form')?.addEventListener('submit', async (event) => {
    event.preventDefault();
    const fd = new FormData(event.currentTarget);
    const keys = parseKeyValueLines(fd.get('data'));
    if (!Object.keys(keys).length) {
      toast('Add at least one KEY=value line', 'error');
      return;
    }
    try {
      await api('/api/v1/secrets', {
        method: 'POST',
        body: JSON.stringify({
          name: fd.get('name'),
          namespace: fd.get('namespace') || 'default',
          keys,
          data: keys,
        }),
      });
      state.secretModal = false;
      toast(`Secret ${fd.get('name')} created`);
      state.opsLoaded.secrets = false;
      await ensureOpsPage('secrets', true);
      render();
    } catch (err) { toast(err.message, 'error'); }
  });
}

function wireVmConsoleModal() {
  $('#vm-console-overlay')?.addEventListener('click', () => { state.vmConsole = null; render(); });
  $('#vm-console-close')?.addEventListener('click', () => { state.vmConsole = null; render(); });
  $('#vm-console-copy')?.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(state.vmConsole?.virtctl || '');
      toast('virtctl command copied');
    } catch { toast('Copy failed', 'error'); }
  });
}

function wireRowsOnly() {
  $$('[data-workload-id]').forEach(el => el.onclick = () => openDrawer(state.workloads[Number(el.dataset.workloadId)], 'workload'));
  $$('[data-pod-id]').forEach(el => el.onclick = () => openDrawer(state.pods[Number(el.dataset.podId)], 'pod'));
}

function closeDrawer() {
  cleanupPanels();
  state.selected = null;
  state.selectedType = null;
  state.drawerTab = 'overview';
  render();
}

function wireDrawer() {
  $('#drawer-close')?.addEventListener('click', closeDrawer);
  $('#drawer-overlay')?.addEventListener('click', closeDrawer);
  $$('[data-drawer-tab]').forEach(btn => btn.onclick = () => {
    cleanupPanels();
    state.drawerTab = btn.dataset.drawerTab;
    if (btn.dataset.drawerTab === 'terminal') state.presenceKnocked = false;
    if (state.selectedType === 'node') state.nodeDrawerTab = btn.dataset.drawerTab;
    render();
    if (state.selectedType === 'workload') loadDrawerExtras();
  });
  $$('[data-fix-action]').forEach(btn => btn.onclick = () => {
    const action = btn.dataset.fixAction;
    if (action === 'logs') { state.drawerTab = 'logs'; render(); return; }
    if (action === 'events') { state.drawerTab = 'events'; render(); return; }
    if (action === 'nodes') { closeDrawer(); state.page = 'nodes'; render(); return; }
    if (action === 'restart') { doAction('restart'); }
  });
  $$('[data-rollout]').forEach(btn => btn.onclick = async () => {
    const w = state.selected;
    if (!w) return;
    try {
      await mutate(`/api/v1/workloads/${encodeURIComponent(w.namespace)}/${encodeURIComponent(w.kind)}/${encodeURIComponent(w.name)}/rollout/${btn.dataset.rollout}`, { method: 'POST' }, `Rollout ${btn.dataset.rollout} requested`);
      loadDrawerExtras();
    } catch (error) { toast(error.message, 'error'); }
  });
  $('#manifest-copy')?.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(state.resourceDetail?.manifest || {}, null, 2));
      toast('Manifest copied');
    } catch { toast('Copy failed', 'error'); }
  });
  $$('[data-still]').forEach(btn => btn.onclick = () => openStillRing(btn.dataset.still));
  $$('[data-action]').forEach(btn => btn.onclick = () => doAction(btn.dataset.action));
  $('#follow-wires')?.addEventListener('click', () => followWires(state.selected?.namespace));
  $('#open-window')?.addEventListener('click', () => { if (state.selectedType === 'pod') openWindow(state.selected); });
  $('#place-sibling')?.addEventListener('click', () => { if (state.selectedType === 'pod') placeSibling(state.selected); });
  $('#scale-down')?.addEventListener('click', () => { state.scaleDraft = Math.max(0, (state.scaleDraft ?? 0) - 1); render(); });
  $('#scale-up')?.addEventListener('click', () => { state.scaleDraft = (state.scaleDraft ?? 0) + 1; render(); });
  $('#scale-input')?.addEventListener('input', e => { state.scaleDraft = Number(e.target.value); });
  $('#scale-apply')?.addEventListener('click', async () => {
    const n = Number(state.scaleDraft);
    if (Number.isFinite(n) && n >= 0) await workloadAction('scale', { replicas: n });
  });
  $$('[data-pod-name]').forEach(el => el.onclick = () => {
    const p = state.pods.find(x => x.namespace === (el.dataset.podNs || state.selected?.namespace) && x.name === el.dataset.podName);
    if (p) openDrawer(p, 'pod');
  });
  if (state.drawerTab === 'logs') wireLogPanel();
  if (state.drawerTab === 'terminal' && state.selectedType === 'pod') wireTerminalPanel();
}

function requestConfirm({ title, name, confirmLabel, onConfirm }) {
  state.confirm = { title, name, confirmLabel, onConfirm };
  render();
}

function wireHelmRollback() {
  const close = () => { state.helmRollback = null; render(); };
  $('#helm-rollback-cancel')?.addEventListener('click', close);
  $('#helm-rollback-overlay')?.addEventListener('click', close);
  $('#helm-rollback-go')?.addEventListener('click', async () => {
    const h = state.helmRollback;
    if (!h) return;
    const revision = Number($('#helm-rollback-rev')?.value) || 0;
    state.helmRollback = null;
    try {
      const res = await api(`/api/v1/helm/releases/${encodeURIComponent(h.namespace)}/${encodeURIComponent(h.name)}/rollback`, {
        method: 'POST',
        body: JSON.stringify({ revision }),
      });
      trackJob({ kind: 'helm', label: `Rollback ${h.name}`, namespace: h.namespace, name: h.name, jobName: res.jobName, phase: 'Pending' });
      toast(res.message || `Rollback started for ${h.name}`);
      state.opsLoaded.helm = false;
      await ensureOpsPage('helm', true);
      render();
    } catch (error) {
      toast(error.message || 'Rollback failed', 'error');
      render();
    }
  });
}

function wireConfirm() {
  const input = $('#confirm-input');
  const go = $('#confirm-go');
  const sync = () => { go.disabled = input.value !== state.confirm.name; };
  input?.addEventListener('input', sync);
  sync();
  $('#confirm-cancel')?.addEventListener('click', () => { state.confirm = null; render(); });
  $('#confirm-overlay')?.addEventListener('click', () => { state.confirm = null; render(); });
  go?.addEventListener('click', async () => {
    if (input.value !== state.confirm.name) return;
    const fn = state.confirm.onConfirm;
    state.confirm = null;
    await fn();
  });
}

async function workloadAction(action, body) {
  const w = state.selected;
  await mutate(`/api/v1/workloads/${encodeURIComponent(w.namespace)}/${encodeURIComponent(w.kind)}/${encodeURIComponent(w.name)}/${action}`, { method: 'POST', body: body ? JSON.stringify(body) : undefined }, `${action} requested`);
}

async function doAction(action) {
  try {
    if (['start', 'stop', 'restart', 'scale'].includes(action)) return workloadAction(action);
    if (action === 'unseal') {
      const w = state.selected;
      return mutate(`/api/v1/workloads/${encodeURIComponent(w.namespace)}/${encodeURIComponent(w.kind)}/${encodeURIComponent(w.name)}/unseal`, { method: 'POST' }, 'Isolation removed');
    }
    if (action === 'delete-workload') {
      const w = state.selected;
      return requestConfirm({ title: 'Delete workload', name: w.name, confirmLabel: 'Delete', onConfirm: async () => {
        await mutate(`/api/v1/workloads/${encodeURIComponent(w.namespace)}/${encodeURIComponent(w.kind)}/${encodeURIComponent(w.name)}`, { method: 'DELETE' }, 'Workload deleted', true);
      }});
    }
    if (action === 'delete-pod') {
      const p = state.selected;
      return requestConfirm({ title: 'Terminate pod', name: p.name, confirmLabel: 'Terminate', onConfirm: async () => {
        await mutate(`/api/v1/pods/${encodeURIComponent(p.namespace)}/${encodeURIComponent(p.name)}`, { method: 'DELETE' }, 'Pod terminated', true);
      }});
    }
    if (action === 'evict-pod') {
      const p = state.selected;
      return requestConfirm({ title: 'Evict pod', name: p.name, confirmLabel: 'Evict', onConfirm: async () => {
        await mutate(`/api/v1/pods/${encodeURIComponent(p.namespace)}/${encodeURIComponent(p.name)}/evict`, { method: 'POST' }, 'Eviction requested', true);
      }});
    }
    if (action === 'drain') {
      const n = state.selected;
      return requestConfirm({ title: 'Drain node', name: n.name, confirmLabel: 'Drain', onConfirm: async () => {
        await mutate(`/api/v1/nodes/${encodeURIComponent(n.name)}/drain`, { method: 'POST' }, 'Drain requested');
      }});
    }
    if (['cordon', 'uncordon'].includes(action)) return mutate(`/api/v1/nodes/${encodeURIComponent(state.selected.name)}/${action}`, { method: 'POST' }, `${action} requested`);
  } catch (error) { toast(error.message, 'error'); }
}

async function mutate(url, opts, message, close = false) {
  try {
    await api(url, opts);
    toast(message);
    if (close) { state.selected = null; state.selectedType = null; state.drawerTab = 'overview'; }
    await refreshAll(false);
    render();
  } catch (error) { toast(error.message, 'error'); }
}

function logPanelConfig() {
  const panel = $('.log-panel');
  if (!panel) return null;
  return {
    ns: panel.dataset.logNs,
    pod: panel.dataset.logPod,
    tail: Number($('#log-tail')?.value || 200),
    container: $('#log-container')?.value || '',
    follow: $('#log-follow')?.checked ?? true,
    wrap: $('#log-wrap')?.checked ?? false,
    previous: $('#log-previous')?.checked ?? false,
    filter: ($('#log-filter')?.value || '').toLowerCase(),
  };
}

function applyLogDisplay(text, cfg) {
  const view = $('#log-view');
  if (!view) return;
  view.classList.toggle('wrap', cfg.wrap);
  const lines = text.split('\n');
  const filtered = cfg.filter ? lines.filter(l => l.toLowerCase().includes(cfg.filter)) : lines;
  const colored = filtered.map(line => {
    const low = line.toLowerCase();
    if (/\berror\b|fatal|panic|exception/.test(low)) return `<span class="log-error">${enc(line)}</span>`;
    if (/\bwarn(ing)?\b/.test(low)) return `<span class="log-warn">${enc(line)}</span>`;
    if (/\binfo\b/.test(low)) return `<span class="log-info">${enc(line)}</span>`;
    return enc(line);
  });
  view.innerHTML = colored.join('\n') || '(no log output)';
  if (cfg.follow) view.scrollTop = view.scrollHeight;
  const meta = $('#log-meta');
  if (meta) meta.textContent = cfg.follow ? 'Following…' : 'Paused · ' + new Date().toLocaleTimeString();
}

async function fetchLogsOnce(cfg) {
  const params = new URLSearchParams({ tail: String(cfg.tail) });
  if (cfg.container) params.set('container', cfg.container);
  if (cfg.previous) params.set('previous', 'true');
  return api(`/api/v1/pods/${encodeURIComponent(cfg.ns)}/${encodeURIComponent(cfg.pod)}/logs?${params}`);
}

function wireLogPanel() {
  cleanupPanels();
  const cfg = logPanelConfig();
  if (!cfg) return;
  let rawText = '';
  let followPaused = false;

  const refresh = async () => {
    try {
      rawText = await fetchLogsOnce(cfg);
      applyLogDisplay(rawText, logPanelConfig());
    } catch (error) {
      $('#log-view').textContent = error.message.includes('not found') ? 'No pods found — they may have been cleaned up.' : error.message;
    }
  };

  const startFollow = async () => {
    logPanelAbort?.abort();
    logPanelAbort = new AbortController();
    const live = logPanelConfig();
    const params = new URLSearchParams({ tail: String(live.tail), follow: 'true' });
    if (live.container) params.set('container', live.container);
    if (live.previous) params.set('previous', 'true');
    try {
      const res = await fetch(`/api/v1/pods/${encodeURIComponent(live.ns)}/${encodeURIComponent(live.pod)}/logs?${params}`, {
        headers: { Authorization: authHeader() },
        signal: logPanelAbort.signal,
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      rawText = '';
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        rawText += decoder.decode(value, { stream: true });
        if (!followPaused) applyLogDisplay(rawText, logPanelConfig());
      }
    } catch (error) {
      if (error.name !== 'AbortError') $('#log-view').textContent = error.message;
    }
  };

  const syncMode = () => {
    const live = logPanelConfig();
    if (logPollTimer) clearInterval(logPollTimer);
    logPollTimer = null;
    logPanelAbort?.abort();
    if (live.follow) startFollow();
    else {
      refresh();
      logPollTimer = setInterval(refresh, 3000);
    }
  };

  $('#log-view')?.addEventListener('scroll', () => {
    const view = $('#log-view');
    followPaused = view.scrollTop + view.clientHeight < view.scrollHeight - 50;
  });

  ['log-tail', 'log-container', 'log-follow', 'log-wrap', 'log-previous', 'log-filter'].forEach(id => {
    $(`#${id}`)?.addEventListener('change', () => { followPaused = false; syncMode(); });
    $(`#${id}`)?.addEventListener('input', () => applyLogDisplay(rawText, logPanelConfig()));
  });
  $('#log-refresh')?.addEventListener('click', syncMode);
  $('#log-copy')?.addEventListener('click', async () => {
    try { await navigator.clipboard.writeText($('#log-view')?.textContent || ''); toast('Logs copied'); } catch { toast('Copy failed', 'error'); }
  });
  syncMode();
}

function wireTerminalPanel() {
  cleanupPanels();
  const pod = state.selected;
  $('#presence-knock')?.addEventListener('click', () => { state.presenceKnocked = true; render(); });
  if (!state.presenceKnocked) return;

  let presenceSeconds = 40 * 60;
  const timerEl = $('#presence-timer');
  const timerTick = setInterval(() => {
    presenceSeconds = Math.max(0, presenceSeconds - 1);
    if (timerEl) timerEl.textContent = `${String(Math.floor(presenceSeconds / 60)).padStart(2, '0')}:${String(presenceSeconds % 60).padStart(2, '0')}`;
  }, 1000);
  const resetTimer = () => { presenceSeconds = 40 * 60; };
  document.addEventListener('keydown', resetTimer, { once: false });

  const runQuick = async cmd => {
    const output = $('#term-screen');
    if (!output) return;
    output.textContent += `$ ${cmd}\n`;
    try {
      const result = await api(`/api/v1/pods/${encodeURIComponent(pod.namespace)}/${encodeURIComponent(pod.name)}/exec`, { method: 'POST', body: JSON.stringify({ container: (pod.containers || [])[0] || '', command: cmd }) });
      output.textContent += `${result.output}\n\n`;
    } catch (error) { output.textContent += `error: ${error.message}\n\n`; }
    output.scrollTop = output.scrollHeight;
    resetTimer();
  };

  $$('[data-quick-cmd]').forEach(btn => btn.onclick = () => runQuick(btn.dataset.quickCmd));
  $('#presence-netstat')?.addEventListener('click', () => {
    const ports = (pod.ports || []).map(p => `${p.containerPort}/${p.protocol} (${p.container})`).join('\n') || 'No ports declared in pod spec';
    const output = $('#term-screen');
    if (output) {
      output.textContent += `$ netstat (from spec)\n${ports}\n\n`;
      output.scrollTop = output.scrollHeight;
    }
    resetTimer();
  });

  const interactive = state.realtimeCapabilities?.interactiveExec;
  if (!interactive) {
    const input = $('#terminal-input');
    input?.focus();
    input?.addEventListener('keydown', async event => {
      if (event.key !== 'Enter') return;
      const cmd = input.value.trim();
      if (!cmd) return;
      input.value = '';
      resetTimer();
      await runQuick(cmd);
    });
    return;
  }

  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  const container = (pod.containers || [])[0] || '';
  const qs = streamAuthQuery();
  const url = `${proto}://${location.host}/api/v1/pods/${encodeURIComponent(pod.namespace)}/${encodeURIComponent(pod.name)}/terminal?container=${encodeURIComponent(container)}&${qs}`;
  const screen = $('#term-screen');
  const status = $('#term-status');
  let buffer = '';

  const sendResize = () => {
    if (!terminalSocket || terminalSocket.readyState !== WebSocket.OPEN) return;
    const cols = Math.max(20, Math.floor((screen?.clientWidth || 400) / 8));
    const rows = Math.max(8, Math.floor((screen?.clientHeight || 200) / 16));
    terminalSocket.send(`__resize__:${cols}x${rows}`);
  };

  terminalSocket = new WebSocket(url);
  terminalSocket.binaryType = 'arraybuffer';
  terminalSocket.onopen = () => { status.textContent = 'Present'; sendResize(); screen.focus(); resetTimer(); };
  terminalSocket.onclose = () => { status.textContent = 'Disconnected'; clearInterval(timerTick); };
  terminalSocket.onerror = () => { status.textContent = 'Unavailable'; };
  terminalSocket.onmessage = event => {
    const chunk = typeof event.data === 'string' ? event.data : new TextDecoder().decode(event.data);
    buffer += chunk;
    screen.textContent = buffer;
    screen.scrollTop = screen.scrollHeight;
    resetTimer();
  };

  screen.tabIndex = 0;
  screen.addEventListener('keydown', event => {
    if (!terminalSocket || terminalSocket.readyState !== WebSocket.OPEN) return;
    if (event.key.length === 1) terminalSocket.send(event.key);
    else if (event.key === 'Enter') terminalSocket.send('\r');
    else if (event.key === 'Backspace') terminalSocket.send('\u007f');
    else if (event.key === 'Tab') { event.preventDefault(); terminalSocket.send('\t'); }
    event.preventDefault();
    resetTimer();
  });
  window.addEventListener('resize', sendResize);
}

function wireNamespaceModal() {
  $('#namespace-overlay')?.addEventListener('click', () => { state.namespaceModal = false; render(); });
  $('#namespace-cancel')?.addEventListener('click', () => { state.namespaceModal = false; render(); });
  $('#namespace-form')?.addEventListener('submit', async event => {
    event.preventDefault();
    const name = new FormData(event.currentTarget).get('name');
    try {
      await api('/api/v1/namespaces', { method: 'POST', body: JSON.stringify({ name }) });
      state.namespaceModal = false;
      toast(`Namespace ${name} created`);
      await refreshAll(false);
      state.page = 'network';
      render();
    } catch (error) { toast(error.message, 'error'); }
  });
}

function readPlaceForm() {
  return {
    name: $('#form-name')?.value?.trim() || state.placeForm.name,
    namespace: $('#form-namespace')?.value?.trim() || state.placeForm.namespace || 'default',
    image: $('#form-image')?.value?.trim() || state.placeForm.image,
    replicas: Number($('#form-replicas')?.value || state.placeForm.replicas || 1),
    memory: $('#form-memory')?.value?.trim() || state.placeForm.memory || '256Mi',
    port: Number($('#form-port')?.value || state.placeForm.port || 80),
  };
}

function parseSimpleWorkloadYaml(raw) {
  const out = { ...state.placeForm };
  String(raw || '').split(/\n/).forEach(line => {
    const m = line.match(/^\s*([A-Za-z0-9_.-]+)\s*[:=]\s*(.+?)\s*$/);
    if (!m) return;
    const key = m[1].toLowerCase();
    const val = m[2].replace(/^["']|["']$/g, '');
    if (key === 'name' || key === 'metadata.name') out.name = val;
    else if (key === 'namespace' || key === 'metadata.namespace') out.namespace = val;
    else if (key === 'image' || key === 'spec.image') out.image = val;
    else if (key === 'replicas' || key === 'spec.replicas') out.replicas = Number(val) || 1;
    else if (key === 'memory' || key === 'spec.resources.memory') out.memory = val;
    else if (key === 'port') out.port = Number(val) || 80;
  });
  return out;
}

async function deployLaunchRequest(req) {
  const workload = await api('/api/v1/workloads', {
    method: 'POST',
    body: JSON.stringify({
      namespace: req.namespace || 'default',
      name: req.name,
      image: req.image || 'nginx:1.27',
      replicas: Number(req.replicas) || 1,
      port: Number(req.port) || 80,
      cpu: req.cpu || '250m',
      memory: req.memory || '256Mi',
      expose: true,
      environment: {},
    }),
  });
  state.launch = false;
  state.placeStep = 1;
  state.placeMode = 'intent';
  toast(`${workload.name} running`);
  await refreshAll(false);
  state.page = 'workloads';
  openDrawer(workload, 'workload');
}

function wirePlace() {
  let parseTimer = null;
  function schedulePlacePreview() {
    clearTimeout(parseTimer);
    parseTimer = setTimeout(async () => {
      const text = $('#place-intent')?.value || state.placeIntent;
      if (!text.trim() || state.placeStep !== 1 || state.placeMode !== 'intent') return;
      try {
        const res = await api('/api/v1/intent/parse', { method: 'POST', body: JSON.stringify({ intent: text }) });
        state.placePreview = res.draft;
        const el = $('.place-parse-preview');
        if (el) el.textContent = `Parsed ${res.draft.name} ×${res.draft.replicas} · ${res.draft.image} · ${res.draft.memory} · ${res.draft.namespace}`;
        else render();
      } catch {}
    }, 350);
  }
  $$('[data-place-mode]').forEach(btn => btn.onclick = () => {
    state.placeMode = btn.dataset.placeMode;
    state.placeStep = 1;
    state.validateResult = null;
    render();
  });
  $('#modal-overlay')?.addEventListener('click', () => { state.launch = false; state.placeStep = 1; state.placePreview = null; state.validateResult = null; render(); });
  $('#launch-cancel')?.addEventListener('click', () => {
    if (state.placeMode === 'intent' && state.placeStep > 1) { state.placeStep -= 1; render(); return; }
    state.launch = false; state.placeStep = 1; state.placePreview = null; state.validateResult = null; render();
  });
  $$('.place-suggest').forEach(btn => btn.onclick = () => {
    state.placeIntent = btn.textContent;
    const ta = $('#place-intent');
    if (ta) ta.value = state.placeIntent;
    schedulePlacePreview();
  });
  $('#place-intent')?.addEventListener('input', e => { state.placeIntent = e.target.value; schedulePlacePreview(); });
  $('#place-yaml')?.addEventListener('input', e => { state.placeYaml = e.target.value; state.validateResult = null; });
  $('#place-validate')?.addEventListener('click', async () => {
    state.placeYaml = $('#place-yaml')?.value || state.placeYaml;
    if (!String(state.placeYaml || '').trim()) {
      toast('Paste YAML to validate', 'error');
      return;
    }
    try {
      const res = await api('/api/v1/validate', {
        method: 'POST',
        body: JSON.stringify({ yaml: state.placeYaml, manifest: state.placeYaml }),
      });
      const errors = res.errors || res.error && [res.error] || [];
      state.validateResult = {
        valid: res.valid !== false && !errors.length && res.ok !== false,
        errors: Array.isArray(errors) ? errors : [String(errors)],
        error: res.error,
      };
      toast(state.validateResult.valid ? 'Manifest validated' : 'Validation failed', state.validateResult.valid ? 'ok' : 'error');
      render();
    } catch (err) {
      state.validateResult = { valid: false, errors: [err.message] };
      toast(err.message, 'error');
      render();
    }
  });
  if (state.placeMode === 'intent') schedulePlacePreview();
  $('#place-next')?.addEventListener('click', async () => {
    try {
      if (state.placeMode === 'form') {
        state.placeForm = readPlaceForm();
        if (!state.placeForm.name || !state.placeForm.image) {
          toast('Name and image are required', 'error');
          return;
        }
        await deployLaunchRequest(state.placeForm);
        return;
      }
      if (state.placeMode === 'yaml') {
        state.placeYaml = $('#place-yaml')?.value || state.placeYaml;
        const parsed = parseSimpleWorkloadYaml(state.placeYaml);
        if (!parsed.name || !parsed.image) {
          toast('YAML needs at least name and image', 'error');
          return;
        }
        state.placeForm = parsed;
        await deployLaunchRequest(parsed);
        return;
      }
      if (state.placeStep === 1) {
        state.placeIntent = $('#place-intent')?.value || state.placeIntent;
        const res = await api('/api/v1/intent/parse', { method: 'POST', body: JSON.stringify({ intent: state.placeIntent }) });
        state.placeDraft = res.draft;
        state.placeReceipt = res.receipt;
        state.placeStep = 2;
        render();
        return;
      }
      if (state.placeStep === 2 && state.placeDraft) {
        state.placeDraft = {
          ...state.placeDraft,
          name: $('#place-name')?.value || state.placeDraft.name,
          replicas: Number($('#place-replicas')?.value || state.placeDraft.replicas),
          namespace: $('#place-namespace')?.value || state.placeDraft.namespace,
          memory: $('#place-memory')?.value || state.placeDraft.memory,
          image: $('#place-image')?.value || state.placeDraft.image,
        };
        state.placeIntent = `Run ${state.placeDraft.name}, ${state.placeDraft.replicas} copies, ${state.placeDraft.memory}, image ${state.placeDraft.image}, namespace ${state.placeDraft.namespace}`;
        const res = await api('/api/v1/intent/parse', { method: 'POST', body: JSON.stringify({ intent: state.placeIntent }) });
        state.placeDraft = { ...res.draft, ...state.placeDraft };
        state.placeReceipt = res.receipt;
        state.placeStep = 3;
        render();
        return;
      }
      if (state.placeStep === 3) { state.placeStep = 4; render(); return; }
      if (state.placeStep === 4) {
        if (state.placeDraft?.image && state.placeDraft?.name) {
          await deployLaunchRequest({
            name: state.placeDraft.name,
            namespace: state.placeDraft.namespace,
            image: state.placeDraft.image,
            replicas: state.placeDraft.replicas,
            memory: state.placeDraft.memory,
            port: 80,
          });
          return;
        }
        const workload = await api('/api/v1/intent/place', { method: 'POST', body: JSON.stringify({ intent: state.placeIntent }) });
        state.launch = false;
        state.placeStep = 1;
        toast(`${workload.name} deployed`);
        await refreshAll(false);
        state.page = 'workloads';
        render();
      }
    } catch (error) { toast(error.message, 'error'); }
  });
}

function wireStill() {
  $('#still-overlay')?.addEventListener('click', () => { state.still = null; render(); });
  $('#still-cancel')?.addEventListener('click', () => { state.still = null; render(); });
  $('#still-confirm')?.addEventListener('click', async () => {
    const s = state.still;
    if (!s) return;
    state.still = null;
    const w = s.workload;
    if (s.action === 'hush') return workloadAction('stop');
    if (s.action === 'sleep') {
      const name = (s.preview?.names || [])[0];
      if (name) {
        return mutate(`/api/v1/pods/${encodeURIComponent(w.namespace)}/${encodeURIComponent(name)}`, { method: 'DELETE' }, 'Pod deleted — replacement incoming');
      }
      return workloadAction('restart');
    }
    if (s.action === 'seal') {
      return mutate(`/api/v1/workloads/${encodeURIComponent(w.namespace)}/${encodeURIComponent(w.kind)}/${encodeURIComponent(w.name)}/seal`, { method: 'POST' }, 'Workload isolated');
    }
    if (s.action === 'end') {
      return requestConfirm({ title: 'Delete workload', name: w.name, confirmLabel: 'Delete', onConfirm: async () => {
        await mutate(`/api/v1/workloads/${encodeURIComponent(w.namespace)}/${encodeURIComponent(w.kind)}/${encodeURIComponent(w.name)}`, { method: 'DELETE' }, 'Workload deleted', true);
      }});
    }
    render();
  });
}

function wireLaunch() {
  wirePlace();
}

async function runPaletteCommand(command) {
  state.palette = false;
  state.paletteQuery = '';
  if (command.startsWith('pod-enter:')) {
    openDrawer(state.pods[Number(command.split(':')[1])], 'pod', 'terminal');
    state.presenceKnocked = false;
    return render();
  }
  if (command.startsWith('pod-breath:')) {
    openDrawer(state.pods[Number(command.split(':')[1])], 'pod', 'logs');
    return render();
  }
  if (command.startsWith('pod-still:')) {
    const [, idx, action] = command.split(':');
    openDrawer(state.pods[Number(idx)], 'pod');
    return openStillRing(action || 'hush');
  }
  if (command.startsWith('workload:')) {
    openDrawer(state.workloads[Number(command.split(':')[1])], 'workload');
    return render();
  }
  if (command === 'launch') { state.launch = true; state.placeStep = 1; state.placeMode = 'form'; return render(); }
  if (command === 'refresh') { await refreshAll(false); return render(); }
  if (command === 'drawer-logs') { state.drawerTab = 'logs'; return render(); }
  if (command === 'drawer-terminal') { state.drawerTab = 'terminal'; return render(); }
  if (command === 'drawer-wires') { followWires(state.selected?.namespace); return; }
  if (command === 'drawer-still-hush') { await openStillRing('hush'); return; }
  if (command === 'drawer-still-sleep') { await openStillRing('sleep'); return; }
  if (command === 'drawer-still-end') { await openStillRing('end'); return; }
  if (command === 'drawer-restart') { await doAction('restart'); return; }
  if (command === 'drawer-scale') { state.drawerTab = 'overview'; return render(); }
  if (command === 'drawer-delete') { await doAction('delete-workload'); return; }
  if (command === 'drawer-evict') { await doAction('evict-pod'); return; }
  navigateTo(command);
}

function visiblePaletteItems() {
  return $$('.palette-item').filter(el => el.style.display !== 'none');
}

function wirePalette() {
  $('#palette-overlay')?.addEventListener('click', () => { state.palette = false; render(); });
  const input = $('#palette-input');
  input.focus();
  const syncHighlight = () => {
    $$('.palette-item').forEach((item, idx) => item.classList.toggle('highlight', idx === state.paletteHighlight));
  };
  input.oninput = () => {
    state.paletteQuery = input.value;
    state.paletteHighlight = 0;
    const root = $('.palette-list');
    if (root) {
      const items = paletteItems(state.paletteQuery);
      root.innerHTML = items.length ? items.map((i, idx) => `<div class="palette-item ${idx === 0 ? 'highlight' : ''}" data-command="${enc(i[0])}"><div class="palette-icon">${navIcon(i[0].startsWith('drawer') || i[0].startsWith('pod') ? 'pods' : i[0] === 'refresh' ? 'events' : i[0].split(':')[0] === 'workload' ? 'workloads' : i[0])}</div><div><div class="palette-label">${i[1]}</div><div class="palette-meta">${i[2]}</div></div></div>`).join('') : '<div class="palette-empty">No matching resources.</div>';
      $$('.palette-item').forEach((el, idx) => el.onclick = async () => {
        state.paletteHighlight = idx;
        await runPaletteCommand(el.dataset.command);
      });
    }
  };
  input.onkeydown = async event => {
    const visible = visiblePaletteItems();
    if (event.key === 'ArrowDown') { event.preventDefault(); state.paletteHighlight = Math.min(state.paletteHighlight + 1, visible.length - 1); syncHighlight(); }
    else if (event.key === 'ArrowUp') { event.preventDefault(); state.paletteHighlight = Math.max(state.paletteHighlight - 1, 0); syncHighlight(); }
    else if (event.key === 'Enter') {
      event.preventDefault();
      const item = visible[state.paletteHighlight] || visible[0];
      if (item) await runPaletteCommand(item.dataset.command);
    }
  };
  $$('.palette-item').forEach((el, idx) => el.onclick = async () => {
    state.paletteHighlight = idx;
    await runPaletteCommand(el.dataset.command);
  });
  syncHighlight();
}

window.addEventListener('keydown', event => {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault();
    if (state.session) { state.palette = !state.palette; state.paletteHighlight = 0; render(); }
  }
  if (event.key === 'Escape') {
    if (state.still) { state.still = null; render(); }
    else if (state.confirm) { state.confirm = null; render(); }
    else if (state.helmRollback) { state.helmRollback = null; render(); }
    else if (state.palette) { state.palette = false; render(); }
    else if (state.launch) { state.launch = false; state.placeStep = 1; render(); }
    else if (state.selected) closeDrawer();
  }
});

applyTheme();
restoreCreds();
(async () => {
  try {
    state.authProviders = await fetch('/api/v1/auth/providers', { credentials: 'same-origin' }).then(r => r.ok ? r.json() : null);
  } catch { state.authProviders = null; }
  state.loginShowDemo = false;

  const ssoReturn = new URLSearchParams(location.search).get('sso') === '1';
  if (ssoReturn) {
    history.replaceState({}, '', location.pathname);
  }

  async function enterConsole() {
    state.session = await api('/api/v1/session');
    if (!state.credentials) {
      setCreds(state.session.user || 'oidc', '', { oidc: true });
    } else if (state.session.auth === 'oidc') {
      setCreds(state.session.user || state.credentials.user, '', { oidc: true });
    }
    state.realtimeCapabilities = await api('/api/v1/realtime/capabilities').catch(() => null);
    render();
    connectEventStream();
    refreshAll(true).catch(() => {});
  }

  if (state.credentials || ssoReturn) {
    try {
      await enterConsole();
    } catch {
      state.credentials = null;
      sessionStorage.removeItem('axiom.credentials');
      render();
    }
  } else {
    // Cookie session from a prior OIDC login (no sessionStorage).
    try {
      const probe = await fetch('/api/v1/session', { credentials: 'same-origin' });
      if (probe.ok) {
        await enterConsole();
        return;
      }
    } catch {}
    render();
  }
})();
