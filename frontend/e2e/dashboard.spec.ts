import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockExpense, type MockIncome, type MockSaving } from "./helpers";

function today() {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

function previousMonth() {
  const d = new Date();
  d.setDate(1);
  d.setMonth(d.getMonth() - 1);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

const expense = (over: Partial<MockExpense>): MockExpense => ({
  id: 1,
  date: today(),
  description: "Gasto",
  category: "Renta",
  payment_method: "Transferencia",
  currency: "MXN",
  amount: "15000.00",
  exchange_rate: null,
  amount_mxn: "15000.00",
  ...over,
});

const salary: MockIncome = {
  id: 1,
  date: today(),
  description: "Sueldo",
  category: "Sueldo",
  payment_method: "Transferencia",
  currency: "MXN",
  amount: "60000.00",
  exchange_rate: null,
  amount_mxn: "60000.00",
};

const emergencySaving: MockSaving = {
  id: 1,
  date: today(),
  description: "Fondo inicial",
  category: "Fondo de emergencia",
  instrument: "i2",
  payment_method: "Transferencia",
  currency: "MXN",
  amount: "20000.00",
  exchange_rate: null,
  amount_mxn: "20000.00",
};

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

async function open(page: Page, opts: Parameters<typeof mockApi>[1] = {}) {
  const api = await mockApi(page, opts);
  await seedSession(page);
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1, name: "Panel" })).toBeVisible();
  return api;
}

test.describe("dashboard page", () => {
  test("is the main page and shows totals, budget against actual, emergency fund and tax", async ({ page }) => {
    const api = await open(page, {
      expenses: [expense({}), expense({ id: 2, category: "Comida", amount: "1000.00", amount_mxn: "1000.00" })],
      income: [salary],
      savings: [emergencySaving],
    });

    await expect(page.getByRole("heading", { level: 2, name: "Presupuesto por categoría" })).toBeVisible();
    expect(api.dashboardMonths).toEqual([today().slice(0, 7)]);

    const totals = page.getByRole("region", { name: "Resumen del mes" });
    await expect(totals.getByText("Ingresos").locator("xpath=following-sibling::dd")).toHaveText("$60,000.00");
    await expect(totals.getByText("Gastos").locator("xpath=following-sibling::dd")).toHaveText("$16,000.00");
    await expect(totals.getByText("Ahorros").locator("xpath=following-sibling::dd")).toHaveText("$20,000.00");
    // Available = income - expenses - savings.
    await expect(totals.getByText("Disponible").locator("xpath=following-sibling::dd")).toHaveText("$24,000.00");

    const renta = page.getByRole("row", { name: /Renta/ });
    await expect(renta).toContainText("$15,000.00");
    await expect(renta).toContainText("$12,000.50");
    await expect(renta).toContainText("-$2,999.50");
    await expect(renta).toContainText("Presupuesto excedido");
    await expect(renta.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "100");

    const comida = page.getByRole("row", { name: /Comida/ });
    await expect(comida).toContainText("$5,000.00");
    await expect(comida).not.toContainText("Presupuesto excedido");
    await expect(comida.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "17");

    const fund = page.getByRole("region", { name: "Fondo de emergencia" });
    await expect(fund).toContainText("$20,000.00 de $60,000.00 (33.33%)");

    const tax = page.getByRole("region", { name: "ISR RESICO estimado" });
    await expect(tax).toContainText("$900.00");
    await expect(tax).toContainText("1.5%");
    // No payment status yet: only the estimate and the rate.
    await expect(tax).not.toContainText("Pagado");
    await expect(tax).not.toContainText("Pendiente");
  });

  test("shows the empty state for a month without movements", async ({ page }) => {
    await open(page);
    await expect(page.getByText("Aún no hay movimientos en este mes.")).toBeVisible();
    await expect(page.getByRole("row", { name: /Renta/ })).toContainText("$12,000.50");
    await expect(page.getByRole("region", { name: "Resumen del mes" }).getByText("Disponible").locator("xpath=following-sibling::dd")).toHaveText("$0.00");
  });

  test("changing the month reloads the dashboard for that month", async ({ page }) => {
    const api = await open(page, { expenses: [expense({})], income: [salary] });
    await expect(page.getByRole("row", { name: /Renta/ })).toContainText("$15,000.00");

    const previous = previousMonth();
    await page.getByLabel("Mes", { exact: true }).fill(previous);
    await expect(page.getByText("Aún no hay movimientos en este mes.")).toBeVisible();
    expect(api.dashboardMonths.at(-1)).toBe(previous);
    await expect(page.getByRole("row", { name: /Renta/ })).toContainText("$0.00");
  });

  test("explains a missing tax estimate", async ({ page }) => {
    await open(page, { dashboardNoTax: true });
    await expect(page.getByRole("region", { name: "ISR RESICO estimado" })).toContainText("Sin estimación");
  });

  test("shows the error with a retry when the request fails", async ({ page }) => {
    await open(page, { dashboardFail: "server" });
    await expect(page.getByText("El servidor tuvo un problema")).toBeVisible();
    await expect(page.getByRole("button", { name: "Reintentar" })).toBeVisible();
  });

  test("points to Configuración when the settings are incomplete", async ({ page }) => {
    await open(page, { dashboardFail: "incomplete" });
    await expect(page.getByText("Faltan parámetros fiscales")).toBeVisible();
    await page.getByRole("link", { name: "Ir a Configuración" }).click();
    await expect(page).toHaveURL(/\/configuracion$/);
  });

  test("has no horizontal overflow, with data and with an error", async ({ page }) => {
    await open(page, {
      expenses: [expense({}), expense({ id: 2, category: "Comida", amount: "1000.00", amount_mxn: "1000.00" })],
      income: [salary],
      savings: [emergencySaving],
    });
    await expect(page.getByRole("row", { name: /Renta/ })).toContainText("$15,000.00");
    await expectNoHorizontalOverflow(page, "dashboard with data");
  });

  test("has no horizontal overflow in the error state", async ({ page }) => {
    await open(page, { dashboardFail: "incomplete" });
    await expect(page.getByText("Faltan parámetros fiscales")).toBeVisible();
    await expectNoHorizontalOverflow(page, "dashboard error");
  });
});
