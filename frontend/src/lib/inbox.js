import { useCallback, useEffect, useMemo, useState } from "react";
import { useAlertWarnings } from "@/lib/alerts";

// Notification inbox state: the warnings come from the server (/alerts), whether *you* have read
// or dismissed one is local to this browser. An alert is identified by what it says and which
// object it is about, so it stays dismissed across polls and reloads, and comes back as new
// only if the same condition is reported again after it cleared.

const KEY = "axiom-inbox";

export function alertId(w) {
    return [w.namespace || "", w.object || "", w.reason || "", w.message || ""].join("|");
}

function load() {
    try {
        const raw = JSON.parse(localStorage.getItem(KEY) || "{}");
        return { read: raw.read || {}, dismissed: raw.dismissed || {} };
    } catch {
        return { read: {}, dismissed: {} };
    }
}

function persist(state) {
    try {
        localStorage.setItem(KEY, JSON.stringify(state));
    } catch {
        /* private mode — read/dismiss state lasts for this session only */
    }
}

/** @returns {{items: Array, unread: number, markRead: Function, markAllRead: Function, dismiss: Function, dismissAll: Function}} */
export function useInbox(intervalMs = 6000) {
    const warnings = useAlertWarnings(intervalMs);
    const [state, setState] = useState(load);

    // Forget read/dismissed marks for alerts that are no longer reported, so the stored set stays
    // bounded and a recurring alert is announced again.
    useEffect(() => {
        if (!warnings.length && !Object.keys(state.read).length && !Object.keys(state.dismissed).length) return;
        const live = new Set(warnings.map(alertId));
        const prune = (m) => Object.fromEntries(Object.entries(m).filter(([id]) => live.has(id)));
        const next = { read: prune(state.read), dismissed: prune(state.dismissed) };
        if (Object.keys(next.read).length !== Object.keys(state.read).length || Object.keys(next.dismissed).length !== Object.keys(state.dismissed).length) {
            // only prune once we actually have a feed (an empty first load is just "not loaded yet")
            if (warnings.length) {
                setState(next);
                persist(next);
            }
        }
    }, [warnings, state]);

    const update = useCallback((fn) => {
        setState((prev) => {
            const next = fn(prev);
            persist(next);
            return next;
        });
    }, []);

    const items = useMemo(
        () =>
            warnings
                .map((w) => ({ ...w, id: alertId(w) }))
                .filter((w) => !state.dismissed[w.id])
                .map((w) => ({ ...w, read: !!state.read[w.id] })),
        [warnings, state]
    );
    const unread = items.filter((i) => !i.read).length;

    return {
        items,
        unread,
        markRead: (id) => update((s) => ({ ...s, read: { ...s.read, [id]: 1 } })),
        markAllRead: () => update((s) => ({ ...s, read: { ...s.read, ...Object.fromEntries(items.map((i) => [i.id, 1])) } })),
        dismiss: (id) => update((s) => ({ ...s, dismissed: { ...s.dismissed, [id]: 1 } })),
        dismissAll: () => update((s) => ({ ...s, dismissed: { ...s.dismissed, ...Object.fromEntries(items.map((i) => [i.id, 1])) } })),
    };
}
