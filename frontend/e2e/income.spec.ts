import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockIncome } from "./helpers";

function today() {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

const seeded = (over: Partial<MockIncome> = {}): MockIncome => ({
  id: 1,
  date: today(),
  description: "Sueldo octubre",
  category: "Sueldo",
  payment_method: "Transferencia",
  currency: "MXN",
  amount: "20000.00",
  exchange_rate: null,
  amount_mxn: "20000.00",
  ...over,
});

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

/** Opens the page; the create form is collapsed, so it is expanded unless `expanded` is false. */
async function open(page: Page, opts: Parameters<typeof mockApi>[1] = {}, { expanded = true } = {}) {
  const api = await mockApi(page, opts);
  await seedSession(page);
  await page.goto("/ingresos");
  await expect(page.getByRole("heading", { level: 1, name: "Ingresos" })).toBeVisible();
  if (expanded) {
    await page.getByRole("button", { name: "+ Nuevo ingreso" }).click();
    await expect(page.getByLabel("Categoría")).toBeVisible();
  } else {
    await expect(page.getByRole("button", { name: "+ Nuevo ingreso" })).toBeVisible();
  }
  return api;
}

test.describe("income page", () => {
  test("the create form is collapsed by default and expands inline from the button", async ({ page }) => {
    await open(page, { income: [seeded()] }, { expanded: false });
    const toggle = page.getByRole("button", { name: "+ Nuevo ingreso" });
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(toggle).toHaveAttribute("aria-controls", "income-form-panel");
    await expect(page.getByRole("form")).toHaveCount(0);
    await expect(page.getByLabel("Monto")).toHaveCount(0);

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    const form = page.getByRole("form", { name: "Nuevo ingreso" });
    await expect(form).toBeVisible();
    await expect(form.getByLabel("Descripción")).toBeFocused();

    await toggle.click();
    await expect(page.getByRole("form")).toHaveCount(0);
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(toggle).toBeFocused();
  });

  test("collapses after a successful save", async ({ page }) => {
    await open(page);
    await page.getByLabel("Monto").fill("500");
    await page.getByRole("button", { name: "Agregar ingreso" }).click();
    await expect(page.getByText("Ingreso guardado.")).toBeVisible();
    await expect(page.getByRole("form")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "+ Nuevo ingreso" })).toHaveAttribute("aria-expanded", "false");
  });

  test("starting to edit a row opens the form, keeps the green highlight and collapses on cancel", async ({ page }) => {
    await open(page, { income: [seeded()] }, { expanded: false });
    await expect(page.getByRole("form")).toHaveCount(0);
    await page.getByRole("button", { name: "Editar Sueldo octubre" }).click();

    const form = page.getByRole("form", { name: "Editar ingreso" });
    await expect(form).toBeVisible();
    await expect(form).toHaveClass(/(^|\s)edit-highlight(\s|$)/);
    await expect(form.getByLabel("Descripción")).toBeFocused();

    await page.getByRole("button", { name: "Cancelar edición" }).click();
    await expect(page.getByRole("form")).toHaveCount(0);
  });

  test("adds a Sueldo income and shows the month summary", async ({ page }) => {
    const api = await open(page);
    await expect(page.getByText("No hay ingresos registrados en este periodo.")).toBeVisible();
    await expect(page.getByLabel("Método de pago")).toHaveValue("Transferencia");
    // Only income categories are offered.
    await expect(page.getByLabel("Categoría").locator("option")).toHaveText([
      "Automática (según la descripción)",
      "Sueldo",
      "Contrato extra",
    ]);

    await page.getByLabel("Descripción").fill("Sueldo quincena");
    await expect(page.getByText("Sugerida según la descripción: Sueldo.")).toBeVisible();
    await page.getByLabel("Monto").fill("15000.50");
    await page.getByRole("button", { name: "Agregar ingreso" }).click();

    await expect(page.getByText("Ingreso guardado.")).toBeVisible();
    const summary = page.getByRole("status").filter({ hasText: "Ingreso guardado." });
    await expect(summary).toContainText("Total del mes");
    await expect(summary).toContainText("$15,000.50 MXN");
    await expect(summary).toContainText("$150.01 (1%)");
    await expect(page.getByText("Reparto sugerido")).toHaveCount(0);
    await expect(page.getByText("tasa RESICO mayor")).toHaveCount(0);
    await expect(page.getByRole("listitem").filter({ hasText: "Sueldo quincena" })).toContainText("$15,000.50 MXN");

    const body = api.requests.find((r) => r.path === "/income")?.body as Record<string, unknown>;
    expect(body.amount).toBe("15000.50");
    expect(typeof body.amount).toBe("string");
    expect(body.category).toBe("Sueldo");
    expect(body.payment_method).toBe("Transferencia");
    // The form collapses after the save and opens empty again.
    await expect(page.getByRole("form")).toHaveCount(0);
    await page.getByRole("button", { name: "+ Nuevo ingreso" }).click();
    await expect(page.getByLabel("Descripción")).toHaveValue("");
    await expect(page.getByLabel("Monto")).toHaveValue("");
  });

  test("shows the suggested split for a Contrato extra and says it is not saved", async ({ page }) => {
    const api = await open(page);
    await page.getByLabel("Categoría").selectOption("Contrato extra");
    await page.getByLabel("Monto").fill("1000");
    await page.getByRole("button", { name: "Agregar ingreso" }).click();

    const panel = page.getByRole("region", { name: "Reparto sugerido" });
    await expect(panel).toBeVisible();
    await expect(panel).toContainText("sugerencia y no se guarda");
    await expect(panel).toContainText("módulo de Ahorros");
    await expect(panel).toContainText("Reserva SAT");
    await expect(panel).toContainText("$165.00");
    await expect(panel).toContainText("Fondo de emergencia");
    await expect(panel).toContainText("$418.00");
    await expect(panel).toContainText("Inversiones");
    await expect(panel.getByLabel("Desglose de inversiones")).toContainText("VOO");
    await expect(panel.getByLabel("Desglose de inversiones")).toContainText("$292.00");
    await expect(panel).toContainText("Aguinaldo y vacaciones");
    await expect(panel).toContainText("$125.00");
    // The split is a suggestion: only the income itself is written.
    expect(api.writes).toHaveLength(0);
    expect(api.requests.map((r) => r.path)).toEqual(["/income"]);
  });

  test("warns when the income moves the month to a higher RESICO rate", async ({ page }) => {
    await open(page, { income: [seeded()] });
    await page.getByLabel("Monto").fill("10000");
    await page.getByRole("button", { name: "Agregar ingreso" }).click();
    await expect(page.getByText("$30,000.00 MXN")).toBeVisible();
    await expect(page.getByText("$450.00 (1.5%)")).toBeVisible();
    await expect(page.getByText("Este ingreso te movió a una tasa RESICO mayor: 1% → 1.5%.")).toBeVisible();
  });

  test("sends USD with the exchange rate as decimal strings", async ({ page }) => {
    const api = await open(page);
    await page.getByLabel("Moneda").selectOption("USD");
    await page.getByLabel("Monto").fill("3383.33");
    await page.getByLabel("Tipo de cambio (opcional)").fill("17.74");
    await page.getByRole("button", { name: "Agregar ingreso" }).click();
    await expect(page.getByText("Ingreso guardado.")).toBeVisible();
    const body = api.requests.find((r) => r.path === "/income")?.body as Record<string, unknown>;
    expect(body.currency).toBe("USD");
    expect(body.amount).toBe("3383.33");
    expect(body.exchange_rate).toBe("17.74");
    await expect(page.getByRole("listitem").filter({ hasText: "TC 17.74" })).toContainText("≈ $60,020.27 MXN");
  });

  test("blocks an invalid amount and sends nothing", async ({ page }) => {
    const api = await open(page);
    await page.getByLabel("Monto").fill("12,5");
    await page.getByRole("button", { name: "Agregar ingreso" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "monto mayor a cero" })).toBeVisible();
    await expect(page.getByLabel("Monto")).toBeFocused();
    expect(api.requests.filter((r) => r.path === "/income")).toHaveLength(0);
  });

  test("edits an income through the form", async ({ page }) => {
    const api = await open(page, { income: [seeded()] });
    await page.getByRole("button", { name: "Editar Sueldo octubre" }).click();

    await expect(page.getByRole("heading", { level: 2, name: "Editar ingreso" })).toBeVisible();
    await expect(page.getByLabel("Monto")).toHaveValue("20000.00");
    await page.getByLabel("Monto").fill("21000.75");
    await page.getByRole("button", { name: "Guardar cambios" }).click();

    await expect(page.getByText("Ingreso guardado.")).toBeVisible();
    // The form collapses after saving.
    await expect(page.getByRole("form")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "+ Nuevo ingreso" })).toHaveAttribute("aria-expanded", "false");
    await expect(page.getByRole("listitem").filter({ hasText: "Sueldo octubre" })).toContainText("$21,000.75 MXN");
    const put = api.writes.find((w) => w.method === "PUT" && w.path === "/income/1");
    expect((put?.body as { amount: string }).amount).toBe("21000.75");
  });

  test("deletes an income after an inline confirmation", async ({ page }) => {
    page.on("dialog", (d) => {
      throw new Error(`unexpected native dialog: ${d.message()}`);
    });
    const api = await open(page, { income: [seeded()] });

    await page.getByRole("button", { name: "Eliminar Sueldo octubre" }).click();
    await expect(page.getByText("¿Eliminar este ingreso?")).toBeVisible();
    await page.getByRole("button", { name: "Cancelar", exact: true }).click();
    await expect(page.getByText("Sueldo octubre")).toBeVisible();
    expect(api.writes).toHaveLength(0);

    await page.getByRole("button", { name: "Eliminar Sueldo octubre" }).click();
    await page.getByRole("button", { name: "Sí, eliminar" }).click();
    await expect(page.getByText("No hay ingresos registrados en este periodo.")).toBeVisible();
    expect(api.writes.some((w) => w.method === "DELETE" && w.path === "/income/1")).toBe(true);
  });

  test("shows the backend message when the server rejects the income", async ({ page }) => {
    await open(page, { incomeFail: "exchange rate is required for USD" });
    await page.getByLabel("Monto").fill("10");
    await page.getByRole("button", { name: "Agregar ingreso" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "exchange rate is required for USD" })).toBeVisible();
  });

  test("changing the month lists that month's income", async ({ page }) => {
    await open(page, { income: [seeded(), seeded({ id: 2, date: "2025-01-10", description: "Ingreso de enero" })] });
    await expect(page.getByText("Ingreso de enero")).toHaveCount(0);
    await page.getByLabel("Mes").fill("2025-01");
    await expect(page.getByText("Ingreso de enero")).toBeVisible();
    await expect(page.getByText("Sueldo octubre")).toHaveCount(0);
  });

  for (const vp of [
    { name: "375", width: 375, height: 800 },
    { name: "768", width: 768, height: 900 },
    { name: "1440", width: 1440, height: 900 },
  ]) {
    test(`has no horizontal overflow at ${vp.name}px`, async ({ page }) => {
      await page.setViewportSize({ width: vp.width, height: vp.height });
      await open(page, {
        income: [
          seeded({ description: "Una descripción extremadamente larga sin espacios ".repeat(3) + "x".repeat(60) }),
          seeded({ id: 2, currency: "USD", amount: "3383.33", exchange_rate: "17.74", amount_mxn: "60020.27" }),
        ],
      });
      await expectNoHorizontalOverflow(page, `${vp.name}px list`);
      await page.getByLabel("Categoría").selectOption("Contrato extra");
      await page.getByLabel("Monto").fill("10");
      await page.getByRole("button", { name: "Agregar ingreso" }).click();
      await expect(page.getByRole("region", { name: "Reparto sugerido" })).toBeVisible();
      await expectNoHorizontalOverflow(page, `${vp.name}px summary`);
      await page.getByRole("button", { name: /^Eliminar/ }).first().click();
      await expectNoHorizontalOverflow(page, `${vp.name}px form + confirm`);
    });
  }
});
