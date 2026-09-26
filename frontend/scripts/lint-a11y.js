#!/usr/bin/env node
/*
 * Accessibility guard for the conventions in docs/DESIGN-SYSTEM.md ("Accessibility
 * conventions"). Parses JSX (so multi-line tags are handled) under src/pages and
 * src/components and fails on:
 *   1. scroll containers (overflow-auto / -x-auto / -y-auto / -scroll) with no
 *      tabIndex — keyboard users cannot scroll them (axe: scrollable-region-focusable)
 *   2. icon-only buttons / button-styled links with no accessible name
 *      (no aria-label / aria-labelledby / title and no visible text)
 * This complements, not replaces, the axe-core sweep (colour contrast, label
 * association etc. need a rendered DOM). Add `// a11y-ok: <reason>` on the line above
 * an element to silence a deliberate exception.
 */
const fs = require("fs");
const path = require("path");
const parser = require("@babel/parser");

const ROOT = path.join(__dirname, "..", "src");
const DIRS = ["pages", "components"];
// shadcn primitives under components/ui own their semantics, except table.jsx whose scroll wrapper we check
const SKIP = (f) => f.split(path.sep).includes("ui") && !f.endsWith("table.jsx");
const SCROLL = /\boverflow-(?:x-|y-)?(?:auto|scroll)\b/;

const walkFiles = (dir) =>
  fs.readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const p = path.join(dir, e.name);
    return e.isDirectory() ? walkFiles(p) : /\.jsx$/.test(e.name) ? [p] : [];
  });

const nameOf = (n) => (n.type === "JSXIdentifier" ? n.name : n.type === "JSXMemberExpression" ? `${nameOf(n.object)}.${n.property.name}` : "");
const attr = (el, name) => el.attributes.find((a) => a.type === "JSXAttribute" && a.name.name === name);
const classText = (el, src) => {
  const a = attr(el, "className");
  return a ? src.slice(a.start, a.end) : "";
};
// visible text = literal text, or a text-ish expression; icon elements (Capitalised, self-closing) don't count
const hasVisibleText = (node, src) =>
  node.children.some((c) => {
    if (c.type === "JSXText") return c.value.trim().length > 0;
    if (c.type === "JSXExpressionContainer") {
      const t = src.slice(c.start, c.end);
      return c.expression.type !== "JSXEmptyExpression" && !/^\{\s*<\s*[A-Z]/.test(t) && !/Icon\b/.test(t);
    }
    if (c.type === "JSXElement") {
      const n = nameOf(c.openingElement.name);
      if (/^[A-Z]/.test(n)) return false; // component (icons, badges) — not counted as text
      if (/sr-only/.test(classText(c.openingElement, src))) return true;
      return hasVisibleText(c, src);
    }
    return false;
  });

let problems = 0;
const report = (file, line, msg) => {
  problems++;
  console.log(`✗ ${path.relative(path.join(ROOT, ".."), file)}:${line}  ${msg}`);
};

for (const d of DIRS) {
  for (const file of walkFiles(path.join(ROOT, d))) {
    if (SKIP(file)) continue;
    const src = fs.readFileSync(file, "utf8");
    const lines = src.split("\n");
    const ast = parser.parse(src, { sourceType: "module", plugins: ["jsx"] });
    const ok = (line) => /a11y-ok/.test(lines[line - 2] || "");
    (function walk(n) {
      if (!n || typeof n.type !== "string") return;
      if (n.type === "JSXElement") {
        const el = n.openingElement;
        const tag = nameOf(el.name);
        const line = el.loc.start.line;
        const cls = classText(el, src);
        if (!ok(line)) {
          if (SCROLL.test(cls) && !attr(el, "tabIndex"))
            report(file, line, `scroll container <${tag}> needs tabIndex={0} role="region" aria-label="…" (keyboard scrolling)`);
          const isButton = tag === "button" || tag === "Button" || ((tag === "Link" || tag === "a") && /\bbtn\b/.test(cls));
          if (isButton) {
            const named = ["aria-label", "aria-labelledby", "title"].some((a) => attr(el, a));
            if (!named && !hasVisibleText(n, src)) report(file, line, `icon-only <${tag}> needs aria-label (and a title tooltip)`);
          }
        }
      }
      for (const k of Object.keys(n)) {
        if (k === "loc") continue;
        const c = n[k];
        if (Array.isArray(c)) c.forEach(walk);
        else if (c && typeof c.type === "string") walk(c);
      }
    })(ast.program);
  }
}

if (problems) {
  console.log(`\n${problems} accessibility problem(s). See docs/DESIGN-SYSTEM.md → Accessibility conventions.`);
  process.exit(1);
}
console.log("✓ a11y lint clean");
