import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockClose } from "./helpers";

const pad = (n: number) => String(n).padStart(2, "0");
const MONTHS = ["enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"];

/** YYYY-MM of the month `offset` months from today (local time). */
function month(offset: number) {
  const d = new Date();
  const first = new Date(d.getFullYear(), d.getMonth() + offset, 1);
  return `${first.getFullYear()}-${pad(first.getMonth() + 1)}`;
}
const label = (period: string) => `${MONTHS[Number(period.slice(5)) - 1]} de ${period.slice(0, 4)}`;

const PREV = month(-1);
const OLDER = month(-2);
const NEXT = month(1);

const expense = (id: number, category: string, amount: string, period = PREV) => ({
  id,
  date: `${period}-10`,
  description: category,
  category,
  payment_method: "Débito",
  currency: "MXN",
  amount,
  exchange_rate: null,
  amount_mxn: amount,
});

/** Previous month: income 30,000, expenses 21,000.50 (Comida over budget, Servicios far under), 5,000 to the emergency fund => 3,999.50 left. */
const MOVEMENTS = {
  income: [{ ...expense(1, "Sueldo", "30000.00"), description: "Sueldo" }],
  expenses: [expense(1, "Renta", "12000.50"), expense(2, "Comida", "8000.00"), expense(3, "Servicios", "1000.00")],
  savings: [{ ...expense(1, "Fondo de emergencia", "5000.00"), instrument: "i2" }],
};

const stored = (period: string, over: Partial<MockClose> = {}): MockClose => ({
  period,
  closed_at: `${period}-28T18:30:00.000Z`,
  categories: [
    { category: "Renta", spent: "12000.50", budget: "12000.50", remaining: "0.00", over_budget: false },
    { category: "Comida", spent: "9000.00", budget: "6000", remaining: "-3000.00", over_budget: true },
    { category: "Suscripciones", spent: "139.00", budget: null, remaining: null, over_budget: false },
  ],
  income: "30000.00",
  expenses: "21139.50",
  savings: "1000.00",
  available: "7860.50",
  emergency: { accumulated: "20000.00", goal: "60000.00" },
  suggestion: { to_emergency_fund: "7860.50", to_investments: "0", to_future_expenses: "0", investments_paused: false },
  adjustments: [{ category: "Comida", kind: "Gasto", budget: "6000", real: "9000.00", deviation_pct: "50.00" }],
  tax_filing_status: "pendiente",
  ...over,
});

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

async function openClose(page: Page, opts: Parameters<typeof mockApi>[1] = {}) {
  const api = await mockApi(page, opts);
  await seedSession(page);
  await page.goto("/cierre-de-mes");
  await expect(page.getByRole("heading", { level: 1, name: "Cierre de mes" })).toBeVisible();
  return api;
}

const withMovements = { income: MOVEMENTS.income, expenses: MOVEMENTS.expenses, savings: MOVEMENTS.savings };
const historyRow = (page: Page, period: string) => page.getByRole("button", { name: `Ver cierre de ${label(period)}` });

test.describe("month close preview", () => {
  test("defaults to the previous month and shows the empty states", async ({ page }) => {
    await openClose(page);
    await expect(page.getByLabel("Mes a cerrar")).toHaveValue(PREV);
    await expect(page.getByRole("heading", { name: `Vista previa del cierre de ${label(PREV)}` })).toBeVisible();
    await expect(page.getByText("Ninguna categoría de gasto excedió su presupuesto.")).toBeVisible();
    await expect(page.getByText("No quedó dinero disponible este mes")).toBeVisible();
    // Nothing was spent, so every budgeted category is 100% under its budget.
    await expect(page.getByRole("list", { name: "Ajustes de presupuesto sugeridos" })).toContainText("-100.00%");
    await expect(page.getByText("Aún no has cerrado ningún mes")).toBeVisible();
  });

  test("shows totals, over-budget categories, emergency progress, suggestion and hints without storing anything", async ({ page }) => {
    const api = await openClose(page, withMovements);
    const totals = page.getByRole("region", { name: "Resumen del mes" });
    await expect(totals).toContainText("$30,000.00");
    await expect(totals).toContainText("$21,000.50");
    await expect(totals).toContainText("$5,000.00");
    await expect(totals).toContainText("$3,999.50");

    const over = page.getByRole("list", { name: "Categorías sobre presupuesto" });
    await expect(over.getByRole("listitem")).toHaveCount(1);
    await expect(over).toContainText("Comida");
    await expect(over).toContainText("$8,000.00 contra presupuesto de $6,000.00");

    // The budget rows are a table from md up and compact cards on a phone.
    const phone = (page.viewportSize()?.width ?? 1024) < 768;
    await expect(phone ? page.getByTestId("budget-card").filter({ hasText: "Servicios" }) : page.getByRole("table")).toContainText("Servicios");
    await expect(page.getByText("$5,000.00 de $60,000.00 (8.33%)")).toBeVisible();

    const suggestion = page.getByRole("list", { name: "Sugerencia del dinero disponible" });
    await expect(suggestion.getByRole("listitem")).toHaveCount(1);
    await expect(suggestion).toContainText("Mover a Fondo de emergencia");
    await expect(suggestion).toContainText("$3,999.50");

    const hints = page.getByRole("list", { name: "Ajustes de presupuesto sugeridos" });
    await expect(hints.getByRole("listitem")).toHaveCount(2);
    await expect(hints).toContainText("Comida");
    await expect(hints).toContainText("+33.33%");
    await expect(hints).toContainText("Servicios");
    await expect(hints).toContainText("-50.00%");
    // Renta is exactly on budget and Suscripciones has none: no hints for them.
    await expect(hints).not.toContainText("Renta");
    await expect(hints).not.toContainText("Suscripciones");

    await expect(page.getByRole("region", { name: "Declaración del mes" })).toContainText("Sin declarar");
    expect(api.closeWrites).toHaveLength(0);
    expect(api.closes).toHaveLength(0);
  });

  test("recomputes when the month changes", async ({ page }) => {
    await openClose(page, withMovements);
    await page.getByLabel("Mes a cerrar").fill(OLDER);
    await expect(page.getByRole("heading", { name: `Vista previa del cierre de ${label(OLDER)}` })).toBeVisible();
    await expect(page.getByText("Ninguna categoría de gasto excedió su presupuesto.")).toBeVisible();
  });

  test("a paused month sends the remainder to Gastos futuros instead of Inversiones", async ({ page }) => {
    // The mocked settings pause investments in 2026-10 and 2026-11.
    await openClose(page, {
      income: [{ ...MOVEMENTS.income[0], date: "2026-10-05" }],
      expenses: [{ ...MOVEMENTS.expenses[0], date: "2026-10-06" }],
      savings: [{ ...MOVEMENTS.savings[0], date: "2026-08-01", amount: "60000.00", amount_mxn: "60000.00" }],
    });
    await page.getByLabel("Mes a cerrar").fill("2026-10");
    const suggestion = page.getByRole("list", { name: "Sugerencia del dinero disponible" });
    await expect(suggestion).toContainText("Mover a Gastos futuros");
    await expect(suggestion).toContainText("Inversiones está en pausa este mes");
    await expect(suggestion).not.toContainText("Mover a Inversiones");
    await expect(suggestion).not.toContainText("Mover a Fondo de emergencia");
  });

  test("a complete emergency fund sends everything to Inversiones", async ({ page }) => {
    await openClose(page, {
      ...withMovements,
      savings: [{ ...MOVEMENTS.savings[0], date: `${OLDER}-01`, amount: "60000.00", amount_mxn: "60000.00" }],
    });
    const suggestion = page.getByRole("list", { name: "Sugerencia del dinero disponible" });
    await expect(suggestion).toContainText("Mover a Inversiones");
    await expect(suggestion).not.toContainText("Fondo de emergencia");
    await expect(page.getByText("Meta alcanzada.")).toBeVisible();
  });

  test("a future month can be previewed but not closed", async ({ page }) => {
    await openClose(page);
    await page.getByLabel("Mes a cerrar").fill(NEXT);
    await expect(page.getByText("Este mes aún no ha llegado")).toBeVisible();
    await expect(page.getByRole("button", { name: "Cerrar mes" })).toBeDisabled();
  });

  test("shows the preview error with a retry", async ({ page }) => {
    await openClose(page, { monthCloseFail: "preview" });
    await expect(page.getByRole("alert").filter({ hasText: "El servidor tuvo un problema" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Reintentar" }).first()).toBeVisible();
  });

  test("explains incomplete settings and links to Configuración", async ({ page }) => {
    await openClose(page, { monthCloseFail: "incomplete" });
    await expect(page.getByRole("alert").filter({ hasText: "Faltan datos en Configuración" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Ir a Configuración" })).toBeVisible();
  });
});

test.describe("closing a month", () => {
  test("stores the snapshot only after confirming", async ({ page }) => {
    const api = await openClose(page, withMovements);
    await page.getByRole("button", { name: "Cerrar mes" }).click();
    const dialog = page.getByRole("dialog", { name: `Cerrar ${label(PREV)}` });
    await expect(dialog).toContainText("foto fija");
    await dialog.getByRole("button", { name: "Cancelar" }).click();
    await expect(dialog).toBeHidden();
    expect(api.closeWrites).toHaveLength(0);

    await page.getByRole("button", { name: "Cerrar mes" }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Sí, cerrar mes" }).click();
    await expect(page.getByRole("dialog")).toBeHidden();
    await expect(page.getByRole("status").filter({ hasText: `Cierre de ${label(PREV)} guardado` })).toBeVisible();
    expect(api.closeWrites).toEqual([{ method: "POST", path: "/month-close", body: { period: PREV } }]);
    expect(api.closes).toHaveLength(1);

    // The history lists it with its detail open, and the preview now knows it is closed.
    await expect(historyRow(page, PREV)).toBeVisible();
    await expect(page.getByRole("region", { name: `Detalle del cierre de ${label(PREV)}` })).toBeVisible();
    await expect(page.getByText("Este mes ya está cerrado desde el")).toBeVisible();
    await expect(page.getByRole("button", { name: "Cerrar mes" })).toHaveCount(0);
  });

  test("a stored close keeps its figures when the movements change afterwards", async ({ page }) => {
    const api = await openClose(page, withMovements);
    await page.getByRole("button", { name: "Cerrar mes" }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Sí, cerrar mes" }).click();
    await expect(historyRow(page, PREV)).toBeVisible();

    // A new expense in the closed month is recorded afterwards.
    api.expenses.push(expense(99, "Comida", "5000.00"));
    await page.reload();

    await expect(page.getByRole("region", { name: "Resumen del mes" }).first()).toContainText("$26,000.50");
    await historyRow(page, PREV).click();
    const detail = page.getByRole("region", { name: `Detalle del cierre de ${label(PREV)}` });
    await expect(detail.getByRole("region", { name: "Resumen del mes" })).toContainText("$21,000.50");
    await expect(detail.getByRole("region", { name: "Resumen del mes" })).toContainText("$3,999.50");
    await expect(detail).toContainText("Estas cifras no cambian");
  });

  test("keeps the dialog open and shows the error when storing fails", async ({ page }) => {
    const api = await openClose(page, { ...withMovements, monthCloseFail: "create" });
    await page.getByRole("button", { name: "Cerrar mes" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("button", { name: "Sí, cerrar mes" }).click();
    await expect(dialog.getByRole("alert").filter({ hasText: "El servidor tuvo un problema" })).toBeVisible();
    await expect(dialog).toBeVisible();
    expect(api.closes).toHaveLength(0);
  });

  test("a month that is already closed offers the stored close instead of closing again", async ({ page }) => {
    await openClose(page, { ...withMovements, closes: [stored(PREV)] });
    await expect(page.getByText("Este mes ya está cerrado desde el")).toBeVisible();
    await expect(page.getByText("elimínalo primero")).toBeVisible();
    await expect(page.getByRole("button", { name: "Cerrar mes" })).toHaveCount(0);
    await page.getByRole("button", { name: "Ver cierre guardado" }).click();
    await expect(page.getByRole("region", { name: `Detalle del cierre de ${label(PREV)}` })).toBeVisible();
  });
});

test.describe("month close history", () => {
  const CLOSES = [stored(OLDER, { available: "-150.00", suggestion: null, adjustments: [], categories: [], tax_filing_status: null }), stored(PREV)];

  test("lists the stored closes newest first with a summary", async ({ page }) => {
    await openClose(page, { closes: CLOSES });
    const items = page.getByRole("list", { name: "Cierres guardados" }).getByRole("listitem");
    await expect(items).toHaveCount(2);
    await expect(items.nth(0)).toContainText(label(PREV));
    await expect(items.nth(0)).toContainText("$7,860.50");
    await expect(items.nth(0)).toContainText("1 categoría excedida");
    await expect(items.nth(1)).toContainText(label(OLDER));
    await expect(items.nth(1)).toContainText("-$150.00");
  });

  test("opens the detail of a close and closes it", async ({ page }) => {
    await openClose(page, { closes: CLOSES });
    await historyRow(page, PREV).click();
    const detail = page.getByRole("region", { name: `Detalle del cierre de ${label(PREV)}` });
    await expect(detail).toContainText("Guardado el");
    await expect(detail.getByRole("list", { name: "Categorías sobre presupuesto" })).toContainText("Comida");
    await expect(detail.getByRole("region", { name: "Declaración del mes" })).toContainText("Pago pendiente");
    await expect(detail.getByRole("list", { name: "Sugerencia del dinero disponible" })).toContainText("$7,860.50");
    await expect(detail.getByRole("list", { name: "Ajustes de presupuesto sugeridos" })).toContainText("+50.00%");

    await detail.getByRole("button", { name: "Cerrar detalle" }).click();
    await expect(detail).toBeHidden();

    // A close without a suggestion, hints or filing status says so.
    await historyRow(page, OLDER).click();
    const older = page.getByRole("region", { name: `Detalle del cierre de ${label(OLDER)}` });
    await expect(older.getByText("No quedó dinero disponible este mes")).toBeVisible();
    await expect(older.getByText("Ningún presupuesto se desvió más de 20%")).toBeVisible();
    await expect(older.getByRole("region", { name: "Declaración del mes" })).toHaveCount(0);
  });

  test("deletes a close only after confirming, warning that the snapshot is discarded", async ({ page }) => {
    const api = await openClose(page, { ...withMovements, closes: CLOSES });
    await historyRow(page, PREV).click();
    await page.getByRole("button", { name: "Eliminar cierre" }).click();
    const dialog = page.getByRole("dialog", { name: `Eliminar el cierre de ${label(PREV)}` });
    await expect(dialog).toContainText("Se descarta el resumen guardado");
    await expect(dialog).toContainText("Tus ingresos, gastos y ahorros no se tocan");
    await dialog.getByRole("button", { name: "No, conservarlo" }).click();
    await expect(dialog).toBeHidden();
    expect(api.closeWrites).toHaveLength(0);

    await page.getByRole("button", { name: "Eliminar cierre" }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Sí, eliminar cierre" }).click();
    await expect(page.getByRole("dialog")).toBeHidden();
    await expect(page.getByRole("status").filter({ hasText: `Cierre de ${label(PREV)} eliminado` })).toBeVisible();
    expect(api.closeWrites).toEqual([{ method: "DELETE", path: `/month-close/${PREV}`, body: undefined }]);
    expect(api.closes.map((c) => c.period)).toEqual([OLDER]);
    // Movements are never touched, and the month can be closed again.
    expect(api.expenses).toHaveLength(3);
    await expect(historyRow(page, PREV)).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Cerrar mes" })).toBeVisible();
  });

  test("shows the delete error inside the dialog and keeps the close", async ({ page }) => {
    const api = await openClose(page, { closes: CLOSES, monthCloseFail: "delete" });
    await historyRow(page, PREV).click();
    await page.getByRole("button", { name: "Eliminar cierre" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("button", { name: "Sí, eliminar cierre" }).click();
    await expect(dialog.getByRole("alert").filter({ hasText: "El servidor tuvo un problema" })).toBeVisible();
    expect(api.closes).toHaveLength(2);
  });

  test("shows the history error with a retry", async ({ page }) => {
    await openClose(page, { monthCloseFail: "list" });
    await expect(page.getByRole("alert").filter({ hasText: "El servidor tuvo un problema" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Reintentar" })).toBeVisible();
  });
});

test.describe("month close layout", () => {
  const LONG = "Una categoría con un nombre larguísimo que debe partirse sin desbordar la pantalla ".repeat(2);

  test("has no horizontal overflow with data, the detail and the dialogs", async ({ page }) => {
    await openClose(page, {
      ...withMovements,
      closes: [
        stored(OLDER, {
          categories: [{ category: LONG, spent: "9000.00", budget: "6000", remaining: "-3000.00", over_budget: true }],
          adjustments: [{ category: LONG, kind: "Gasto", budget: "6000", real: "9000.00", deviation_pct: "50.00" }],
        }),
      ],
    });
    await expect(page.getByRole("list", { name: "Sugerencia del dinero disponible" })).toBeVisible();
    await expectNoHorizontalOverflow(page, "month close preview");

    await historyRow(page, OLDER).click();
    await expect(page.getByRole("region", { name: `Detalle del cierre de ${label(OLDER)}` })).toBeVisible();
    await expectNoHorizontalOverflow(page, "month close detail");

    await page.getByRole("button", { name: "Eliminar cierre" }).click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await expectNoHorizontalOverflow(page, "month close delete dialog");
    await page.getByRole("button", { name: "No, conservarlo" }).click();

    await page.getByRole("button", { name: "Cerrar mes" }).click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await expectNoHorizontalOverflow(page, "month close confirm dialog");
  });

  test("has no horizontal overflow in the empty and error states", async ({ page }) => {
    await openClose(page);
    await expect(page.getByText("Aún no has cerrado ningún mes")).toBeVisible();
    await expectNoHorizontalOverflow(page, "month close empty");
  });

  test("has no horizontal overflow in the error state", async ({ page }) => {
    await openClose(page, { monthCloseFail: "preview" });
    await expect(page.getByText("El servidor tuvo un problema").first()).toBeVisible();
    await expectNoHorizontalOverflow(page, "month close error");
  });
});
