#!/usr/bin/env python3
"""Build the GitHub Pages site: copies site/ and renders docs/*.md to
docs/*.html with the shared Apple-style layout.  Usage: build-site.py OUT_DIR
Needs: pip install markdown"""
import html, re, shutil, sys
from pathlib import Path
import markdown

ROOT = Path(__file__).resolve().parent.parent
OUT = Path(sys.argv[1] if len(sys.argv) > 1 else ROOT / "_site")
REPO = "https://github.com/zyvorai/axiom/blob/main/"

# (markdown file, slug, nav title, blurb, group)
PAGES = [
    ("QUICKSTART.md", "quickstart", "Quickstart", "Install with Helm and sign in within minutes.", "Start"),
    ("DEPLOYMENT.md", "deployment", "Deployment", "Manifests, TLS, the k3s lab flow and the production checklist.", "Start"),
    ("ARCHITECTURE.md", "architecture", "Architecture", "How the binary, providers and auth fit together.", "Understand"),
    ("API.md", "api", "API reference", "Every REST endpoint, role and response shape.", "Understand"),
    ("BACKUP-RESTORE.md", "backup-restore", "Backup & restore", "Protect Axiom’s own configuration and cluster state.", "Operate"),
    ("COMPLIANCE.md", "compliance", "Compliance", "Controls, evidence and audit guidance.", "Operate"),
    ("ROADMAP.md", "roadmap", "Roadmap", "What shipped, what is next, and how Axiom compares.", "Understand"),
    ("GAPS.md", "gaps", "Known limitations", "What is intentionally missing or still open.", "Operate"),
    ("VERIFICATION.md", "verification", "Verification checklist", "What has been proven on real systems, and what to check on yours.", "Operate"),
    ("DESIGN-SYSTEM.md", "design-system", "Design system", "Tokens, components, accessibility and translations of the console UI.", "Understand"),
    ("SUBSCRIPTION-MODEL.md", "plans", "Plans & licensing", "The Zyvor Production License and enterprise subscription plans.", "Start"),
]
SLUG = {p[0]: p[1] for p in PAGES}

def slugify(v, sep="-"):
    return re.sub(r"[^a-z0-9]+", sep, v.lower()).strip(sep)

def fix_links(body):
    def sub(m):
        href = m.group(1)
        if re.match(r"^(https?:|mailto:|#)", href):
            return m.group(0)
        path, _, frag = href.partition("#")
        name = Path(path).name
        if name in SLUG:
            return 'href="%s.html%s"' % (SLUG[name], "#" + frag if frag else "")
        if path:
            return 'href="%s%s"' % (REPO + "docs/" + path.lstrip("./") if not path.startswith("../") else REPO + path[3:], "#" + frag if frag else "")
        return m.group(0)
    return re.sub(r'href="([^"]*)"', sub, body)

def toc_of(md_html):
    items = re.findall(r'<h([23]) id="([^"]+)">(.*?)</h\1>', md_html, flags=re.S)
    out = []
    for lvl, i, t in items:
        t = re.sub(r"<[^>]+>", "", t)
        out.append('<a class="l%s" href="#%s">%s</a>' % (lvl, i, t))
    return "\n".join(out)

def layout(title, desc, slug, side, body, prevnext=""):
    return f"""<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{html.escape(title)} — Zyvor Axiom Docs</title>
<meta name="description" content="{html.escape(desc)}">
<meta name="theme-color" content="#000000">
<link rel="icon" href="../favicon.svg" type="image/svg+xml">
<link rel="stylesheet" href="../assets/site.css">
<meta property="og:title" content="{html.escape(title)} — Zyvor Axiom Docs">
<meta property="og:image" content="https://zyvorai.github.io/axiom/og.png">
<meta name="twitter:card" content="summary_large_image">
</head><body>
<a class="skip" href="#main">Skip to content</a>
<nav class="top"><div><b><a href="../">Axiom</a></b><span><a href="../#apps">Product</a><a class="on" href="./">Docs</a><a href="https://github.com/zyvorai/axiom/releases/latest">Download</a><a href="https://github.com/zyvorai/axiom">GitHub</a></span></div></nav>
<div class="docs">{side}<article id="main">{body}{prevnext}</article></div>
<footer><div class="in">© 2026 Zyvor AI Labs. Licensed under the <a href="https://github.com/zyvorai/axiom/blob/main/LICENSE">Zyvor Production License v1.0</a>. <a href="https://github.com/zyvorai/axiom/edit/main/docs/">Edit these docs on GitHub</a>.</div></footer>
</body></html>"""

def sidebar(active, toc=""):
    groups = {}
    for p in PAGES:
        groups.setdefault(p[4], []).append(p)
    s = '<aside class="side"><a href="./"%s>Overview</a>' % (' class="on"' if active == "" else "")
    for g, ps in groups.items():
        s += "<h4 style='margin-top:20px'>%s</h4>" % g
        for _, slug, title, _, _ in ps:
            s += '<a href="%s.html"%s>%s</a>' % (slug, ' class="on"' if slug == active else "", title)
    if toc:
        s += '<div class="toc"><h4>On this page</h4>%s</div>' % toc
    return s + "</aside>"

def main():
    if OUT.exists():
        shutil.rmtree(OUT)
    shutil.copytree(ROOT / "site", OUT)
    (OUT / "docs").mkdir(exist_ok=True)
    for i, (fn, slug, title, blurb, _) in enumerate(PAGES):
        md = markdown.Markdown(extensions=["fenced_code", "tables", "toc", "sane_lists"],
                               extension_configs={"toc": {"slugify": lambda v, s: slugify(v)}})
        text = (ROOT / "docs" / fn).read_text()
        text = re.sub(r"^# .*\n", "", text, count=1)  # title rendered by the layout
        raw = md.convert(text)
        body = fix_links(raw)
        group = PAGES[i][4]
        body = f'<p class="crumb">{group}</p><h1>{html.escape(title)}</h1>' + body
        pn = '<div class="pn">'
        pn += ('<a href="%s.html"><small>Previous</small>%s</a>' % (PAGES[i-1][1], PAGES[i-1][2])) if i else "<span></span>"
        pn += ('<a href="%s.html"><small>Next</small>%s</a>' % (PAGES[i+1][1], PAGES[i+1][2])) if i + 1 < len(PAGES) else "<span></span>"
        pn += "</div>"
        (OUT / "docs" / f"{slug}.html").write_text(layout(title, blurb, slug, sidebar(slug, toc_of(raw)), body, pn))
    cards = "".join('<a class="card" href="%s.html"><p class="k">%s</p><h3>%s</h3><p>%s</p></a>' % (s, g, t, b) for _, s, t, b, g in PAGES)
    hub = ('<p class="crumb">Documentation</p><h1>Learn Axiom.</h1>'
           '<p class="lead" style="text-align:left;margin:0">Everything from your first install to the production checklist.</p>'
           '<div class="hub">%s</div>' % cards)
    (OUT / "docs" / "index.html").write_text(layout("Documentation", "Guides and reference for Zyvor Axiom.", "", sidebar(""), hub))
    base = "https://zyvorai.github.io/axiom/"
    urls = [base] + [base + "docs/"] + [base + "docs/%s.html" % p[1] for p in PAGES]
    (OUT / "sitemap.xml").write_text('<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n'
                                     + "".join("  <url><loc>%s</loc></url>\n" % u for u in urls) + "</urlset>\n")
    (OUT / "robots.txt").write_text("User-agent: *\nAllow: /\nSitemap: %ssitemap.xml\n" % base)
    print("built", OUT, len(PAGES) + 1, "docs pages")

main()
