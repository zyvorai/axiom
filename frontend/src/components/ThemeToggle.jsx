import { useTheme } from "@/lib/theme";
import { Sun, Moon } from "lucide-react";
import { NAV } from "@/constants/testIds";

export default function ThemeToggle() {
    const { theme, setTheme } = useTheme();
    const isDark = theme === "dark";
    return (
        <button
            data-testid={NAV.themeToggle}
            onClick={() => setTheme(isDark ? "light" : "dark")}
            className="glass-inset specular relative inline-flex h-9 w-16 items-center rounded-full px-1 transition-colors"
            aria-label="Toggle theme"
        >
            <span
                className="absolute top-1 h-7 w-7 rounded-full bg-white/90 dark:bg-white/10 shadow-md transition-transform duration-500 flex items-center justify-center"
                style={{ transform: isDark ? "translateX(28px)" : "translateX(0)" }}
            >
                {isDark ? (
                    <Moon className="h-3.5 w-3.5 text-sky-300" />
                ) : (
                    <Sun className="h-3.5 w-3.5 text-amber-500" />
                )}
            </span>
        </button>
    );
}
