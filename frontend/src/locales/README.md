# UI translations

The UI uses [i18next](https://www.i18next.com/) with
[react-i18next](https://react.i18next.com/). Setup, language detection and the
formatting helpers live in `src/lib/i18n.js`.

```
src/locales/
  en/  common.json nav.json auth.json pages.json dialogs.json toasts.json errors.json  index.js
  de/  …same files…
  es/  fr/  pt/  it/  ja/  zh/  …
  GLOSSARY.md   terminology per language (read this before translating)
```

- **English is the source of truth.** Every key must exist in `en`. English is
  bundled with the main chunk and is the fallback for any missing key.
- **Other languages are lazy.** Each language is one webpack chunk, loaded when
  it is selected (`loadLanguage()`), together with its date-fns locale.
- **Language choice:** `localStorage["axiom.lang"]`, else the browser language
  (`de-AT` → `de`), else English. It can be changed from the user menu, the
  login page footer, and Settings → Appearance (`data-testid="language-select"`).

## Namespaces

| Namespace | What goes in it |
|---|---|
| `common` | Generic buttons (`actions.*`), states, field/column names (`fields.*`), status labels used by `StatusBadge` (`status.*`), counts |
| `nav` | Sidebar groups and items (by `navItems.js` key), top bar, command palette, shortcuts, user menu |
| `auth` | Login, account security / two-factor, MFA gate, Users page |
| `pages` | Per page, keyed by page: `pages.<page>.title`, `.description`, `.tabs.*`, `.columns.*`, and other page text |
| `dialogs` | Dialog and form text: `dialogs.<page>.<dialog>.*` |
| `toasts` | Toast messages: `toasts.<page>.*` |
| `errors` | Client-side errors and validation: `errors.<page>.*` (server error text is shown as sent) |

## Adding a string

1. Add it to `en/<namespace>.json` with the **exact** English wording.
2. Use it with a fully qualified key:
   ```jsx
   import { useTranslation, Trans } from "react-i18next";
   const { t } = useTranslation();
   <h2>{t("pages:pods.title")}</h2>
   toast.success(t("toasts:pods.deleted", { name }));            // "Deleted {{name}}"
   t("pages:pods.selected", { count })                            // pods.selected_one / pods.selected_other
   <Trans i18nKey="pages:pods.hint" components={{ code: <code className="mono" /> }} />  // "Run <code>kubectl</code>…"
   ```
   Outside components (helpers, class components) use `import i18n from "@/lib/i18n"`
   and call `i18n.t(...)` at render time — never at module top level.
3. Add the same key to every other language (`de`, `es`, `fr`, `pt`, `it`, `ja`,
   `zh`). Keep the same `{{placeholders}}` and `<tags>`. Plurals follow the
   language's CLDR categories: `_one` and `_other` everywhere except Japanese and
   Chinese, which have only `_other` (no `_one` key). `_many` (es/fr/pt/it) is
   optional: `_other` is used for it.
4. Run `npm run lint:i18n`.

Rules: never concatenate translated fragments into a sentence (use
interpolation); don't translate Kubernetes kinds, YAML, field paths, metric
names, `kubectl` commands or text from the API (see GLOSSARY.md); keep values
used in logic (option values, tab ids, test ids) in English and translate only
what is displayed. Numbers, money and dates go through `formatNumber`,
`formatCurrency`, `formatDate`, `formatRelative` (or `dateFnsLocale()` for
date-fns) from `@/lib/i18n`.

## Adding a namespace

Create `<ns>.json` in **every** language folder, import it in each
`<lang>/index.js`, and add it to `NAMESPACES` in `src/lib/i18n.js`.

## Adding a language

1. Copy `en/` to `<code>/` (ISO 639-1, e.g. `it`) and translate every value.
2. Add `{ code, name }` (native name) to `SUPPORTED_LANGUAGES` and a loader to
   `LOADERS` in `src/lib/i18n.js` (copy an existing line; it also loads the
   matching `date-fns/locale/<code>`).
3. Add a column to `GLOSSARY.md`.
4. `npm run lint:i18n` must pass (it checks every folder in `src/locales`).

## `npm run lint:i18n`

Part of `npm run lint`. It **fails** on a key missing from or extra in a
language, differing placeholders/tags, a plural set that does not
match the language's CLDR categories (via `Intl.PluralRules`), bad
JSON, or a `t("ns:key")` in `src/` whose key English lacks. It prints each
language's completion, and **warns** (never fails) about JSX text and
`placeholder`/`aria-label`/`title`/`label` attributes or toast/confirm strings
that still look like hard-coded English. `--verbose` lists every warning.
