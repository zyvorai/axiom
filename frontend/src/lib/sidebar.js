const COLLAPSE_KEY = "axiom:sidebar-collapsed";

export function loadSidebarCollapsed() {
    try {
        return localStorage.getItem(COLLAPSE_KEY) === "1";
    } catch {
        return false;
    }
}

export function saveSidebarCollapsed(value) {
    try {
        localStorage.setItem(COLLAPSE_KEY, value ? "1" : "0");
    } catch {
        // ignore
    }
}
