import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockPersonalFuture, type MockPersonalMovement } from "./helpers";

const pad = (n: number) => String(n).padStart(2, "0");

/** Local calendar date `offset` days from today, as YYYY-MM-DD. */
function day(offset = 0) {
  const d = new Date();
  d.setDate(d.getDate() + offset);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

const isDesktop = (width: number | undefined) => (width ?? 0) >= 1024;

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({ scrollWidth: document.documentElement.scrollWidth, clientWidth: document.documentElement.clientWidth }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

/** Obviously fake examples: the repository is public. */
const VIAJE: MockPersonalFuture = { id: 1, name: "Viaje", target_amount: "6000.00", due_date: day(60), saved: "1500.00" };
const CURSO: MockPersonalFuture = { id: 2, name: "Curso", target_amount: "2000.00", due_date: day(20), saved: "0" };
const BOLSA: MockPersonalFuture = { id: 3, name: "Bolsa", target_amount: "900.00", due_date: day(-30), status: "paid", paid_at: day(-28), amount_paid: "850.00" };
const FREE: MockPersonalMovement = { id: 50, kind: "Ahorro", category: "Gastos futuros", amount: "400.00", description: "Ahorro anterior", date: day(-3) };

async function open(page: Page, personalFuture: MockPersonalFuture[] = [], personal: MockPersonalMovement[] = []) {
  const api = await mockApi(page, { role: "household", personal, personalFuture });
  await seedSession(page, "household");
  await page.goto("/mis-gastos-futuros");
  await expect(page.getByRole("heading", { level: 1, name: "Mis gastos futuros" })).toBeVisible();
  return api;
}

const activeList = (page: Page) => page.getByRole("list", { name: "Gastos futuros activos" });
const rowOf = (page: Page, name: string) => activeList(page).getByRole("listitem").filter({ hasText: name }).first();
/** Opens the detail dialog of an active item (its card is one button). */
async function openDetail(page: Page, name: string) {
  await page.getByRole("list", { name: "Gastos futuros activos" }).getByRole("button", { name: `Ver ${name}` }).click();
  await expect(page.getByRole("dialog", { name })).toBeVisible();
}

const dialog = (page: Page) => page.getByRole("dialog");
const freeBalance = (page: Page) => page.getByRole("region", { name: "Saldo libre" });

test.describe("household: Mis gastos futuros", () => {
  test("is a tab of hers and lists the free balance, the items and the history", async ({ page }) => {
    await open(page, [VIAJE, CURSO, BOLSA], [FREE]);
    if (!isDesktop(page.viewportSize()?.width)) await page.getByRole("button", { name: "Abrir menú" }).click();
    await expect(page.getByRole("navigation", { name: "Módulos" }).getByRole("link", { name: "Mis gastos futuros" })).toHaveAttribute("aria-current", "page");

    await expect(freeBalance(page)).toContainText("$400.00");
    const rows = activeList(page).getByRole("listitem");
    await expect(rows).toHaveCount(2);
    await expect(rows.nth(0)).toContainText("Curso"); // earliest due date first
    await expect(rowOf(page, "Viaje")).toContainText("$1,500.00 de $6,000.00 (25%)");
    await expect(rowOf(page, "Viaje").getByRole("progressbar", { name: "Ahorro para Viaje" })).toHaveAttribute("aria-valuenow", "25");
    await expect(rowOf(page, "Viaje")).toContainText(/Aparta \$[\d,]+\.\d{2} al mes/);
    await expect(page.getByRole("list", { name: "Gastos futuros pagados" })).toContainText("Bolsa");
    await expectNoHorizontalOverflow(page, "Mis gastos futuros");
  });

  test("fits a 375px phone", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await open(page, [VIAJE, CURSO, BOLSA], [FREE]);
    await expect(rowOf(page, "Viaje")).toBeVisible();
    await expectNoHorizontalOverflow(page, "375px Mis gastos futuros");
  });

  test("creates an item from the collapsed form", async ({ page }) => {
    const api = await open(page);
    await expect(page.getByText("Aún no tienes gastos futuros.")).toBeVisible();
    await page.getByRole("button", { name: "Nuevo gasto futuro" }).click();
    await page.getByLabel("Nombre").fill("Clases de pintura");
    await page.getByLabel("Monto a juntar (MXN)").fill("1200");
    await page.getByLabel("Fecha de vencimiento").fill(day(90));
    await page.getByRole("button", { name: "Agregar gasto futuro" }).click();
    await expect(rowOf(page, "Clases de pintura")).toBeVisible();
    expect((await api).personalFutureWrites[0]).toMatchObject({ method: "POST", path: "/personal/future-expenses", body: { name: "Clases de pintura", target_amount: "1200", due_date: day(90) } });
  });

  test("adds a saving to an item", async ({ page }) => {
    const api = await open(page, [VIAJE]);
    await openDetail(page, "Viaje");
    await page.getByRole("button", { name: "Agregar ahorro a Viaje" }).click();
    await dialog(page).getByLabel("Monto (MXN)").fill("500");
    await dialog(page).getByRole("button", { name: "Agregar ahorro" }).click();
    await expect(dialog(page)).toHaveCount(0);
    await expect(rowOf(page, "Viaje")).toContainText("$2,000.00 de $6,000.00 (33%)");
    expect((await api).personalFutureWrites.at(-1)).toMatchObject({ path: "/personal/future-expenses/1/savings", body: { amount: "500" } });
  });

  test("assigns from the free balance, only while there is one", async ({ page }) => {
    const api = await open(page, [VIAJE], [FREE]);
    await openDetail(page, "Viaje");
    await page.getByRole("button", { name: "Asignar saldo libre a Viaje" }).click();
    await dialog(page).getByLabel("Monto (MXN)").fill("500");
    await dialog(page).getByRole("button", { name: "Asignar", exact: true }).click();
    await expect(dialog(page)).toContainText("El saldo libre es $400.00");
    await dialog(page).getByLabel("Monto (MXN)").fill("150.50");
    await dialog(page).getByRole("button", { name: "Asignar", exact: true }).click();
    await expect(dialog(page)).toHaveCount(0);
    await expect(freeBalance(page)).toContainText("$249.50");
    await expect(rowOf(page, "Viaje")).toContainText("$1,650.50 de $6,000.00");
    expect((await api).personalFutureWrites.at(-1)).toMatchObject({ path: "/personal/future-expenses/1/assign", body: { amount: "150.50" } });
  });

  test("hides the assign action when there is no free balance", async ({ page }) => {
    await open(page, [VIAJE]);
    await expect(freeBalance(page)).toContainText("$0.00");
    await openDetail(page, "Viaje");
    await expect(page.getByRole("button", { name: "Marcar pagado Viaje" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Asignar saldo libre a Viaje" })).toHaveCount(0);
  });

  test("pays with one of her categories, the remainder returns to the free balance", async ({ page }) => {
    const api = await open(page, [VIAJE], [FREE]);
    await expect(freeBalance(page)).toContainText("$400.00");
    await openDetail(page, "Viaje");
    await page.getByRole("button", { name: "Marcar pagado Viaje" }).click();
    await expect(dialog(page).getByLabel("Monto pagado (MXN)")).toHaveValue("6000.00");
    await expect(dialog(page).getByLabel("Fecha de pago")).toHaveValue(day(0));
    // Only her Gasto categories, no automatic option.
    await expect(dialog(page).getByLabel("Categoría del gasto").locator("option")).toHaveText([
      "Elige una categoría", "Belleza", "Salud", "Tratamientos", "Tienda de conveniencia", "Ropa", "Pintura", "Maquillaje", "Compras en línea",
    ]);
    await dialog(page).getByLabel("Monto pagado (MXN)").fill("1000");
    // The category is required.
    await dialog(page).getByRole("button", { name: "Confirmar pago" }).click();
    await expect(dialog(page)).toContainText("Elige la categoría del gasto.");
    await dialog(page).getByLabel("Categoría del gasto").selectOption({ label: "Ropa" });
    await expectNoHorizontalOverflow(page, "pay dialog");
    await dialog(page).getByRole("button", { name: "Confirmar pago" }).click();
    await expect(dialog(page)).toHaveCount(0);

    await expect(page.getByRole("status").filter({ hasText: "se marcó como pagado" })).toContainText("$1,000.00 en Ropa");
    // 1,500 saved, 1,000 paid: 500 back to the free balance (400 + 500).
    await expect(freeBalance(page)).toContainText("$900.00");
    await expect(activeList(page)).toHaveCount(0);
    await expect(page.getByRole("list", { name: "Gastos futuros pagados" })).toContainText("Viaje");
    const pay = (await api).personalFutureWrites.at(-1) as { path: string; body: Record<string, unknown> };
    expect(pay.path).toBe("/personal/future-expenses/1/pay");
    expect(pay.body).toMatchObject({ amount: "1000", date: day(0) });
    expect(typeof pay.body.category_id).toBe("number");

    // The Gasto is in Mis gastos and the release is in Mis ahorros.
    await page.goto("/mis-gastos");
    await expect(page.getByRole("listitem").filter({ hasText: "Viaje" })).toContainText("Ropa");
    await page.goto("/mis-ahorros");
    await expect(page.getByTestId("personal-total")).toHaveText("$900.00 MXN");
  });

  test("edits an item and deletes it with the savings returning to the free balance", async ({ page }) => {
    const api = await open(page, [VIAJE]);
    await openDetail(page, "Viaje");
    await page.getByRole("button", { name: "Editar Viaje" }).click();
    await page.getByLabel("Monto a juntar (MXN)").fill("7000");
    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(rowOf(page, "Viaje")).toContainText("de $7,000.00");
    expect((await api).personalFutureWrites.at(-1)).toMatchObject({ method: "PUT", path: "/personal/future-expenses/1" });

    await openDetail(page, "Viaje");
    await page.getByRole("button", { name: "Eliminar Viaje" }).click();
    await expect(page.getByText("Su ahorro no se borra: vuelve al saldo libre.")).toBeVisible();
    await page.getByRole("button", { name: "Sí, eliminar" }).click();
    await expect(page.getByText("Aún no tienes gastos futuros.")).toBeVisible();
    await expect(freeBalance(page)).toContainText("$1,500.00");
  });

  test("shows the API failure when the list cannot load", async ({ page }) => {
    await mockApi(page, { role: "household" });
    await seedSession(page, "household");
    await page.route("**/personal/future-expenses", (route) => route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ error: "internal_error" }) }));
    await page.goto("/mis-gastos-futuros");
    await expect(page.getByRole("button", { name: "Reintentar" })).toBeVisible();
  });
});
