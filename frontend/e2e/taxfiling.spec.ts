import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockFiling, type MockInvoice } from "./helpers";

function today() {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

const PERIOD = "2026-10";
const UUID_A = "6F1C2B3A-4D5E-4F60-8A7B-9C0D1E2F3A4B";
const UUID_B = "11111111-2222-3333-4444-555555555555";

const invoice = (over: Partial<MockInvoice> = {}): MockInvoice => ({
  id: 1,
  client_id: "usa",
  collection_date: `${PERIOD}-15`,
  period: PERIOD,
  currency: "USD",
  exchange_rate: "17.50",
  subtotal: "2500.00",
  subtotal_mxn: "43750.00",
  iva: "0.00",
  isr_withheld: "0.00",
  iva_withheld: "0.00",
  total: "2500.00",
  expected_deposit_mxn: "43750.00",
  state: "emitida",
  uuid: UUID_A,
  movement_id: null,
  declaration_period: null,
  created_at: "2026-10-01T12:00:00Z",
  ...over,
});

/** October: client USA (43,750) and client B (10,000 + 1,600 IVA) issued, plus a prepared and a cancelled invoice that must not count. */
const OCTOBER: MockInvoice[] = [
  invoice(),
  invoice({
    id: 2, client_id: "b", currency: "MXN", exchange_rate: null, collection_date: `${PERIOD}-20`, subtotal: "10000.00", subtotal_mxn: "10000.00",
    iva: "1600.00", total: "11600.00", expected_deposit_mxn: "11600.00", uuid: UUID_B,
  }),
  invoice({ id: 3, client_id: "b", currency: "MXN", exchange_rate: null, subtotal: "999.00", subtotal_mxn: "999.00", state: "preparada", uuid: null }),
  invoice({ id: 4, client_id: "b", currency: "MXN", exchange_rate: null, subtotal: "888.00", subtotal_mxn: "888.00", state: "cancelada" }),
];

const filing = (over: Partial<MockFiling> = {}): MockFiling => ({
  period: PERIOD,
  filing_date: "2026-11-05",
  due_date: "2026-11-17",
  income_collected: "53750.00",
  isr_rate: "0.011",
  isr_accrued: "591.25",
  isr_withheld: "0.00",
  isr_due: "591.25",
  iva_transferred: "1600.00",
  iva_withheld: "0.00",
  iva_acreditable: "0.00",
  iva_due: "1600.00",
  total_to_pay: "2191.25",
  folio: "ACUSE-2026-10",
  status: "pendiente",
  payment: null,
  expense_movement_id: null,
  invoice_ids: [1, 2],
  created_at: "2026-11-05T12:00:00Z",
  ...over,
});

const paidFiling = (over: Partial<MockFiling> = {}) =>
  filing({
    period: "2026-09",
    due_date: "2026-10-17",
    folio: "ACUSE-2026-09",
    filing_date: "2026-10-10",
    status: "pagada",
    payment: { date: "2026-10-12", isr_paid: "500.00", iva_paid: "100.00", total_paid: "600.00" },
    invoice_ids: [],
    ...over,
  });

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

async function openTaxFiling(page: Page, opts: Parameters<typeof mockApi>[1] = {}, path = `/declaracion?period=${PERIOD}`) {
  const api = await mockApi(page, { invoices: OCTOBER, ...opts });
  await seedSession(page);
  await page.goto(path);
  await expect(page.getByRole("heading", { level: 1, name: "Declaración" })).toBeVisible();
  return api;
}

async function openRecords(page: Page, opts: Parameters<typeof mockApi>[1] = {}) {
  const api = await mockApi(page, opts);
  await seedSession(page);
  await page.goto("/declaraciones-presentadas");
  await expect(page.getByRole("heading", { level: 1, name: "Declaraciones presentadas" })).toBeVisible();
  return api;
}

const breakdown = (page: Page, name: string) => page.getByRole("region", { name });
const row = (page: Page, name: RegExp | string) => page.getByRole("row", { name });

test.describe("tax filing page", () => {
  test("computes the declaration from the issued invoices of the period", async ({ page }) => {
    await openTaxFiling(page);
    await expect(page.getByRole("heading", { level: 2, name: /Declaración de octubre de 2026/ })).toBeVisible();

    const income = breakdown(page, "Ingresos e ISR");
    await expect(income).toContainText("Ingresos cobrados (sin IVA)$53,750.00");
    await expect(income).toContainText("Tasa RESICO aplicable1.1%");
    await expect(income).toContainText("ISR causado$591.25");
    await expect(income).toContainText("ISR a pagar$591.25");
    const iva = breakdown(page, "IVA");
    await expect(iva).toContainText("IVA trasladado$1,600.00");
    await expect(iva).toContainText("IVA a cargo$1,600.00");
    await expect(iva).toContainText("Actos a tasa 0% (exportación, Cliente USA)$43,750.00");
    await expect(page.getByText("Total a pagar al SAT")).toBeVisible();
    await expect(page.getByText("$2,191.25", { exact: true })).toBeVisible();
    await expect(page.getByText("17 de noviembre de 2026")).toBeVisible();

    // Only the issued invoices are listed; the prepared one is a warning.
    const included = page.getByRole("list", { name: "Facturas incluidas" });
    await expect(included.getByRole("listitem")).toHaveCount(2);
    await expect(included).toContainText("Factura #1");
    await expect(included).toContainText("Factura #2");
    await expect(included).not.toContainText("Factura #3");
    await expect(included).not.toContainText("Factura #4");
    await expect(page.getByRole("status", { name: "Advertencias" })).toContainText("(#3)");
  });

  test("shows an empty period as a declaration at zero", async ({ page }) => {
    await openTaxFiling(page, {}, "/declaracion?period=2026-01");
    await expect(page.getByText("No hay facturas emitidas en este periodo")).toBeVisible();
    await expect(breakdown(page, "Ingresos e ISR")).toContainText("Ingresos cobrados (sin IVA)$0.00");
    await expect(page.getByRole("button", { name: "Registrar declaración" })).toBeVisible();
  });

  test("recalculates with the creditable IVA and validates the fields", async ({ page }) => {
    await openTaxFiling(page);
    await page.getByLabel("IVA acreditable (opcional)").fill("500");
    await page.getByRole("button", { name: "Calcular" }).click();
    await expect(breakdown(page, "IVA")).toContainText("IVA acreditable (gastos deducibles)$500.00");
    await expect(breakdown(page, "IVA")).toContainText("IVA a cargo$1,100.00");
    await expect(page.getByText("$1,691.25", { exact: true })).toBeVisible();

    // More creditable IVA than transferred shows the balance in favor.
    await page.getByLabel("IVA acreditable (opcional)").fill("2000");
    await page.getByRole("button", { name: "Calcular" }).click();
    await expect(breakdown(page, "IVA")).toContainText("IVA a favor (saldo a favor)$400.00");

    await page.getByLabel("IVA acreditable (opcional)").fill("abc");
    await page.getByRole("button", { name: "Calcular" }).click();
    await expect(page.getByText("Escribe un monto de cero o más")).toBeVisible();

    await page.getByLabel("IVA acreditable (opcional)").fill("");
    await page.getByLabel("Periodo", { exact: true }).fill("");
    await page.getByRole("button", { name: "Calcular" }).click();
    await expect(page.getByText("Elige el periodo (mes y año) que vas a declarar.")).toBeVisible();
  });

  test("registers the filing with the payment pending and links the invoices", async ({ page }) => {
    const api = await openTaxFiling(page);
    await page.getByLabel("Folio del acuse (opcional)").fill("ACUSE-123");
    await page.getByRole("button", { name: "Registrar declaración" }).click();

    await expect(page.getByRole("status").filter({ hasText: "registrada con el pago pendiente" })).toBeVisible();
    const write = api.taxWrites.find((w) => w.method === "POST" && w.path === "/tax-filing");
    expect(write?.body).toEqual({ period: PERIOD, filing_date: today(), folio: "ACUSE-123" });
    expect(api.filings).toHaveLength(1);
    expect(api.filings[0]).toMatchObject({ status: "pendiente", payment: null, expense_movement_id: null, invoice_ids: [1, 2] });
    // The included invoices carry the declaration period now.
    expect(api.invoices.filter((i) => i.declaration_period === PERIOD).map((i) => i.id)).toEqual([1, 2]);
    expect(api.expenses).toHaveLength(0);

    // The page now shows the period as filed instead of the register form.
    await expect(page.getByRole("button", { name: "Registrar declaración" })).toHaveCount(0);
    await expect(page.getByText(/Este periodo se presentó el/)).toBeVisible();
    await expect(page.getByRole("link", { name: "Ver en Declaraciones presentadas" })).toBeVisible();
  });

  test("registers a paid filing without an expense unless asked", async ({ page }) => {
    const api = await openTaxFiling(page);
    await page.getByRole("checkbox", { name: "Ya pagué esta declaración al SAT" }).check();
    // The amounts default to what the declaration says to pay.
    await expect(page.getByLabel("ISR pagado (MXN)")).toHaveValue("591.25");
    await expect(page.getByLabel("IVA pagado (MXN)")).toHaveValue("1600.00");
    await expect(page.getByRole("checkbox", { name: /Registrar el pago como gasto/ })).not.toBeChecked();
    await page.getByRole("button", { name: "Registrar declaración" }).click();

    await expect(page.getByRole("status").filter({ hasText: "registrada y pagada" })).toBeVisible();
    const body = api.taxWrites.find((w) => w.path === "/tax-filing")?.body as { payment: Record<string, unknown> };
    expect(body.payment).toEqual({ date: today(), isr_paid: "591.25", iva_paid: "1600.00" });
    expect(api.expenses).toHaveLength(0);
    expect(api.filings[0].status).toBe("pagada");
  });

  test("registers a paid filing and its Impuestos expense when the box is checked", async ({ page }) => {
    const api = await openTaxFiling(page);
    await page.getByRole("checkbox", { name: "Ya pagué esta declaración al SAT" }).check();
    await page.getByLabel("ISR pagado (MXN)").fill("600");
    await page.getByRole("checkbox", { name: /Registrar el pago como gasto/ }).check();
    await page.getByRole("button", { name: "Registrar declaración" }).click();

    await expect(page.getByRole("status").filter({ hasText: "Se registró el gasto de Impuestos" })).toBeVisible();
    const body = api.taxWrites.find((w) => w.path === "/tax-filing")?.body as { payment: Record<string, unknown> };
    expect(body.payment).toMatchObject({ isr_paid: "600", iva_paid: "1600.00", record_expense: true });
    expect(api.expenses).toHaveLength(1);
    expect(api.expenses[0]).toMatchObject({ category: "Impuestos", currency: "MXN", amount_mxn: "2200.00" });
    expect(api.filings[0].expense_movement_id).toBe(api.expenses[0].id);
  });

  test("validates the payment before registering", async ({ page }) => {
    const api = await openTaxFiling(page);
    await page.getByRole("checkbox", { name: "Ya pagué esta declaración al SAT" }).check();
    await page.getByLabel("ISR pagado (MXN)").fill("-5");
    await page.getByRole("button", { name: "Registrar declaración" }).click();
    await expect(page.getByText("Escribe un monto de cero o más")).toBeVisible();
    expect(api.taxWrites).toHaveLength(0);

    // Zero amounts cannot become an expense.
    await page.getByLabel("ISR pagado (MXN)").fill("0");
    await page.getByLabel("IVA pagado (MXN)").fill("0");
    await page.getByRole("checkbox", { name: /Registrar el pago como gasto/ }).check();
    await page.getByRole("button", { name: "Registrar declaración" }).click();
    await expect(page.getByText("Para registrar el gasto, el ISR o el IVA pagado debe ser mayor a cero.")).toBeVisible();
    expect(api.taxWrites).toHaveLength(0);
  });

  test("a filed period shows its status instead of the register form", async ({ page }) => {
    await openTaxFiling(page, { filings: [filing({ status: "pagada", payment: { date: "2026-11-12", isr_paid: "591.25", iva_paid: "1600.00", total_paid: "2191.25" } })] });
    await expect(page.getByText("Pagada", { exact: true }).first()).toBeVisible();
    await expect(page.getByText(/ya fue declarado/)).toBeVisible();
    await expect(page.getByText(/folio ACUSE-2026-10/)).toBeVisible();
    await expect(page.getByRole("button", { name: "Registrar declaración" })).toHaveCount(0);
  });

  test("defaults to the previous month without a period in the link", async ({ page }) => {
    await openTaxFiling(page, {}, "/declaracion");
    const d = new Date();
    d.setDate(1);
    d.setMonth(d.getMonth() - 1);
    const expected = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
    await expect(page.getByLabel("Periodo", { exact: true })).toHaveValue(expected);
  });

  test("shows the error with a retry when the calculation fails", async ({ page }) => {
    await openTaxFiling(page, { taxFilingFail: "preview" });
    await expect(page.getByText("El servidor tuvo un problema")).toBeVisible();
    await expect(page.getByRole("button", { name: "Reintentar" })).toBeVisible();
  });

  test("points to Configuración when the tax settings are incomplete", async ({ page }) => {
    await openTaxFiling(page, { taxFilingFail: "incomplete" });
    await expect(page.getByText("Faltan parámetros fiscales")).toBeVisible();
    await page.getByRole("link", { name: "Ir a Configuración" }).click();
    await expect(page).toHaveURL(/\/configuracion$/);
  });

  test("has no horizontal overflow with the register form, the payment fields and an error", async ({ page }) => {
    await openTaxFiling(page);
    await expect(breakdown(page, "IVA")).toBeVisible();
    await expectNoHorizontalOverflow(page, "tax filing preview");
    await page.getByRole("checkbox", { name: "Ya pagué esta declaración al SAT" }).check();
    await page.getByLabel("ISR pagado (MXN)").fill("-1");
    await page.getByRole("button", { name: "Registrar declaración" }).click();
    await expect(page.getByText("Escribe un monto de cero o más")).toBeVisible();
    await expectNoHorizontalOverflow(page, "tax filing with payment errors");
  });

  test("has no horizontal overflow in the error state", async ({ page }) => {
    await openTaxFiling(page, { taxFilingFail: "incomplete" });
    await expect(page.getByText("Faltan parámetros fiscales")).toBeVisible();
    await expectNoHorizontalOverflow(page, "tax filing error");
  });
});

test.describe("filed records page", () => {
  test("shows the empty states", async ({ page }) => {
    await openRecords(page);
    await expect(page.getByText("No tienes periodos pendientes de declarar")).toBeVisible();
    await expect(page.getByText("Aún no hay declaraciones registradas.")).toBeVisible();
  });

  test("lists the periods still to file and links to the declaration", async ({ page }) => {
    await openRecords(page, {
      invoices: [
        invoice({ id: 1, period: "2025-01", collection_date: "2025-01-10" }),
        invoice({ id: 2, period: "2099-01", collection_date: "2099-01-10", uuid: UUID_B }),
        invoice({ id: 3, period: "2025-02", collection_date: "2025-02-10", state: "preparada", uuid: null }),
      ],
      // A filed period is not pending.
      filings: [filing({ period: "2099-01", invoice_ids: [2] })],
    });
    const pending = page.getByRole("list", { name: "Periodos pendientes de declarar" });
    await expect(pending.getByRole("listitem")).toHaveCount(1);
    const item = pending.getByRole("listitem").first();
    await expect(item).toContainText("enero de 2025");
    await expect(item).toContainText("Vencido");
    await expect(item).toContainText("17 de febrero de 2025");
    await item.getByRole("link", { name: "Declarar enero de 2025" }).click();
    await expect(page).toHaveURL(/\/declaracion\?period=2025-01$/);
    await expect(page.getByLabel("Periodo", { exact: true })).toHaveValue("2025-01");
  });

  test("alerts about invoices issued after their period was filed", async ({ page }) => {
    await openRecords(page, {
      invoices: [
        invoice({ id: 1, declaration_period: PERIOD }),
        // Issued after the filing was registered: never linked, its income is undeclared.
        invoice({ id: 2, client_id: "b", currency: "MXN", exchange_rate: null, subtotal: "10000.00", subtotal_mxn: "10000.00", uuid: UUID_B }),
      ],
      filings: [filing({ invoice_ids: [1] })],
    });
    const alert = page.getByRole("alert").filter({ hasText: "Facturas emitidas en un periodo ya declarado" });
    await expect(alert).toBeVisible();
    await expect(alert.getByRole("listitem")).toHaveCount(1);
    await expect(alert.getByRole("listitem")).toContainText("Factura #2");
    await expect(alert.getByRole("listitem")).toContainText("octubre de 2026");
  });

  test("shows no unfiled-invoices alert when every invoice is linked to its filing", async ({ page }) => {
    await openRecords(page, { invoices: [invoice({ id: 1, declaration_period: PERIOD })], filings: [filing({ invoice_ids: [1] })] });
    await expect(page.getByRole("heading", { name: "Historial de declaraciones" })).toBeVisible();
    await expect(page.getByRole("alert").filter({ hasText: "Facturas emitidas en un periodo ya declarado" })).toHaveCount(0);
  });

  test("a future period is pending without being overdue", async ({ page }) => {
    await openRecords(page, { invoices: [invoice({ period: "2099-01", collection_date: "2099-01-10" })] });
    const item = page.getByRole("list", { name: "Periodos pendientes de declarar" }).getByRole("listitem").first();
    await expect(item).toContainText("enero de 2099");
    await expect(item).not.toContainText("Vencido");
  });

  test("shows the history with status badges and filters it", async ({ page }) => {
    await openRecords(page, { invoices: OCTOBER, filings: [filing(), paidFiling(), paidFiling({ period: "2025-12", filing_date: "2026-01-10" })] });
    const table = page.getByRole("table", { name: "Historial de declaraciones presentadas" });
    await expect(table.getByRole("row")).toHaveCount(4); // header + 3
    const pending = row(page, /octubre de 2026/);
    await expect(pending).toContainText("Pago pendiente");
    await expect(pending).toContainText("ACUSE-2026-10");
    await expect(pending).toContainText("$591.25");
    await expect(pending).toContainText("$1,600.00");
    await expect(pending.getByRole("button", { name: /Registrar pago de octubre de 2026/ })).toBeVisible();
    const paid = row(page, /septiembre de 2026/);
    await expect(paid).toContainText("Pagada");
    await expect(paid).toContainText("$600.00");
    await expect(paid.getByRole("button", { name: /Registrar pago/ })).toHaveCount(0);

    await page.getByLabel("Estado del pago").selectOption({ label: "Pago pendiente" });
    await expect(table.getByRole("row")).toHaveCount(2);
    await expect(table).toContainText("octubre de 2026");
    await page.getByRole("button", { name: "Quitar filtros" }).click();
    await expect(table.getByRole("row")).toHaveCount(4);

    await page.getByLabel("Año").fill("2025");
    await expect(table.getByRole("row")).toHaveCount(2);
    await expect(table).toContainText("diciembre de 2025");
    await page.getByLabel("Año").fill("2019");
    await expect(page.getByText("No hay declaraciones con estos filtros.")).toBeVisible();
  });

  test("marks a filing as paid without creating an expense by default", async ({ page }) => {
    const api = await openRecords(page, { invoices: OCTOBER, filings: [filing()] });
    await row(page, /octubre de 2026/).getByRole("button", { name: /Registrar pago de octubre de 2026/ }).click();

    const dialog = page.getByRole("dialog", { name: "Registrar pago de octubre de 2026" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByLabel("ISR pagado (MXN)")).toHaveValue("591.25");
    await expect(dialog.getByLabel("IVA pagado (MXN)")).toHaveValue("1600.00");
    await expect(dialog.getByLabel("Fecha de pago")).toHaveValue(today());
    const expense = dialog.getByRole("checkbox", { name: /Registrar el pago como gasto/ });
    await expect(expense).not.toBeChecked();
    await dialog.getByLabel("ISR pagado (MXN)").fill("590.50");
    await dialog.getByRole("button", { name: "Registrar pago" }).click();

    await expect(dialog).toBeHidden();
    await expect(page.getByRole("status").filter({ hasText: "Pago de octubre de 2026 registrado." })).toBeVisible();
    const write = api.taxWrites.find((w) => w.path === `/tax-filing/${PERIOD}/payment`);
    expect(write?.body).toEqual({ date: today(), isr_paid: "590.50", iva_paid: "1600.00" });
    expect(api.expenses).toHaveLength(0);
    const paid = row(page, /octubre de 2026/);
    await expect(paid).toContainText("Pagada");
    await expect(paid).toContainText("$2,190.50");
    await expect(paid.getByRole("button", { name: /Registrar pago/ })).toHaveCount(0);
  });

  test("records the payment as an Impuestos expense only when the box is checked", async ({ page }) => {
    const api = await openRecords(page, { invoices: OCTOBER, filings: [filing()] });
    await row(page, /octubre de 2026/).getByRole("button", { name: /Registrar pago de octubre de 2026/ }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("checkbox", { name: /Registrar el pago como gasto/ }).check();
    await dialog.getByRole("button", { name: "Registrar pago" }).click();

    await expect(page.getByRole("status").filter({ hasText: "Se registró el gasto de Impuestos." })).toBeVisible();
    const write = api.taxWrites.find((w) => w.path === `/tax-filing/${PERIOD}/payment`);
    expect(write?.body).toMatchObject({ isr_paid: "591.25", iva_paid: "1600.00", record_expense: true });
    expect(api.expenses).toHaveLength(1);
    expect(api.expenses[0]).toMatchObject({ category: "Impuestos", amount_mxn: "2191.25", date: today() });
    expect(api.filings[0].expense_movement_id).toBe(api.expenses[0].id);
  });

  test("validates the payment dialog and closes it with Escape, restoring focus", async ({ page }) => {
    const api = await openRecords(page, { invoices: OCTOBER, filings: [filing()] });
    const opener = row(page, /octubre de 2026/).getByRole("button", { name: /Registrar pago de octubre de 2026/ });
    await opener.click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("IVA pagado (MXN)").fill("1,600");
    await dialog.getByRole("button", { name: "Registrar pago" }).click();
    await expect(dialog.getByText("Escribe un monto de cero o más")).toBeVisible();
    expect(api.taxWrites).toHaveLength(0);

    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(opener).toBeFocused();
  });

  test("shows the API error inside the dialog and keeps it open", async ({ page }) => {
    await openRecords(page, { invoices: OCTOBER, filings: [filing()], taxFilingFail: "pay" });
    await row(page, /octubre de 2026/).getByRole("button", { name: /Registrar pago de octubre de 2026/ }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("button", { name: "Registrar pago" }).click();
    await expect(dialog.getByText("El servidor tuvo un problema")).toBeVisible();
    await expect(dialog).toBeVisible();
  });

  test("opening a filing detail scrolls to it and flashes the highlight", async ({ page }) => {
    await openRecords(page, { filings: [filing()] });
    await row(page, /octubre de 2026/).getByRole("button", { name: /Ver declaración de octubre de 2026/ }).click();
    const detail = page.getByRole("region", { name: "Detalle de la declaración de octubre de 2026" });
    await expect(detail).toBeInViewport();
    const panel = detail.locator("xpath=..");
    await expect(panel).toHaveClass(/(^|\s)edit-highlight(\s|$)/);
    await expect(panel).not.toHaveClass(/(^|\s)edit-highlight(\s|$)/, { timeout: 4_000 });
  });

  test("shows the detail of a pending filing and deletes it after confirming", async ({ page }) => {
    const api = await openRecords(page, { invoices: [{ ...OCTOBER[0], declaration_period: PERIOD }, { ...OCTOBER[1], declaration_period: PERIOD }], filings: [filing()] });
    await row(page, /octubre de 2026/).getByRole("button", { name: /Ver declaración de octubre de 2026/ }).click();
    const detail = page.getByRole("region", { name: "Detalle de la declaración de octubre de 2026" });
    await expect(detail).toContainText("ACUSE-2026-10");
    await expect(detail).toContainText("Pendiente: aún no registras el pago al SAT.");
    await expect(detail.getByRole("list", { name: "Facturas incluidas" }).getByRole("listitem")).toHaveCount(2);
    await expect(detail.getByRole("region", { name: "Ingresos e ISR" })).toContainText("$53,750.00");

    await detail.getByRole("button", { name: "Eliminar registro" }).click();
    await expect(detail.getByText(/¿Eliminar este registro\?/)).toBeVisible();
    await detail.getByRole("button", { name: "No, conservarlo" }).click();
    expect(api.filings).toHaveLength(1);

    await detail.getByRole("button", { name: "Eliminar registro" }).click();
    await detail.getByRole("button", { name: "Sí, eliminar registro" }).click();
    await expect(page.getByRole("status").filter({ hasText: "Se eliminó el registro de octubre de 2026" })).toBeVisible();
    await expect(page.getByText("Aún no hay declaraciones registradas.")).toBeVisible();
    expect(api.filings).toHaveLength(0);
    expect(api.invoices.every((i) => i.declaration_period === null)).toBe(true);
    // Its invoices are pending to file again.
    await expect(page.getByRole("list", { name: "Periodos pendientes de declarar" })).toContainText("octubre de 2026");
  });

  test("a paid filing shows its payment and expense and cannot be deleted", async ({ page }) => {
    await openRecords(page, { filings: [paidFiling({ expense_movement_id: 12 })] });
    await row(page, /septiembre de 2026/).getByRole("button", { name: /Ver declaración/ }).click();
    const detail = page.getByRole("region", { name: "Detalle de la declaración de septiembre de 2026" });
    await expect(detail).toContainText("ISR pagado$500.00");
    await expect(detail).toContainText("IVA pagado$100.00");
    await expect(detail).toContainText("Registrado (movimiento #12)");
    await expect(detail.getByRole("button", { name: "Eliminar registro" })).toHaveCount(0);
    await expect(detail.getByRole("button", { name: "Registrar pago" })).toHaveCount(0);
  });

  test("shows the errors with a retry", async ({ page }) => {
    await openRecords(page, { taxFilingFail: "list" });
    await expect(page.getByText("El servidor tuvo un problema")).toBeVisible();
    await expect(page.getByRole("button", { name: "Reintentar" })).toBeVisible();
  });

  test("shows the pending periods error with a retry", async ({ page }) => {
    await openRecords(page, { taxFilingFail: "pending" });
    await expect(page.getByText("El servidor tuvo un problema")).toBeVisible();
  });

  test("has no horizontal overflow with data, the detail and the dialog open", async ({ page }) => {
    await openRecords(page, { invoices: OCTOBER, filings: [filing(), paidFiling()] });
    await expect(page.getByRole("table")).toBeVisible();
    await expectNoHorizontalOverflow(page, "filed records");
    await row(page, /octubre de 2026/).getByRole("button", { name: /Ver declaración/ }).click();
    await expect(page.getByRole("region", { name: /Detalle de la declaración/ })).toBeVisible();
    await expectNoHorizontalOverflow(page, "filed records detail");
    await row(page, /octubre de 2026/).getByRole("button", { name: /Registrar pago de octubre de 2026/ }).click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await expectNoHorizontalOverflow(page, "filed records dialog");
  });

  test("has no horizontal overflow in the empty and error states", async ({ page }) => {
    await openRecords(page, { taxFilingFail: "list" });
    await expect(page.getByText("El servidor tuvo un problema").first()).toBeVisible();
    await expectNoHorizontalOverflow(page, "filed records error");
  });
});

test.describe("dashboard integration", () => {
  test("the tax card follows the filings", async ({ page }) => {
    const month = today().slice(0, 7);
    const [y, m] = month.split("-").map(Number);
    const prev = m === 1 ? `${y - 1}-12` : `${y}-${String(m - 1).padStart(2, "0")}`;
    // The previous period has an issued invoice and no filing yet.
    await mockApi(page, {
      invoices: [invoice({ id: 1, period: prev, collection_date: `${prev}-10` })],
      income: [{ id: 1, date: today(), description: "Sueldo", category: "Sueldo", payment_method: "Transferencia", currency: "MXN", amount: "60000.00", exchange_rate: null, amount_mxn: "60000.00" }],
    });
    await seedSession(page);
    await page.goto("/");
    const tax = page.getByRole("region", { name: "ISR RESICO estimado" });
    await expect(tax).toContainText("Sin declarar");
    await expect(tax).toContainText("sigue pendiente");
  });
});
