import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession } from "./helpers";
import { expectNoHorizontalOverflow, gotoSettings, hasSidebar, switchSection } from "./settingsNav";

async function open(page: Page, section: string, opts: Parameters<typeof mockApi>[1] = {}) {
  const api = await mockApi(page, opts);
  await seedSession(page);
  await gotoSettings(page, section);
  return api;
}

const dialog = (page: Page, name: string | RegExp) => page.getByRole("dialog", { name });
const row = (page: Page, name: string) => page.getByRole("button", { name: new RegExp(`^${name}`) });
const lastPut = (api: Awaited<ReturnType<typeof mockApi>>, path: string) => api.writes.filter((w) => w.method === "PUT" && w.path === path).at(-1);

test.describe("settings: navigation", () => {
  test("opens General by default and writes the section into the URL", async ({ page }) => {
    await mockApi(page);
    await seedSession(page);
    await page.goto("/configuracion");
    await expect(page.getByRole("heading", { level: 2, name: "General" })).toBeVisible();
    await switchSection(page, "Clientes");
    await expect(page).toHaveURL(/\/configuracion\?seccion=clientes$/);
    await expect(page.getByRole("heading", { level: 2, name: "Clientes" })).toBeVisible();
  });

  test("a ?seccion= deep link opens that section; an unknown one opens General", async ({ page }) => {
    await mockApi(page);
    await seedSession(page);
    for (const [id, heading] of [
      ["categorias", "Categorías y presupuestos"],
      ["clientes", "Clientes"],
      ["instrumentos", "Instrumentos"],
      ["resico", "Rangos de RESICO"],
      ["pausa", "Pausa de inversiones"],
      ["general", "General"],
    ]) {
      await page.goto(`/configuracion?seccion=${id}`);
      await expect(page.getByRole("heading", { level: 2, name: heading })).toBeVisible();
    }
    await page.goto("/configuracion?seccion=nope");
    await expect(page.getByRole("heading", { level: 2, name: "General" })).toBeVisible();
  });

  test("on wide screens a vertical tablist switches sections with the arrow keys", async ({ page }) => {
    test.skip(!hasSidebar(page), "the sidebar only exists from 1024px");
    await open(page, "general");
    const tablist = page.getByRole("tablist", { name: "Secciones de configuración" });
    await expect(tablist).toHaveAttribute("aria-orientation", "vertical");
    await expect(page.getByLabel("Sección")).toBeHidden();
    await page.getByRole("tab", { name: "General", exact: true }).focus();
    await page.keyboard.press("ArrowDown");
    await expect(page.getByRole("tab", { name: "Categorías y presupuestos" })).toBeFocused();
    await expect(page.getByRole("tab", { name: "Categorías y presupuestos" })).toHaveAttribute("aria-selected", "true");
    await expect(page).toHaveURL(/seccion=categorias/);
    await page.keyboard.press("End");
    await expect(page.getByRole("tab", { name: "Usuarios" })).toBeFocused();
    await page.keyboard.press("Home");
    await expect(page.getByRole("tab", { name: "General", exact: true })).toHaveAttribute("aria-selected", "true");
  });

  test("on small screens a labelled select replaces the tabs", async ({ page }) => {
    test.skip(hasSidebar(page), "small screens only");
    await open(page, "general");
    await expect(page.getByRole("tab")).toHaveCount(0);
    const select = page.getByLabel("Sección");
    await expect(select).toHaveValue("general");
    await select.selectOption({ label: "Rangos de RESICO" });
    await expect(page).toHaveURL(/seccion=resico/);
    await expect(page.getByRole("heading", { level: 2, name: "Rangos de RESICO" })).toBeVisible();
    await expectNoHorizontalOverflow(page, "section select");
  });

  test("has no horizontal overflow in any section", async ({ page }) => {
    await open(page, "general");
    for (const name of ["General", "Categorías y presupuestos", "Clientes", "Instrumentos", "Rangos de RESICO", "Pausa de inversiones"]) {
      await switchSection(page, name);
      await expect(page.getByRole("heading", { level: 2, name, exact: true })).toBeVisible();
      await expectNoHorizontalOverflow(page, name);
    }
  });
});

test.describe("settings: categories", () => {
  test("groups the categories by kind with their budget", async ({ page }) => {
    await open(page, "categorias");
    for (const kind of ["Gasto", "Ahorro", "Ingreso"]) await expect(page.getByRole("heading", { level: 3, name: kind, exact: true })).toBeVisible();
    await expect(row(page, "Renta")).toContainText("$12,000.50");
    await expect(row(page, "Inversiones")).toContainText("Sin presupuesto");
    // Nothing is editable until a category is opened.
    await expect(page.getByLabel("Presupuesto", { exact: true })).toHaveCount(0);
  });

  test("edits a category budget in a dialog, saves it as a decimal string and shows the saved value", async ({ page }) => {
    const api = await open(page, "categorias");

    await row(page, "Renta").click();
    const d = dialog(page, "Editar Renta");
    await expect(d).toBeVisible();
    const budget = d.getByLabel("Presupuesto", { exact: true });
    await expect(budget).toHaveValue("12000.50");
    await expect(d.getByLabel("Incluye")).toHaveValue("Renta y mantenimiento");
    await expect(d.getByLabel("Palabras clave")).toHaveValue("renta, alquiler");
    await budget.fill("13500.75");
    await expectNoHorizontalOverflow(page, "category dialog");
    await d.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(d).toHaveCount(0);

    const put = lastPut(api, "/settings/categories");
    expect(put, "PUT /settings/categories was sent").toBeTruthy();
    const body = put?.body as { name: string; budget: string | null }[];
    expect(body).toHaveLength(9);
    expect(body.find((c) => c.name === "Renta")?.budget).toBe("13500.75");
    expect(body.find((c) => c.name === "Comida")?.budget).toBe("6000");
    expect(body.find((c) => c.name === "Inversiones")?.budget).toBeNull();
    await expect(row(page, "Renta")).toContainText("$13,500.75");
    // Focus goes back to the row that opened the dialog.
    await expect(row(page, "Renta")).toBeFocused();

    await page.reload();
    await expect(row(page, "Renta")).toContainText("$13,500.75");
  });

  test("Escape and Cancelar close the dialog without saving and return focus to the row", async ({ page }) => {
    const api = await open(page, "categorias");
    await row(page, "Comida").click();
    await dialog(page, "Editar Comida").getByLabel("Presupuesto", { exact: true }).fill("1");
    await page.keyboard.press("Escape");
    await expect(dialog(page, "Editar Comida")).toHaveCount(0);
    await expect(row(page, "Comida")).toBeFocused();

    await row(page, "Comida").click();
    await dialog(page, "Editar Comida").getByRole("button", { name: "Cancelar" }).click();
    await expect(dialog(page, "Editar Comida")).toHaveCount(0);
    expect(api.writes).toHaveLength(0);
    await expect(row(page, "Comida")).toContainText("$6,000.00");
  });

  test("adds a category from a dialog, saves it at the end of the list and keeps saved ones read-only", async ({ page }) => {
    const api = await open(page, "categorias");

    // A saved category has no name or kind inputs.
    await row(page, "Renta").click();
    await expect(dialog(page, "Editar Renta").getByLabel("Nombre")).toHaveCount(0);
    await expect(dialog(page, "Editar Renta").getByLabel("Tipo")).toHaveCount(0);
    await page.keyboard.press("Escape");

    await page.getByRole("button", { name: "Agregar categoría" }).click();
    const d = dialog(page, "Agregar categoría");
    await expect(d.getByLabel("Tipo")).toHaveValue("Gasto");
    await d.getByLabel("Nombre").fill("  Donaciones ");
    await d.getByLabel("Presupuesto", { exact: true }).fill("500");
    await d.getByLabel("Palabras clave").fill("iglesia, diezmo");
    await expectNoHorizontalOverflow(page, "new category dialog");
    await d.getByRole("button", { name: "Agregar", exact: true }).click();
    await expect(d).toHaveCount(0);

    const body = lastPut(api, "/settings/categories")?.body as { name: string; kind: string; budget: string | null; keywords: string[] }[];
    expect(body).toHaveLength(10);
    expect(body[body.length - 1]).toMatchObject({ name: "Donaciones", kind: "Gasto", budget: "500", keywords: ["iglesia", "diezmo"] });
    await expect(row(page, "Donaciones")).toContainText("$500.00");

    // Once saved it is read-only like the rest.
    await row(page, "Donaciones").click();
    await expect(dialog(page, "Editar Donaciones").getByLabel("Nombre")).toHaveCount(0);
  });

  test("blocks an empty or repeated name inside the dialog and sends nothing", async ({ page }) => {
    const api = await open(page, "categorias");
    await page.getByRole("button", { name: "Agregar categoría" }).click();
    const d = dialog(page, "Agregar categoría");
    await d.getByRole("button", { name: "Agregar", exact: true }).click();
    await expect(d.getByText("Escribe el nombre de la categoría.")).toBeVisible();
    await expect(d.getByLabel("Nombre")).toBeFocused();

    await d.getByLabel("Nombre").fill("renta");
    await d.getByRole("button", { name: "Agregar", exact: true }).click();
    await expect(d.getByText("Ya existe una categoría con ese nombre.")).toBeVisible();
    expect(api.writes).toHaveLength(0);

    await d.getByLabel("Nombre").fill("Donaciones");
    await d.getByRole("button", { name: "Agregar", exact: true }).click();
    await expect(d).toHaveCount(0);
    expect(api.writes.filter((w) => w.method === "PUT")).toHaveLength(1);
  });

  test("blocks an invalid budget, focuses the field and sends nothing", async ({ page }) => {
    const api = await open(page, "categorias");
    await row(page, "Comida").click();
    const d = dialog(page, "Editar Comida");
    const budget = d.getByLabel("Presupuesto", { exact: true });
    await budget.fill("12,5");
    await d.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(d.getByRole("alert").filter({ hasText: "número válido" })).toBeVisible();
    await expect(budget).toBeFocused();
    expect(api.writes).toHaveLength(0);
  });

  test("shows the backend message inside the dialog when the server rejects the settings", async ({ page }) => {
    await open(page, "categorias", { settingsFail: "budget for Renta exceeds income" });
    await row(page, "Renta").click();
    const d = dialog(page, "Editar Renta");
    await d.getByLabel("Presupuesto", { exact: true }).fill("99999");
    await d.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(d.getByRole("alert").filter({ hasText: "budget for Renta exceeds income" })).toBeVisible();
    await expect(d).toBeVisible();
  });
});

test.describe("settings: general blocks", () => {
  test("shows each block as a summary card with the period preview", async ({ page }) => {
    await open(page, "general", { cycleStartDay: 31 });
    await expect(page.getByRole("heading", { level: 3, name: "Ingreso base" })).toBeVisible();
    await expect(page.getByText("2,500 USD × 17.50 = $43,750.00 al mes")).toBeVisible();
    await expect(page.getByText(/^Periodo actual: \d{1,2} \w{3} → \d{1,2} \w{3}$/)).toBeVisible();
    await expect(page.getByText("Último día del mes")).toBeVisible();
    await expect(page.getByLabel("Inicio del periodo")).toHaveCount(0);
  });

  test("edits the pay cycle start day in its block and saves the whole general object", async ({ page }) => {
    const api = await open(page, "general");
    await page.getByRole("button", { name: "Editar periodo" }).click();
    const d = dialog(page, "Editar periodo");
    const select = d.getByLabel("Inicio del periodo");
    await expect(select).toHaveValue("0");
    await select.selectOption({ label: "Último día hábil (lun–vie)" });
    await expect(d.getByText(/^Periodo actual: /)).toBeVisible();
    await d.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(d).toHaveCount(0);

    const body = lastPut(api, "/settings/general")?.body as { cycle_start_day: number; salary_usd: string; extra_income_split: Record<string, string> };
    expect(body.cycle_start_day).toBe(32);
    expect(body.salary_usd).toBe("2500.00");
    expect(body.extra_income_split.sat_reserve_rate).toBe("0.165");
    await expect(page.getByText("Último día hábil (lun–vie)")).toBeVisible();
    await expect(page.getByRole("button", { name: "Editar periodo" })).toBeFocused();

    await page.reload();
    await expect(page.getByText("Último día hábil (lun–vie)")).toBeVisible();
  });

  test("edits the base income and blocks an invalid salary", async ({ page }) => {
    const api = await open(page, "general");
    await page.getByRole("button", { name: "Editar ingreso base" }).click();
    const d = dialog(page, "Editar ingreso base");
    await d.getByLabel("Salario mensual (USD)").fill("abc");
    await d.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(d.getByText("Escribe un número válido")).toBeVisible();
    await expect(d.getByLabel("Salario mensual (USD)")).toBeFocused();
    expect(api.writes).toHaveLength(0);

    await d.getByLabel("Salario mensual (USD)").fill("3000");
    await d.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(d).toHaveCount(0);
    expect((lastPut(api, "/settings/general")?.body as { salary_usd: string }).salary_usd).toBe("3000");
    await expect(page.getByText("3,000 USD × 17.50 = $52,500.00 al mes")).toBeVisible();
  });
});

test.describe("settings: clients", () => {
  test("lists compact cards and edits a client in a dialog grouped in fieldsets", async ({ page }) => {
    const api = await open(page, "clientes");
    await expect(page.getByRole("button", { name: /Acme Inc\./ })).toContainText("RFC XEXX010101000");
    await expect(page.getByRole("button", { name: /Acme Inc\./ })).toContainText("Régimen 616");
    // No add button: clients cannot be created from the app today.
    await expect(page.getByRole("button", { name: /Agregar cliente/ })).toHaveCount(0);

    await page.getByRole("button", { name: /Acme Inc\./ }).click();
    const d = dialog(page, "Editar Acme Inc.");
    for (const legend of ["Fiscal", "Factura (CFDI)", "Contrato"]) await expect(d.getByRole("group", { name: legend })).toBeVisible();

    const postal = d.getByLabel("Código postal");
    await expect(postal).toHaveValue("");
    await postal.fill("6600");
    await d.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(d.getByText("El código postal debe tener 5 dígitos o dejarse vacío.")).toBeVisible();
    expect(api.writes.some((w) => w.path === "/settings/clients")).toBe(false);

    await postal.fill("06600");
    await d.getByLabel("Contrato").fill("MSA-2026");
    await d.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(d).toHaveCount(0);
    const body = lastPut(api, "/settings/clients")?.body as { id: string; postal_code: string; contract: string }[];
    expect(body).toHaveLength(2);
    expect(body[0]).toMatchObject({ id: "usa", postal_code: "06600", contract: "MSA-2026" });
    expect(body[1].id).toBe("b");
  });
});

test.describe("settings: instruments", () => {
  test("lists instruments and edits one in a dialog", async ({ page }) => {
    const api = await open(page, "instrumentos");
    await expect(page.getByRole("button", { name: /VOO/ })).toContainText("GBM");
    await page.getByRole("button", { name: /CETES/ }).click();
    const d = dialog(page, "Editar CETES");
    await d.getByLabel("Nombre").fill("");
    await d.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(d.getByText("Este campo es obligatorio.")).toBeVisible();
    expect(api.writes).toHaveLength(0);

    await d.getByLabel("Nombre").fill("CETES 28");
    await d.getByLabel("Plataforma").fill("Hey Banco");
    await d.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(d).toHaveCount(0);
    const body = lastPut(api, "/settings/instruments")?.body as { instruments: { id: string; name: string; platform: string }[]; by_category: Record<string, string> };
    expect(body.instruments[1]).toMatchObject({ id: "i2", name: "CETES 28", platform: "Hey Banco" });
    expect(body.instruments[0].name).toBe("VOO");
    expect(body.by_category.Inversiones).toBe("i1");
  });
});

test.describe("settings: RESICO brackets", () => {
  test("shows a read-only table and edits the set in a dialog", async ({ page }) => {
    const api = await open(page, "resico");
    const table = page.getByRole("table", { name: "Rangos de RESICO" });
    await expect(table.getByRole("row")).toHaveCount(3);
    await expect(table).toContainText("$25,000.00");
    await expect(table).toContainText("1.1%");
    await expect(page.getByLabel(/Límite superior/)).toHaveCount(0);

    await page.getByRole("button", { name: "Editar rangos" }).click();
    const d = dialog(page, "Editar rangos de RESICO");
    await expect(d.getByLabel("Límite superior (rango 1)")).toHaveValue("25000.00");
    await d.getByRole("button", { name: "Agregar rango" }).click();
    await d.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(d.getByText("Este campo es obligatorio.").first()).toBeVisible();
    expect(api.writes).toHaveLength(0);

    await d.getByLabel("Límite superior (rango 3)").fill("83333.33");
    await d.getByLabel("Tasa de ISR (rango 3)").fill("0.0150");
    await d.getByRole("button", { name: "Quitar rango 1" }).click();
    await d.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(d).toHaveCount(0);
    expect(lastPut(api, "/settings/brackets")?.body).toEqual([
      { upper: "50000.00", rate: "0.0110" },
      { upper: "83333.33", rate: "0.0150" },
    ]);
    await expect(table.getByRole("row")).toHaveCount(3);
    await expect(table).toContainText("$83,333.33");
  });
});

test.describe("settings: investment pause", () => {
  test("shows the status card first and the month lookup after it", async ({ page }) => {
    await open(page, "pausa");
    await expect(page.getByText("Pausa activa: 2 meses, reanuda en febrero de 2027")).toBeVisible();
    await expect(page.getByLabel("Meses en pausa")).toHaveCount(0);
    await expect(page.getByRole("heading", { level: 3, name: "Presupuesto efectivo del mes" })).toBeVisible();
  });

  test("edits the pause in a dialog", async ({ page }) => {
    const api = await open(page, "pausa");
    await page.getByRole("button", { name: "Editar", exact: true }).click();
    const d = dialog(page, "Editar pausa de inversiones");
    await expect(d.getByLabel("Meses en pausa")).toHaveValue("2026-10, 2026-11");
    await d.getByLabel("Mes de reanudación").fill("2027-03");
    await d.getByLabel("Nota").fill("Viaje");
    await d.getByRole("button", { name: "Guardar pausa" }).click();
    await expect(d).toHaveCount(0);
    expect(lastPut(api, "/settings/investment-pause")?.body).toMatchObject({ resume_month: "2027-03", note: "Viaje" });
    await expect(page.getByText("reanuda en marzo de 2027")).toBeVisible();
  });

  test("cancels the investment pause after confirming, then offers to configure one", async ({ page }) => {
    const api = await open(page, "pausa");
    await page.getByRole("button", { name: "Editar", exact: true }).click();
    const d = dialog(page, "Editar pausa de inversiones");
    await d.getByRole("button", { name: "Cancelar pausa" }).click();
    await expect(d.getByText("¿Cancelar la pausa?")).toBeVisible();
    await d.getByRole("button", { name: "Conservar pausa" }).click();
    expect(api.writes).toHaveLength(0);

    await d.getByRole("button", { name: "Cancelar pausa" }).click();
    await d.getByRole("button", { name: "Sí, cancelar pausa" }).click();
    await expect(d).toHaveCount(0);
    await expect(page.getByText("Sin pausa", { exact: true })).toBeVisible();
    expect(api.writes.some((w) => w.method === "DELETE" && w.path === "/settings/investment-pause")).toBe(true);
    await expect(page.getByRole("button", { name: "Configurar pausa" })).toBeVisible();
  });
});
