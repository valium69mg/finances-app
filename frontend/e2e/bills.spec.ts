import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockBill } from "./helpers";

const pad = (n: number) => String(n).padStart(2, "0");

/** Local calendar date `offset` days from today, as YYYY-MM-DD. */
function day(offset = 0) {
  const d = new Date();
  d.setDate(d.getDate() + offset);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

const bill = (over: Partial<MockBill> = {}): MockBill => ({
  id: 1,
  name: "Megacable",
  category: "Servicios",
  amount: "550.00",
  currency: "MXN",
  recurrence: "monthly",
  next_due_date: day(-5),
  reminder_lead_days: 3,
  active: true,
  notes: "",
  ...over,
});

/** An overdue fixed bill, a variable one due in two days and one far away. */
const BILLS: MockBill[] = [
  bill(),
  bill({ id: 2, name: "Luz", amount: null, recurrence: "bimonthly", next_due_date: day(2), notes: "Recibo CFE" }),
  bill({ id: 3, name: "Netflix", category: "Suscripciones", amount: "139.00", next_due_date: day(40) }),
];

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

/** Opens the page; the create form is collapsed, so it is expanded unless `expanded` is false. */
async function openBills(page: Page, opts: Parameters<typeof mockApi>[1] = {}, { expanded = true } = {}) {
  const api = await mockApi(page, opts);
  await seedSession(page);
  await page.goto("/pagos-recurrentes");
  await expect(page.getByRole("heading", { level: 1, name: "Pagos recurrentes" })).toBeVisible();
  if (expanded) await page.getByRole("button", { name: "+ Nuevo pago recurrente" }).click();
  return api;
}

const rowOf = (page: Page, name: string) => page.getByRole("list", { name: "Pagos recurrentes" }).getByRole("listitem").filter({ hasText: name }).first();

test.describe("bills page", () => {
  test("the bill form is collapsed by default, expands inline and opens by itself on edit", async ({ page }) => {
    await openBills(page, { bills: BILLS }, { expanded: false });
    const toggle = page.getByRole("button", { name: "+ Nuevo pago recurrente" });
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(toggle).toHaveAttribute("aria-controls", "bill-form-panel");
    await expect(page.getByRole("form")).toHaveCount(0);

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByRole("form", { name: "Nuevo pago recurrente" })).toBeVisible();
    await expect(page.getByLabel("Nombre")).toBeFocused();
    await toggle.click();
    await expect(page.getByRole("form")).toHaveCount(0);

    await rowOf(page, "Megacable").getByRole("button", { name: "Editar Megacable" }).click();
    const form = page.getByRole("form", { name: "Editar Megacable" });
    await expect(form).toBeVisible();
    await expect(form).toHaveClass(/(^|\s)edit-highlight(\s|$)/);
    await page.getByRole("button", { name: "Cancelar edición" }).click();
    await expect(page.getByRole("form")).toHaveCount(0);
  });

  test("shows the empty state", async ({ page }) => {
    await openBills(page);
    await expect(page.getByText("Aún no tienes pagos recurrentes")).toBeVisible();
    await expect(page.getByRole("form", { name: "Nuevo pago recurrente" })).toBeVisible();
  });

  test("lists the bills by due date with amount or variable, recurrence and badges", async ({ page }) => {
    await openBills(page, { bills: BILLS });
    const items = page.getByRole("list", { name: "Pagos recurrentes" }).getByRole("listitem");
    await expect(items).toHaveCount(3);
    await expect(items.nth(0)).toContainText("Megacable");
    await expect(items.nth(0)).toContainText("$550.00 MXN");
    await expect(items.nth(0)).toContainText("Mensual");
    await expect(items.nth(0)).toContainText("Vencido hace 5 días");
    await expect(items.nth(1)).toContainText("Luz");
    await expect(items.nth(1)).toContainText("Monto variable");
    await expect(items.nth(1)).toContainText("Bimestral");
    await expect(items.nth(1)).toContainText("Vence en 2 días");
    await expect(items.nth(1)).toContainText("Recibo CFE");
    await expect(items.nth(2)).toContainText("Netflix");
    await expect(items.nth(2)).not.toContainText("Vence");
    await expect(items.nth(2)).not.toContainText("Vencido");
  });

  test("a bill due today is due soon, not overdue, and the lead time is configurable per bill", async ({ page }) => {
    await openBills(page, {
      bills: [bill({ id: 1, name: "Hoy", next_due_date: day(0) }), bill({ id: 2, name: "Lejano", next_due_date: day(6), reminder_lead_days: 7 }), bill({ id: 3, name: "Fuera", next_due_date: day(6), reminder_lead_days: 5 })],
    });
    await expect(rowOf(page, "Hoy")).toContainText("Vence hoy");
    await expect(rowOf(page, "Lejano")).toContainText("Vence en 6 días");
    await expect(rowOf(page, "Fuera")).not.toContainText("Vence en");
    await expect(rowOf(page, "Lejano")).toContainText("Avisar 7 días antes");
  });

  test("creates a fixed monthly bill", async ({ page }) => {
    const api = await openBills(page);
    await page.getByLabel("Nombre").fill("Megacable");
    await page.getByLabel("Monto", { exact: true }).fill("550");
    await page.getByLabel("Próximo vencimiento").fill(day(10));
    await page.getByRole("button", { name: "Agregar pago recurrente" }).click();

    await expect(page.getByRole("status").filter({ hasText: "Megacable agregado" })).toBeVisible();
    const write = api.billWrites.find((w) => w.method === "POST" && w.path === "/bills");
    expect(write?.body).toEqual({
      name: "Megacable",
      category: "Servicios",
      amount: "550",
      currency: "MXN",
      recurrence: "monthly",
      next_due_date: day(10),
      reminder_lead_days: 3,
      active: true,
      notes: "",
    });
    await expect(rowOf(page, "Megacable")).toContainText("$550.00 MXN");
    // The form collapses after the save and opens empty again.
    await expect(page.getByRole("form")).toHaveCount(0);
    await page.getByRole("button", { name: "+ Nuevo pago recurrente" }).click();
    await expect(page.getByLabel("Nombre")).toHaveValue("");
  });

  test("creates a variable bill with a custom recurrence and reminder", async ({ page }) => {
    const api = await openBills(page);
    await page.getByLabel("Nombre").fill("Luz");
    await page.getByRole("checkbox", { name: "Monto variable (sin monto fijo)" }).check();
    await expect(page.getByLabel("Monto", { exact: true })).toBeDisabled();
    await page.getByLabel("Recurrencia").selectOption({ label: "Bimestral" });
    await page.getByLabel("Categoría del gasto").selectOption("Servicios");
    await page.getByLabel("Avisar con (días de anticipación)").fill("7");
    await page.getByRole("button", { name: "Agregar pago recurrente" }).click();

    await expect(rowOf(page, "Luz")).toContainText("Monto variable");
    const write = api.billWrites.find((w) => w.path === "/bills");
    expect(write?.body).toMatchObject({ name: "Luz", amount: null, recurrence: "bimonthly", reminder_lead_days: 7 });
  });

  test("validates the form before saving", async ({ page }) => {
    const api = await openBills(page);
    await page.getByRole("button", { name: "Agregar pago recurrente" }).click();
    await expect(page.getByText("Escribe el nombre del pago")).toBeVisible();
    await expect(page.getByText("Escribe un monto mayor a cero")).toBeVisible();
    expect(api.billWrites).toHaveLength(0);

    await page.getByLabel("Nombre").fill("Agua");
    await page.getByLabel("Monto", { exact: true }).fill("400");
    await page.getByLabel("Avisar con (días de anticipación)").fill("400");
    await page.getByRole("button", { name: "Agregar pago recurrente" }).click();
    await expect(page.getByText("Escribe un número de días entre 0 y 365")).toBeVisible();
    expect(api.billWrites).toHaveLength(0);
  });

  test("shows the API error when saving fails", async ({ page }) => {
    await openBills(page, { billsFail: "save" });
    await page.getByLabel("Nombre").fill("Agua");
    await page.getByLabel("Monto", { exact: true }).fill("400");
    await page.getByRole("button", { name: "Agregar pago recurrente" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "Los datos no son válidos" })).toBeVisible();
  });

  test("edits a bill", async ({ page }) => {
    const api = await openBills(page, { bills: BILLS });
    await rowOf(page, "Megacable").getByRole("button", { name: "Editar Megacable" }).click();
    await expect(page.getByRole("form", { name: "Editar Megacable" })).toBeVisible();
    await expect(page.getByLabel("Nombre")).toHaveValue("Megacable");
    await expect(page.getByLabel("Monto", { exact: true })).toHaveValue("550.00");
    await page.getByLabel("Monto", { exact: true }).fill("600");
    await page.getByLabel("Recurrencia").selectOption({ label: "Anual" });
    await page.getByRole("button", { name: "Guardar cambios" }).click();

    await expect(page.getByRole("status").filter({ hasText: "Cambios de Megacable guardados" })).toBeVisible();
    const write = api.billWrites.find((w) => w.method === "PUT" && w.path === "/bills/1");
    expect(write?.body).toMatchObject({ amount: "600", recurrence: "yearly", next_due_date: day(-5), active: true });
    await expect(rowOf(page, "Megacable")).toContainText("$600.00 MXN");
    await expect(rowOf(page, "Megacable")).toContainText("Anual");
    await expect(page.getByRole("form")).toHaveCount(0);
  });

  test("pays a fixed bill with the defaults, registers the expense and generates the next occurrence", async ({ page }) => {
    const api = await openBills(page, { bills: BILLS });
    await rowOf(page, "Megacable").getByRole("button", { name: "Pagar Megacable" }).click();

    const dialog = page.getByRole("dialog", { name: "Pagar Megacable" });
    await expect(dialog.getByLabel("Monto (MXN)")).toHaveValue("550.00");
    await expect(dialog.getByLabel("Categoría")).toHaveValue("Servicios");
    await expect(dialog.getByLabel("Fecha de pago")).toHaveValue(day(0));
    await dialog.getByRole("button", { name: "Registrar pago" }).click();

    await expect(dialog).toBeHidden();
    await expect(page.getByRole("status").filter({ hasText: "Pago de Megacable registrado: $550.00 MXN en Servicios" })).toBeVisible();
    const write = api.billWrites.find((w) => w.path === "/bills/1/pay");
    expect(write?.body).toEqual({ date: day(0), amount: "550.00", category: "Servicios" });
    expect(api.expenses).toHaveLength(1);
    expect(api.expenses[0]).toMatchObject({ description: "Megacable", category: "Servicios", currency: "MXN", amount: "550.00", date: day(0) });
    // The next occurrence replaces the paid one: no longer overdue.
    await expect(rowOf(page, "Megacable")).not.toContainText("Vencido");
    expect(api.bills[0].history).toHaveLength(1);
    expect(api.bills[0].history[0]).toMatchObject({ status: "paid", expense_movement_id: api.expenses[0].id });
  });

  test("lets the amount, category, date and description be overridden when paying", async ({ page }) => {
    const api = await openBills(page, { bills: BILLS });
    await rowOf(page, "Megacable").getByRole("button", { name: "Pagar Megacable" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("Monto (MXN)").fill("499.99");
    await dialog.getByLabel("Categoría").selectOption("Suscripciones");
    await dialog.getByLabel("Fecha de pago").fill(day(-2));
    await dialog.getByLabel("Descripción (opcional)").fill("Descuento por pronto pago");
    await dialog.getByRole("button", { name: "Registrar pago" }).click();

    await expect(page.getByRole("status").filter({ hasText: "$499.99 MXN en Suscripciones" })).toBeVisible();
    expect(api.billWrites.find((w) => w.path === "/bills/1/pay")?.body).toEqual({
      date: day(-2),
      amount: "499.99",
      category: "Suscripciones",
      description: "Descuento por pronto pago",
    });
    expect(api.expenses[0]).toMatchObject({ description: "Descuento por pronto pago", category: "Suscripciones", amount: "499.99" });
    // The bill's own amount and category are untouched.
    expect(api.bills[0]).toMatchObject({ amount: "550.00", category: "Servicios" });
  });

  test("requires the amount when the bill has none", async ({ page }) => {
    const api = await openBills(page, { bills: BILLS });
    await rowOf(page, "Luz").getByRole("button", { name: "Pagar Luz" }).click();
    const dialog = page.getByRole("dialog", { name: "Pagar Luz" });
    await expect(dialog.getByLabel("Monto (MXN)")).toHaveValue("");
    await expect(dialog.getByText(/no tiene monto fijo/)).toBeVisible();
    await dialog.getByRole("button", { name: "Registrar pago" }).click();
    await expect(dialog.getByText("Escribe un monto mayor a cero")).toBeVisible();
    expect(api.billWrites).toHaveLength(0);

    await dialog.getByLabel("Monto (MXN)").fill("212.40");
    await dialog.getByRole("button", { name: "Registrar pago" }).click();
    await expect(dialog).toBeHidden();
    expect(api.expenses[0]).toMatchObject({ description: "Luz", amount: "212.40" });
    // A bimonthly bill moves two months ahead.
    expect(api.bills.find((b) => b.id === 2)?.pending.due_date).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });

  test("closes the payment dialog with Escape, restoring focus, without paying", async ({ page }) => {
    const api = await openBills(page, { bills: BILLS });
    const opener = rowOf(page, "Megacable").getByRole("button", { name: "Pagar Megacable" });
    await opener.click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog")).toBeHidden();
    await expect(opener).toBeFocused();
    expect(api.billWrites).toHaveLength(0);
  });

  test("shows the API error inside the payment dialog and keeps it open", async ({ page }) => {
    await openBills(page, { bills: BILLS, billsFail: "pay" });
    await rowOf(page, "Megacable").getByRole("button", { name: "Pagar Megacable" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("button", { name: "Registrar pago" }).click();
    await expect(dialog.getByText("El servidor tuvo un problema")).toBeVisible();
    await expect(dialog).toBeVisible();
  });

  test("skips an occurrence only after confirming and registers no expense", async ({ page }) => {
    const api = await openBills(page, { bills: BILLS });
    const row = rowOf(page, "Megacable");
    await row.getByRole("button", { name: "Omitir Megacable" }).click();
    await expect(row.getByText(/¿Omitir este vencimiento\?/)).toBeVisible();
    await row.getByRole("button", { name: "Cancelar" }).click();
    expect(api.billWrites).toHaveLength(0);

    await row.getByRole("button", { name: "Omitir Megacable" }).click();
    await row.getByRole("button", { name: "Sí, omitir" }).click();
    await expect(page.getByRole("status").filter({ hasText: "Se omitió el vencimiento de Megacable. No se registró ningún gasto" })).toBeVisible();
    expect(api.billWrites.find((w) => w.path === "/bills/1/skip")).toBeTruthy();
    expect(api.expenses).toHaveLength(0);
    expect(api.bills[0].history[0]).toMatchObject({ status: "skipped", expense_movement_id: null });
    await expect(rowOf(page, "Megacable")).not.toContainText("Vencido");
  });

  test("shows the API error of a failed skip", async ({ page }) => {
    await openBills(page, { bills: BILLS, billsFail: "skip" });
    const row = rowOf(page, "Megacable");
    await row.getByRole("button", { name: "Omitir Megacable" }).click();
    await row.getByRole("button", { name: "Sí, omitir" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "El servidor tuvo un problema" })).toBeVisible();
  });

  test("shows the history of paid and skipped occurrences", async ({ page }) => {
    await openBills(page, {
      bills: [
        bill({
          history: [
            { id: 9, due_date: day(-35), status: "paid", paid_on: day(-33), expense_movement_id: 42, amount_paid: "499.50", currency: "MXN", resolved_at: `${day(-33)}T10:00:00Z` },
            { id: 8, due_date: day(-65), status: "skipped", paid_on: null, expense_movement_id: null, amount_paid: null, currency: null, resolved_at: `${day(-60)}T10:00:00Z` },
          ],
        }),
      ],
    });
    const row = rowOf(page, "Megacable");
    await row.getByRole("button", { name: "Ver historial de Megacable" }).click();
    const history = row.getByRole("list", { name: "Historial de Megacable" });
    await expect(history.getByRole("listitem")).toHaveCount(2);
    await expect(history).toContainText(`Pagado el ${day(-33)} · $499.50 MXN · gasto #42`);
    await expect(history).toContainText("Omitido: no se registró ningún gasto");
    await row.getByRole("button", { name: "Ocultar historial de Megacable" }).click();
    await expect(history).toBeHidden();
  });

  test("an empty history is explained", async ({ page }) => {
    await openBills(page, { bills: [bill()] });
    await rowOf(page, "Megacable").getByRole("button", { name: "Ver historial de Megacable" }).click();
    await expect(page.getByText("Aún no hay pagos ni omisiones")).toBeVisible();
  });

  test("shows the history error with a retry", async ({ page }) => {
    await openBills(page, { bills: [bill()], billsFail: "detail" });
    await rowOf(page, "Megacable").getByRole("button", { name: "Ver historial de Megacable" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "El servidor tuvo un problema" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Reintentar" })).toBeVisible();
  });

  test("deactivates a bill after confirming and reactivates it from the inactive list", async ({ page }) => {
    const api = await openBills(page, { bills: BILLS });
    const row = rowOf(page, "Netflix");
    await row.getByRole("button", { name: "Desactivar Netflix" }).click();
    await row.getByRole("button", { name: "Cancelar" }).click();
    expect(api.billWrites).toHaveLength(0);

    await row.getByRole("button", { name: "Desactivar Netflix" }).click();
    await row.getByRole("button", { name: "Sí, desactivar" }).click();
    await expect(page.getByRole("status").filter({ hasText: "Netflix se desactivó. Su historial y sus gastos se conservan." })).toBeVisible();
    await expect(page.getByRole("list", { name: "Pagos recurrentes" }).getByRole("listitem")).toHaveCount(2);
    expect(api.bills.find((b) => b.id === 3)?.active).toBe(false);
    expect(api.expenses).toHaveLength(0);

    await page.getByRole("checkbox", { name: "Mostrar desactivados" }).check();
    const inactive = rowOf(page, "Netflix");
    await expect(inactive).toContainText("Desactivado");
    await expect(inactive.getByRole("button", { name: "Pagar Netflix" })).toHaveCount(0);
    await inactive.getByRole("button", { name: "Reactivar Netflix" }).click();
    await expect(page.getByRole("status").filter({ hasText: "Netflix se reactivó." })).toBeVisible();
    await expect(rowOf(page, "Netflix")).not.toContainText("Desactivado");
    expect(api.bills.find((b) => b.id === 3)?.active).toBe(true);
  });

  test("shows the list error with a retry", async ({ page }) => {
    await openBills(page, { billsFail: "list" });
    await expect(page.getByRole("alert").filter({ hasText: "El servidor tuvo un problema" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Reintentar" })).toBeVisible();
  });

});

test.describe("bills layout", () => {
  test("has no horizontal overflow with data, the history, the form errors and the dialog", async ({ page }) => {
    await openBills(page, {
      bills: [
        ...BILLS,
        bill({ id: 4, name: "Un servicio con un nombre muy largo que debe partirse sin desbordar la pantalla", notes: "Notas largas ".repeat(12) }),
      ],
    });
    await expect(page.getByRole("list", { name: "Pagos recurrentes" })).toBeVisible();
    await expectNoHorizontalOverflow(page, "bills list");

    await rowOf(page, "Megacable").getByRole("button", { name: "Ver historial de Megacable" }).click();
    await expect(page.getByText("Aún no hay pagos ni omisiones")).toBeVisible();
    await expectNoHorizontalOverflow(page, "bills history");

    await page.getByRole("button", { name: "Agregar pago recurrente" }).click();
    await expect(page.getByText("Escribe el nombre del pago")).toBeVisible();
    await expectNoHorizontalOverflow(page, "bills form errors");

    await rowOf(page, "Megacable").getByRole("button", { name: "Omitir Megacable" }).click();
    await expect(page.getByRole("group", { name: "Confirmar omitir Megacable" })).toBeVisible();
    await expectNoHorizontalOverflow(page, "bills skip confirmation");
    await page.getByRole("button", { name: "Cancelar" }).click();

    await rowOf(page, "Luz").getByRole("button", { name: "Pagar Luz" }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Registrar pago" }).click();
    await expect(page.getByRole("dialog").getByText("Escribe un monto mayor a cero")).toBeVisible();
    await expectNoHorizontalOverflow(page, "bills pay dialog");
  });

  test("has no horizontal overflow in the empty and error states", async ({ page }) => {
    await openBills(page);
    await expect(page.getByText("Aún no tienes pagos recurrentes")).toBeVisible();
    await expectNoHorizontalOverflow(page, "bills empty");
  });

  test("has no horizontal overflow in the list error state", async ({ page }) => {
    await openBills(page, { billsFail: "list" });
    await expect(page.getByText("El servidor tuvo un problema")).toBeVisible();
    await expectNoHorizontalOverflow(page, "bills error");
  });
});
