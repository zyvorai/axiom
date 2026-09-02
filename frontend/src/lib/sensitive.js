/** Heuristic: env / label keys that should not show raw values in the UI. */
const SENSITIVE_RE =
    /(password|passwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|credential|auth|bearer|webhook)/i;

export function isSensitiveKey(name) {
    return SENSITIVE_RE.test(String(name || ""));
}

export function maskEnvDisplay(name, value, valueFrom) {
    if (valueFrom) return "<from ref>";
    if (value == null || value === "") return "";
    if (isSensitiveKey(name)) return "••••••••";
    return String(value);
}
