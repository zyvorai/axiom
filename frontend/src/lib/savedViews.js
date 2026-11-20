// Saved views: a named snapshot of a page's filters (search text, namespace, kind, ...),
// stored per page in this browser. A view is just { name, filters } where filters maps URL
// parameter names to values, so applying one is writing those parameters back to the URL.

const KEY_PREFIX = "axiom-views:";
const MAX_VIEWS = 20;

function storageKey(pageKey) {
    return `${KEY_PREFIX}${pageKey}`;
}

export function loadViews(pageKey) {
    try {
        const raw = JSON.parse(localStorage.getItem(storageKey(pageKey)) || "[]");
        return Array.isArray(raw)
            ? raw.filter((v) => v && typeof v.name === "string" && v.filters && typeof v.filters === "object")
            : [];
    } catch {
        return [];
    }
}

function persist(pageKey, views) {
    try {
        localStorage.setItem(storageKey(pageKey), JSON.stringify(views));
    } catch {
        /* private mode — views last for this session's component state only */
    }
    return views;
}

/** Saves (or overwrites, by case-insensitive name) a view. Returns the new list. */
export function saveView(pageKey, name, filters) {
    const trimmed = String(name || "").trim().slice(0, 60);
    if (!trimmed) return loadViews(pageKey);
    const clean = Object.fromEntries(Object.entries(filters).filter(([, v]) => v !== "" && v != null).map(([k, v]) => [k, String(v)]));
    const rest = loadViews(pageKey).filter((v) => v.name.toLowerCase() !== trimmed.toLowerCase());
    return persist(pageKey, [...rest, { name: trimmed, filters: clean }].slice(-MAX_VIEWS));
}

export function deleteView(pageKey, name) {
    return persist(
        pageKey,
        loadViews(pageKey).filter((v) => v.name !== name)
    );
}
