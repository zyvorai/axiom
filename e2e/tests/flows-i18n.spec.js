// @ts-check
// Languages: the UI renders in English (default), Deutsch, Español and Français,
// the choice is made from the language select (login page, user menu, Settings)
// and persists across reloads (localStorage "axiom.lang"); without a saved
// choice the browser language decides. Runs against a demo-mode server.
const { test, expect } = require("@playwright/test");
const { signIn } = require("./helpers");

// A few strings per language, straight from frontend/src/locales/<lang>/*.json.
const L = {
  en: { hero: "Sign in to Axiom.", continue: "Continue", username: "Username", overview: "Overview", events: "Events", reason: "Reason", workloads: "Workloads", shortcuts: "Keyboard shortcuts", settings: "Settings" },
  de: { hero: "Bei Axiom anmelden.", continue: "Weiter", username: "Benutzername", overview: "Übersicht", events: "Ereignisse", reason: "Grund", workloads: "Workloads", shortcuts: "Tastenkürzel", settings: "Einstellungen" },
  es: { hero: "Inicie sesión en Axiom.", continue: "Continuar", username: "Usuario", overview: "Resumen", events: "Eventos", reason: "Motivo", workloads: "Cargas de trabajo", shortcuts: "Atajos de teclado", settings: "Ajustes" },
  fr: { hero: "Connectez-vous à Axiom.", continue: "Continuer", username: "Nom d'utilisateur", overview: "Vue d'ensemble", events: "Événements", reason: "Raison", workloads: "Charges de travail", shortcuts: "Raccourcis clavier", settings: "Paramètres" },
};

// The login rate limiter counts the signed-out session probes that every login
// page load (and reload) makes. A valid request clears this IP's counter, so
// clear it around each test: before signing in, and after for the next spec.
async function clearThrottle(page) {
  for (let i = 0; i < 70; i++) {
    const r = await page.request.get("/api/v1/session", { headers: { Authorization: `Basic ${Buffer.from("demo:demo").toString("base64")}` } });
    if (r.status() !== 429) return;
    await new Promise((res) => setTimeout(res, Math.max(1000, Number(r.headers()["retry-after"] || 1) * 1000)));
  }
}
test.beforeEach(async ({ page }) => clearThrottle(page));
test.afterEach(async ({ page }) => clearThrottle(page));

test("login page: English by default, switching language translates it and persists across reload", async ({ page }) => {
  await page.goto("/login");
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  await expect(page.getByRole("heading", { name: L.en.hero })).toBeVisible();
  const select = page.getByTestId("language-select");
  await expect(select).toHaveValue("en");

  for (const lang of ["de", "es", "fr"]) {
    await select.selectOption(lang);
    await expect(page.getByRole("heading", { name: L[lang].hero })).toBeVisible();
    await expect(page.getByRole("button", { name: L[lang].continue, exact: true })).toBeVisible();
    await expect(page.getByPlaceholder(L[lang].username)).toBeVisible();
    await expect(page.locator("html")).toHaveAttribute("lang", lang);
  }

  await page.reload();
  await expect(page.getByRole("heading", { name: L.fr.hero })).toBeVisible();
  await expect(page.getByTestId("language-select")).toHaveValue("fr");
  expect(await page.evaluate(() => localStorage.getItem("axiom.lang"))).toBe("fr");

  await page.getByTestId("language-select").selectOption("en");
  await expect(page.getByRole("heading", { name: L.en.hero })).toBeVisible();
  await expect(page.getByRole("button", { name: "Continue", exact: true })).toBeVisible();
});

test.describe("browser language", () => {
  test.use({ locale: "de-AT" });
  test("is used when nothing is saved (de-AT -> Deutsch)", async ({ page }) => {
    await page.goto("/login");
    await expect(page.getByRole("heading", { name: L.de.hero })).toBeVisible();
    await expect(page.locator("html")).toHaveAttribute("lang", "de");
  });
});

for (const lang of ["de", "es", "fr"]) {
  test(`signed-in UI in ${lang}: shell, overview, a list page and a dialog; persists across reload`, async ({ page }) => {
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await signIn(page); // English by default

    // switch from the user menu
    await page.getByTestId("user-menu-trigger").click();
    await page.getByTestId("language-select").selectOption(lang);
    await page.keyboard.press("Escape");

    const s = L[lang];
    await expect(page.getByTestId("nav-link-overview")).toContainText(s.overview);
    await expect(page.getByTestId("nav-link-settings")).toContainText(s.settings);
    await expect(page.locator("html")).toHaveAttribute("lang", lang);

    // list page: heading + a column heading
    await page.getByTestId("nav-link-events").click();
    await expect(page.getByRole("heading", { level: 1, name: s.events })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: s.reason })).toBeVisible();

    await page.getByTestId("nav-link-workloads").click();
    await expect(page.getByRole("heading", { level: 1, name: s.workloads })).toBeVisible();

    // a dialog: keyboard shortcuts ("?")
    await page.locator("main").click({ position: { x: 5, y: 5 } });
    await page.keyboard.press("?");
    await expect(page.getByRole("dialog").getByRole("heading", { name: s.shortcuts })).toBeVisible();
    await page.keyboard.press("Escape");

    // persists across a reload
    await page.reload();
    await expect(page.getByRole("heading", { level: 1, name: s.workloads })).toBeVisible();
    await expect(page.getByTestId("nav-link-overview")).toContainText(s.overview);

    // back to English from Settings → Appearance
    await page.getByTestId("nav-link-settings").click();
    await page.getByTestId("language-select").selectOption("en");
    await expect(page.getByTestId("nav-link-overview")).toContainText("Overview");
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
    expect(errors).toEqual([]);
  });
}
