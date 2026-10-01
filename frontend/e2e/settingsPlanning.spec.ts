import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession } from "./helpers";

// Fake planning data: 3,500 USD x 17.74 = 62,090.00 base. Budgets add up to 41,433.00 (66.7 %).
/* eslint-disable-next-line @typescript-eslint/no-explicit-any */
function planning(settings: any) {
  settings.general.salary_usd = "3500";
  settings.general.fx_rate_applied = "17.74";
  settings.categories = [
    { name: "Vivienda", kind: "Gasto", budget: "3600", includes: "", keywords: [] },
    { name: "Mandado", kind: "Gasto", budget: "10833", includes: "", keywords: [] },
    { name: "Ocio", kind: "Gasto", budget: null, includes: "", keywords: [] },
    { name: "Fondo de emergencia", kind: "Ahorro", budget: "15000", includes: "", keywords: [] },
    { name: "Gastos futuros", kind: "Ahorro", budget: "12000", includes: "", keywords: [] },
    { name: "Sueldo", kind: "Ingreso", budget: null, includes: "", keywords: [] },
  ];
}

/* eslint-disable-next-line @typescript-eslint/no-explicit-any */
const withoutSalary = (settings: any) => {
  planning(settings);
  settings.general.salary_usd = "0";
};

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({ scrollWidth: document.documentElement.scrollWidth, clientWidth: document.documentElement.clientWidth }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

async function open(page: Page, tab: string, opts: Parameters<typeof mockApi>[1] = {}) {
  const api = await mockApi(page, opts);
  await seedSession(page);
  await page.goto("/configuracion");
  await page.getByRole("tab", { name: tab, exact: true }).click();
  return api;
}

test.describe("settings: extra-income split and allocation as percentages", () => {
  test("shows decimals as percentages and saves them back exactly", async ({ page }) => {
    const api = await open(page, "General");
    await expect(page.getByLabel("Reserva SAT (%)")).toHaveValue("16.5");
    await expect(page.getByLabel("Fondo de emergencia (%)")).toHaveValue("50");
    await expect(page.getByLabel("Inversiones (%)")).toHaveValue("35");
    await expect(page.getByLabel("Aguinaldo y vacaciones (%)")).toHaveValue("15");
    await expect(page.getByLabel("Inversión: VOO (%)")).toHaveValue("60");
    await expect(page.getByLabel("Inversión: BTC (%)")).toHaveValue("40");
    await expect(page.getByLabel("Reserva SAT (%)")).toHaveAttribute("inputmode", "decimal");
    await expect(page.getByText("Suma de los tres destinos: 100 %")).toBeVisible();

    await page.getByLabel("Reserva SAT (%)").fill("17.25");
    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByText("Cambios guardados.")).toBeVisible();

    const put = api.writes.find((w) => w.method === "PUT" && w.path === "/settings/general");
    const body = put?.body as { extra_income_split: Record<string, string>; investment_allocation: { key: string; value: string }[] };
    expect(body.extra_income_split).toEqual({ sat_reserve_rate: "0.1725", fondo_emergencia: "0.5", inversiones: "0.35", aguinaldo_vacaciones: "0.15" });
    expect(body.investment_allocation).toEqual([
      { key: "VOO", value: "0.6" },
      { key: "BTC", value: "0.4" },
    ]);
  });

  test("blocks a split that does not add up to 100 % and sends nothing", async ({ page }) => {
    const api = await open(page, "General");
    await page.getByLabel("Aguinaldo y vacaciones (%)").fill("14.9");
    await expect(page.getByText("Suma de los tres destinos: 99.9 % (debe sumar 100 %)")).toBeVisible();

    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "deben sumar exactamente 100 %. Ahora suman 99.9 %" })).toBeVisible();
    await expect(page.getByLabel("Fondo de emergencia (%)")).toBeFocused();
    expect(api.writes).toHaveLength(0);

    await page.getByLabel("Aguinaldo y vacaciones (%)").fill("15");
    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByText("Cambios guardados.")).toBeVisible();
  });

  test("lets the SAT reserve be anything from 0 to 100 and rejects more", async ({ page }) => {
    const api = await open(page, "General");
    await page.getByLabel("Reserva SAT (%)").fill("100.5");
    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "entre 0 y 100" })).toBeVisible();
    expect(api.writes).toHaveLength(0);
    await page.getByLabel("Reserva SAT (%)").fill("90");
    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByText("Cambios guardados.")).toBeVisible();
  });

  test("blocks an investment allocation that does not add up to 100 %", async ({ page }) => {
    const api = await open(page, "General");
    await page.getByLabel("Inversión: BTC (%)").fill("30");
    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "distribución de inversiones debe sumar exactamente 100 %. Ahora suma 90 %" })).toBeVisible();
    expect(api.writes).toHaveLength(0);
  });
});

test.describe("settings: category budgets against the base income", () => {
  test("shows the base note, the distribution bar and each budget as a percentage", async ({ page }) => {
    await open(page, "Categorías y presupuestos", { settingsPatch: planning });

    const bar = page.getByRole("region", { name: "Distribución del ingreso base" });
    await expect(bar).toContainText("$41,433.00 asignados de $62,090.00");
    await expect(bar).toContainText("66.7 %");
    await expect(bar).toContainText("Gasto$14,433.00");
    await expect(bar).toContainText("Ahorro$27,000.00");
    await expect(bar).toContainText("Sin asignar$20,657.00 · 33.3 %");
    await expect(bar).toContainText("Base: 3,500 USD × 17.74 = $62,090.00");
    await expect(bar).toContainText("estimación para planear, no el depósito real");
    await expect(bar).toContainText("no incluye los ajustes por mes ni el plan de pausa de inversiones");

    await expect(page.getByLabel("Porcentaje del ingreso base de Vivienda (%)")).toHaveValue("5.8");
    await expect(page.getByLabel("Porcentaje del ingreso base de Fondo de emergencia (%)")).toHaveValue("24.2");
    await expect(page.getByLabel("Porcentaje del ingreso base de Ocio (%)")).toHaveValue("");
    // Income categories have no budget to plan against.
    await expect(page.getByLabel("Porcentaje del ingreso base de Sueldo (%)")).toHaveCount(0);
  });

  test("typing a percentage derives the amount (rounded to cents) and the bar follows", async ({ page }) => {
    const api = await open(page, "Categorías y presupuestos", { settingsPatch: planning });

    await page.getByLabel("Porcentaje del ingreso base de Ocio (%)").fill("10");
    await expect(page.getByLabel("Presupuesto de Ocio")).toHaveValue("6209.00");
    await expect(page.getByRole("region", { name: "Distribución del ingreso base" })).toContainText("$47,642.00 asignados");

    // Typing the amount derives the percentage instead.
    await page.getByLabel("Presupuesto de Ocio").fill("1000");
    await expect(page.getByLabel("Porcentaje del ingreso base de Ocio (%)")).toHaveValue("1.6");

    await page.getByLabel("Porcentaje del ingreso base de Ocio (%)").fill("2.5");
    await expect(page.getByLabel("Presupuesto de Ocio")).toHaveValue("1552.25");
    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByText("Cambios guardados.")).toBeVisible();
    const put = api.writes.find((w) => w.method === "PUT" && w.path === "/settings/categories");
    expect((put?.body as { name: string; budget: string }[]).find((c) => c.name === "Ocio")?.budget).toBe("1552.25");
  });

  test("blocks saving when the budgets exceed 100 % of the base and names the excess", async ({ page }) => {
    const api = await open(page, "Categorías y presupuestos", { settingsPatch: planning });

    await page.getByLabel("Presupuesto de Ocio").fill("20658");
    const bar = page.getByRole("region", { name: "Distribución del ingreso base" });
    await expect(bar).toContainText("Excedido");
    await expect(bar).toContainText("Los presupuestos exceden el ingreso base por $1.00.");

    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "exceden por $1.00 el ingreso base de $62,090.00" })).toBeVisible();
    expect(api.writes).toHaveLength(0);

    // Exactly 100 % is allowed.
    await page.getByLabel("Presupuesto de Ocio").fill("20657");
    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByText("Cambios guardados.")).toBeVisible();
    expect(api.writes.filter((w) => w.method === "PUT")).toHaveLength(1);
  });

  test("degrades to amount-only editing when the base is not available", async ({ page }) => {
    const api = await open(page, "Categorías y presupuestos", { settingsPatch: withoutSalary });

    await expect(page.getByRole("region", { name: "Distribución del ingreso base" })).toHaveCount(0);
    await expect(page.getByLabel(/Porcentaje del ingreso base/)).toHaveCount(0);
    await expect(page.getByText(/\bNaN\b/)).toHaveCount(0);

    // No cap without a base: a huge budget saves like before.
    await page.getByLabel("Presupuesto de Vivienda").fill("900000");
    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByText("Cambios guardados.")).toBeVisible();
    const put = api.writes.find((w) => w.method === "PUT" && w.path === "/settings/categories");
    expect((put?.body as { name: string; budget: string }[]).find((c) => c.name === "Vivienda")?.budget).toBe("900000");
  });

  test("has no horizontal overflow at 375px, including the over-budget state", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await open(page, "Categorías y presupuestos", { settingsPatch: planning });
    await expect(page.getByRole("region", { name: "Distribución del ingreso base" })).toBeVisible();
    await expectNoHorizontalOverflow(page, "categories at 375px");
    await page.getByLabel("Presupuesto de Ocio").fill("99999");
    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "exceden por" })).toBeVisible();
    await expectNoHorizontalOverflow(page, "categories over budget at 375px");

    await page.getByRole("tab", { name: "General", exact: true }).click();
    await page.getByLabel("Aguinaldo y vacaciones (%)").fill("14.9");
    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "Ahora suman 99.9 %" })).toBeVisible();
    await expectNoHorizontalOverflow(page, "general split error at 375px");
  });

  test("keeps the percent inputs at 44px and the unit visible", async ({ page }) => {
    await open(page, "Categorías y presupuestos", { settingsPatch: planning });
    const field = page.getByLabel("Porcentaje del ingreso base de Vivienda (%)");
    expect((await field.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await expect(field.locator("xpath=..").getByText("%", { exact: true })).toBeVisible();
  });
});

// Screenshots for visual review: set SHOTS_DIR to write them (never committed).
const shots = process.env.SHOTS_DIR;
for (const scheme of ["light", "dark"] as const) {
  for (const width of [375, 1280]) {
    test(`screenshots ${scheme} ${width}px`, async ({ page }, info) => {
      test.skip(!shots || info.project.name !== "mobile", "only with SHOTS_DIR, once");
      await page.setViewportSize({ width, height: width === 375 ? 900 : 1000 });
      await page.emulateMedia({ colorScheme: scheme });
      await open(page, "Categorías y presupuestos", { settingsPatch: planning });
      await expect(page.getByRole("region", { name: "Distribución del ingreso base" })).toBeVisible();
      await page.screenshot({ path: `${shots}/categories-${scheme}-${width}.png`, fullPage: false });
      await page.getByLabel("Presupuesto de Ocio").fill("20658");
      await page.getByRole("button", { name: "Guardar cambios" }).click();
      await expect(page.getByRole("alert").filter({ hasText: "exceden por" })).toBeVisible();
      await page.screenshot({ path: `${shots}/categories-over-${scheme}-${width}.png`, fullPage: false });
      await page.getByRole("tab", { name: "General", exact: true }).click();
      await page.getByLabel("Reserva SAT (%)").scrollIntoViewIfNeeded();
      await page.getByLabel("Aguinaldo y vacaciones (%)").fill("14.9");
      await page.getByRole("button", { name: "Guardar cambios" }).click();
      await expect(page.getByRole("alert").filter({ hasText: "Ahora suman" })).toBeVisible();
      await page.getByRole("group", { name: "Reparto del ingreso extra" }).screenshot({ path: `${shots}/split-${scheme}-${width}.png` });
    });
  }
}
