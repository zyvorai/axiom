import { createContext, useContext, useEffect, useState } from "react";

const ThemeContext = createContext({ theme: "dark", setTheme: () => {} });

export function ThemeProvider({ children }) {
    const [theme, setTheme] = useState(() => localStorage.getItem("kubecloud.theme") || "dark");

    useEffect(() => {
        const root = document.documentElement;
        root.classList.toggle("dark", theme === "dark");
        localStorage.setItem("kubecloud.theme", theme);
    }, [theme]);

    return (
        <ThemeContext.Provider value={{ theme, setTheme }}>
            {children}
        </ThemeContext.Provider>
    );
}

export const useTheme = () => useContext(ThemeContext);
