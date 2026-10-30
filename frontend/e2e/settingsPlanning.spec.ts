import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession } from "./helpers";
import { expectNoHorizontalOverflow, gotoSettings } from "./settingsNav";

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

async function open(page: Page, section: string, opts: Parameters<typeof mockApi>[1] = {}) {
  const api = await mockApi(page, opts);
  await seedSession(page);
  await gotoSettings(page, section);
  return api;
}

const dialog = (page: Page, name: string | RegExp) => page.getByRole("dialog", { name });
const row = (page: Page, name: string) => page.getByRole("button", { name: new RegExp(`^${name}`) });
const lastPut = (api: Awaited<ReturnType<typeof mockApi>>, path: string) => api.writes.filter((w) => w.method === "PUT" && w.path === path).at(-1);
const save = (d: ReturnType<typeof dialog>) => d.getByRole("button", { name: "Guardar cambios" });

test.describe("settings: extra-income split and allocation as percentages", () => {
  test("shows decimals as percentages in the summary and saves them back exactly", async ({ page }) => {
    const api = await open(page, "general");
    const split = page.getByRole("heading", { level: 3, name: "Reparto del ingreso extra" }).locator("xpath=ancestor::div[contains(@class,'rounded-xl')][1]");
    await expect(split).toContainText("Reserva SAT16.5 %");
    await expect(split).toContainText("Fondo de emergencia50 %");
    const alloc = page.getByRole("heading", { level: 3, name: "Distribución de inversiones" }).locator("xpath=ancestor::div[contains(@class,'rounded-xl')][1]");
    await expect(alloc).toContainText("VOO60 %");
    await expect(alloc).toContainText("BTC40 %");

    await page.getByRole("button", { name: "Editar reparto del ingreso extra" }).click();
    const d = dialog(page, "Editar reparto del ingreso extra");
    await expect(d.getByLabel("Reserva SAT (%)")).toHaveValue("16.5");
    await expect(d.getByLabel("Fondo de emergencia (%)")).toHaveValue("50");
    await expect(d.getByLabel("Inversiones (%)")).toHaveValue("35");
    await expect(d.getByLabel("Aguinaldo y vacaciones (%)")).toHaveValue("15");
    await expect(d.getByLabel("Reserva SAT (%)")).toHaveAttribute("inputmode", "decimal");
    await expect(d.getByText("Suma de los tres destinos: 100 %")).toBeVisible();

    await d.getByLabel("Reserva SAT (%)").fill("17.25");
    await save(d).click();
    await expect(d).toHaveCount(0);

    const body = lastPut(api, "/settings/general")?.body as { extra_income_split: Record<string, string>; investment_allocation: { key: string; value: string }[] };
    expect(body.extra_income_split).toEqual({ sat_reserve_rate: "0.1725", fondo_emergencia: "0.5", inversiones: "0.35", aguinaldo_vacaciones: "0.15" });
    expect(body.investment_allocation).toEqual([
      { key: "VOO", value: "0.6" },
      { key: "BTC", value: "0.4" },
    ]);
    await expect(split).toContainText("Reserva SAT17.25 %");
  });

  test("blocks a split that does not add up to 100 % and sends nothing", async ({ page }) => {
    const api = await open(page, "general");
    await page.getByRole("button", { name: "Editar reparto del ingreso extra" }).click();
    const d = dialog(page, "Editar reparto del ingreso extra");
    await d.getByLabel("Aguinaldo y vacaciones (%)").fill("14.9");
    await expect(d.getByText("Suma de los tres destinos: 99.9 % (debe sumar 100 %)")).toBeVisible();

    await save(d).click();
    await expect(d.getByText("deben sumar exactamente 100 %. Ahora suman 99.9 %")).toBeVisible();
    await expect(d.getByLabel("Fondo de emergencia (%)")).toBeFocused();
    expect(api.writes).toHaveLength(0);

    await d.getByLabel("Aguinaldo y vacaciones (%)").fill("15");
    await save(d).click();
    await expect(d).toHaveCount(0);
  });

  test("lets the SAT reserve be anything from 0 to 100 and rejects more", async ({ page }) => {
    const api = await open(page, "general");
    await page.getByRole("button", { name: "Editar reparto del ingreso extra" }).click();
    const d = dialog(page, "Editar reparto del ingreso extra");
    await d.getByLabel("Reserva SAT (%)").fill("100.5");
    await save(d).click();
    await expect(d.getByText("entre 0 y 100")).toBeVisible();
    expect(api.writes).toHaveLength(0);
    await d.getByLabel("Reserva SAT (%)").fill("90");
    await save(d).click();
    await expect(d).toHaveCount(0);
    expect(api.writes).toHaveLength(1);
  });

  test("blocks an investment allocation that does not add up to 100 %", async ({ page }) => {
    const api = await open(page, "general");
    await page.getByRole("button", { name: "Editar distribución de inversiones" }).click();
    const d = dialog(page, "Editar distribución de inversiones");
    await expect(d.getByLabel("Inversión: VOO (%)")).toHaveValue("60");
    await d.getByLabel("Inversión: BTC (%)").fill("30");
    await save(d).click();
    await expect(d.getByText("distribución de inversiones debe sumar exactamente 100 %. Ahora suma 90 %")).toBeVisible();
    expect(api.writes).toHaveLength(0);
  });
});

test.describe("settings: category budgets against the base income", () => {
  test("shows the distribution bar, the budgeted line and each budget with its share", async ({ page }) => {
    await open(page, "categorias", { settingsPatch: planning });

    const bar = page.getByRole("region", { name: "Distribución del ingreso base" });
    await expect(bar).toContainText("$41,433.00 asignados de $62,090.00");
    await expect(bar).toContainText("66.7 %");
    await expect(bar).toContainText("Gasto$14,433.00");
    await expect(bar).toContainText("Ahorro$27,000.00");
    await expect(bar).toContainText("Sin asignar$20,657.00 · 33.3 %");
    await expect(bar).toContainText("Base: 3,500 USD × 17.74 = $62,090.00");
    await expect(page.getByText("Presupuestado: $41,433.00 de $62,090.00 (66.7 %)")).toBeVisible();

    await expect(row(page, "Vivienda")).toContainText("$3,600.00");
    await expect(row(page, "Vivienda")).toContainText("5.8 %");
    await expect(row(page, "Fondo de emergencia")).toContainText("24.2 %");
    await expect(row(page, "Ocio")).toContainText("Sin presupuesto");
    await expect(row(page, "Ocio")).not.toContainText("%");
    // Income categories have no budget to plan against.
    await expect(row(page, "Sueldo")).not.toContainText("%");
  });

  test("typing a percentage in the dialog derives the amount (rounded to cents)", async ({ page }) => {
    const api = await open(page, "categorias", { settingsPatch: planning });

    await row(page, "Ocio").click();
    const d = dialog(page, "Editar Ocio");
    const amount = d.getByLabel("Presupuesto", { exact: true });
    const pct = d.getByLabel("Porcentaje del ingreso base (%)");
    await expect(pct).toHaveValue("");
    await pct.fill("10");
    await expect(amount).toHaveValue("6209.00");

    // Typing the amount derives the percentage instead.
    await amount.fill("1000");
    await expect(pct).toHaveValue("1.6");

    await pct.fill("2.5");
    await expect(amount).toHaveValue("1552.25");
    await save(d).click();
    await expect(d).toHaveCount(0);
    expect((lastPut(api, "/settings/categories")?.body as { name: string; budget: string }[]).find((c) => c.name === "Ocio")?.budget).toBe("1552.25");
    await expect(page.getByRole("region", { name: "Distribución del ingreso base" })).toContainText("$42,985.25 asignados");
  });

  test("an income category has no percentage field", async ({ page }) => {
    await open(page, "categorias", { settingsPatch: planning });
    await row(page, "Sueldo").click();
    const d = dialog(page, "Editar Sueldo");
    await expect(d.getByLabel("Presupuesto", { exact: true })).toBeVisible();
    await expect(d.getByLabel(/Porcentaje del ingreso base/)).toHaveCount(0);
  });

  test("blocks saving inside the dialog when the budgets exceed 100 % of the base and names the excess", async ({ page }) => {
    const api = await open(page, "categorias", { settingsPatch: planning });

    await row(page, "Ocio").click();
    const d = dialog(page, "Editar Ocio");
    await d.getByLabel("Presupuesto", { exact: true }).fill("20658");
    await save(d).click();
    await expect(d.getByRole("alert").filter({ hasText: "exceden por $1.00 el ingreso base de $62,090.00" })).toBeVisible();
    expect(api.writes).toHaveLength(0);

    // Exactly 100 % is allowed.
    await d.getByLabel("Presupuesto", { exact: true }).fill("20657");
    await save(d).click();
    await expect(d).toHaveCount(0);
    expect(api.writes.filter((w) => w.method === "PUT")).toHaveLength(1);
    await expect(page.getByText("Presupuestado: $62,090.00 de $62,090.00 (100.0 %)")).toBeVisible();
  });

  test("degrades to amount-only editing when the base is not available", async ({ page }) => {
    const api = await open(page, "categorias", { settingsPatch: withoutSalary });

    await expect(page.getByRole("region", { name: "Distribución del ingreso base" })).toHaveCount(0);
    await expect(page.getByText(/Presupuestado:/)).toHaveCount(0);
    await expect(page.getByText(/\bNaN\b/)).toHaveCount(0);
    await expect(row(page, "Vivienda")).not.toContainText("%");

    await row(page, "Vivienda").click();
    const d = dialog(page, "Editar Vivienda");
    await expect(d.getByLabel(/Porcentaje del ingreso base/)).toHaveCount(0);
    // No cap without a base: a huge budget saves like before.
    await d.getByLabel("Presupuesto", { exact: true }).fill("900000");
    await save(d).click();
    await expect(d).toHaveCount(0);
    expect((lastPut(api, "/settings/categories")?.body as { name: string; budget: string }[]).find((c) => c.name === "Vivienda")?.budget).toBe("900000");
  });

  test("has no horizontal overflow at 375px, including the over-budget state", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await open(page, "categorias", { settingsPatch: planning });
    await expect(page.getByRole("region", { name: "Distribución del ingreso base" })).toBeVisible();
    await expectNoHorizontalOverflow(page, "categories at 375px");
    await row(page, "Ocio").click();
    const d = dialog(page, "Editar Ocio");
    await d.getByLabel("Presupuesto", { exact: true }).fill("99999");
    await save(d).click();
    await expect(d.getByRole("alert").filter({ hasText: "exceden por" })).toBeVisible();
    await expectNoHorizontalOverflow(page, "categories over budget at 375px");
    await page.keyboard.press("Escape");

    await gotoSettings(page, "general");
    await page.getByRole("button", { name: "Editar reparto del ingreso extra" }).click();
    const split = dialog(page, "Editar reparto del ingreso extra");
    await split.getByLabel("Aguinaldo y vacaciones (%)").fill("14.9");
    await save(split).click();
    await expect(split.getByText("Ahora suman 99.9 %")).toBeVisible();
    await expectNoHorizontalOverflow(page, "general split error at 375px");
  });

  test("keeps the percent input at 44px and the unit visible", async ({ page }) => {
    await open(page, "categorias", { settingsPatch: planning });
    await row(page, "Vivienda").click();
    const field = dialog(page, "Editar Vivienda").getByLabel("Porcentaje del ingreso base (%)");
    await expect(field).toHaveValue("5.8");
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
      await open(page, "categorias", { settingsPatch: planning, cycleStartDay: 31 });
      await expect(page.getByRole("region", { name: "Distribución del ingreso base" })).toBeVisible();
      await page.screenshot({ path: `${shots}/categories-${scheme}-${width}.png`, fullPage: true });
      await row(page, "Mandado").click();
      await expect(dialog(page, "Editar Mandado")).toBeVisible();
      await page.screenshot({ path: `${shots}/category-dialog-${scheme}-${width}.png` });
      await page.keyboard.press("Escape");
      await gotoSettings(page, "general");
      await expect(page.getByRole("heading", { level: 3, name: "Periodo" })).toBeVisible();
      await page.screenshot({ path: `${shots}/general-${scheme}-${width}.png`, fullPage: true });
      await page.getByRole("button", { name: "Editar reparto del ingreso extra" }).click();
      await dialog(page, "Editar reparto del ingreso extra").getByLabel("Aguinaldo y vacaciones (%)").fill("14.9");
      await save(dialog(page, "Editar reparto del ingreso extra")).click();
      await expect(dialog(page, "Editar reparto del ingreso extra").getByRole("alert")).toBeVisible();
      await page.screenshot({ path: `${shots}/split-error-${scheme}-${width}.png` });
    });
  }
}
