import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession } from "./helpers";
import { isDesktop, moreSheet, openMore, sidebar } from "./nav";

async function open(page: Page) {
  await mockApi(page);
  await seedSession(page);
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
}

/** The selector lives in the sidebar footer on desktop and in the Más sheet below lg. */
async function selector(page: Page) {
  if (isDesktop(page)) return sidebar(page).locator("xpath=..").getByRole("radiogroup", { name: "Tema" });
  await openMore(page);
  return moreSheet(page).getByRole("radiogroup", { name: "Tema" });
}

const html = (page: Page) => page.locator("html");
const themeColors = (page: Page) => page.locator('meta[name="theme-color"]').evaluateAll((els) => els.map((e) => (e as HTMLMetaElement).content));

test.describe("theme selector", () => {
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
    await group.getByText("Oscuro", { exact: true }).click();
    await expect(html(page)).toHaveAttribute("data-theme", "dark");
    expect(await themeColors(page)).toEqual(["#0B1120", "#0B1120"]);
    expect(await page.evaluate(() => localStorage.getItem("theme"))).toBe("dark");
    // The page itself is dark: the background token resolves to the dark one.
    expect(await page.evaluate(() => getComputedStyle(document.body).getPropertyValue("color-scheme") || getComputedStyle(document.documentElement).colorScheme)).toBe("dark");

    await page.reload();
    await expect(html(page)).toHaveAttribute("data-theme", "dark");
    group = await selector(page);
    await expect(group.getByRole("radio", { name: "Oscuro" })).toBeChecked();

    await group.getByText("Claro", { exact: true }).click();
    await expect(html(page)).toHaveAttribute("data-theme", "light");
    expect(await themeColors(page)).toEqual(["#F8FAFC", "#F8FAFC"]);

    await group.getByText("Automático", { exact: true }).click();
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
