import { expect, test } from "@playwright/test";
import { MODULE_ROUTES, goToPasswordStep, mockApi, seedSession } from "./helpers";
import { dock, isDesktop, moduleLink, moreButton, moreSheet, openMore, sidebar } from "./nav";

test.describe("authentication", () => {
  test("redirects an unauthenticated visit to /login", async ({ page }) => {
    await mockApi(page);
    await page.goto("/");
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByRole("heading", { name: "Iniciar sesión" })).toBeVisible();
  });

  test("password toggle switches the input type", async ({ page }) => {
    await mockApi(page);
    await goToPasswordStep(page);
    const input = page.getByLabel("Contraseña", { exact: true });
    await expect(input).toHaveAttribute("type", "password");
    await page.getByRole("button", { name: "Mostrar contraseña" }).click();
    await expect(input).toHaveAttribute("type", "text");
    await page.getByRole("button", { name: "Ocultar contraseña" }).click();
    await expect(input).toHaveAttribute("type", "password");
  });
});

test.describe("layout", () => {
  test.beforeEach(async ({ page }) => {
    await mockApi(page);
    await seedSession(page);
  });

  test("has no horizontal overflow on any module route", async ({ page }) => {
    for (const route of MODULE_ROUTES) {
      await page.goto(route);
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();

      const page_ = await page.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth,
        clientWidth: document.documentElement.clientWidth,
      }));
      expect(page_.scrollWidth, `page overflows on ${route}`).toBeLessThanOrEqual(page_.clientWidth);

      const nav = await (isDesktop(page) ? sidebar(page) : dock(page)).evaluate((el) => ({ scrollWidth: el.scrollWidth, clientWidth: el.clientWidth }));
      expect(nav.scrollWidth, `nav overflows on ${route}`).toBeLessThanOrEqual(nav.clientWidth);
    }
  });

  test("marks the active module with aria-current", async ({ page }) => {
    await page.goto("/gastos");
    await expect(await moduleLink(page, "Gastos")).toHaveAttribute("aria-current", "page");
  });

  test("moves focus to the content after navigating", async ({ page }) => {
    await page.goto("/");
    // The Panel's own Ingresos card is a link too: pick the one of the module menu.
    await (await moduleLink(page, "Ingresos")).click();
    await expect(page).toHaveURL(/\/ingresos$/);
    await expect(page.locator("main")).toBeFocused();
  });

  test("the \"Más\" sheet opens and closes with the close button and Escape, restoring focus", async ({ page }) => {
    test.skip(isDesktop(page), "The dock only exists below 1024px");
    await page.goto("/");
    await expect(moreSheet(page)).toBeHidden();
    await moreButton(page).click();
    await expect(moreSheet(page)).toBeVisible();

    await moreSheet(page).getByRole("button", { name: "Cerrar", exact: true }).click();
    await expect(moreSheet(page)).toBeHidden();
    await expect(moreButton(page)).toBeFocused();

    await moreButton(page).click();
    await expect(moreSheet(page)).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(moreSheet(page)).toBeHidden();
    await expect(moreButton(page)).toBeFocused();
  });

  test("desktop shows a permanent sidebar and no dock", async ({ page }) => {
    test.skip(!isDesktop(page), "Sidebar layout starts at 1024px");
    await page.goto("/");
    await expect(sidebar(page)).toBeVisible();
    await expect(dock(page)).toBeHidden();
    await expect(moreButton(page)).toBeHidden();
  });

  test("shows the user email and a separate logout button", async ({ page }) => {
    await page.goto("/");
    if (!isDesktop(page)) await openMore(page);
    await expect((isDesktop(page) ? page.locator("header") : moreSheet(page)).getByText("admin@example.com")).toBeVisible();
    await expect(page.getByRole("button", { name: "Cerrar sesión" })).toBeVisible();
    await page.getByRole("button", { name: "Cerrar sesión" }).click();
    // It asks first; Cancelar has the focus and leaves the session alone.
    const confirm = page.getByRole("dialog", { name: "¿Cerrar sesión?" });
    await expect(confirm).toBeVisible();
    await expect(confirm.getByRole("button", { name: "Cancelar" })).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(confirm).toBeHidden();
    await expect(page).not.toHaveURL(/\/login$/);
    await page.getByRole("button", { name: "Cerrar sesión" }).click();
    await page.getByRole("dialog", { name: "¿Cerrar sesión?" }).getByRole("button", { name: "Cerrar sesión" }).click();
    await expect(page).toHaveURL(/\/login$/);
  });

  test("skip link moves focus to the main content", async ({ page }) => {
    await page.goto("/");
    await page.keyboard.press("Tab");
    const skip = page.getByRole("link", { name: "Saltar al contenido" });
    await expect(skip).toBeFocused();
    await expect(skip).toBeVisible();
    await page.keyboard.press("Enter");
    await expect(page.locator("main")).toBeFocused();
  });
});
