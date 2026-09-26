import { createContext, useContext, useEffect, useState } from "react";

const STORAGE_KEY = "axiom-theme";
const LEGACY_KEY = "kubecloud.theme";

const ThemeContext = createContext({ theme: "dark", setTheme: () => {} });

// Keep in sync with the pre-paint script in public/index.html.
function readStoredTheme() {
    try {
        const t = localStorage.getItem(STORAGE_KEY) || localStorage.getItem(LEGACY_KEY);
        return t === "light" ? "light" : "dark";
    } catch {
        return "dark";
    }
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
    const [theme, setTheme] = useState(readStoredTheme);

    useEffect(() => {
        applyTheme(theme);
        try {
            localStorage.setItem(STORAGE_KEY, theme);
        } catch {
            /* private mode — theme still applies for this session */
        }
    }, [theme]);

    return (
        <ThemeContext.Provider value={{ theme, setTheme }}>
            {children}
        </ThemeContext.Provider>
    );
}

export const useTheme = () => useContext(ThemeContext);
