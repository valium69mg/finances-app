import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockExpense } from "./helpers";

function today() {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

const seeded = (over: Partial<MockExpense> = {}): MockExpense => ({
  id: 1,
  date: today(),
  description: "Tacos del centro",
  category: "Comida",
  payment_method: "Efectivo",
  currency: "MXN",
  amount: "180.00",
  exchange_rate: null,
  amount_mxn: "180.00",
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
  await page.goto("/gastos");
  await expect(page.getByRole("heading", { level: 1, name: "Gastos" })).toBeVisible();
  if (expanded) {
    await page.getByRole("button", { name: "+ Nuevo gasto" }).click();
    await expect(page.getByLabel("Categoría")).toBeVisible();
  } else {
    await expect(page.getByRole("button", { name: "+ Nuevo gasto" })).toBeVisible();
  }
  return api;
}

test.describe("expenses page", () => {
  test("the create form is collapsed by default and expands inline from the button", async ({ page }) => {
    await open(page, { expenses: [seeded()] }, { expanded: false });
    const toggle = page.getByRole("button", { name: "+ Nuevo gasto" });
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(toggle).toHaveAttribute("aria-controls", "expense-form-panel");
    await expect(page.getByRole("form")).toHaveCount(0);
    await expect(page.getByLabel("Monto")).toHaveCount(0);

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    const form = page.getByRole("form", { name: "Nuevo gasto" });
    await expect(form).toBeVisible();
    await expect(page.locator("#expense-form-panel")).toContainText("Agregar gasto");
    await expect(form.getByLabel("Descripción")).toBeFocused();

    // Toggling again collapses it and keeps the focus on the button.
    await toggle.click();
    await expect(page.getByRole("form")).toHaveCount(0);
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(toggle).toBeFocused();
  });

  test("starting to edit a row opens the form, keeps the green highlight and collapses on cancel", async ({ page }) => {
    await open(page, { expenses: [seeded()] }, { expanded: false });
    await expect(page.getByRole("form")).toHaveCount(0);
    await page.getByRole("button", { name: "Editar Tacos del centro" }).click();

    const form = page.getByRole("form", { name: "Editar gasto" });
    await expect(form).toBeVisible();
    await expect(form).toHaveClass(/(^|\s)edit-highlight(\s|$)/);
    await expect(form.getByLabel("Descripción")).toBeFocused();
    await expect(form.getByLabel("Monto")).toHaveValue("180.00");

    await page.getByRole("button", { name: "Cancelar edición" }).click();
    await expect(page.getByRole("form")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "+ Nuevo gasto" })).toHaveAttribute("aria-expanded", "false");
  });

  test("with a pay cycle starting on the last day, a 2026-09-30 expense lists under 2026-10, not 2026-09", async ({ page }) => {
    await open(page, { cycleStartDay: 31, expenses: [seeded({ id: 7, date: "2026-09-30", description: "Mandado fin de mes" })] });

    const month = page.getByLabel("Mes", { exact: true });
    await month.fill("2026-10");
    await expect(page.getByText("30 sep – 30 oct", { exact: true })).toBeVisible();
    await expect(page.getByRole("listitem").filter({ hasText: "Mandado fin de mes" })).toBeVisible();

    await month.fill("2026-09");
    await expect(page.getByText("No hay gastos registrados en este periodo.")).toBeVisible();

    await page.getByLabel("Fecha").fill("2026-09-30");
    await expect(page.getByText("Se cuenta en el periodo 30 sep – 30 oct")).toBeVisible();
  });

  test("adds an expense with a suggested category and shows budget feedback", async ({ page }) => {
    const api = await open(page);
    await expect(page.getByText("No hay gastos registrados en este periodo.")).toBeVisible();

    await page.getByLabel("Descripción").fill("Tacos al pastor");
    await expect(page.getByText("Sugerida según la descripción: Comida.")).toBeVisible();
    await expect(page.getByLabel("Categoría")).toHaveValue("Comida");
    await page.getByLabel("Monto").fill("250.50");
    await page.getByRole("button", { name: "Agregar gasto" }).click();

    await expect(page.getByText("Gasto guardado.")).toBeVisible();
    await expect(page.getByText("Comida: $250.50 de $6,000.00")).toBeVisible();
    await expect(page.getByRole("listitem").filter({ hasText: "Tacos al pastor" })).toContainText("$250.50 MXN");

    const post = api.requests.find((r) => r.path === "/expenses");
    const body = post?.body as Record<string, unknown>;
    expect(body.amount).toBe("250.50");
    expect(typeof body.amount).toBe("string");
    expect(body.category).toBe("Comida");
    expect(body.currency).toBe("MXN");
    expect(body.date).toBe(today());
    // The form collapses after a successful save and opens empty again.
    await expect(page.getByRole("form")).toHaveCount(0);
    await page.getByRole("button", { name: "+ Nuevo gasto" }).click();
    await expect(page.getByLabel("Descripción")).toHaveValue("");
    await expect(page.getByLabel("Monto")).toHaveValue("");
  });

  test("lets the user override the suggested category", async ({ page }) => {
    const api = await open(page);
    await page.getByLabel("Descripción").fill("Tacos");
    await expect(page.getByLabel("Categoría")).toHaveValue("Comida");
    await page.getByLabel("Categoría").selectOption("Renta");
    await page.getByLabel("Monto").fill("99");
    await page.getByRole("button", { name: "Agregar gasto" }).click();
    await expect(page.getByText("Gasto guardado.")).toBeVisible();
    expect((api.requests.find((r) => r.path === "/expenses")?.body as { category: string }).category).toBe("Renta");
  });

  test("sends USD with the exchange rate as decimal strings", async ({ page }) => {
    const api = await open(page);
    await expect(page.getByLabel("Tipo de cambio (opcional)")).toHaveCount(0);
    await page.getByLabel("Moneda").selectOption("USD");
    await page.getByLabel("Monto").fill("62.03");
    await page.getByLabel("Tipo de cambio (opcional)").fill("17.74");
    await page.getByLabel("Categoría").selectOption("Comida");
    await page.getByRole("button", { name: "Agregar gasto" }).click();

    await expect(page.getByText("Gasto guardado.")).toBeVisible();
    const body = api.requests.find((r) => r.path === "/expenses")?.body as Record<string, unknown>;
    expect(body.currency).toBe("USD");
    expect(body.amount).toBe("62.03");
    expect(body.exchange_rate).toBe("17.74");
    await expect(page.getByRole("listitem").filter({ hasText: "TC 17.74" })).toContainText("≈ $1,100.41 MXN");
  });

  test("blocks an invalid amount and sends nothing", async ({ page }) => {
    const api = await open(page);
    await page.getByLabel("Monto").fill("12,5");
    await page.getByRole("button", { name: "Agregar gasto" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "monto mayor a cero" })).toBeVisible();
    await expect(page.getByLabel("Monto")).toBeFocused();
    expect(api.requests.filter((r) => r.path === "/expenses")).toHaveLength(0);
  });

  test("warns when the category goes over budget", async ({ page }) => {
    await open(page);
    await page.getByLabel("Categoría").selectOption("Comida");
    await page.getByLabel("Monto").fill("7000");
    await page.getByRole("button", { name: "Agregar gasto" }).click();
    await expect(page.getByText("Comida: $7,000.00 de $6,000.00")).toBeVisible();
    await expect(page.getByText("Te pasaste del presupuesto por $1,000.00.")).toBeVisible();
  });

  test("says when the category has no budget", async ({ page }) => {
    await open(page);
    await page.getByLabel("Categoría").selectOption("Inversiones");
    await page.getByLabel("Monto").fill("500");
    await page.getByRole("button", { name: "Agregar gasto" }).click();
    await expect(page.getByText("Sin presupuesto definido para Inversiones.")).toBeVisible();
  });

  test("edits an expense through the form", async ({ page }) => {
    const api = await open(page, { expenses: [seeded()] });
    await page.getByRole("button", { name: "Editar Tacos del centro" }).click();

    await expect(page.getByRole("heading", { level: 2, name: "Editar gasto" })).toBeVisible();
    await expect(page.getByLabel("Monto")).toHaveValue("180.00");
    await page.getByLabel("Monto").fill("220.75");
    await page.getByRole("button", { name: "Guardar cambios" }).click();

    await expect(page.getByText("Gasto guardado.")).toBeVisible();
    // The form collapses after saving.
    await expect(page.getByRole("form")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "+ Nuevo gasto" })).toHaveAttribute("aria-expanded", "false");
    await expect(page.getByRole("listitem").filter({ hasText: "Tacos del centro" })).toContainText("$220.75 MXN");
    const put = api.writes.find((w) => w.method === "PUT" && w.path === "/expenses/1");
    expect((put?.body as { amount: string }).amount).toBe("220.75");
  });

  test("deletes an expense after an inline confirmation", async ({ page }) => {
    page.on("dialog", (d) => {
      throw new Error(`unexpected native dialog: ${d.message()}`);
    });
    const api = await open(page, { expenses: [seeded()] });

    await page.getByRole("button", { name: "Eliminar Tacos del centro" }).click();
    await expect(page.getByText("¿Eliminar este gasto?")).toBeVisible();
    await page.getByRole("button", { name: "Cancelar", exact: true }).click();
    await expect(page.getByText("Tacos del centro")).toBeVisible();
    expect(api.writes).toHaveLength(0);

    await page.getByRole("button", { name: "Eliminar Tacos del centro" }).click();
    await page.getByRole("button", { name: "Sí, eliminar" }).click();
    await expect(page.getByText("No hay gastos registrados en este periodo.")).toBeVisible();
    expect(api.writes.some((w) => w.method === "DELETE" && w.path === "/expenses/1")).toBe(true);
  });

  test("shows the backend message when the server rejects the expense", async ({ page }) => {
    await open(page, { expensesFail: "exchange rate is required for USD" });
    await page.getByLabel("Monto").fill("10");
    await page.getByRole("button", { name: "Agregar gasto" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "exchange rate is required for USD" })).toBeVisible();
  });

  test("changing the month lists that month's expenses", async ({ page }) => {
    await open(page, { expenses: [seeded(), seeded({ id: 2, date: "2025-01-10", description: "Gasto de enero" })] });
    await expect(page.getByText("Gasto de enero")).toHaveCount(0);
    await page.getByLabel("Mes").fill("2025-01");
    await expect(page.getByText("Gasto de enero")).toBeVisible();
    await expect(page.getByText("Tacos del centro")).toHaveCount(0);
  });

  for (const vp of [
    { name: "375", width: 375, height: 800 },
    { name: "768", width: 768, height: 900 },
    { name: "1440", width: 1440, height: 900 },
  ]) {
    test(`has no horizontal overflow at ${vp.name}px`, async ({ page }) => {
      await page.setViewportSize({ width: vp.width, height: vp.height });
      await open(page, {
        expenses: [
          seeded({ description: "Una descripción extremadamente larga sin espacios ".repeat(3) + "x".repeat(60) }),
          seeded({ id: 2, currency: "USD", amount: "62.03", exchange_rate: "17.74", amount_mxn: "1100.4122" }),
        ],
      });
      await expectNoHorizontalOverflow(page, `${vp.name}px list`);
      await page.getByLabel("Moneda").selectOption("USD");
      await page.getByLabel("Monto").fill("10");
      await page.getByRole("button", { name: "Agregar gasto" }).click();
      await expect(page.getByText("Gasto guardado.")).toBeVisible();
      await page.getByRole("button", { name: /^Eliminar/ }).first().click();
      await expectNoHorizontalOverflow(page, `${vp.name}px form + confirm`);
    });
  }
});
