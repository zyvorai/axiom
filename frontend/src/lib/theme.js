import { createContext, useContext, useEffect, useState } from "react";

const STORAGE_KEY = "axiom-theme";
const LEGACY_KEY = "kubecloud.theme";

export const THEME_PREFERENCES = ["light", "dark", "system"];

// `theme` is always the RESOLVED appearance ("light" | "dark") so existing consumers keep
// working; `preference` is what the user chose, which may be "system" (follow the OS).
const ThemeContext = createContext({ theme: "dark", preference: "dark", setTheme: () => {} });

// Keep in sync with the pre-paint script in public/index.html.
function readStoredPreference() {
    try {
        const t = localStorage.getItem(STORAGE_KEY) || localStorage.getItem(LEGACY_KEY);
        return THEME_PREFERENCES.includes(t) ? t : "dark";
    } catch {
        return "dark";
    }
}

function systemPrefersDark() {
    try {
        return window.matchMedia("(prefers-color-scheme: dark)").matches;
    } catch {
        return true;
    }
}

export function resolveTheme(preference) {
    return preference === "system" ? (systemPrefersDark() ? "dark" : "light") : preference;
}

function applyTheme(theme) {
    const root = document.documentElement;
    root.classList.toggle("dark", theme === "dark");
    root.dataset.theme = theme;
    // browser chrome follows the page canvas token (public/index.html's pre-paint
    // script must keep literal values: it runs before any CSS is loaded)
    document
        .querySelector('meta[name="theme-color"]')
        ?.setAttribute("content", getComputedStyle(root).getPropertyValue("--bg-page").trim());
}

export function ThemeProvider({ children }) {
    const [preference, setPreference] = useState(readStoredPreference);
    const [theme, setResolved] = useState(() => resolveTheme(readStoredPreference()));

    useEffect(() => {
        setResolved(resolveTheme(preference));
        try {
            localStorage.setItem(STORAGE_KEY, preference);
        } catch {
            /* private mode — theme still applies for this session */
        }
        if (preference !== "system") return undefined;
        // follow the OS live while "system" is selected
        let mq;
        try {
            mq = window.matchMedia("(prefers-color-scheme: dark)");
        } catch {
            return undefined;
        }
        const onChange = () => setResolved(mq.matches ? "dark" : "light");
        mq.addEventListener?.("change", onChange);
        return () => mq.removeEventListener?.("change", onChange);
    }, [preference]);

    useEffect(() => {
        applyTheme(theme);
    }, [theme]);

    const setTheme = (next) => setPreference(THEME_PREFERENCES.includes(next) ? next : "dark");

    return <ThemeContext.Provider value={{ theme, preference, setTheme }}>{children}</ThemeContext.Provider>;
}

export const useTheme = () => useContext(ThemeContext);

/** Next preference in the light → dark → system cycle. */
export function nextPreference(preference) {
    return THEME_PREFERENCES[(THEME_PREFERENCES.indexOf(preference) + 1) % THEME_PREFERENCES.length];
}

export const THEME_LABELS = { light: "Light", dark: "Dark", system: "System" };
