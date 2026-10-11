import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockBill, type MockExpense, type MockFutureExpense, type MockIncome, type MockSaving } from "./helpers";

/**
 * The owner's interactive Panel (phase 13): every card and row is a link that carries the Panel's month (?mes=), the
 * target pages read it, Gastos and Ahorros filter by ?categoria=<name>, and ?editar=<id> / ?pago=<id> open one
 * record's edit form or pay dialog.
 */

const pad = (n: number) => String(n).padStart(2, "0");
function day(offset = 0) {
  const d = new Date();
  d.setDate(d.getDate() + offset);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}
/** Fixed past cycles (calendar cycle in the mock) so the data never depends on the day the suite runs. */
const AUG = "2026-08";
const JUL = "2026-07";

const expense = (over: Partial<MockExpense>): MockExpense => ({
  id: 1,
  date: "2026-08-05",
  description: "Gasto",
  category: "Servicios",
  payment_method: "Transferencia",
  currency: "MXN",
  amount: "550.00",
  exchange_rate: null,
  amount_mxn: "550.00",
  ...over,
});
const EXPENSES: MockExpense[] = [
  expense({ id: 1, description: "Megacable", category: "Servicios" }),
  expense({ id: 2, description: "Netflix", category: "Suscripciones", amount: "139.00", amount_mxn: "139.00", date: "2026-08-06" }),
  // Another cycle: the Panel's recent list shows it, and its link must open July, not August.
  expense({ id: 3, description: "Gasolina julio", category: "Servicios", amount: "700.00", amount_mxn: "700.00", date: "2026-07-20" }),
];
const INCOME: MockIncome[] = [
  { id: 11, date: "2026-08-02", description: "Sueldo agosto", category: "Sueldo", payment_method: "Transferencia", currency: "MXN", amount: "60000.00", exchange_rate: null, amount_mxn: "60000.00" },
];
const saving = (over: Partial<MockSaving>): MockSaving => ({
  id: 21,
  date: "2026-08-07",
  description: "Fondo inicial",
  category: "Fondo de emergencia",
  instrument: "i2",
  payment_method: "Transferencia",
  currency: "MXN",
  amount: "20000.00",
  exchange_rate: null,
  amount_mxn: "20000.00",
  ...over,
});
const SAVINGS: MockSaving[] = [
  saving({ id: 21 }),
  saving({ id: 22, description: "Compra ETF", category: "Inversión ETF", instrument: "i1", amount: "500.00", amount_mxn: "500.00", date: "2026-08-08" }),
];
const BILLS: MockBill[] = [
  { id: 1, name: "Internet", category: "Servicios", amount: "550.00", currency: "MXN", recurrence: "monthly", next_due_date: day(-2), reminder_lead_days: 3, active: true, notes: "" },
  { id: 2, name: "Luz", category: "Servicios", amount: null, currency: "MXN", recurrence: "bimonthly", next_due_date: day(3), reminder_lead_days: 3, active: true, notes: "" },
];
const FUTURE: MockFutureExpense[] = [
  { id: 1, name: "Laptop", target_amount: "8000.00", due_date: day(120), saved: "2000.00" },
  { id: 2, name: "Viaje", target_amount: "24000.00", due_date: day(60), saved: "0" },
];

async function openPanel(page: Page, opts: Parameters<typeof mockApi>[1] = {}, month = AUG) {
  const api = await mockApi(page, { expenses: EXPENSES, income: INCOME, savings: SAVINGS, bills: BILLS, futureExpenses: FUTURE, ...opts });
  await seedSession(page);
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1, name: "Panel" })).toBeVisible();
  await page.getByLabel("Mes", { exact: true }).fill(month);
  // The links carry the month the dashboard answered for.
  await expect(main(page).getByRole("link", { name: "Ingresos", exact: true })).toHaveAttribute("href", `/ingresos?mes=${month}`);
  return api;
}

async function openPage(page: Page, path: string, opts: Parameters<typeof mockApi>[1] = {}) {
  const api = await mockApi(page, { expenses: EXPENSES, income: INCOME, savings: SAVINGS, bills: BILLS, futureExpenses: FUTURE, ...opts });
  await seedSession(page);
  await page.goto(path);
  return api;
}

const param = (page: Page, name: string) => new URL(page.url()).searchParams.get(name);
const main = (page: Page) => page.locator("main");

test.describe("owner Panel: totals and cycle", () => {
  test("Ingresos, Gastos and Ahorros open their page on the Panel's month", async ({ page }) => {
    await openPanel(page);
    const targets: [string, string, string, string, string][] = [
      ["Ingresos", "/ingresos", "Ingresos", "Sueldo agosto", "Ingresos del mes"],
      ["Gastos", "/gastos", "Gastos", "Megacable", "Gastos del mes"],
      ["Ahorros", "/ahorros", "Ahorros", "Fondo inicial", "Ahorros del mes"],
    ];
    for (const [card, path, title, row, list] of targets) {
      await page.goto("/");
      await page.getByLabel("Mes", { exact: true }).fill(AUG);
      await main(page).getByRole("link", { name: card, exact: true }).click();
      await expect(page).toHaveURL(new RegExp(`${path}\\?mes=${AUG}$`));
      await expect(page.getByRole("heading", { level: 1, name: title })).toBeVisible();
      await expect(page.getByRole("heading", { level: 2, name: list })).toBeVisible();
      await expect(page.getByLabel("Mes", { exact: true })).toHaveValue(AUG);
      await expect(main(page).getByText(row, { exact: true })).toBeVisible();
    }
  });

  test("without a picked month the links carry the month the server answered", async ({ page }) => {
    const api = await mockApi(page, { expenses: EXPENSES });
    await seedSession(page);
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 1, name: "Panel" })).toBeVisible();
    const month = await page.getByLabel("Mes", { exact: true }).inputValue();
    expect(month).toMatch(/^\d{4}-\d{2}$/);
    await expect(main(page).getByRole("link", { name: "Gastos", exact: true })).toHaveAttribute("href", `/gastos?mes=${month}`);
    expect(api.dashboardMonths.length).toBeGreaterThan(0);
  });

  test("Disponible is informative: not a link and no chevron", async ({ page }) => {
    await openPanel(page);
    await expect(main(page).getByRole("link", { name: "Disponible", exact: true })).toHaveCount(0);
    const available = page.locator("dl > div", { has: page.getByText("Disponible", { exact: true }) });
    await expect(available).toBeVisible();
    await expect(available.locator("svg.lucide-chevron-right")).toHaveCount(0);
    await expect(available).not.toHaveClass(/\bgroup\b/);
    await expect(page.locator("dl > div", { has: page.getByRole("link", { name: "Ingresos", exact: true }) }).locator("svg.lucide-chevron-right")).toHaveCount(1);
  });

  test("the cards are keyboard accessible and show focus", async ({ page }) => {
    await openPanel(page);
    const link = main(page).getByRole("link", { name: "Gastos", exact: true });
    await link.focus();
    const ring = await link.evaluate((el) => getComputedStyle(el, "::after").boxShadow);
    expect(ring).not.toBe("none");
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(new RegExp(`/gastos\\?mes=${AUG}$`));
  });

  test("Progreso del ciclo opens Gastos of the cycle", async ({ page }) => {
    await openPanel(page);
    await main(page).getByRole("link", { name: "Progreso del ciclo" }).click();
    await expect(page).toHaveURL(new RegExp(`/gastos\\?mes=${AUG}$`));
    await expect(page.getByLabel("Mes", { exact: true })).toHaveValue(AUG);
    await expect(main(page).getByText("Megacable", { exact: true })).toBeVisible();
  });
});

test.describe("owner Panel: category filters", () => {
  test("a budget row opens Gastos filtered by that category, and the chip clears it", async ({ page }) => {
    await openPanel(page);
    await main(page).getByRole("link", { name: "Servicios", exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/gastos\\?mes=${AUG}&categoria=Servicios$`));
    await expect(page.getByText("Categoría: Servicios")).toBeVisible();
    await expect(main(page).getByText("Megacable", { exact: true })).toBeVisible();
    await expect(main(page).getByText("Netflix", { exact: true })).toHaveCount(0);

    await page.getByRole("button", { name: "Quitar filtro de categoría Servicios" }).click();
    await expect(page.getByText("Categoría: Servicios")).toHaveCount(0);
    await expect(main(page).getByText("Netflix", { exact: true })).toBeVisible();
    expect(param(page, "categoria")).toBeNull();
    expect(param(page, "mes")).toBe(AUG);
  });

  test("Fondo de emergencia opens Ahorros filtered by the emergency fund category", async ({ page }) => {
    await openPanel(page);
    await main(page).getByRole("link", { name: "Fondo de emergencia" }).click();
    await expect(page).toHaveURL(new RegExp(`/ahorros\\?mes=${AUG}&categoria=Fondo\\+de\\+emergencia$`));
    await expect(page.getByText("Categoría: Fondo de emergencia")).toBeVisible();
    await expect(main(page).getByText("Fondo inicial", { exact: true })).toBeVisible();
    await expect(main(page).getByText("Compra ETF", { exact: true })).toHaveCount(0);

    await page.getByRole("button", { name: "Quitar filtro de categoría Fondo de emergencia" }).click();
    await expect(main(page).getByText("Compra ETF", { exact: true })).toBeVisible();
    expect(param(page, "categoria")).toBeNull();
  });

  test("the filter survives a month change and an unknown category is ignored", async ({ page }) => {
    await openPage(page, `/gastos?mes=${AUG}&categoria=Servicios`);
    await expect(page.getByText("Categoría: Servicios")).toBeVisible();
    await page.getByLabel("Mes", { exact: true }).fill(JUL);
    await expect(main(page).getByText("Gasolina julio", { exact: true })).toBeVisible();
    expect(param(page, "categoria")).toBe("Servicios");
    expect(param(page, "mes")).toBe(JUL);
  });

  for (const [path, title, row] of [
    ["/gastos", "Gastos", "Netflix"],
    ["/ahorros", "Ahorros", "Compra ETF"],
  ] as const) {
    test(`${title} ignores a category that does not exist (and one of another kind)`, async ({ page }) => {
      await openPage(page, `${path}?mes=${AUG}&categoria=Inexistente`);
      await expect(page.getByRole("heading", { level: 1, name: title })).toBeVisible();
      await expect(main(page).getByText(row, { exact: true })).toBeVisible();
      await expect(page.getByText(/^Categoría:/)).toHaveCount(0);
      await page.goto(`${path}?mes=${AUG}&categoria=Sueldo`);
      await expect(main(page).getByText(row, { exact: true })).toBeVisible();
      await expect(page.getByText(/^Categoría:/)).toHaveCount(0);
    });
  }
});

test.describe("owner Panel: taxes", () => {
  test("ISR RESICO opens Declaración for the Panel's month", async ({ page }) => {
    await openPanel(page);
    await main(page).getByRole("link", { name: "ISR RESICO estimado" }).click();
    await expect(page).toHaveURL(new RegExp(`/declaracion\\?period=${AUG}$`));
    await expect(page.getByLabel("Periodo")).toHaveValue(AUG);
  });

  test("the pending previous period warning keeps its own link to Declaraciones presentadas", async ({ page }) => {
    await openPanel(page, { dashboardFiling: { filing_status: "ninguna", previous_period_pending: true } });
    const warning = main(page).getByRole("link", { name: "Ver declaraciones" });
    await expect(warning).toHaveAttribute("href", "/declaraciones-presentadas");
    await warning.click();
    await expect(page).toHaveURL(/\/declaraciones-presentadas$/);
  });
});

test.describe("owner Panel: rows open one record", () => {
  test("a recent expense opens Gastos on its month with its edit form, and closing removes the parameter", async ({ page }) => {
    await openPanel(page);
    await main(page).getByRole("link", { name: "Megacable", exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/gastos\\?mes=${AUG}&editar=1$`));
    await expect(page.getByRole("heading", { name: "Editar gasto" })).toBeVisible();
    await expect(page.getByLabel("Descripción")).toHaveValue("Megacable");
    await page.getByRole("button", { name: "Cancelar" }).click();
    await expect(page.getByRole("heading", { name: "Editar gasto" })).toHaveCount(0);
    expect(param(page, "editar")).toBeNull();
    expect(param(page, "mes")).toBe(AUG);
  });

  test("a movement of another cycle opens that cycle, not the Panel's", async ({ page }) => {
    await openPanel(page);
    await main(page).getByRole("link", { name: "Gasolina julio", exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/gastos\\?mes=${JUL}&editar=3$`));
    await expect(page.getByLabel("Mes", { exact: true })).toHaveValue(JUL);
    await expect(page.getByLabel("Descripción")).toHaveValue("Gasolina julio");
  });

  test("a recent income opens Ingresos with its edit form", async ({ page }) => {
    await openPanel(page);
    await main(page).getByRole("link", { name: "Sueldo agosto", exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/ingresos\\?mes=${AUG}&editar=11$`));
    await expect(page.getByRole("heading", { name: "Editar ingreso" })).toBeVisible();
    await page.getByRole("button", { name: "Cancelar" }).click();
    expect(param(page, "editar")).toBeNull();
  });

  test("a recent saving opens Ahorros with its edit form", async ({ page }) => {
    await openPanel(page);
    await main(page).getByRole("link", { name: "Compra ETF", exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/ahorros\\?mes=${AUG}&editar=22$`));
    await expect(page.getByRole("heading", { name: "Editar ahorro" })).toBeVisible();
    await page.getByRole("button", { name: "Cancelar" }).click();
    expect(param(page, "editar")).toBeNull();
  });

  test("an upcoming payment opens Pagos recurrentes with that bill's Pagar dialog, and closing removes the parameter", async ({ page }) => {
    await openPanel(page);
    await main(page).getByRole("link", { name: "Luz", exact: true }).click();
    await expect(page).toHaveURL(/\/pagos-recurrentes\?pago=2$/);
    const dialog = page.getByRole("dialog", { name: "Pagar Luz" });
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Cancelar" }).click();
    await expect(dialog).toHaveCount(0);
    expect(param(page, "pago")).toBeNull();
  });

  test("a future expense opens Gastos futuros with that item's edit form", async ({ page }) => {
    await openPanel(page);
    await main(page).getByRole("link", { name: "Viaje", exact: true }).click();
    await expect(page).toHaveURL(/\/gastos-futuros\?editar=2$/);
    await expect(page.getByRole("heading", { name: "Editar Viaje" })).toBeVisible();
    await page.getByRole("button", { name: "Cancelar" }).click();
    await expect(page.getByRole("heading", { name: "Editar Viaje" })).toHaveCount(0);
    expect(param(page, "editar")).toBeNull();
  });

  test("the manage links stay", async ({ page }) => {
    await openPanel(page);
    await expect(main(page).getByRole("link", { name: "Ver pagos recurrentes" })).toHaveAttribute("href", "/pagos-recurrentes");
    await expect(main(page).getByRole("link", { name: "Administrar gastos futuros" })).toHaveAttribute("href", "/gastos-futuros");
  });
});

test.describe("deep links with an unknown id", () => {
  for (const [path, title, note] of [
    [`/gastos?mes=${AUG}&editar=999`, "Gastos", "No se encontró el gasto del enlace"],
    [`/ingresos?mes=${AUG}&editar=999`, "Ingresos", "No se encontró el ingreso del enlace"],
    [`/ahorros?mes=${AUG}&editar=999`, "Ahorros", "No se encontró el ahorro del enlace"],
    ["/pagos-recurrentes?pago=999", "Pagos recurrentes", "No se encontró el pago recurrente del enlace"],
    ["/gastos-futuros?editar=999", "Gastos futuros", "No se encontró el gasto futuro del enlace"],
  ] as const) {
    test(`${title} shows the page normally with a short note`, async ({ page }) => {
      await openPage(page, path);
      await expect(page.getByRole("heading", { level: 1, name: title })).toBeVisible();
      await expect(page.getByText(note)).toBeVisible();
      await expect(page.getByRole("dialog")).toHaveCount(0);
      await expect(page.getByRole("heading", { name: /^Editar / })).toHaveCount(0);
      expect(param(page, "editar")).toBeNull();
      expect(param(page, "pago")).toBeNull();
    });
  }

  test("a malformed id is ignored without a note", async ({ page }) => {
    await openPage(page, `/gastos?mes=${AUG}&editar=abc`);
    await expect(main(page).getByText("Megacable", { exact: true })).toBeVisible();
    await expect(page.getByText(/No se encontró/)).toHaveCount(0);
  });

  test("an id of another month is not found, and the month in the link finds it", async ({ page }) => {
    await openPage(page, `/gastos?mes=${AUG}&editar=3`);
    await expect(page.getByText("No se encontró el gasto del enlace")).toBeVisible();
    await page.goto(`/gastos?mes=${JUL}&editar=3`);
    await expect(page.getByLabel("Descripción")).toHaveValue("Gasolina julio");
  });

  test("a transfer cannot be edited: the link leaves the page as it is", async ({ page }) => {
    await openPage(page, `/ahorros?mes=${AUG}&editar=25`, {
      savings: [...SAVINGS, saving({ id: 25, description: "Traspaso salida", amount: "-100.00", amount_mxn: "-100.00", transfer_id: "t-1" })],
    });
    await expect(page.getByText("No se encontró el ahorro del enlace")).toBeVisible();
    await expect(page.getByRole("heading", { name: "Editar ahorro" })).toHaveCount(0);
  });
});

test.describe("the household Panel is unchanged", () => {
  test("it keeps its own links and none of the owner's parameters", async ({ page }) => {
    await mockApi(page, { role: "household" });
    await seedSession(page, "household");
    await page.goto(`/?mes=${AUG}`);
    await expect(page.getByRole("heading", { level: 2, name: "Mi resumen" })).toBeVisible();
    const hrefs = await main(page).getByRole("link").evaluateAll((links) => links.map((a) => a.getAttribute("href") ?? ""));
    for (const href of hrefs) expect(href, href).toMatch(/^\/(peticiones|mis-ingresos|mis-gastos|mis-ahorros)(\?|$)/);
    await expect(main(page).getByRole("link", { name: "Ingresos", exact: true })).toHaveAttribute("href", `/mis-ingresos?mes=${AUG}`);
  });
});
