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
            className="relative inline-flex h-9 w-16 items-center rounded-full border border-border bg-secondary px-1 transition-colors duration-300 ease-apple"
            aria-label="Toggle theme"
        >
            <span
                className="absolute top-1 flex h-7 w-7 items-center justify-center rounded-full bg-card shadow-pop transition-transform duration-300 ease-apple"
                style={{ transform: isDark ? "translateX(28px)" : "translateX(0)" }}
            >
                {isDark ? (
                    <Moon className="h-3.5 w-3.5 text-tone-link" />
                ) : (
                    <Sun className="h-3.5 w-3.5 text-tone-amber" />
                )}
            </span>
        </button>
    );
}
