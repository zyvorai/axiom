#!/usr/bin/env node
// i18n guard (npm run lint:i18n, part of npm run lint). See src/locales/README.md.
//
// FAILS when, compared with English (src/locales/en, the source of truth):
//   - a language is missing a key, or has a key English does not have
//   - a translation's {{placeholders}} or <tags> (react-i18next <Trans>) differ
//   - a plural set does not match the language's CLDR categories: `x_other` is
//     always required, `x_one` only where CLDR has "one" (not ja/zh), and only
//     the language's own categories may appear (`x_few` is fine for pl, not de)
//   - a JSON file does not parse, or a value is not a string
//   - a static t("ns:key") / i18nKey="ns:key" in src/ names a key English lacks
// WARNS (never fails) about likely-unconverted UI text: JSX text and
// placeholder/aria-label/title/label attributes that are plain literals.
"use strict";
const fs = require("fs");
const path = require("path");

const ROOT = path.join(__dirname, "..");
const LOCALES = path.join(ROOT, "src", "locales");
const SRC = path.join(ROOT, "src");
const BASE = "en";
const verbose = process.argv.includes("--verbose");

let errors = 0;
const fail = (m) => {
    errors++;
    console.log(`✗ ${m}`);
};

function flatten(obj, prefix, out, file) {
    for (const [k, v] of Object.entries(obj)) {
        const key = prefix ? `${prefix}.${k}` : k;
        if (v && typeof v === "object" && !Array.isArray(v)) flatten(v, key, out, file);
        else if (typeof v === "string") out[key] = v;
        else fail(`${file}: ${key} is not a string`);
    }
    return out;
}

function load(lang) {
    const dir = path.join(LOCALES, lang);
    const res = {};
    for (const f of fs.readdirSync(dir).filter((f) => f.endsWith(".json")).sort()) {
        const ns = f.replace(/\.json$/, "");
        try {
            res[ns] = flatten(JSON.parse(fs.readFileSync(path.join(dir, f), "utf8")), "", {}, `${lang}/${f}`);
        } catch (e) {
            fail(`${lang}/${f}: ${e.message}`);
            res[ns] = {};
        }
    }
    return res;
}

const tokens = (s) =>
    [...new Set((s.match(/\{\{\s*[\w.]+\s*\}\}|<\/?[\w]+\s*\/?>/g) || []).map((x) => x.replace(/\s+/g, "")))].sort().join(" ");

const langs = fs
    .readdirSync(LOCALES)
    .filter((d) => fs.statSync(path.join(LOCALES, d)).isDirectory())
    .sort((a, b) => (a === BASE ? -1 : b === BASE ? 1 : a.localeCompare(b)));
const all = Object.fromEntries(langs.map((l) => [l, load(l)]));
const en = all[BASE];
const enCount = Object.values(en).reduce((n, ns) => n + Object.keys(ns).length, 0);

// plural sets, per the language's CLDR categories (Intl.PluralRules)
const PLURAL_RE = /^(.*)_(zero|one|two|few|many|other)$/;
const categories = (lang) => new Set(new Intl.PluralRules(lang).resolvedOptions().pluralCategories);
// "many" is optional everywhere: i18n.js fills it from `_other` at load time.
const requiredPlural = (cat, cats) => cat === "other" || (cat === "one" && cats.has("one"));
for (const lang of langs) {
    const cats = categories(lang);
    for (const [ns, keys] of Object.entries(all[lang])) {
        for (const k of Object.keys(keys)) {
            const m = k.match(PLURAL_RE);
            if (!m) continue;
            if (!cats.has(m[2])) fail(`${lang}/${ns}.json: ${k}: "${m[2]}" is not a CLDR plural category for ${lang} (${[...cats].join(", ")})`);
            for (const c of cats) if (requiredPlural(c, cats) && !(`${m[1]}_${c}` in keys)) fail(`${lang}/${ns}.json: ${k} has no ${m[1]}_${c}`);
        }
    }
}

// key sets + placeholders against English
const summary = [];
for (const lang of langs) {
    if (lang === BASE) continue;
    const cur = all[lang];
    const cats = categories(lang);
    let have = 0;
    let expected = 0;
    for (const ns of new Set([...Object.keys(en), ...Object.keys(cur)])) {
        const e = en[ns] || {};
        const o = cur[ns];
        if (!o) {
            fail(`${lang}: missing namespace file ${ns}.json`);
            continue;
        }
        if (!en[ns]) fail(`${lang}: namespace ${ns}.json does not exist in ${BASE}`);
        for (const k of Object.keys(e)) {
            const pm = k.match(PLURAL_RE);
            if (pm && !cats.has(pm[2])) continue; // e.g. English `_one` in ja/zh: that form does not exist there
            expected++;
            if (!(k in o)) {
                fail(`${lang}/${ns}.json: missing ${k}`);
                continue;
            }
            have++;
            if (tokens(e[k]) !== tokens(o[k])) fail(`${lang}/${ns}.json: ${k} placeholders/tags [${tokens(o[k])}] differ from ${BASE} [${tokens(e[k])}]`);
        }
        for (const k of Object.keys(o)) {
            if (k in e) continue;
            const pm = k.match(PLURAL_RE);
            if (pm && `${pm[1]}_other` in e && cats.has(pm[2])) {
                // a plural form English lacks (`_few`, `_many`, …): same tokens as English `_other`
                if (tokens(e[`${pm[1]}_other`]) !== tokens(o[k])) fail(`${lang}/${ns}.json: ${k} placeholders/tags [${tokens(o[k])}] differ from ${BASE} ${pm[1]}_other [${tokens(e[`${pm[1]}_other`])}]`);
                continue;
            }
            fail(`${lang}/${ns}.json: extra key ${k} (not in ${BASE})`);
        }
    }
    // "translated" = present and not just a copy of the English text (copies are fine
    // for Kubernetes terms, but they are reported so the percentage stays honest)
    let same = 0;
    for (const [ns, e] of Object.entries(en)) for (const [k, v] of Object.entries(e)) if (cur[ns] && cur[ns][k] === v) same++;
    summary.push({ lang, pct: expected ? (100 * have) / expected : 100, same });
}

// static keys used in source must exist in English
function walk(dir, out = []) {
    for (const f of fs.readdirSync(dir)) {
        const p = path.join(dir, f);
        if (fs.statSync(p).isDirectory()) {
            if (f !== "locales") walk(p, out);
        } else if (/\.(jsx?|tsx?)$/.test(f)) out.push(p);
    }
    return out;
}
const files = walk(SRC);
const hasKey = (ns, key) => en[ns] && (key in en[ns] || `${key}_one` in en[ns] || `${key}_other` in en[ns]);
let used = 0;
for (const f of files) {
    const src = fs.readFileSync(f, "utf8");
    const re = /(?:\bt|i18n\.t)\(\s*(["'])([\w-]+):([\w.\- ]+)\1|i18nKey=(?:\{\s*)?(["'])([\w-]+):([\w.\- ]+)\4/g;
    let m;
    while ((m = re.exec(src))) {
        const ns = m[2] || m[5];
        const key = m[3] || m[6];
        used++;
        if (!hasKey(ns, key)) fail(`${path.relative(ROOT, f)}: t("${ns}:${key}") has no ${BASE} string`);
    }
}

// heuristic scan for unconverted literals (warnings only)
const ALLOW = [
    /^(Axiom|Zyvor|Atlas|Cilium|Hubble|KubeVirt|Velero|Helm|PacketWolf|Prometheus|Grafana|Postgres|GitHub|Slack|OIDC|SAML|SSO|YAML|JSON|CSV|kubectl|kubeconfig)\b/i,
    /^[A-Z][A-Za-z]+(Set|Map|Job|Policy|Binding|Role|Claim|Class|Account|Resource|Definition)s?$/, // Kubernetes kinds
    /^(Pod|Node|Namespace|Deployment|Service|Ingress|Secret|ConfigMap|CronJob|HPA|PVC|CRD|RBAC|API|URL|CPU|GPU|TCP|UDP|ICMP|HTTP|HTTPS|TLS|DNS|IP|ID|VM)s?$/,
    /^[a-z0-9]+([-_./:][a-z0-9]+)+$/, // identifiers, paths, labels
    // kept English by design (GLOSSARY.md): API terms and values, product names, units
    /^(Auto|TriggerAuthentication|Argo CD|Flux|OCI|Gateway API|StorageClasses|PersistentVolumes|ReadWrite(Once|Many|OncePod)|ReadOnlyMany|(Prefer)?NoSchedule|NoExecute|\/?GiB|vCPU|RAM)$/,
    /^,\s*\w+:$/, // object-literal fragments after an interpolation (`}, mono:`), not text
];
const SKIP_FILES = /[\\/]components[\\/]ui[\\/]/; // shadcn primitives
const warnings = [];
for (const f of files) {
    if (SKIP_FILES.test(f) || !/\.jsx$/.test(f)) continue;
    const lines = fs.readFileSync(f, "utf8").split("\n");
    lines.forEach((line, i) => {
        if (/^\s*(\/\/|\*|\/\*)/.test(line)) return;
        const hits = [];
        for (const m of line.matchAll(/>\s*([^<>{}]*?[A-Za-z]{3,}[^<>{}]*?)\s*</g)) hits.push(m[1]);
        const t = line.trim();
        if (/^[A-Za-z][^<>{}=;]*[A-Za-z.…?!:)]$/.test(t) && /[A-Za-z]{3,}/.test(t) && !/^(import|export|const|let|return|if|else|function|case|default)\b/.test(t) && !/[()[\]]$/.test(t) && !/^\w+$/.test(t) && !/^[\w.]+\s*[?:|&]/.test(t)) hits.push(t);
        for (const m of line.matchAll(/\b(placeholder|aria-label|title|label|alt|description)="([^"]*[A-Za-z]{3,}[^"]*)"/g)) hits.push(m[2]);
        // toast("…") and confirm({ title: "…" }) style literals
        for (const m of line.matchAll(/\btoast(?:\.\w+)?\(\s*["`]([^"`$]*[A-Za-z]{3,}[^"`$]*)["`]/g)) hits.push(m[1]);
        for (const m of line.matchAll(/\b(title|description|confirmLabel|cancelLabel|label):\s*"([^"]*[A-Z][a-z]+ [^"]*)"/g)) hits.push(m[2]);
        for (const h of hits) {
            const s = h.trim();
            if (!s || ALLOW.some((r) => r.test(s))) continue;
            if (/&&|\|\||[!=]==|=>|^\w+\.\w+/.test(s)) continue; // code (conditions, member access), not text
            if (/^https?:\/\//.test(s) || /^[a-z0-9-]+(, [a-z0-9-]+)*$/.test(s)) continue; // URLs, sample identifiers
            warnings.push(`${path.relative(ROOT, f)}:${i + 1}: ${s.length > 70 ? s.slice(0, 67) + "…" : s}`);
        }
    });
}

console.log(`i18n: ${langs.length} languages, ${enCount} English strings in ${Object.keys(en).length} namespaces, ${used} static key uses in src/`);
for (const s of summary) console.log(`  ${s.lang}: ${s.pct.toFixed(1)}% complete (${s.same} strings identical to English)`);
if (warnings.length) {
    const byFile = {};
    for (const w of warnings) byFile[w.split(":")[0]] = (byFile[w.split(":")[0]] || 0) + 1;
    console.log(`⚠ ${warnings.length} possible unconverted UI literal(s) in ${Object.keys(byFile).length} file(s) (heuristic, not an error${verbose ? "" : "; --verbose lists them"}):`);
    if (verbose) for (const w of warnings) console.log(`    ${w}`);
    else
        for (const [f, n] of Object.entries(byFile).sort((a, b) => b[1] - a[1]).slice(0, 15)) console.log(`    ${String(n).padStart(4)}  ${f}`);
}
if (errors) {
    console.log(`✗ i18n lint failed with ${errors} error(s)`);
    process.exit(1);
}
console.log("✓ i18n lint clean");
