import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockFutureExpense } from "./helpers";

const pad = (n: number) => String(n).padStart(2, "0");

/** Local calendar date `offset` days from today, as YYYY-MM-DD. */
function day(offset = 0) {
  const d = new Date();
  d.setDate(d.getDate() + offset);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

/** Obviously fake examples: the repository is public. */
const LAPTOP: MockFutureExpense = { id: 1, name: "Laptop", target_amount: "8000.00", due_date: day(120), saved: "2000.00" };
const TRIP: MockFutureExpense = { id: 2, name: "Viaje", target_amount: "24000.00", due_date: day(60), saved: "0" };
const PHONE: MockFutureExpense = { id: 3, name: "Teléfono", target_amount: "5000.00", due_date: day(-30), status: "paid", paid_at: day(-28), amount_paid: "4800.00" };

async function openSection(page: Page, opts: Parameters<typeof mockApi>[1] = {}) {
  const api = await mockApi(page, opts);
  await seedSession(page);
  await page.goto("/configuracion");
  await page.getByRole("tab", { name: "Gastos futuros" }).click();
  await expect(page.getByRole("heading", { level: 2, name: "Gastos futuros" })).toBeVisible();
  return api;
}

const rowOf = (page: Page, name: string) => page.getByRole("list", { name: "Gastos futuros activos" }).getByRole("listitem").filter({ hasText: name }).first();

test.describe("future expenses in the configuration", () => {
  test("lives in its own configuration tab with the free balance, the items and the history", async ({ page }) => {
    await openSection(page, { futureExpenses: [LAPTOP, TRIP, PHONE], futureFreeBalance: "350.25" });
    await expect(page.getByRole("region", { name: "Saldo libre" })).toContainText("$350.25");
    const rows = page.getByRole("list", { name: "Gastos futuros activos" }).getByRole("listitem");
    await expect(rows).toHaveCount(2);
    await expect(rows.nth(0)).toContainText("Viaje"); // earliest due date first
    await expect(rowOf(page, "Laptop")).toContainText("$2,000.00 de $8,000.00 (25%)");
    await expect(rowOf(page, "Laptop").getByRole("progressbar", { name: "Ahorro para Laptop" })).toHaveAttribute("aria-valuenow", "25");
    await expect(rowOf(page, "Laptop")).toContainText(/Aparta \$[\d,]+\.\d{2} al mes \(\d+ ciclos? restantes?\)/);
    await expect(page.getByRole("list", { name: "Gastos futuros pagados" })).toContainText("Teléfono");
    await expect(page.getByRole("list", { name: "Gastos futuros pagados" })).toContainText("$4,800.00");
    await expectNoHorizontalOverflow(page, "future expenses section");
  });

  test("the create form is collapsed, validates and sends the decimal string", async ({ page }) => {
    const api = await openSection(page);
    const toggle = page.getByRole("button", { name: "+ Nuevo gasto futuro" });
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(page.getByText("Aún no tienes gastos futuros")).toBeVisible();
    await toggle.click();
    await expectNoHorizontalOverflow(page, "create form open");

    await page.getByRole("button", { name: "Agregar gasto futuro" }).click();
    await expect(page.getByLabel("Nombre")).toBeFocused();
    await expect(page.getByRole("alert").filter({ hasText: "nombre del gasto" })).toBeVisible();
    expect(api.futureWrites).toHaveLength(0);

    await page.getByLabel("Nombre").fill("Laptop");
    await page.getByLabel("Monto a juntar (MXN)").fill("8000.50");
    await page.getByLabel("Fecha de vencimiento").fill(day(120));
    await page.getByRole("button", { name: "Agregar gasto futuro" }).click();
    await expect(page.getByRole("status").filter({ hasText: "Laptop agregado" })).toBeVisible();
    expect(api.futureWrites).toEqual([{ method: "POST", path: "/future-expenses", body: { name: "Laptop", target_amount: "8000.50", due_date: day(120) } }]);
    await expect(rowOf(page, "Laptop")).toContainText("$0.00 de $8,000.50 (0%)");
  });

  test("edits an item", async ({ page }) => {
    const api = await openSection(page, { futureExpenses: [LAPTOP] });
    await rowOf(page, "Laptop").getByRole("button", { name: "Editar Laptop" }).click();
    await expect(page.getByLabel("Nombre")).toHaveValue("Laptop");
    await page.getByLabel("Monto a juntar (MXN)").fill("9000");
    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByRole("status").filter({ hasText: "Cambios de Laptop guardados" })).toBeVisible();
    expect(api.futureWrites.at(-1)).toMatchObject({ method: "PUT", path: "/future-expenses/1", body: { target_amount: "9000" } });
    await expect(rowOf(page, "Laptop")).toContainText("$2,000.00 de $9,000.00");
  });

  test("adds a saving to an item", async ({ page }) => {
    const api = await openSection(page, { futureExpenses: [LAPTOP] });
    await rowOf(page, "Laptop").getByRole("button", { name: "Agregar ahorro a Laptop" }).click();
    const dialog = page.getByRole("dialog", { name: "Agregar ahorro a Laptop" });
    await dialog.getByLabel("Monto (MXN)").fill("500");
    await dialog.getByRole("button", { name: "Agregar ahorro" }).click();
    await expect(page.getByRole("status").filter({ hasText: "Se agregaron $500.00 de ahorro a Laptop" })).toBeVisible();
    expect(api.futureWrites.at(-1)).toMatchObject({ method: "POST", path: "/future-expenses/1/savings", body: { amount: "500" } });
    await expect(rowOf(page, "Laptop")).toContainText("$2,500.00 de $8,000.00");
  });

  test("assigns part of the free balance only when the owner asks for it", async ({ page }) => {
    const api = await openSection(page, { futureExpenses: [LAPTOP], futureFreeBalance: "350.25" });
    // Nothing moved by itself: the item keeps its own savings and the pool stays whole.
    await expect(rowOf(page, "Laptop")).toContainText("$2,000.00 de $8,000.00");
    await expect(page.getByRole("region", { name: "Saldo libre" })).toContainText("$350.25");

    await rowOf(page, "Laptop").getByRole("button", { name: "Asignar saldo libre a Laptop" }).click();
    const dialog = page.getByRole("dialog", { name: "Asignar saldo libre a Laptop" });
    await dialog.getByLabel("Monto (MXN)").fill("400");
    await dialog.getByRole("button", { name: "Asignar" }).click();
    await expect(dialog.getByRole("alert")).toContainText("El saldo libre es $350.25");
    expect(api.futureWrites).toHaveLength(0);

    await dialog.getByLabel("Monto (MXN)").fill("100.25");
    await dialog.getByRole("button", { name: "Asignar" }).click();
    await expect(page.getByRole("status").filter({ hasText: "Se asignaron $100.25 del saldo libre a Laptop" })).toBeVisible();
    expect(api.futureWrites.at(-1)).toMatchObject({ path: "/future-expenses/1/assign", body: { amount: "100.25" } });
    await expect(page.getByRole("region", { name: "Saldo libre" })).toContainText("$250.00");
    await expect(rowOf(page, "Laptop")).toContainText("$2,100.25 de $8,000.00");
  });

  test("the assign action is hidden while there is no free balance", async ({ page }) => {
    await openSection(page, { futureExpenses: [LAPTOP] });
    await expect(rowOf(page, "Laptop").getByRole("button", { name: "Agregar ahorro a Laptop" })).toBeVisible();
    await expect(page.getByRole("button", { name: /Asignar saldo libre/ })).toHaveCount(0);
  });

  test("Marcar pagado registers the expense with the real amount and releases the saving", async ({ page }) => {
    const api = await openSection(page, { futureExpenses: [LAPTOP], futureFreeBalance: "0" });
    await rowOf(page, "Laptop").getByRole("button", { name: "Marcar pagado Laptop" }).click();
    const dialog = page.getByRole("dialog", { name: "Marcar pagado Laptop" });
    await expect(dialog.getByLabel("Monto pagado (MXN)")).toHaveValue("8000.00"); // the target by default
    await expect(dialog).toContainText("la diferencia vuelve al saldo libre");
    await expectNoHorizontalOverflow(page, "pay dialog");

    await dialog.getByLabel("Monto pagado (MXN)").fill("1500");
    await dialog.getByLabel("Categoría del gasto").selectOption("Servicios");
    await dialog.getByRole("button", { name: "Confirmar pago" }).click();

    await expect(page.getByRole("status").filter({ hasText: "Laptop se marcó como pagado" })).toContainText("$1,500.00");
    const pay = api.futureWrites.at(-1);
    expect(pay).toMatchObject({ path: "/future-expenses/1/pay", body: { amount: "1500", category: "Servicios" } });
    // 2,000 saved, 1,500 paid: the 500 left returns to the free balance.
    await expect(page.getByRole("region", { name: "Saldo libre" })).toContainText("$500.00");
    await expect(page.getByRole("list", { name: "Gastos futuros activos" })).toHaveCount(0);
    await expect(page.getByRole("list", { name: "Gastos futuros pagados" })).toContainText("Laptop");
    expect(api.expenses.some((e) => e.description === "Laptop" && e.category === "Servicios" && e.amount === "1500.00")).toBe(true);
  });

  test("Marcar pagado validates the amount before sending", async ({ page }) => {
    const api = await openSection(page, { futureExpenses: [LAPTOP] });
    await rowOf(page, "Laptop").getByRole("button", { name: "Marcar pagado Laptop" }).click();
    const dialog = page.getByRole("dialog", { name: "Marcar pagado Laptop" });
    await dialog.getByLabel("Monto pagado (MXN)").fill("0");
    await dialog.getByRole("button", { name: "Confirmar pago" }).click();
    await expect(dialog.getByLabel("Monto pagado (MXN)")).toBeFocused();
    expect(api.futureWrites).toHaveLength(0);
  });

  test("deleting asks first and explains the savings return to the free balance", async ({ page }) => {
    const api = await openSection(page, { futureExpenses: [LAPTOP], futureFreeBalance: "10.00" });
    await rowOf(page, "Laptop").getByRole("button", { name: "Eliminar Laptop" }).click();
    await expect(page.getByText("Su ahorro no se borra: vuelve al saldo libre.")).toBeVisible();
    expect(api.futureWrites).toHaveLength(0);
    await page.getByRole("button", { name: "Sí, eliminar" }).click();
    await expect(page.getByRole("status").filter({ hasText: "Laptop se eliminó" })).toBeVisible();
    expect(api.futureWrites).toEqual([{ method: "DELETE", path: "/future-expenses/1", body: undefined }]);
    await expect(page.getByRole("region", { name: "Saldo libre" })).toContainText("$2,010.00");
  });

  test("shows an error with a retry when the list cannot be loaded", async ({ page }) => {
    await openSection(page, { futureFail: "list" });
    await expect(page.getByRole("alert")).toBeVisible();
    await expect(page.getByRole("button", { name: "Reintentar" })).toBeVisible();
  });

  test("no horizontal overflow with long names, several actions and open dialogs", async ({ page }) => {
    const long: MockFutureExpense = { id: 9, name: "Compra muy grande con un nombre bastante largo para probar el ajuste de línea en pantallas angostas", target_amount: "999999999.99", due_date: day(200), saved: "123456789.12" };
    await openSection(page, { futureExpenses: [LAPTOP, long, PHONE], futureFreeBalance: "1234567.89" });
    await expectNoHorizontalOverflow(page, "long names");
    await page.getByRole("button", { name: "+ Nuevo gasto futuro" }).click();
    await expectNoHorizontalOverflow(page, "create form");
    await rowOf(page, "Compra muy grande").getByRole("button", { name: /Asignar saldo libre/ }).click();
    await expectNoHorizontalOverflow(page, "assign dialog");
  });
});

test.describe("future expenses on the dashboard", () => {
  test("shows what the module holds, the free balance and links to the configuration", async ({ page }) => {
    await mockApi(page, { futureExpenses: [LAPTOP, TRIP], futureFreeBalance: "75.50" });
    await seedSession(page);
    await page.goto("/");
    const card = page.getByRole("region", { name: "Gastos futuros" });
    await expect(card).toContainText("Viaje");
    await expect(card).toContainText("Laptop");
    await expect(card).toContainText("$2,000.00 de $8,000.00 (25%)");
    await expect(card.getByText("Total", { exact: true }).locator("xpath=following-sibling::dd[1]")).toHaveText("$32,000.00");
    await expect(card).toContainText("Saldo libre: $75.50");
    await card.getByRole("link", { name: "Administrar gastos futuros" }).click();
    await expect(page).toHaveURL(/\/configuracion$/);
    await expectNoHorizontalOverflow(page, "configuration");
  });

  test("a paid item leaves the dashboard card", async ({ page }) => {
    await mockApi(page, { futureExpenses: [LAPTOP, PHONE] });
    await seedSession(page);
    await page.goto("/");
    const card = page.getByRole("region", { name: "Gastos futuros" });
    await expect(card).toContainText("Laptop");
    await expect(card).not.toContainText("Teléfono");
  });
});
