import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Languages } from "lucide-react";
import { useLanguage } from "@/lib/i18n";
import { cn } from "@/lib/utils";

/**
 * Compact native <select> for the UI language (user menu, login page, Settings).
 * A native control is used on purpose: it is keyboard/screen-reader friendly and
 * works inside the Radix dropdown without fighting its focus handling.
 * Each option is written in its own language (English, Deutsch, Español, Français).
 */
export default function LanguageSelect({ className, compact = false }) {
    const { t } = useTranslation();
    const { language, setLanguage, languages } = useLanguage();
    const id = useId();
    return (
        <div
            className={cn("flex items-center gap-2 text-xs text-muted-foreground", className)}
            // keep the surrounding menu's typeahead/arrow-key handling away from the select
            onKeyDown={(e) => e.stopPropagation()}
        >
            <Languages className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            <label htmlFor={id} className={compact ? "sr-only" : undefined}>
                {t("common:language.label")}
            </label>
            <select
                id={id}
                data-testid="language-select"
                value={language}
                onChange={(e) => setLanguage(e.target.value)}
                className="ml-auto h-7 rounded-lg border border-border bg-background px-2 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
                {languages.map((l) => (
                    <option key={l.code} value={l.code} lang={l.code}>
                        {l.name}
                    </option>
                ))}
            </select>
        </div>
    );
}
