// Renders the social card. Usage: node scripts/site-og.mjs <path-to-playwright-module>
// Writes site/og.png (1200x630: Pages, X, LinkedIn) and site/social-preview.png (1280x640: GitHub repo social preview).
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import path from "node:path";
import fs from "node:fs";
const { chromium } = createRequire(import.meta.url)(process.argv[2]);
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const shot = "data:image/png;base64," + fs.readFileSync(path.join(root, "site/shots/overview.png")).toString("base64");
const html = (w, h) => `<!doctype html><meta charset="utf-8"><style>
*{box-sizing:border-box;margin:0}
body{width:${w}px;height:${h}px;overflow:hidden;background:#000;color:#f5f5f7;font-family:-apple-system,BlinkMacSystemFont,"SF Pro Display","Helvetica Neue",Inter,Arial,sans-serif;position:relative}
.glow{position:absolute;inset:0;background:
 radial-gradient(60% 70% at 78% 40%,rgba(142,108,255,.42),transparent 70%),
 radial-gradient(50% 60% at 15% 0%,rgba(41,151,255,.38),transparent 70%),
 radial-gradient(40% 40% at 100% 100%,rgba(240,171,252,.18),transparent 70%)}
.brand{position:absolute;left:${w * 0.06}px;top:${h * 0.115}px;display:flex;align-items:center;gap:14px;font-size:${h * 0.05}px;font-weight:600;letter-spacing:-.01em}
.mark{width:${h * 0.085}px;height:${h * 0.085}px;border-radius:${h * 0.02}px;background:linear-gradient(135deg,#2997ff,#8e6cff);display:grid;place-items:center;font-weight:800;font-size:${h * 0.05}px;color:#fff}
h1{position:absolute;left:${w * 0.06}px;top:${h * 0.27}px;font-size:${h * 0.122}px;line-height:1.03;letter-spacing:-.045em;font-weight:700;width:${w * 0.6}px;white-space:nowrap}
h1 em{font-style:normal;background:linear-gradient(100deg,#7cc4ff,#a78bfa 55%,#f0abfc);-webkit-background-clip:text;background-clip:text;color:transparent}
.sub{position:absolute;left:${w * 0.06}px;top:${h * 0.6}px;font-size:${h * 0.04}px;color:#a1a1a6;width:${w * 0.42}px;line-height:1.25;letter-spacing:-.01em}
.chips{position:absolute;left:${w * 0.06}px;bottom:${h * 0.09}px;display:flex;gap:10px;flex-wrap:nowrap}
.chips span{border:1px solid rgba(255,255,255,.22);background:rgba(255,255,255,.06);border-radius:99px;padding:${h * 0.012}px ${h * 0.026}px;font-size:${h * 0.028}px;color:#d2d2d7;white-space:nowrap}
.dev{position:absolute;left:${w * 0.585}px;top:${h * 0.2}px;width:${w * 0.62}px;border-radius:${h * 0.03}px;overflow:hidden;border:1px solid rgba(255,255,255,.2);background:#111;transform:perspective(1600px) rotateY(-14deg) rotateX(5deg) rotate(1.5deg);transform-origin:left center;box-shadow:0 60px 140px -20px rgba(142,108,255,.55),0 30px 60px rgba(0,0,0,.7)}
.dev .bar{height:${h * 0.05}px;background:#2a2a2d;display:flex;align-items:center;gap:7px;padding-left:${h * 0.025}px}
.dev .bar i{width:${h * 0.016}px;height:${h * 0.016}px;border-radius:50%;display:block}
.dev img{display:block;width:100%}
.fade{position:absolute;inset:0;background:linear-gradient(90deg,rgba(0,0,0,.0) 55%,rgba(0,0,0,0) 100%)}
</style><body><div class="glow"></div>
<div class="brand"><div class="mark">A</div>Zyvor Axiom</div>
<h1>Your cluster.<br><em>Finally legible.</em></h1>
<div class="sub">The private-cloud console for k3s and Kubernetes. One Go binary.</div>
<div class="chips"><span>k3s &amp; Kubernetes</span><span>Argo CD · Flux · Helm</span><span>8 languages</span></div>
<div class="dev"><div class="bar"><i style="background:#ff5f57"></i><i style="background:#febc2e"></i><i style="background:#28c840"></i></div><img src="${shot}"></div>
</body>`;
const browser = await chromium.launch();
for (const [w, h, file] of [[1200, 630, "site/og.png"], [1280, 640, "site/social-preview.png"]]) {
  const page = await (await browser.newContext({ viewport: { width: w, height: h }, deviceScaleFactor: 1 })).newPage();
  await page.setContent(html(w, h));
  await page.waitForLoadState("load");
  await page.waitForTimeout(400);
  await page.screenshot({ path: path.join(root, file), clip: { x: 0, y: 0, width: w, height: h } });
  console.log("wrote", file);
}
await browser.close();
