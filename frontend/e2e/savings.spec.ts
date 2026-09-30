import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockSaving, type MockValuation } from "./helpers";

function today() {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

const seeded = (over: Partial<MockSaving> = {}): MockSaving => ({
  id: 1,
  date: today(),
  description: "Aportación VOO",
  category: "Inversión ETF",
  instrument: "i1",
  payment_method: "Transferencia",
  currency: "MXN",
  amount: "10000.00",
  exchange_rate: null,
  amount_mxn: "10000.00",
  ...over,
});

const emergency = (over: Partial<MockSaving> = {}) =>
  seeded({ id: 2, description: "Fondo inicial", category: "Fondo de emergencia", instrument: "i2", amount: "20000.00", amount_mxn: "20000.00", ...over });

const vooValuation: MockValuation = { date: "2026-09-30", instrument: "i1", value_mxn: "12000.00", note: "GBM" };

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

async function open(page: Page, opts: Parameters<typeof mockApi>[1] = {}) {
  const api = await mockApi(page, opts);
  await seedSession(page);
  await page.goto("/ahorros");
  await expect(page.getByRole("heading", { level: 1, name: "Ahorros" })).toBeVisible();
  await expect(page.getByRole("form", { name: "Nuevo ahorro" })).toBeVisible();
  return api;
}

const savingForm = (page: Page) => page.getByRole("form", { name: /ahorro$/ });
const transferForm = (page: Page) => page.getByRole("form", { name: "Traspasar entre instrumentos" });
const valuationForm = (page: Page) => page.getByRole("form", { name: "Registrar valuación" });

test.describe("savings page", () => {
  test("shows the portfolio with totals, subtotals and the emergency fund progress", async ({ page }) => {
    await open(page, { savings: [seeded(), emergency()], valuations: [vooValuation] });

    const voo = page.getByRole("row", { name: /VOO/ });
    await expect(voo).toContainText("$10,000.00");
    await expect(voo).toContainText("$12,000.00");
    await expect(voo).toContainText("2026-09-30");
    await expect(voo).toContainText("$2,000.00");
    await expect(voo).toContainText("20.00%");
    await expect(voo).toContainText("100.00%");
    const cetes = page.getByRole("row", { name: /CETES/ });
    await expect(cetes).toContainText("$20,000.00");
    await expect(cetes).toContainText("sin valuar");

    const total = page.getByRole("row", { name: /^Total/ });
    await expect(total).toContainText("$30,000.00");
    await expect(total).toContainText("$12,000.00");

    await expect(page.getByRole("region", { name: "Subtotales por tipo" })).toContainText("ETF");
    await expect(page.getByRole("region", { name: "Subtotales por tipo" })).toContainText("Renta fija");
    await expect(page.getByRole("region", { name: "Subtotales por destino" })).toContainText("Inversión ETF");
    await expect(page.getByRole("region", { name: "Subtotales por destino" })).toContainText("$10,000.00");

    const card = page.getByRole("region", { name: "Fondo de emergencia" });
    await expect(card).toContainText("$20,000.00 de $60,000.00 (33.33%)");
    await expect(card.getByRole("progressbar", { name: "Avance del fondo de emergencia" })).toHaveAttribute("aria-valuenow", "33");
  });

  test("shows empty states when nothing is registered", async ({ page }) => {
    await open(page);
    await expect(page.getByText("No hay ahorros registrados en este mes.")).toBeVisible();
    await expect(page.getByRole("row", { name: /CETES/ })).toContainText("sin valuar");
    await expect(page.getByRole("row", { name: /^Total/ })).toContainText("$0.00");
  });

  test("adds a saving as a decimal string and refreshes the portfolio", async ({ page }) => {
    const api = await open(page);
    await expect(savingForm(page).getByLabel("Categoría").locator("option")).toHaveText([
      "Automática (según la descripción)",
      "Fondo de emergencia",
      "Inversión ETF",
    ]);
    await expect(savingForm(page).getByLabel("Instrumento").locator("option")).toHaveText(["Automático (según la categoría)", "VOO", "CETES"]);

    await savingForm(page).getByLabel("Descripción").fill("Aportación quincena");
    await savingForm(page).getByLabel("Categoría").selectOption("Inversión ETF");
    await savingForm(page).getByLabel("Instrumento").selectOption("i1");
    await savingForm(page).getByLabel("Monto").fill("1500.50");
    await savingForm(page).getByRole("button", { name: "Agregar ahorro" }).click();

    await expect(page.getByText("Ahorro guardado.")).toBeVisible();
    await expect(page.getByRole("listitem").filter({ hasText: "Aportación quincena" })).toContainText("$1,500.50 MXN");
    await expect(page.getByRole("row", { name: /VOO/ })).toContainText("$1,500.50");

    const body = api.requests.find((r) => r.path === "/savings")?.body as Record<string, unknown>;
    expect(body.amount).toBe("1500.50");
    expect(typeof body.amount).toBe("string");
    expect(body.category).toBe("Inversión ETF");
    expect(body.instrument).toBe("i1");
    await expect(savingForm(page).getByLabel("Descripción")).toHaveValue("");
    await expect(savingForm(page).getByLabel("Monto")).toHaveValue("");
  });

  test("registers a withdrawal with a negative amount", async ({ page }) => {
    const api = await open(page, { savings: [seeded()] });
    await savingForm(page).getByLabel("Descripción").fill("Retiro parcial");
    await savingForm(page).getByLabel("Categoría").selectOption("Inversión ETF");
    await savingForm(page).getByLabel("Monto").fill("-500.00");
    await savingForm(page).getByRole("button", { name: "Agregar ahorro" }).click();

    const row = page.getByRole("listitem").filter({ hasText: "Retiro parcial" });
    await expect(row).toContainText("-$500.00 MXN");
    await expect(row).toContainText("Retiro");
    await expect(page.getByRole("row", { name: /VOO/ })).toContainText("$9,500.00");
    expect((api.requests.find((r) => r.path === "/savings")?.body as { amount: string }).amount).toBe("-500.00");
  });

  test("blocks an invalid or zero amount and sends nothing", async ({ page }) => {
    const api = await open(page);
    for (const bad of ["12,5", "0", ""]) {
      await savingForm(page).getByLabel("Monto").fill(bad);
      await savingForm(page).getByRole("button", { name: "Agregar ahorro" }).click();
      await expect(page.getByRole("alert").filter({ hasText: "monto distinto de cero" })).toBeVisible();
      await expect(savingForm(page).getByLabel("Monto")).toBeFocused();
    }
    expect(api.requests.filter((r) => r.path === "/savings")).toHaveLength(0);
  });

  test("edits a saving through the form", async ({ page }) => {
    const api = await open(page, { savings: [seeded()] });
    await page.getByRole("button", { name: "Editar Aportación VOO" }).click();

    await expect(page.getByRole("heading", { level: 2, name: "Editar ahorro" })).toBeVisible();
    await expect(savingForm(page).getByLabel("Monto")).toHaveValue("10000.00");
    await expect(savingForm(page).getByLabel("Instrumento")).toHaveValue("i1");
    await savingForm(page).getByLabel("Monto").fill("11000.75");
    await savingForm(page).getByRole("button", { name: "Guardar cambios" }).click();

    await expect(page.getByText("Ahorro guardado.")).toBeVisible();
    await expect(page.getByRole("heading", { level: 2, name: "Nuevo ahorro" })).toBeVisible();
    await expect(page.getByRole("listitem").filter({ hasText: "Aportación VOO" })).toContainText("$11,000.75 MXN");
    const put = api.writes.find((w) => w.method === "PUT" && w.path === "/savings/1");
    expect((put?.body as { amount: string }).amount).toBe("11000.75");
  });

  test("editing only the description keeps a USD saving's currency, rate, method and MXN amount", async ({ page }) => {
    const usd = seeded({
      id: 3,
      description: "Aporte en dólares",
      currency: "USD",
      amount: "10.00",
      exchange_rate: "20.50",
      amount_mxn: "205.00",
      payment_method: "Efectivo",
      date: "2026-09-03",
    });
    const api = await open(page, { savings: [usd] });
    await page.getByLabel("Mes").fill("2026-09");
    await page.getByRole("button", { name: "Editar Aporte en dólares" }).click();

    const form = savingForm(page);
    await expect(form.getByLabel("Moneda")).toHaveValue("USD");
    await expect(form.getByLabel("Tipo de cambio (opcional)")).toHaveValue("20.50");
    await expect(form.getByLabel("Método de pago")).toHaveValue("Efectivo");
    await expect(form.getByLabel("Fecha")).toHaveValue("2026-09-03");
    await form.getByLabel("Descripción").fill("Aporte corregido");
    await form.getByRole("button", { name: "Guardar cambios" }).click();

    await expect(page.getByText("Ahorro guardado.")).toBeVisible();
    const put = api.writes.find((w) => w.method === "PUT" && w.path === "/savings/3")?.body as Record<string, unknown>;
    expect(put).toMatchObject({ currency: "USD", exchange_rate: "20.50", payment_method: "Efectivo", amount: "10.00", date: "2026-09-03" });
    const row = page.getByRole("listitem").filter({ hasText: "Aporte corregido" });
    await expect(row).toContainText("$10.00 USD");
    await expect(row).toContainText("≈ $205.00 MXN (TC 20.50)");
    expect(api.savings[0]).toMatchObject({ currency: "USD", exchange_rate: "20.50", payment_method: "Efectivo", amount_mxn: "205.00" });
  });

  test("deletes a saving after an inline confirmation", async ({ page }) => {
    page.on("dialog", (d) => {
      throw new Error(`unexpected native dialog: ${d.message()}`);
    });
    const api = await open(page, { savings: [seeded()] });

    await page.getByRole("button", { name: "Eliminar Aportación VOO" }).click();
    await expect(page.getByText("¿Eliminar este ahorro?")).toBeVisible();
    await page.getByRole("button", { name: "Cancelar", exact: true }).click();
    await expect(page.getByRole("listitem").filter({ hasText: "Aportación VOO" })).toBeVisible();
    expect(api.writes).toHaveLength(0);

    await page.getByRole("button", { name: "Eliminar Aportación VOO" }).click();
    await page.getByRole("button", { name: "Sí, eliminar" }).click();
    await expect(page.getByText("No hay ahorros registrados en este mes.")).toBeVisible();
    expect(api.writes.some((w) => w.method === "DELETE" && w.path === "/savings/1")).toBe(true);
    await expect(page.getByRole("row", { name: /VOO/ })).toContainText("$0.00");
  });

  test("transfers between instruments and lists both legs", async ({ page }) => {
    const api = await open(page, { savings: [seeded()] });
    await transferForm(page).getByLabel("Desde").selectOption("i1");
    await transferForm(page).getByLabel("Hacia").selectOption("i2");
    await transferForm(page).getByLabel("Monto a traspasar").fill("2500.25");
    await transferForm(page).getByRole("button", { name: "Traspasar" }).click();

    await expect(page.getByText("Traspaso registrado.")).toBeVisible();
    const legs = page.getByRole("listitem").filter({ hasText: "Traspaso i1 -> i2" });
    await expect(legs).toHaveCount(2);
    await expect(legs.filter({ hasText: "-$2,500.25" })).toHaveCount(1);
    await expect(page.getByRole("row", { name: /VOO/ })).toContainText("$7,499.75");
    await expect(page.getByRole("row", { name: /CETES/ })).toContainText("$2,500.25");
    const body = api.requests.find((r) => r.path === "/savings/transfers")?.body as Record<string, unknown>;
    expect(body).toEqual({ from: "i1", to: "i2", amount: "2500.25" });
    await expect(transferForm(page).getByLabel("Monto a traspasar")).toHaveValue("");
  });

  test("validates the transfer before sending it", async ({ page }) => {
    const api = await open(page);
    await transferForm(page).getByRole("button", { name: "Traspasar" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "instrumento de origen" })).toBeVisible();

    await transferForm(page).getByLabel("Desde").selectOption("i1");
    await transferForm(page).getByLabel("Hacia").selectOption("i1");
    await transferForm(page).getByLabel("Monto a traspasar").fill("100");
    await transferForm(page).getByRole("button", { name: "Traspasar" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "distinto al origen" })).toBeVisible();
    expect(api.requests.filter((r) => r.path === "/savings/transfers")).toHaveLength(0);
  });

  test("appends a valuation and offers no way to edit or delete one", async ({ page }) => {
    const api = await open(page, { savings: [seeded()], valuations: [vooValuation] });
    await expect(page.getByRole("button", { name: /(Editar|Eliminar).*valuación/i })).toHaveCount(0);

    await valuationForm(page).getByLabel("Instrumento a valuar").selectOption("i1");
    await valuationForm(page).getByLabel("Valor actual (MXN)").fill("15000.50");
    await valuationForm(page).getByLabel("Fecha de la valuación").fill("2026-10-05");
    await valuationForm(page).getByLabel("Nota (opcional)").fill("Cierre de octubre");
    await valuationForm(page).getByRole("button", { name: "Registrar valuación" }).click();

    await expect(page.getByText("Valuación registrada.")).toBeVisible();
    const voo = page.getByRole("row", { name: /VOO/ });
    await expect(voo).toContainText("$15,000.50");
    await expect(voo).toContainText("2026-10-05");
    await expect(voo).toContainText("$5,000.50");
    const body = api.requests.find((r) => r.path === "/savings/valuations")?.body as Record<string, unknown>;
    expect(body).toEqual({ instrument: "i1", value_mxn: "15000.50", date: "2026-10-05", note: "Cierre de octubre" });
    expect(api.valuations).toHaveLength(2);
    await expect(page.getByRole("button", { name: /(Editar|Eliminar).*valuación/i })).toHaveCount(0);
  });

  test("blocks an invalid valuation and sends nothing", async ({ page }) => {
    const api = await open(page);
    await valuationForm(page).getByLabel("Valor actual (MXN)").fill("-3");
    await valuationForm(page).getByRole("button", { name: "Registrar valuación" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "Elige un instrumento." })).toBeVisible();
    await expect(page.getByRole("alert").filter({ hasText: "valor mayor a cero" })).toBeVisible();
    expect(api.requests.filter((r) => r.path === "/savings/valuations")).toHaveLength(0);
  });

  test("shows the backend message when the server rejects a saving", async ({ page }) => {
    await open(page, { savingsFail: "instrument \"zzz\" does not exist" });
    await savingForm(page).getByLabel("Monto").fill("10");
    await savingForm(page).getByRole("button", { name: "Agregar ahorro" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "instrument \"zzz\" does not exist" })).toBeVisible();
  });

  test("shows the backend message when the server rejects a valuation", async ({ page }) => {
    await open(page, { savingsFail: "value must be greater than zero" });
    await valuationForm(page).getByLabel("Instrumento a valuar").selectOption("i1");
    await valuationForm(page).getByLabel("Valor actual (MXN)").fill("10");
    await valuationForm(page).getByRole("button", { name: "Registrar valuación" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "value must be greater than zero" })).toBeVisible();
  });

  test("shows an error with retry when the portfolio and the list fail to load", async ({ page }) => {
    await open(page, { portfolioFail: true, savingsListFail: true });
    await expect(page.getByRole("alert").filter({ hasText: "El servidor tuvo un problema" })).toHaveCount(2);
    await expect(page.getByRole("button", { name: "Reintentar" })).toHaveCount(2);
  });

  test("changing the month lists that month's savings", async ({ page }) => {
    await open(page, { savings: [seeded(), seeded({ id: 2, date: "2025-01-10", description: "Ahorro de enero" })] });
    await expect(page.getByText("Ahorro de enero")).toHaveCount(0);
    await page.getByLabel("Mes").fill("2025-01");
    await expect(page.getByText("Ahorro de enero")).toBeVisible();
    await expect(page.getByRole("listitem").filter({ hasText: "Aportación VOO" })).toHaveCount(0);
  });

  for (const vp of [
    { name: "375", width: 375, height: 800 },
    { name: "768", width: 768, height: 900 },
    { name: "1440", width: 1440, height: 900 },
  ]) {
    test(`has no horizontal overflow at ${vp.name}px`, async ({ page }) => {
      await page.setViewportSize({ width: vp.width, height: vp.height });
      await open(page, {
        savings: [
          seeded({ description: "Una descripción extremadamente larga sin espacios ".repeat(3) + "x".repeat(60) }),
          emergency(),
        ],
        valuations: [vooValuation],
      });
      await expectNoHorizontalOverflow(page, `${vp.name}px list`);
      await savingForm(page).getByLabel("Monto").fill("-10");
      await savingForm(page).getByRole("button", { name: "Agregar ahorro" }).click();
      await expect(page.getByText("Ahorro guardado.")).toBeVisible();
      await expectNoHorizontalOverflow(page, `${vp.name}px after save`);
      await page.getByRole("button", { name: /^Eliminar/ }).first().click();
      await expectNoHorizontalOverflow(page, `${vp.name}px confirm`);
    });
  }
});
