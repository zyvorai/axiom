import { formatDistanceToNowStrict } from "date-fns";
import { resourceHref } from "@/lib/resourceHref";

/** "3 minutes ago"-style label; falls back to the raw value for unparseable input. */
export function relTime(iso) {
    try {
        return formatDistanceToNowStrict(new Date(iso), { addSuffix: true });
    } catch {
        return String(iso ?? "");
    }
}

/** Deep link for a "Kind/name" object, falling back to its namespace page. */
export function objectHref(namespace, object) {
    return resourceHref(namespace, object) || (namespace ? `/namespaces/${encodeURIComponent(namespace)}` : null);
}

/** Link to the change timeline filtered to one object. */
export function timelineHref({ namespace, object } = {}) {
    const p = new URLSearchParams();
    if (namespace) p.set("namespace", namespace);
    if (object) p.set("object", object);
    const qs = p.toString();
    return qs ? `/timeline?${qs}` : "/timeline";
}

/** The kubectl command that forwards a pod port to the same local port. */
export function portForwardCommand(namespace, pod, port) {
    return `kubectl port-forward -n ${namespace} pod/${pod} ${port}:${port}`;
}

/**
 * Copies text to the clipboard. Uses the async Clipboard API where available
 * (secure contexts) and a hidden-textarea fallback elsewhere. Resolves true on success.
 */
export async function copyText(text) {
    try {
        if (navigator.clipboard?.writeText) {
            await navigator.clipboard.writeText(text);
            return true;
        }
    } catch {
        // fall through to the legacy path
    }
    try {
        const ta = document.createElement("textarea");
        ta.value = text;
        ta.setAttribute("readonly", "");
        ta.style.position = "fixed";
        ta.style.opacity = "0";
        document.body.appendChild(ta);
        ta.select();
        const ok = document.execCommand("copy");
        document.body.removeChild(ta);
        return ok;
    } catch {
        return false;
    }
}

/**
 * Splits a log line into { pod, ts, text }. Workload log lines look like
 * "[pod/container] 2026-01-02T03:04:05.123Z message"; single-pod lines omit the
 * bracketed prefix. Both are optional so plain text still renders.
 */
const LOG_LINE = /^(?:\[([^\]]+)\]\s)?(\d{4}-\d{2}-\d{2}T[\d:.]+(?:Z|[+-]\d{2}:\d{2}))?\s?([\s\S]*)$/;
export function parseLogLine(line) {
    const m = LOG_LINE.exec(line);
    if (!m) return { pod: "", ts: "", text: line };
    return { pod: m[1] || "", ts: m[2] || "", text: m[3] ?? "" };
}

/** Compiles a user regex (case-insensitive). Returns { re, error }; an empty pattern matches everything. */
export function compileFilter(pattern) {
    if (!pattern) return { re: null, error: "" };
    try {
        return { re: new RegExp(pattern, "i"), error: "" };
    } catch (e) {
        return { re: null, error: e.message.replace(/^Invalid regular expression: /, "") };
    }
}
