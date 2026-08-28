import axios from "axios";

export const API = "/api/v1";

export const api = axios.create({ baseURL: API, timeout: 15000 });

const TOKEN_KEY = "axiom.basicAuth";

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
