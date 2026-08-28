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
