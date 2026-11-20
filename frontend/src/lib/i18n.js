// Internationalisation: i18next + react-i18next, see src/locales/README.md.
//
// English is bundled with the main chunk (it is also the fallback for every
// missing key). Every other language is a separate webpack chunk loaded on
// demand by loadLanguage(), together with its date-fns locale.
import { useCallback, useSyncExternalStore } from "react";
import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import en from "@/locales/en";

export const NAMESPACES = ["common", "nav", "auth", "pages", "dialogs", "toasts", "errors"];

// Native names: a language is always offered in its own language.
export const SUPPORTED_LANGUAGES = [
    { code: "en", name: "English" },
    { code: "de", name: "Deutsch" },
    { code: "es", name: "Español" },
    { code: "fr", name: "Français" },
    { code: "pt", name: "Português" },
    { code: "it", name: "Italiano" },
    { code: "ja", name: "日本語" },
    { code: "zh", name: "简体中文" },
];
const SUPPORTED = SUPPORTED_LANGUAGES.map((l) => l.code);
export const STORAGE_KEY = "axiom.lang";

// Explicit map (not a template import) so webpack emits exactly one chunk per
// language and never pulls English in twice.
const LOADERS = {
    de: () => Promise.all([import(/* webpackChunkName: "locale-de" */ "@/locales/de"), import(/* webpackChunkName: "locale-de" */ "date-fns/locale/de")]),
    es: () => Promise.all([import(/* webpackChunkName: "locale-es" */ "@/locales/es"), import(/* webpackChunkName: "locale-es" */ "date-fns/locale/es")]),
    fr: () => Promise.all([import(/* webpackChunkName: "locale-fr" */ "@/locales/fr"), import(/* webpackChunkName: "locale-fr" */ "date-fns/locale/fr")]),
    // "pt" formats as pt-BR in Intl, so dates use the matching date-fns locale.
    pt: () => Promise.all([import(/* webpackChunkName: "locale-pt" */ "@/locales/pt"), import(/* webpackChunkName: "locale-pt" */ "date-fns/locale/pt-BR")]),
    it: () => Promise.all([import(/* webpackChunkName: "locale-it" */ "@/locales/it"), import(/* webpackChunkName: "locale-it" */ "date-fns/locale/it")]),
    ja: () => Promise.all([import(/* webpackChunkName: "locale-ja" */ "@/locales/ja"), import(/* webpackChunkName: "locale-ja" */ "date-fns/locale/ja")]),
    zh: () => Promise.all([import(/* webpackChunkName: "locale-zh" */ "@/locales/zh"), import(/* webpackChunkName: "locale-zh" */ "date-fns/locale/zh-CN")]),
};

const dateFnsLocales = {}; // code -> date-fns locale (en uses date-fns' built-in default)

const isDev = process.env.NODE_ENV === "development";

/** Base language for a BCP 47 tag if it is supported ("de-AT" -> "de"), else null. */
export function baseLanguage(tag) {
    const base = String(tag || "").toLowerCase().split(/[-_]/)[0];
    return SUPPORTED.includes(base) ? base : null;
}

/** localStorage `axiom.lang` -> navigator.language(s) -> "en". */
export function detectLanguage() {
    try {
        const stored = baseLanguage(window.localStorage.getItem(STORAGE_KEY));
        if (stored) return stored;
    } catch {
        /* storage blocked: fall through */
    }
    const nav = typeof navigator !== "undefined" ? [...(navigator.languages || []), navigator.language] : [];
    for (const tag of nav) {
        const base = baseLanguage(tag);
        if (base) return base;
    }
    return "en";
}

i18n.use(initReactI18next).init({
    resources: { en },
    lng: "en",
    fallbackLng: "en",
    supportedLngs: SUPPORTED,
    ns: NAMESPACES,
    defaultNS: "common",
    partialBundledLanguages: true, // other languages are added with addResourceBundle
    keySeparator: ".",
    nsSeparator: ":",
    returnEmptyString: false,
    returnNull: false,
    interpolation: { escapeValue: false }, // React already escapes
    react: { useSuspense: false },
    // A missing key never throws: production silently shows the English
    // fallback; development also warns so the gap gets noticed.
    saveMissing: isDev,
    missingKeyHandler: isDev
        ? (lngs, ns, key) => console.warn(`[i18n] missing key "${ns}:${key}" for ${[].concat(lngs).join(",")}`)
        : undefined,
});

// Locale files only carry `_one`/`_other` (Japanese and Chinese only `_other`:
// CLDR gives them no other category). Spanish, French, Portuguese and Italian
// also have a CLDR "many" category (1 000 000 …); without a `_many` key i18next
// would fall back to English, so `_other` doubles as `_many` unless a file
// defines it.
function withManyPlurals(node) {
    if (!node || typeof node !== "object") return node;
    const out = {};
    for (const [k, v] of Object.entries(node)) out[k] = withManyPlurals(v);
    for (const k of Object.keys(out)) {
        if (k.endsWith("_other") && typeof out[k] === "string") {
            const many = `${k.slice(0, -"_other".length)}_many`;
            if (!(many in out)) out[many] = out[k];
        }
    }
    return out;
}

/** Fetches a language's resources (once) and switches to it. Unknown codes fall back to English. */
export async function loadLanguage(code) {
    const lng = baseLanguage(code) || "en";
    if (lng !== "en" && !i18n.hasResourceBundle(lng, "common")) {
        try {
            const [bundle, dfns] = await LOADERS[lng]();
            const resources = bundle.default || bundle;
            for (const ns of Object.keys(resources)) i18n.addResourceBundle(lng, ns, withManyPlurals(resources[ns]), true, true);
            dateFnsLocales[lng] = dfns.default || dfns[lng] || dfns;
        } catch (e) {
            if (isDev) console.warn(`[i18n] could not load "${lng}"`, e);
            return applyLanguage("en");
        }
    }
    return applyLanguage(lng);
}

async function applyLanguage(lng) {
    await i18n.changeLanguage(lng);
    if (typeof document !== "undefined") document.documentElement.lang = lng;
    return lng;
}

/** Called once from index.js before the first render, so the first paint is already in the right language. */
export function initLanguage() {
    return loadLanguage(detectLanguage());
}

const subscribe = (cb) => {
    i18n.on("languageChanged", cb);
    return () => i18n.off("languageChanged", cb);
};
const currentLanguage = () => baseLanguage(i18n.language) || "en";

/** Current language, a setter that persists it, and the supported list (with native names). */
export function useLanguage() {
    const language = useSyncExternalStore(subscribe, currentLanguage, currentLanguage);
    const setLanguage = useCallback((code) => {
        const lng = baseLanguage(code) || "en";
        try {
            window.localStorage.setItem(STORAGE_KEY, lng);
        } catch {
            /* storage blocked: the choice lasts for this page load */
        }
        return loadLanguage(lng);
    }, []);
    return { language, setLanguage, languages: SUPPORTED_LANGUAGES };
}

// ---- Locale-aware formatting -------------------------------------------------
// Each helper formats for the active language. For English the output matches
// what the UI printed before i18n (en-US conventions), so nothing shifts.

/** BCP 47 tag for Intl: "en" stays en-US so English output is unchanged. */
export const intlLocale = (lng = currentLanguage()) => (lng === "en" ? "en-US" : lng);

/** date-fns locale object for the active language, or undefined (= date-fns' en-US default). */
export const dateFnsLocale = () => dateFnsLocales[currentLanguage()];

export function formatNumber(value, options) {
    const n = Number(value);
    if (!Number.isFinite(n)) return String(value ?? "");
    return new Intl.NumberFormat(intlLocale(), options).format(n);
}

/** USD by default (the cost model is in dollars); digits default to "up to 2". */
export function formatCurrency(value, currency = "USD", options = {}) {
    const n = Number(value || 0);
    return new Intl.NumberFormat(intlLocale(), {
        style: "currency",
        currency,
        minimumFractionDigits: 0,
        maximumFractionDigits: 2,
        ...options,
    }).format(Number.isFinite(n) ? n : 0);
}

/** Date + time by default (same as Date#toLocaleString), or pass Intl.DateTimeFormat options. */
export function formatDate(value, options) {
    const d = value instanceof Date ? value : new Date(value);
    if (Number.isNaN(d.getTime())) return String(value ?? "");
    return options ? new Intl.DateTimeFormat(intlLocale(), options).format(d) : d.toLocaleString(intlLocale());
}

const RELATIVE_UNITS = [
    ["year", 365 * 24 * 3600],
    ["month", 30 * 24 * 3600],
    ["week", 7 * 24 * 3600],
    ["day", 24 * 3600],
    ["hour", 3600],
    ["minute", 60],
    ["second", 1],
];

/** "3 hours ago" / "vor 3 Stunden" via Intl.RelativeTimeFormat. */
export function formatRelative(value, { now = Date.now(), style = "long", numeric = "always" } = {}) {
    const d = value instanceof Date ? value : new Date(value);
    if (Number.isNaN(d.getTime())) return String(value ?? "");
    const diff = Math.round((d.getTime() - now) / 1000);
    const abs = Math.abs(diff);
    const [unit, secs] = RELATIVE_UNITS.find(([, s]) => abs >= s) || ["second", 1];
    return new Intl.RelativeTimeFormat(intlLocale(), { style, numeric }).format(Math.round(diff / secs), unit);
}

export default i18n;
