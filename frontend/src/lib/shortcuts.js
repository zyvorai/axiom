import { useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";

// Keyboard shortcuts. Single source of truth: the handler below and the `?` help dialog are both
// driven by this list, so the documentation cannot drift from behaviour.
export const SHORTCUTS = [
    { keys: ["⌘K", "Ctrl K"], label: "Open the command palette", group: "General" },
    { keys: ["/"], label: "Focus the search box on this page", group: "General" },
    { keys: ["?"], label: "Show keyboard shortcuts", group: "General" },
    { keys: ["Esc"], label: "Close a dialog or menu", group: "General" },
    { keys: ["g", "o"], label: "Go to Overview", group: "Go to", to: "/overview" },
    { keys: ["g", "p"], label: "Go to Pods", group: "Go to", to: "/pods" },
    { keys: ["g", "w"], label: "Go to Workloads", group: "Go to", to: "/workloads" },
    { keys: ["g", "n"], label: "Go to Namespaces", group: "Go to", to: "/namespaces" },
    { keys: ["g", "e"], label: "Go to Events", group: "Go to", to: "/events" },
    { keys: ["g", "l"], label: "Go to Logs", group: "Go to", to: "/logs" },
];

const GO_TARGETS = Object.fromEntries(
    SHORTCUTS.filter((s) => s.to).map((s) => [s.keys[1], s.to])
);

/** Event the shell listens to for opening the help dialog (also dispatched by the palette). */
export const SHORTCUTS_EVENT = "axiom:shortcuts";

/** True when the key press is going to a text field, so it is typing, not a shortcut. */
export function isTypingTarget(el) {
    if (!el || !(el instanceof Element)) return false;
    const tag = el.tagName;
    if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return true;
    return el.isContentEditable || el.closest?.("[contenteditable='true'], [role='textbox'], .cm-editor") != null;
}

function modalOpen() {
    return document.querySelector("[role='dialog'][data-state='open'], [role='alertdialog'][data-state='open']") != null;
}

function focusSearch() {
    const candidates = document.querySelectorAll("[data-search-input]");
    for (const el of candidates) {
        if (el.offsetParent !== null) {
            el.focus();
            if (typeof el.select === "function") el.select();
            return true;
        }
    }
    return false;
}

/**
 * Installs the global shortcuts: `/` focuses the page search, `g` then a letter navigates,
 * `?` opens the help dialog. Shortcuts never fire while typing in a field, with a modifier held
 * (so browser/OS combos and ⌘K stay untouched) or while a modal dialog is open.
 */
export function useShortcuts() {
    const navigate = useNavigate();
    // navigate() changes identity on every route change; keep it out of the effect's deps so the
    // listener (and the pending "g" prefix) is not torn down and rebuilt mid-sequence.
    const navigateRef = useRef(navigate);
    navigateRef.current = navigate;
    useEffect(() => {
        let pendingG = 0;
        const onKey = (e) => {
            if (e.defaultPrevented || e.ctrlKey || e.metaKey || e.altKey) return;
            if (isTypingTarget(e.target) || modalOpen()) return;
            const now = Date.now();
            if (pendingG && now - pendingG < 1200) {
                pendingG = 0;
                const to = GO_TARGETS[e.key.toLowerCase()];
                if (to) {
                    e.preventDefault();
                    navigateRef.current(to);
                    return;
                }
            }
            if (e.key === "g") {
                pendingG = now;
                return;
            }
            pendingG = 0;
            if (e.key === "/") {
                if (focusSearch()) e.preventDefault();
            } else if (e.key === "?") {
                e.preventDefault();
                window.dispatchEvent(new Event(SHORTCUTS_EVENT));
            }
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    }, []);
}
