import { expect, test, type Page } from "@playwright/test";
import { API_ORIGIN, MODULE_ROUTES, mockApi, seedSession, type MockExpense } from "./helpers";

const isDesktop = (width: number | undefined) => (width ?? 0) >= 1024;

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

const pad = (n: number) => String(n).padStart(2, "0");
const today = () => {
  const d = new Date();
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
};

/** A Renta expense above its 12,000.50 budget so the exceeded flag shows. */
const RENT: MockExpense = {
  id: 1,
  date: today(),
  description: "Renta",
  category: "Renta",
  payment_method: "Transferencia",
  currency: "MXN",
  amount: "13000.00",
  exchange_rate: null,
  amount_mxn: "13000.00",
};

async function openAsHousehold(page: Page, path = "/") {
  const api = await mockApi(page, { role: "household", expenses: [RENT] });
  await seedSession(page, "household");
  await page.goto(path);
  return api;
}

test.describe("household session", () => {
  test("the dashboard shows only the budget by category", async ({ page }) => {
    const api = await openAsHousehold(page);
    await expect(page.getByRole("heading", { level: 1, name: "Panel" })).toBeVisible();
    await expect(page.getByRole("heading", { level: 2, name: "Presupuesto por categoría" })).toBeVisible();

    // Both layouts of the budget are in the DOM; one is hidden by the viewport.
    const renta =
      (page.viewportSize()?.width ?? 0) >= 768 ? page.getByRole("row", { name: /Renta/ }) : page.getByTestId("budget-card").filter({ hasText: "Renta" });
    await expect(renta).toContainText("$13,000.00");
    await expect(renta).toContainText(/excedido/i);

    // Nothing else of the owner's dashboard is rendered.
    for (const heading of ["Resumen del mes", "Movimientos recientes", "Próximos pagos", "Fondo de emergencia"]) {
      await expect(page.getByRole("heading", { name: heading })).toHaveCount(0);
    }
    await expect(page.locator("main").getByRole("link")).toHaveCount(0);

    // The page never touched the settings, and the server was asked for the current cycle.
    expect(api.dashboardMonths).toEqual([""]);
    await expectNoHorizontalOverflow(page, "household dashboard");
  });

  test("the payload the page got has no figure outside the budget rows", async ({ page }) => {
    await mockApi(page, { role: "household", expenses: [RENT] });
    await seedSession(page, "household");
    const response = page.waitForResponse((r) => r.url().startsWith(`${API_ORIGIN}/dashboard`));
    await page.goto("/");
    const body = await (await response).json();
    expect(Object.keys(body).sort()).toEqual(["categories", "month", "period_end", "period_start"]);
  });

  test("the navigation lists only the dashboard and the requests", async ({ page }) => {
    await openAsHousehold(page);
    if (!isDesktop(page.viewportSize()?.width)) await page.getByRole("button", { name: "Abrir menú" }).click();
    const links = page.getByRole("navigation", { name: "Módulos" }).getByRole("link");
    await expect(links).toHaveText(["Panel", "Peticiones"]);
  });

  test("every other module redirects to the dashboard", async ({ page }) => {
    await openAsHousehold(page);
    for (const path of MODULE_ROUTES.filter((p) => p !== "/" && p !== "/peticiones")) {
      await page.goto(path);
      await expect(page, path).toHaveURL(/\/$/);
      await expect(page.getByRole("heading", { level: 2, name: "Presupuesto por categoría" })).toBeVisible();
    }
    await page.goto("/ruta-que-no-existe");
    await expect(page).toHaveURL(/\/$/);
  });

  test("the Usuarios section is not reachable", async ({ page }) => {
    await openAsHousehold(page, "/configuracion");
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByRole("tab", { name: "Usuarios" })).toHaveCount(0);
  });

  test("a 403 from the server shows a message instead of crashing the page", async ({ page }) => {
    // The session still looks like the owner's, but the server refuses every module route.
    await mockApi(page, { denyAll: true });
    await seedSession(page, "owner");
    await page.goto("/ingresos");
    await expect(page.getByRole("heading", { level: 1, name: "Ingresos" })).toBeVisible();
    await expect(page.getByRole("alert").first()).toContainText("no tiene permiso");
    await page.goto("/");
    await expect(page.getByRole("alert").first()).toContainText("no tiene permiso");
    await page.goto("/configuracion");
    await expect(page.getByRole("heading", { level: 1, name: "Configuración" })).toBeVisible();
    await expect(page.getByRole("alert").first()).toBeVisible();
  });

  test("a stale owner session corrects itself from /auth/me and lands on the household view", async ({ page }) => {
    await mockApi(page, { role: "household" });
    await seedSession(page, "owner");
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 2, name: "Presupuesto por categoría" })).toBeVisible();
    if (!isDesktop(page.viewportSize()?.width)) await page.getByRole("button", { name: "Abrir menú" }).click();
    await expect(page.getByRole("navigation", { name: "Módulos" }).getByRole("link")).toHaveCount(2);
  });

  test("login stores the role and a household user lands on the reduced dashboard", async ({ page }) => {
    await mockApi(page, { role: "household", expenses: [RENT] });
    await page.goto("/login");
    await page.getByLabel("Correo electrónico").fill("her@example.com");
    await page.getByRole("button", { name: "Continuar" }).click();
    await page.getByLabel("Contraseña", { exact: true }).fill("a long enough password");
    await page.getByRole("button", { name: "Iniciar sesión" }).click();
    await expect(page.getByRole("heading", { level: 2, name: "Presupuesto por categoría" })).toBeVisible();
    expect(await page.evaluate(() => localStorage.getItem("role"))).toBe("household");
  });
});

test.describe("owner session is unchanged", () => {
  test("shows every module in the navigation and the full dashboard", async ({ page }) => {
    await mockApi(page, { expenses: [RENT] });
    await seedSession(page, "owner");
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 2, name: "Resumen del mes" })).toBeVisible();
    if (!isDesktop(page.viewportSize()?.width)) await page.getByRole("button", { name: "Abrir menú" }).click();
    const labels = await page.getByRole("navigation", { name: "Módulos" }).getByRole("link").allInnerTexts();
    expect(labels).toHaveLength(MODULE_ROUTES.length);
    expect(labels).toContain("Sistema");
    expect(labels).toContain("Configuración");
  });

  test("a session stored before roles existed still works as the owner", async ({ page }) => {
    await mockApi(page, { expenses: [RENT] });
    await seedSession(page); // no role stored
    await page.goto("/ingresos");
    await expect(page.getByRole("heading", { level: 1, name: "Ingresos" })).toBeVisible();
  });
});
