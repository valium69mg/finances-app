import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession } from "./helpers";
import { isDesktop, moreSheet, openMore, sidebar } from "./nav";

async function open(page: Page) {
  await mockApi(page);
  await seedSession(page);
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
}

/** The selector is an icon-only control in the top navbar on desktop and lives in the Más sheet below lg. */
async function selector(page: Page) {
  if (isDesktop(page)) return page.getByRole("banner").getByRole("radiogroup", { name: "Tema" });
  await openMore(page);
  return moreSheet(page).getByRole("radiogroup", { name: "Tema" });
}

const html = (page: Page) => page.locator("html");
const themeColors = (page: Page) => page.locator('meta[name="theme-color"]').evaluateAll((els) => els.map((e) => (e as HTMLMetaElement).content));

test.describe("theme selector", () => {
  test("on desktop it sits in the top navbar between the email and Cerrar sesión, not in the sidebar", async ({ page }) => {
    test.skip(!isDesktop(page), "desktop layout only");
    await open(page);
    const bar = page.getByRole("banner");
    const group = bar.getByRole("radiogroup", { name: "Tema" });
    await expect(group).toBeVisible();
    await expect(sidebar(page).locator("xpath=..").getByRole("radiogroup", { name: "Tema" })).toHaveCount(0);
    const [email, theme, logout] = await Promise.all([bar.locator("span.truncate").boundingBox(), group.boundingBox(), bar.getByRole("button", { name: "Cerrar sesión" }).boundingBox()]);
    expect(theme?.x ?? 0).toBeGreaterThan((email?.x ?? 0) + (email?.width ?? 0) - 1);
    expect((theme?.x ?? 0) + (theme?.width ?? 0)).toBeLessThan(logout?.x ?? 0);
  });


  test("offers Automático, Claro and Oscuro, Automático by default", async ({ page }) => {
    await open(page);
    const group = await selector(page);
    await expect(group.getByRole("radio")).toHaveCount(3);
    await expect(group.getByRole("radio", { name: "Automático" })).toBeChecked();
    await expect(html(page)).not.toHaveAttribute("data-theme", /.+/);
  });

  test("sets data-theme and the theme-color, and persists across reloads", async ({ page }) => {
    await open(page);
    let group = await selector(page);
    await group.locator("label", { hasText: "Oscuro" }).click();
    await expect(html(page)).toHaveAttribute("data-theme", "dark");
    expect(await themeColors(page)).toEqual(["#0B1120", "#0B1120"]);
    expect(await page.evaluate(() => localStorage.getItem("theme"))).toBe("dark");
    // The page itself is dark: the background token resolves to the dark one.
    expect(await page.evaluate(() => getComputedStyle(document.body).getPropertyValue("color-scheme") || getComputedStyle(document.documentElement).colorScheme)).toBe("dark");

    await page.reload();
    await expect(html(page)).toHaveAttribute("data-theme", "dark");
    group = await selector(page);
    await expect(group.getByRole("radio", { name: "Oscuro" })).toBeChecked();

    await group.locator("label", { hasText: "Claro" }).click();
    await expect(html(page)).toHaveAttribute("data-theme", "light");
    expect(await themeColors(page)).toEqual(["#F8FAFC", "#F8FAFC"]);

    await group.locator("label", { hasText: "Automático" }).click();
    await expect(html(page)).not.toHaveAttribute("data-theme", /.+/);
    expect(await themeColors(page)).toEqual(["#F8FAFC", "#0B1120"]);
    expect(await page.evaluate(() => localStorage.getItem("theme"))).toBeNull();
  });

  test("a stored theme is applied before the app renders", async ({ page }) => {
    await mockApi(page);
    await seedSession(page);
    await page.addInitScript(() => localStorage.setItem("theme", "dark"));
    // Hold the app bundle back: only public/theme-init.js can have set the attribute.
    await page.route("**/src/main.tsx*", (route) => route.abort());
    await page.goto("/");
    await expect(html(page)).toHaveAttribute("data-theme", "dark");
    await expect(page.locator("#root")).toBeEmpty();
  });

  test("is operable with the keyboard", async ({ page }) => {
    await open(page);
    const group = await selector(page);
    await group.getByRole("radio", { name: "Automático" }).focus();
    await page.keyboard.press("ArrowRight");
    await expect(html(page)).toHaveAttribute("data-theme", "light");
    await page.keyboard.press("ArrowRight");
    await expect(html(page)).toHaveAttribute("data-theme", "dark");
  });
});

test.describe("field outlines", () => {
  for (const [scheme, rgb] of [
    ["light", "rgb(128, 143, 165)"],
    ["dark", "rgb(110, 125, 148)"],
  ] as const) {
    test(`inputs and selects use the stronger outline token in ${scheme}`, async ({ page }) => {
      await page.emulateMedia({ colorScheme: scheme });
      await mockApi(page);
      await seedSession(page);
      await page.goto("/gastos");
      await expect(page.getByRole("heading", { level: 1, name: "Gastos" })).toBeVisible();
      await page.getByLabel("Mes", { exact: true }).waitFor();
      const colors = await page.locator("main input, main select").evaluateAll((els) => els.map((e) => getComputedStyle(e).borderTopColor));
      expect(colors.length).toBeGreaterThan(0);
      for (const c of colors) expect(c).toBe(rgb);
      // Decorative dividers keep the quiet border token.
      const divider = await page.locator("main > section > div").first().evaluate((e) => getComputedStyle(e).borderTopColor);
      expect(divider).not.toBe(rgb);
    });
  }
});
