import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockPersonalMovement } from "./helpers";

const pad = (n: number) => String(n).padStart(2, "0");
const today = () => {
  const d = new Date();
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
};
const isDesktop = (width: number | undefined) => (width ?? 0) >= 1024;

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({ scrollWidth: document.documentElement.scrollWidth, clientWidth: document.documentElement.clientWidth }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

const gasto = (over: Partial<MockPersonalMovement> = {}): MockPersonalMovement => ({
  id: 1,
  kind: "Gasto",
  category: "Ropa",
  amount: "350.00",
  description: "Blusa",
  date: today(),
  ...over,
});

async function open(page: Page, path: string, personal: MockPersonalMovement[] = []) {
  const api = await mockApi(page, { role: "household", personal });
  await seedSession(page, "household");
  await page.goto(path);
  return api;
}

const dialog = (page: Page) => page.getByRole("dialog");

test.describe("household: navigation", () => {
  test("lists the four personal tabs", async ({ page }) => {
    await open(page, "/");
    if (!isDesktop(page.viewportSize()?.width)) await page.getByRole("button", { name: "Abrir menú" }).click();
    const nav = page.getByRole("navigation", { name: "Módulos" });
    await expect(nav.getByRole("link")).toHaveText(["Panel", "Peticiones", "Mis ingresos", "Mis gastos", "Mis ahorros", "Mis gastos futuros", "Lista de deseos"]);
    await nav.getByRole("link", { name: "Mis gastos", exact: true }).click();
    await expect(page).toHaveURL(/\/mis-gastos$/);
    await expect(page.getByRole("heading", { level: 1, name: "Mis gastos" })).toBeVisible();
  });
});

test.describe("owner session", () => {
  test("does not see the personal tabs and is redirected from their paths", async ({ page }) => {
    await mockApi(page);
    await seedSession(page, "owner");
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 2, name: "Resumen del mes" })).toBeVisible();
    if (!isDesktop(page.viewportSize()?.width)) await page.getByRole("button", { name: "Abrir menú" }).click();
    const labels = await page.getByRole("navigation", { name: "Módulos" }).getByRole("link").allInnerTexts();
    for (const hidden of ["Mis ingresos", "Mis gastos", "Mis ahorros", "Mis gastos futuros", "Lista de deseos"]) expect(labels).not.toContain(hidden);
    for (const path of ["/mis-ingresos", "/mis-gastos", "/mis-ahorros", "/mis-gastos-futuros", "/lista-de-deseos"]) {
      await page.goto(path);
      await expect(page, path).toHaveURL(/\/$/);
    }
  });
});

test.describe("Mis gastos", () => {
  test("lists newest first with the total of the period", async ({ page }) => {
    await open(page, "/mis-gastos", [gasto(), gasto({ id: 2, description: "Consulta", category: "Salud", amount: "800.50" })]);
    await expect(page.getByRole("heading", { level: 1, name: "Mis gastos" })).toBeVisible();
    await expect(page.getByTestId("personal-total")).toHaveText("$1,150.50 MXN");
    const items = page.getByRole("listitem").filter({ hasText: "MXN" });
    await expect(items.first()).toContainText("Consulta");
    await expect(items.first()).toContainText("Salud");
    await expectNoHorizontalOverflow(page, "Mis gastos");
  });

  test("creates, edits and deletes an expense", async ({ page }) => {
    const api = await open(page, "/mis-gastos");
    await expect(page.getByText("No hay gastos registrados en este periodo.")).toBeVisible();

    await page.getByRole("button", { name: "Nuevo gasto" }).click();
    await dialog(page).getByLabel("Descripción").fill("Labial");
    await dialog(page).getByLabel("Monto (MXN)").fill("199.90");
    await dialog(page).getByLabel("Categoría").selectOption({ label: "Maquillaje" });
    await expectNoHorizontalOverflow(page, "expense dialog");
    await dialog(page).getByRole("button", { name: "Guardar" }).click();
    await expect(dialog(page)).toHaveCount(0);
    await expect(page.getByText("Labial")).toBeVisible();
    await expect(page.getByTestId("personal-total")).toHaveText("$199.90 MXN");
    const created = (await api).personalWrites[0].body as Record<string, unknown>;
    expect(created).toMatchObject({ kind: "Gasto", amount: "199.90", description: "Labial", date: today() });
    expect(typeof created.category_id).toBe("number");

    await page.getByRole("button", { name: "Editar Labial" }).click();
    await dialog(page).getByLabel("Monto (MXN)").fill("210");
    await dialog(page).getByRole("button", { name: "Guardar cambios" }).click();
    await expect(dialog(page)).toHaveCount(0);
    await expect(page.getByTestId("personal-total")).toHaveText("$210.00 MXN");

    await page.getByRole("button", { name: "Eliminar Labial" }).click();
    await page.getByRole("button", { name: "Sí, eliminar" }).click();
    await expect(page.getByText("No hay gastos registrados en este periodo.")).toBeVisible();
  });

  test("rejects an amount with more than two decimals", async ({ page }) => {
    await open(page, "/mis-gastos");
    await page.getByRole("button", { name: "Nuevo gasto" }).click();
    await dialog(page).getByLabel("Monto (MXN)").fill("10.123");
    await dialog(page).getByRole("button", { name: "Guardar" }).click();
    await expect(dialog(page)).toContainText("máximo dos decimales");
  });

  test("adds and renames a category, and reports a duplicate", async ({ page }) => {
    await open(page, "/mis-gastos", [gasto()]);
    await page.getByRole("button", { name: "Administrar categorías" }).click();

    await dialog(page).getByLabel("Nueva categoría", { exact: true }).fill("Café");
    await dialog(page).getByRole("button", { name: "Agregar categoría" }).click();
    await expect(dialog(page).getByText("Café", { exact: true })).toBeVisible();

    await dialog(page).getByLabel("Nueva categoría", { exact: true }).fill("café");
    await dialog(page).getByRole("button", { name: "Agregar categoría" }).click();
    await expect(dialog(page).getByRole("alert")).toContainText("Ya tienes una categoría con ese nombre");

    await dialog(page).getByRole("button", { name: "Renombrar Ropa" }).click();
    await dialog(page).getByLabel("Nuevo nombre").fill("Vestuario");
    await dialog(page).getByRole("button", { name: "Guardar nombre" }).click();
    await expect(dialog(page).getByText("Vestuario", { exact: true })).toBeVisible();

    await dialog(page).getByRole("button", { name: "Renombrar Salud" }).click();
    await dialog(page).getByLabel("Nuevo nombre").fill("Belleza");
    await dialog(page).getByRole("button", { name: "Guardar nombre" }).click();
    await expect(dialog(page).getByRole("alert")).toContainText("Ya tienes una categoría con ese nombre");

    await dialog(page).getByRole("button", { name: "Cerrar", exact: true }).first().click();
    // The rename reaches the movement list.
    await expect(page.getByText(/Vestuario/)).toBeVisible();
  });
});

test.describe("Mis ingresos", () => {
  test("creates an income with a fixed category and offers no category management", async ({ page }) => {
    const api = await open(page, "/mis-ingresos");
    await expect(page.getByRole("button", { name: "Administrar categorías" })).toHaveCount(0);
    await page.getByRole("button", { name: "Nuevo ingreso" }).click();
    await expect(dialog(page).getByLabel("Categoría").locator("option")).toHaveText(["Trabajo", "Transferencia", "Otros"]);
    await dialog(page).getByLabel("Descripción").fill("Quincena");
    await dialog(page).getByLabel("Monto (MXN)").fill("8000");
    await dialog(page).getByRole("button", { name: "Guardar" }).click();
    await expect(page.getByText("Quincena")).toBeVisible();
    await expect(page.getByTestId("personal-total")).toHaveText("$8,000.00 MXN");
    expect((await api).personalWrites[0].body).toMatchObject({ kind: "Ingreso", amount: "8000" });
  });
});

test.describe("Mis ahorros", () => {
  test("shows a withdrawal as negative and nets the total", async ({ page }) => {
    const api = await open(page, "/mis-ahorros", [{ id: 1, kind: "Ahorro", category: "Ahorro", amount: "1000.00", description: "Aportación inicial", date: today() }]);
    await page.getByRole("button", { name: "Nuevo movimiento" }).click();
    await expect(dialog(page).getByLabel("Categoría").locator("option")).toHaveText(["Ahorro", "Inversión", "Gastos futuros"]);
    await dialog(page).getByLabel("Tipo").selectOption({ label: "Retiro" });
    await dialog(page).getByLabel("Descripción").fill("Retiro emergencia");
    await dialog(page).getByLabel("Monto (MXN)").fill("250.50");
    await dialog(page).getByRole("button", { name: "Guardar" }).click();
    await expect(dialog(page)).toHaveCount(0);

    expect((await api).personalWrites[0].body).toMatchObject({ kind: "Ahorro", amount: "-250.50" });
    const row = page.getByRole("listitem").filter({ hasText: "Retiro emergencia" });
    await expect(row).toContainText("-$250.50 MXN");
    await expect(row).toContainText("Retiro");
    await expect(page.getByTestId("personal-total")).toHaveText("$749.50 MXN");

    // Editing the withdrawal keeps it a withdrawal.
    await page.getByRole("button", { name: "Editar Retiro emergencia" }).click();
    await expect(dialog(page).getByLabel("Tipo")).toHaveValue("retiro");
    await expect(dialog(page).getByLabel("Monto (MXN)")).toHaveValue("250.50");
    await expectNoHorizontalOverflow(page, "Mis ahorros");
  });

  test("asks the server for the picked month", async ({ page }) => {
    const api = await open(page, "/mis-ahorros");
    // Until she picks one the hint is the range of the current cycle, read from /personal/period.
    await expect(page.getByText(/^\d{1,2} \w{3} – \d{1,2} \w{3}$/)).toBeVisible();
    expect(new URL(page.url()).searchParams.get("mes")).toBeNull();
    await page.getByLabel("Mes").fill("2026-08");
    await expect(page.getByText("No hay aportaciones ni retiros en este periodo.")).toBeVisible();
    const months = (await api).personalMonths;
    expect(months[0]).toBe("");
    await expect.poll(() => months).toContain("2026-08");
    await expect.poll(() => new URL(page.url()).searchParams.get("mes")).toBe("2026-08");
  });
});
