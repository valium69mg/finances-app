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

const isPhone = (page: Page) => (page.viewportSize()?.width ?? 1024) < 768;

/** The budget of a category: a compact card below the md breakpoint, a table row from md up. */
const budgetRow = (page: Page, name: RegExp) => (isPhone(page) ? page.getByTestId("budget-card").filter({ hasText: name }) : page.getByRole("row", { name }));

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

    const renta = budgetRow(page, /Renta/);
    await expect(renta).toContainText("$15,000.00");
    await expect(renta).toContainText("$12,000.50");
    await expect(renta).toContainText(isPhone(page) ? "Excedido $2,999.50" : "-$2,999.50");
    if (!isPhone(page)) await expect(renta).toContainText("Presupuesto excedido");
    await expect(renta.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "100");

    const comida = budgetRow(page, /Comida/);
    await expect(comida).toContainText("$5,000.00");
    await expect(comida).not.toContainText("Presupuesto excedido");
    await expect(comida.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "17");

    const fund = page.getByRole("region", { name: "Fondo de emergencia" });
    await expect(fund).toContainText("$20,000.00 de $60,000.00 (33.33%)");

    const tax = page.getByRole("region", { name: "ISR RESICO estimado" });
    await expect(tax).toContainText("$900.00");
    await expect(tax).toContainText("1.5%");
    // Nothing filed yet, nothing pending from the previous period.
    await expect(tax).toContainText("Sin declarar");
    await expect(tax).not.toContainText("sigue pendiente");
  });

  test("the tax card shows the filing status and warns about a pending previous period", async ({ page }) => {
    await open(page, { income: [salary], dashboardFiling: { filing_status: "pendiente", previous_period_pending: true } });
    const tax = page.getByRole("region", { name: "ISR RESICO estimado" });
    await expect(tax).toContainText("Pago pendiente");
    await expect(tax).toContainText("sigue pendiente");
    await tax.getByRole("link", { name: "Ver declaraciones" }).click();
    await expect(page).toHaveURL(/\/declaraciones-presentadas$/);
  });

  test("the tax card shows a paid filing", async ({ page }) => {
    await open(page, { income: [salary], dashboardFiling: { filing_status: "pagada", previous_period_pending: false } });
    const tax = page.getByRole("region", { name: "ISR RESICO estimado" });
    await expect(tax).toContainText("Pagada");
    await expect(tax).not.toContainText("sigue pendiente");
  });

  test("shows the empty state for a month without movements", async ({ page }) => {
    await open(page);
    await expect(page.getByText("Aún no hay movimientos en este periodo.")).toBeVisible();
    await expect(budgetRow(page, /Renta/)).toContainText("$12,000.50");
    await expect(page.getByRole("region", { name: "Resumen del mes" }).getByText("Disponible").locator("xpath=following-sibling::dd")).toHaveText("$0.00");
  });

  test("changing the month reloads the dashboard for that month", async ({ page }) => {
    const api = await open(page, { expenses: [expense({})], income: [salary] });
    await expect(budgetRow(page, /Renta/)).toContainText("$15,000.00");

    const previous = previousMonth();
    await page.getByLabel("Mes", { exact: true }).fill(previous);
    await expect(page.getByText("Aún no hay movimientos en este periodo.")).toBeVisible();
    expect(api.dashboardMonths.at(-1)).toBe(previous);
    await expect(budgetRow(page, /Renta/)).toContainText("$0.00");
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
    await expect(budgetRow(page, /Renta/)).toContainText("$15,000.00");
    await expectNoHorizontalOverflow(page, "dashboard with data");
  });

  test("has no horizontal overflow in the error state", async ({ page }) => {
    await open(page, { dashboardFail: "incomplete" });
    await expect(page.getByText("Faltan parámetros fiscales")).toBeVisible();
    await expectNoHorizontalOverflow(page, "dashboard error");
  });

  test("shows the cycle progress with the spending pace", async ({ page }) => {
    await open(page, { expenses: [expense({ amount: "25000.00", amount_mxn: "25000.00" })], income: [salary] });
    const cycle = page.getByRole("region", { name: "Progreso del ciclo" });
    await expect(cycle).toContainText(/Día \d+ de \d+/);
    await expect(cycle.getByRole("progressbar", { name: "Avance del ciclo" })).toBeVisible();
    // 25,000 against a 20,000.50 budget is ahead of the pace on any day of the cycle.
    await expect(cycle).toContainText("Gastado $25,000.00 de");
    await expect(cycle).toContainText("Vas por encima del ritmo de gasto del ciclo.");
  });

  test("shows the future expenses with progress, the suggested monthly amount and a total", async ({ page }) => {
    await open(page, {
      dashboardFuture: {
        items: [
          { id: 2, name: "Viaje", due_date: "2026-12-15", target: "24000.00", saved: "6000.00", remaining: "18000.00", suggested_monthly: "9000.00", cycles_left: 2 },
          { id: 1, name: "Laptop", due_date: "2027-01-20", target: "8000.00", saved: "0.00", remaining: "8000.00", suggested_monthly: "2000.00", cycles_left: 4 },
        ],
        target: "32000.00",
        saved: "6000.00",
        remaining: "26000.00",
        suggested_monthly: "11000.00",
        free_balance: "0.00",
      },
    });
    const card = page.getByRole("region", { name: "Gastos futuros" });
    await expect(card).toContainText("Viaje");
    await expect(card).toContainText("Vence el 15 de diciembre de 2026");
    await expect(card).toContainText("$6,000.00 de $24,000.00 (25%)");
    await expect(card).toContainText("Aparta $9,000.00 al mes");
    await expect(card.getByRole("progressbar", { name: "Ahorro para Viaje" })).toHaveAttribute("aria-valuenow", "25");
    await expect(card).toContainText("Laptop");
    await expect(card.getByText("Total", { exact: true }).locator("xpath=following-sibling::dd[1]")).toHaveText("$32,000.00");
    await expectNoHorizontalOverflow(page, "dashboard with future expenses");
  });

  test("future expenses explain where to add one when there are none", async ({ page }) => {
    await open(page);
    await expect(page.getByRole("region", { name: "Gastos futuros" })).toContainText("Aún no hay gastos futuros");
  });

  test("lists the latest movements of every kind in one list", async ({ page }) => {
    await open(page, {
      expenses: [expense({ description: "Renta octubre" })],
      income: [salary],
      savings: [emergencySaving],
    });
    const recent = page.getByRole("region", { name: "Últimos movimientos" });
    await expect(recent.getByRole("listitem")).toHaveCount(3);
    await expect(recent).toContainText("Ingreso");
    await expect(recent).toContainText("Gasto");
    await expect(recent).toContainText("Ahorro");
    await expect(recent).toContainText("Renta octubre");
    await expect(recent).toContainText("$60,000.00");
  });

  test("lists the bills due in the next 14 days and leaves later ones out", async ({ page }) => {
    const due = (offset: number) => {
      const d = new Date();
      d.setDate(d.getDate() + offset);
      return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
    };
    const bill = (id: number, name: string, offset: number) => ({
      id, name, category: "Servicios", amount: "200.00", currency: "MXN", recurrence: "monthly" as const,
      next_due_date: due(offset), reminder_lead_days: 3, active: true, notes: "",
    });
    await open(page, { bills: [bill(1, "Luz", 3), bill(2, "Internet", 40), bill(3, "Agua", -2)] });
    const card = page.getByRole("region", { name: "Próximas cuentas" });
    await expect(card.getByRole("listitem")).toHaveCount(2);
    await expect(card).toContainText("Agua");
    await expect(card).toContainText("Vencida hace 2 días");
    await expect(card).toContainText("Luz");
    await expect(card).toContainText("Vence en 3 días");
    await expect(card).not.toContainText("Internet");
    await card.getByRole("link", { name: "Ver pagos recurrentes" }).click();
    await expect(page).toHaveURL(/\/pagos-recurrentes$/);
  });

  test("budget by category is a compact card list on a phone and a table from md up", async ({ page }) => {
    await open(page, {
      expenses: [expense({}), expense({ id: 2, category: "Comida", amount: "1000.00", amount_mxn: "1000.00" })],
      income: [salary],
    });
    if (isPhone(page)) {
      await expect(page.getByRole("table")).toBeHidden();
      const card = page.getByTestId("budget-card").filter({ hasText: "Renta" });
      await expect(card).toContainText("100%");
      await expect(card).toContainText("Gastado $15,000.00 de $12,000.50");
      const excess = card.getByText("Excedido $2,999.50");
      await expect(excess).toBeVisible();
      await expect(excess).toHaveCSS("color", /^rgb/);
      const cardBox = await card.boundingBox();
      const barBox = await card.getByRole("progressbar").boundingBox();
      // The bar spans the full width of the card (minus its padding).
      expect(barBox!.width).toBeGreaterThan(cardBox!.width - 32);
    } else {
      await expect(page.getByRole("table")).toBeVisible();
      await expect(page.getByTestId("budget-card").first()).toBeHidden();
    }
    await expectNoHorizontalOverflow(page, "budget by category");
  });
  test("shows a colored icon per kind in the totals and the latest movements", async ({ page }) => {
    await open(page, { expenses: [expense({})], income: [salary], savings: [emergencySaving] });
    const totals = page.getByRole("region", { name: "Resumen del mes" });
    // Every kind card carries an icon next to its text label.
    for (const label of ["Ingresos", "Gastos", "Ahorros", "Disponible"]) {
      await expect(totals.locator("div").filter({ has: page.getByText(label, { exact: true }) }).locator("svg").first()).toBeVisible();
    }
    const recent = page.getByRole("region", { name: "Últimos movimientos" });
    // The badge is icon plus the kind written out.
    for (const kind of ["Ingreso", "Gasto", "Ahorro"]) await expect(recent.locator("span:has(> svg)", { hasText: new RegExp(`^${kind}$`) })).toBeVisible();
  });

  test("renders in dark mode with no horizontal overflow at 375px", async ({ page }) => {
    await page.emulateMedia({ colorScheme: "dark" });
    await page.setViewportSize({ width: 375, height: 812 });
    await open(page, {
      expenses: [expense({}), expense({ id: 2, category: "Comida", amount: "1000.00", amount_mxn: "1000.00" })],
      income: [salary],
      savings: [emergencySaving],
    });
    await expect(budgetRow(page, /Renta/)).toContainText("$15,000.00");
    // The dark tokens are active: the page background is the dark one, not the light #F8FAFC.
    const bg = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
    expect(bg).toBe("rgb(11, 17, 32)");
    // Kind colors come from the dark tokens too (income is emerald 400).
    const income = await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--color-income").trim());
    expect(income).toBe("52 211 153");
    await expectNoHorizontalOverflow(page, "dashboard in dark mode at 375px");
  });
});
