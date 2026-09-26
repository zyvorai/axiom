import axios from "axios";
import { clearApiFailure, reportApiFailure } from "@/lib/apiHealth";

export const API = "/api/v1";

export const api = axios.create({ baseURL: API, timeout: 15000 });

// Report server/network failures of GETs so the shell can show a Retry banner (see lib/apiHealth.js).
const isGet = (cfg) => (cfg?.method || "get").toLowerCase() === "get";
api.interceptors.response.use(
    (res) => {
        if (isGet(res.config)) clearApiFailure(res.config.url);
        return res;
    },
    (err) => {
        const status = err.response?.status ?? 0; // 0 = no response (offline, timeout, CORS)
        if (isGet(err.config) && (status === 0 || status >= 500) && err.code !== "ERR_CANCELED") {
            reportApiFailure({ url: err.config?.url || "", status });
        }
        return Promise.reject(err);
    }
);

const TOKEN_KEY = "axiom.basicAuth";

// Multi-cluster mode tracks the "active cluster" per caller so two tabs (or
// two users) switching clusters don't stomp on each other — see
// platform.SwitchableProvider on the backend. sessionStorage is scoped per
// tab (unlike localStorage), so a fresh, stable ID minted here and sent on
// every request is enough to give each tab its own independent selection.
const CLUSTER_SESSION_HEADER = "X-Axiom-Session";
const CLUSTER_SESSION_KEY = "axiom.clusterSession";

function clusterSessionId() {
    let id = sessionStorage.getItem(CLUSTER_SESSION_KEY);
    if (!id) {
        id = window.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`;
        sessionStorage.setItem(CLUSTER_SESSION_KEY, id);
    }
    return id;
}

api.defaults.headers.common[CLUSTER_SESSION_HEADER] = clusterSessionId();

export function setBasicAuth(token) {
    if (token) {
        sessionStorage.setItem(TOKEN_KEY, token);
        api.defaults.headers.common["Authorization"] = `Basic ${token}`;
    } else {
        clearBasicAuth();
    }
}

export function clearBasicAuth() {
    sessionStorage.removeItem(TOKEN_KEY);
    delete api.defaults.headers.common["Authorization"];
}

export function restoreBasicAuth() {
    const token = sessionStorage.getItem(TOKEN_KEY);
    if (token) api.defaults.headers.common["Authorization"] = `Basic ${token}`;
    return token;
}

// A plain <a target="_blank"> navigation to an API route won't carry the
// Authorization header a local Basic-auth session relies on (only cookies —
// i.e. OIDC/SAML sessions — survive that). The backend accepts a base64
// user:pass via ?access_token= for exactly this case (see
// internal/auth/basic.go's credentialsFromRequest), so append it for local
// sessions; OIDC/SAML sessions ride their cookie and need nothing extra.
export function withAccessToken(path) {
    const token = sessionStorage.getItem(TOKEN_KEY);
    const url = `${API}${path}`;
    if (!token) return url;
    return url + (url.includes("?") ? "&" : "?") + `access_token=${encodeURIComponent(token)}`;
}

// Streams a pod's log endpoint (chunked text/plain, not SSE) line by line.
// Returns an AbortController the caller can use to stop the stream.
export function streamPodLogs(namespace, name, { container, follow = true, tail = 200, onLine, onError, onEnd } = {}) {
    const controller = new AbortController();
    const params = new URLSearchParams({ follow: String(follow), tail: String(tail) });
    if (container) params.set("container", container);
    const url = `${API}/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/logs?${params.toString()}`;
    const headers = {};
    if (api.defaults.headers.common["Authorization"]) {
        headers["Authorization"] = api.defaults.headers.common["Authorization"];
    }
    headers[CLUSTER_SESSION_HEADER] = clusterSessionId();

    (async () => {
        try {
            const res = await fetch(url, { headers, signal: controller.signal });
            if (!res.ok || !res.body) {
                onError?.(new Error(`log stream failed: ${res.status}`));
                return;
            }
            const reader = res.body.getReader();
            const decoder = new TextDecoder();
            let buffer = "";
            for (;;) {
                const { done, value } = await reader.read();
                if (done) break;
                buffer += decoder.decode(value, { stream: true });
                const lines = buffer.split("\n");
                buffer = lines.pop() ?? "";
                for (const line of lines) onLine?.(line);
            }
            if (buffer) onLine?.(buffer);
            onEnd?.();
        } catch (e) {
            if (e.name !== "AbortError") onError?.(e);
        }
    })();

    return controller;
}
