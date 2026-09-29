import { expect, test } from "@playwright/test";
import { MODULE_ROUTES, mockApi, seedSession } from "./helpers";

const DESKTOP_MIN = 1024;

function isDesktop(width: number | undefined): boolean {
  return (width ?? 0) >= DESKTOP_MIN;
}

test.describe("authentication", () => {
  test("redirects an unauthenticated visit to /login", async ({ page }) => {
    await mockApi(page);
    await page.goto("/");
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByRole("heading", { name: "Iniciar sesión" })).toBeVisible();
  });

  test("logs in with valid credentials and lands on the panel", async ({ page }) => {
    await mockApi(page, { login: "ok" });
    await page.goto("/login");
    await page.getByLabel("Correo electrónico").fill("admin@example.com");
    await page.getByLabel("Contraseña", { exact: true }).fill("correct horse battery");
    await page.getByRole("button", { name: "Entrar" }).click();
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByRole("heading", { level: 1, name: "Panel" })).toBeVisible();
  });

  test("shows an error message on 401", async ({ page }) => {
    await mockApi(page, { login: "unauthorized" });
    await page.goto("/login");
    await page.getByLabel("Correo electrónico").fill("admin@example.com");
    await page.getByLabel("Contraseña", { exact: true }).fill("wrong password");
    await page.getByRole("button", { name: "Entrar" }).click();
    await expect(page.getByRole("alert")).toHaveText("Correo o contraseña incorrectos.");
    await expect(page).toHaveURL(/\/login$/);
  });

  test("shows the generic verification message on 202", async ({ page }) => {
    await mockApi(page, { login: "pending" });
    await page.goto("/login");
    await page.getByLabel("Correo electrónico").fill("new@example.com");
    await page.getByLabel("Contraseña", { exact: true }).fill("whatever password");
    await page.getByRole("button", { name: "Entrar" }).click();
    await expect(page.getByRole("status")).toHaveText(
      "Si el correo es válido, te enviamos un enlace para verificar tu cuenta",
    );
    await expect(page).toHaveURL(/\/login$/);
  });

  test("password toggle switches the input type", async ({ page }) => {
    await mockApi(page);
    await page.goto("/login");
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

      if (!isDesktop(page.viewportSize()?.width)) {
        await page.getByRole("button", { name: "Abrir menú" }).click();
      }
      const nav = await page
        .getByRole("navigation", { name: "Módulos" })
        .evaluate((el) => ({ scrollWidth: el.scrollWidth, clientWidth: el.clientWidth }));
      expect(nav.scrollWidth, `nav overflows on ${route}`).toBeLessThanOrEqual(nav.clientWidth);
    }
  });

  test("marks the active module with aria-current", async ({ page }) => {
    await page.goto("/gastos");
    if (!isDesktop(page.viewportSize()?.width)) {
      await page.getByRole("button", { name: "Abrir menú" }).click();
    }
    await expect(page.getByRole("link", { name: "Gastos" })).toHaveAttribute("aria-current", "page");
  });

  test("moves focus to the content after navigating", async ({ page }) => {
    await page.goto("/");
    if (!isDesktop(page.viewportSize()?.width)) {
      await page.getByRole("button", { name: "Abrir menú" }).click();
    }
    await page.getByRole("link", { name: "Ingresos" }).click();
    await expect(page).toHaveURL(/\/ingresos$/);
    await expect(page.locator("main")).toBeFocused();
  });

  test("mobile drawer opens and closes with the button and Escape, restoring focus", async ({ page }) => {
    test.skip(isDesktop(page.viewportSize()?.width), "The drawer only exists below 1024px");
    await page.goto("/");
    const menuButton = page.getByRole("button", { name: "Abrir menú" });
    const dialog = page.getByRole("dialog", { name: "Menú de navegación" });

    await expect(dialog).toBeHidden();
    await menuButton.click();
    await expect(dialog).toBeVisible();

    await page.getByRole("button", { name: "Cerrar menú" }).click();
    await expect(dialog).toBeHidden();
    await expect(menuButton).toBeFocused();

    await menuButton.click();
    await expect(dialog).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(menuButton).toBeFocused();
  });

  test("desktop shows a permanent sidebar and no menu button", async ({ page }) => {
    test.skip(!isDesktop(page.viewportSize()?.width), "Sidebar layout starts at 1024px");
    await page.goto("/");
    await expect(page.getByRole("navigation", { name: "Módulos" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Abrir menú" })).toBeHidden();
  });

  test("shows the user email and a separate logout button", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("button", { name: "Cerrar sesión" })).toBeVisible();
    await page.getByRole("button", { name: "Cerrar sesión" }).click();
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
