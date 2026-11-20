// Pages fetch on mount and most have no failure path, so a backend outage used to leave a permanent
// skeleton with no message. Instead of adding failure UI to ~40 pages, the API client reports
// server/network failures here and the shell shows one banner with a Retry (LoadErrorBanner).
//
// Only 5xx and network errors count: 4xx are business outcomes (403 per-resource, 404 for an
// optional integration) that pages already handle.
let failures = new Map(); // url -> { url, status }
const listeners = new Set();
let snapshot = [];

const emit = () => {
    snapshot = [...failures.values()];
    listeners.forEach((fn) => fn());
};

export function reportApiFailure(f) {
    failures.set(f.url, f);
    emit();
}
export function clearApiFailure(url) {
    if (failures.delete(url)) emit();
}
export function clearApiFailures() {
    if (failures.size) {
        failures = new Map();
        emit();
    }
}
export const subscribeApiFailures = (fn) => {
    listeners.add(fn);
    return () => listeners.delete(fn);
};
export const getApiFailures = () => snapshot;
