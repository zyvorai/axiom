import { api } from "@/lib/api";
import i18n from "@/lib/i18n";

// Client for the opt-in AI assistant (see internal/api/ai.go). The namespace
// travels in the query string, never the body: that is the value the backend
// authorizes for namespace-restricted accounts.

// `value` is what the API stores; the display name is translated at render time
// (`labelKey`), except product names that stay as they are (`label`).
export const AI_PROVIDERS = [
    { value: "openai-compatible", labelKey: "pages:aiSettings.providers.openaiCompatible", placeholder: "https://api.openai.com/v1" },
    { value: "anthropic", label: "Anthropic", placeholder: "https://api.anthropic.com" },
    { value: "ollama", labelKey: "pages:aiSettings.providers.ollama", placeholder: "http://localhost:11434" },
];

/** Display name of an AI_PROVIDERS entry in the current language. */
export function aiProviderLabel(t, p) {
    return p.labelKey ? t(p.labelKey) : p.label;
}

export const AI_DISABLED = { enabled: false, configured: false, redact: true };

export async function fetchAiStatus() {
    try {
        const { data } = await api.get("/ai/status");
        return data || AI_DISABLED;
    } catch {
        return AI_DISABLED; // fail closed: no status means no AI UI
    }
}

/** kind: "pod" (name = pod name) or "event" (name = "Kind/name" of the involved object). */
export async function explainAi({ namespace, kind, name, reason, question, preview }) {
    const body = { kind, name, preview: !!preview };
    if (reason) body.reason = reason;
    if (question && question.trim()) body.question = question.trim();
    const { data } = await api.post(`/ai/explain?namespace=${encodeURIComponent(namespace)}`, body, {
        timeout: preview ? 15000 : 70000,
    });
    return data;
}

export function aiErrorMessage(e) {
    const status = e?.response?.status;
    const msg = e?.response?.data?.error || e?.message || i18n.t("errors:ai.requestFailed");
    if (status === 429) return i18n.t("errors:ai.rateLimited", { message: msg });
    return msg;
}

/** Events the assistant can explain: Warnings with a Kind/name object in a namespace. */
export function canExplainEvent(e) {
    return e?.type === "Warning" && !!e?.namespace && typeof e?.object === "string" && e.object.includes("/");
}
