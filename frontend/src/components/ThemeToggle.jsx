import { Monitor, Moon, Sun } from "lucide-react";
import { useTranslation } from "react-i18next";
import { NAV } from "@/constants/testIds";
import { THEME_PREFERENCES, useTheme } from "@/lib/theme";

const ICONS = { light: Sun, dark: Moon, system: Monitor };

// Light / Dark / System segmented control (Settings → Appearance). "System" follows the OS
// colour scheme live.
export default function ThemeToggle() {
    const { preference, setTheme } = useTheme();
    const { t } = useTranslation();
    return (
        <div
            data-testid={NAV.themeToggle}
            role="radiogroup"
            aria-label={t("nav:theme.label")}
            className="inline-flex items-center gap-0.5 rounded-full border border-border bg-secondary p-1"
        >
            {THEME_PREFERENCES.map((p) => {
                const Icon = ICONS[p];
                const on = preference === p;
                return (
                    <button
                        key={p}
                        type="button"
                        role="radio"
                        aria-checked={on}
                        data-testid={`theme-option-${p}`}
                        onClick={() => setTheme(p)}
                        className={`flex h-7 items-center gap-1.5 rounded-full px-3 text-[12.5px] transition-colors duration-200 ease-apple ${
                            on ? "bg-card text-foreground shadow-pop" : "text-muted-foreground hover:text-foreground"
                        }`}
                    >
                        <Icon className="h-3.5 w-3.5" aria-hidden="true" />
                        {t(`nav:theme.${p}`)}
                    </button>
                );
            })}
        </div>
    );
}
