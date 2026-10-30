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

/** Opens the page. The forms live in dialogs behind the header buttons, so `form` opens one of them. */
async function open(page: Page, opts: Parameters<typeof mockApi>[1] = {}, { form = "saving" }: { form?: "saving" | "transfer" | "valuation" | null } = {}) {
  const api = await mockApi(page, opts);
  await seedSession(page);
  await page.goto("/ahorros");
  await expect(page.getByRole("heading", { level: 1, name: "Ahorros" })).toBeVisible();
  if (form) await openForm(page, form);
  return api;
}

const HEADER_BUTTON = { saving: "Nuevo ahorro", transfer: "Transferir", valuation: "Actualizar valuación" } as const;

async function openForm(page: Page, form: keyof typeof HEADER_BUTTON) {
  await page.getByRole("button", { name: HEADER_BUTTON[form] }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
}

/** The per-instrument figures: a table row from md up, a card on a phone. */
const isCards = (page: Page) => (page.viewportSize()?.width ?? 0) < 768;
const instrument = (page: Page, name: RegExp) => (isCards(page) ? page.getByTestId("instrument-card").filter({ hasText: name }) : page.getByRole("row", { name }));
const headline = (page: Page) => page.getByRole("list", { name: "Resumen del portafolio" }).or(page.getByLabel("Resumen del portafolio"));

const savingForm = (page: Page) => page.getByRole("form", { name: /ahorro$/ });
const transferForm = (page: Page) => page.getByRole("form", { name: "Traspasar entre instrumentos" });
const valuationForm = (page: Page) => page.getByRole("form", { name: "Registrar valuación" });

test.describe("savings page", () => {
  test("no form sits on the page: the header buttons open each one in a dialog and the focus comes back", async ({ page }) => {
    await open(page, { savings: [seeded()] }, { form: null });
    for (const form of [savingForm(page), transferForm(page), valuationForm(page)]) await expect(form).toHaveCount(0);

    const header = page.locator("section > div").filter({ has: page.getByRole("heading", { level: 1 }) }).first();
    for (const name of ["Nuevo ahorro", "Transferir", "Actualizar valuación"]) await expect(header.getByRole("button", { name })).toBeVisible();

    const nuevo = page.getByRole("button", { name: "Nuevo ahorro" });
    await nuevo.click();
    await expect(page.getByRole("dialog", { name: "Nuevo ahorro" })).toBeVisible();
    await expect(savingForm(page).getByLabel("Instrumento")).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(nuevo).toBeFocused();

    await page.getByRole("button", { name: "Transferir" }).click();
    await expect(page.getByRole("dialog", { name: "Traspasar entre instrumentos" })).toBeVisible();
    await page.getByRole("button", { name: "Cancelar" }).click();
    await page.getByRole("button", { name: "Actualizar valuación" }).click();
    await expect(page.getByRole("dialog", { name: "Registrar valuación" })).toBeVisible();
    await page.getByRole("button", { name: "Cerrar", exact: true }).click();

    // A row opens the edit dialog in place, with the edit cue.
    await page.getByRole("button", { name: "Editar Aportación VOO" }).click();
    const dialog = page.getByRole("dialog", { name: "Editar ahorro" });
    await expect(dialog).toBeVisible();
    await expect(savingForm(page)).toBeVisible();
    await dialog.getByRole("button", { name: "Cancelar" }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });

  test("shows the portfolio with totals, subtotals and the emergency fund progress", async ({ page }) => {
    await open(page, { savings: [seeded(), emergency()], valuations: [vooValuation] }, { form: null });

    const voo = instrument(page, /VOO/);
    await expect(voo).toContainText("$10,000.00");
    await expect(voo).toContainText("$12,000.00");
    await expect(voo).toContainText("2026-09-30");
    await expect(voo).toContainText("$2,000.00");
    await expect(voo).toContainText("20.00%");
    await expect(voo).toContainText("100.00%");
    const cetes = instrument(page, /CETES/);
    await expect(cetes).toContainText("$20,000.00");
    await expect(cetes).toContainText("sin valuar");

    // The headline figures are always there; the totals row only exists in the table.
    await expect(headline(page)).toContainText("Valor del portafolio");
    await expect(headline(page)).toContainText("$12,000.00");
    await expect(headline(page)).toContainText("$30,000.00");
    if (!isCards(page)) {
      const total = page.getByRole("row", { name: /^Total/ });
      await expect(total).toContainText("$30,000.00");
      await expect(total).toContainText("$12,000.00");
    }

    await expect(page.getByRole("region", { name: "Subtotales por tipo" })).toContainText("ETF");
    await expect(page.getByRole("region", { name: "Subtotales por tipo" })).toContainText("Renta fija");
    await expect(page.getByRole("region", { name: "Subtotales por destino" })).toContainText("Inversión ETF");
    await expect(page.getByRole("region", { name: "Subtotales por destino" })).toContainText("$10,000.00");

    const card = page.getByRole("region", { name: "Fondo de emergencia" });
    await expect(card).toContainText("$20,000.00 de $60,000.00 (33.33%)");
    await expect(card.getByRole("progressbar", { name: "Avance del fondo de emergencia" })).toHaveAttribute("aria-valuenow", "33");
  });

  test("renders the instruments as cards on a phone and as a table from md up", async ({ page }) => {
    await open(page, { savings: [seeded(), emergency()], valuations: [vooValuation] }, { form: null });
    await expect(headline(page)).toContainText("$12,000.00");
    if (isCards(page)) {
      await expect(page.getByRole("table")).toHaveCount(0);
      const cards = page.getByTestId("instrument-card");
      await expect(cards).toHaveCount(2);
      await expect(cards.filter({ hasText: "VOO" })).toContainText("20.00%");
    } else {
      await expect(page.getByRole("table", { name: "Portafolio por instrumento" })).toBeVisible();
      await expect(page.getByTestId("instrument-card").first()).toBeHidden();
    }
    await expectNoHorizontalOverflow(page, "portfolio");
    // The page never needs the table's 44rem horizontal scroll on a phone.
    if (isCards(page)) {
      const scrollers = await page.evaluate(() => [...document.querySelectorAll("main *")].filter((el) => el.scrollWidth > el.clientWidth + 1 && getComputedStyle(el).overflowX === "auto").length);
      expect(scrollers).toBe(0);
    }
  });

  test("shows empty states when nothing is registered", async ({ page }) => {
    await open(page, {}, { form: null });
    await expect(page.getByText("No hay ahorros registrados en este periodo.")).toBeVisible();
    await expect(instrument(page, /CETES/)).toContainText("sin valuar");
    await expect(headline(page)).toContainText("$0.00");
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
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByRole("listitem").filter({ hasText: "Aportación quincena" })).toContainText("$1,500.50 MXN");
    await expect(instrument(page, /VOO/)).toContainText("$1,500.50");

    const body = api.requests.find((r) => r.path === "/savings")?.body as Record<string, unknown>;
    expect(body.amount).toBe("1500.50");
    expect(typeof body.amount).toBe("string");
    expect(body.category).toBe("Inversión ETF");
    expect(body.instrument).toBe("i1");
    // The dialog closes after the save and opens empty again.
    await expect(page.getByRole("form", { name: /ahorro$/ })).toHaveCount(0);
    await page.getByRole("button", { name: "Nuevo ahorro" }).click();
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
    await expect(instrument(page, /VOO/)).toContainText("$9,500.00");
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

  test("edits a saving in place, in a dialog", async ({ page }) => {
    const api = await open(page, { savings: [seeded()] }, { form: null });
    await page.getByRole("button", { name: "Editar Aportación VOO" }).click();

    await expect(page.getByRole("heading", { level: 2, name: "Editar ahorro" })).toBeVisible();
    await expect(savingForm(page).getByLabel("Monto")).toHaveValue("10000.00");
    await expect(savingForm(page).getByLabel("Instrumento")).toHaveValue("i1");
    await savingForm(page).getByLabel("Monto").fill("11000.75");
    await savingForm(page).getByRole("button", { name: "Guardar cambios" }).click();

    await expect(page.getByText("Ahorro guardado.")).toBeVisible();
    await expect(page.getByRole("form", { name: /ahorro$/ })).toHaveCount(0);
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
    const api = await open(page, { savings: [usd] }, { form: null });
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

  test("deletes a saving from its dialog after a confirmation", async ({ page }) => {
    page.on("dialog", (d) => {
      throw new Error(`unexpected native dialog: ${d.message()}`);
    });
    const api = await open(page, { savings: [seeded()] }, { form: null });

    // The row has no delete of its own.
    await expect(page.getByRole("button", { name: "Eliminar Aportación VOO" })).toHaveCount(0);
    await page.getByRole("button", { name: "Editar Aportación VOO" }).click();
    await page.getByRole("button", { name: "Eliminar Aportación VOO" }).click();
    await expect(page.getByText("¿Eliminar este ahorro?")).toBeVisible();
    await page.getByRole("group", { name: /Confirmar eliminación/ }).getByRole("button", { name: "Cancelar" }).click();
    await expect(page.getByRole("dialog")).toBeVisible();
    expect(api.writes).toHaveLength(0);

    await page.getByRole("button", { name: "Eliminar Aportación VOO" }).click();
    await page.getByRole("button", { name: "Sí, eliminar" }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByText("No hay ahorros registrados en este periodo.")).toBeVisible();
    expect(api.writes.some((w) => w.method === "DELETE" && w.path === "/savings/1")).toBe(true);
    await expect(instrument(page, /VOO/)).toContainText("$0.00");
  });

  test("transfers between instruments and lists both legs", async ({ page }) => {
    const api = await open(page, { savings: [seeded()] }, { form: "transfer" });
    await transferForm(page).getByLabel("Desde").selectOption("i1");
    await transferForm(page).getByLabel("Hacia").selectOption("i2");
    await transferForm(page).getByLabel("Monto a traspasar").fill("2500.25");
    await transferForm(page).getByRole("button", { name: "Traspasar" }).click();

    await expect(page.getByText("Traspaso registrado.")).toBeVisible();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    const legs = page.getByRole("listitem").filter({ hasText: "Traspaso i1 -> i2" });
    await expect(legs).toHaveCount(2);
    await expect(legs.filter({ hasText: "-$2,500.25" })).toHaveCount(1);
    await expect(instrument(page, /VOO/)).toContainText("$7,499.75");
    await expect(instrument(page, /CETES/)).toContainText("$2,500.25");
    const body = api.requests.find((r) => r.path === "/savings/transfers")?.body as Record<string, unknown>;
    expect(body).toEqual({ from: "i1", to: "i2", amount: "2500.25" });
    // It opens empty again.
    await openForm(page, "transfer");
    await expect(transferForm(page).getByLabel("Monto a traspasar")).toHaveValue("");
  });

  test("transfer legs are badged, cannot be edited and are deleted together from a dialog", async ({ page }) => {
    const api = await open(page, { savings: [seeded()] }, { form: "transfer" });
    await transferForm(page).getByLabel("Desde").selectOption("i1");
    await transferForm(page).getByLabel("Hacia").selectOption("i2");
    await transferForm(page).getByLabel("Monto a traspasar").fill("300");
    await transferForm(page).getByRole("button", { name: "Traspasar" }).click();

    const legs = page.getByRole("listitem").filter({ hasText: "Traspaso i1 -> i2" });
    await expect(legs).toHaveCount(2);
    for (const leg of await legs.all()) {
      await expect(leg.getByText("Traspaso", { exact: true })).toBeVisible();
      await expect(leg.getByRole("button", { name: /^Editar/ })).toHaveCount(0);
    }
    // The plain saving keeps its badge-free row and its edit button.
    const plain = page.getByRole("listitem").filter({ hasText: "Aportación VOO" });
    await expect(plain.getByText("Traspaso", { exact: true })).toHaveCount(0);
    await expect(plain.getByRole("button", { name: /^Editar/ })).toBeVisible();

    // A leg opens a dialog with no form: only the confirmed deletion of the whole transfer.
    await legs.first().getByRole("button").click();
    await expect(page.getByRole("form")).toHaveCount(0);
    await page.getByRole("button", { name: /^Eliminar/ }).click();
    await expect(page.getByText("Se eliminarán ambas partes")).toBeVisible();
    await page.getByRole("button", { name: "Sí, eliminar" }).click();

    await expect(page.getByText("Traspaso i1 -> i2")).toHaveCount(0);
    await expect(plain).toBeVisible();
    expect(api.savings).toHaveLength(1);
    expect(api.writes.filter((w) => w.method === "DELETE")).toHaveLength(1);
    await expect(instrument(page, /VOO/)).toContainText("$10,000.00");
    await expect(instrument(page, /CETES/)).toContainText("$0.00");
  });

  test("validates the transfer before sending it", async ({ page }) => {
    const api = await open(page, {}, { form: "transfer" });
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
    const api = await open(page, { savings: [seeded()], valuations: [vooValuation] }, { form: "valuation" });
    await expect(page.getByRole("button", { name: /(Editar|Eliminar).*valuación/i })).toHaveCount(0);

    await valuationForm(page).getByLabel("Instrumento a valuar").selectOption("i1");
    await valuationForm(page).getByLabel("Valor actual (MXN)").fill("15000.50");
    await valuationForm(page).getByLabel("Fecha de la valuación").fill("2026-10-05");
    await valuationForm(page).getByLabel("Nota (opcional)").fill("Cierre de octubre");
    await valuationForm(page).getByRole("button", { name: "Registrar valuación" }).click();

    await expect(page.getByText("Valuación registrada.")).toBeVisible();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    const voo = instrument(page, /VOO/);
    await expect(voo).toContainText("$15,000.50");
    await expect(voo).toContainText("2026-10-05");
    await expect(voo).toContainText("$5,000.50");
    const body = api.requests.find((r) => r.path === "/savings/valuations")?.body as Record<string, unknown>;
    expect(body).toEqual({ instrument: "i1", value_mxn: "15000.50", date: "2026-10-05", note: "Cierre de octubre" });
    expect(api.valuations).toHaveLength(2);
    await expect(page.getByRole("button", { name: /(Editar|Eliminar).*valuación/i })).toHaveCount(0);
  });

  test("blocks an invalid valuation and sends nothing", async ({ page }) => {
    const api = await open(page, {}, { form: "valuation" });
    await valuationForm(page).getByLabel("Valor actual (MXN)").fill("-3");
    await valuationForm(page).getByRole("button", { name: "Registrar valuación" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "Elige un instrumento." })).toBeVisible();
    await expect(page.getByRole("alert").filter({ hasText: "valor mayor a cero" })).toBeVisible();
    expect(api.requests.filter((r) => r.path === "/savings/valuations")).toHaveLength(0);
  });

  test("blocks valuations with sub-cent precision or above the storage limit", async ({ page }) => {
    const api = await open(page, {}, { form: "valuation" });
    await valuationForm(page).getByLabel("Instrumento a valuar").selectOption("i1");
    for (const bad of ["0.004", "100.555", "1000000000000", "1e12"]) {
      await valuationForm(page).getByLabel("Valor actual (MXN)").fill(bad);
      await valuationForm(page).getByRole("button", { name: "Registrar valuación" }).click();
      await expect(page.getByRole("alert").filter({ hasText: "máximo 2 decimales" })).toBeVisible();
    }
    expect(api.requests.filter((r) => r.path === "/savings/valuations")).toHaveLength(0);

    await valuationForm(page).getByLabel("Valor actual (MXN)").fill("999999999999.99");
    await valuationForm(page).getByRole("button", { name: "Registrar valuación" }).click();
    await expect(page.getByText("Valuación registrada.")).toBeVisible();
  });

  test("an untouched valuation date is resolved at submit time, not at page load", async ({ page }) => {
    const api = await open(page, {}, { form: "valuation" });
    // The page stays open past midnight: the default must follow the clock.
    await page.clock.setFixedTime(new Date(2031, 0, 2, 10, 0, 0));
    await valuationForm(page).getByLabel("Instrumento a valuar").selectOption("i1");
    await valuationForm(page).getByLabel("Valor actual (MXN)").fill("100");
    await valuationForm(page).getByRole("button", { name: "Registrar valuación" }).click();
    await expect(page.getByText("Valuación registrada.")).toBeVisible();
    expect((api.requests.find((r) => r.path === "/savings/valuations")?.body as { date: string }).date).toBe("2031-01-02");

    // A date the user picked is kept.
    await openForm(page, "valuation");
    await valuationForm(page).getByLabel("Instrumento a valuar").selectOption("i1");
    await valuationForm(page).getByLabel("Fecha de la valuación").fill("2030-12-31");
    await page.clock.setFixedTime(new Date(2031, 0, 3, 10, 0, 0));
    await valuationForm(page).getByLabel("Valor actual (MXN)").fill("200");
    await valuationForm(page).getByRole("button", { name: "Registrar valuación" }).click();
    await expect.poll(() => api.requests.filter((r) => r.path === "/savings/valuations").length).toBe(2);
    expect((api.requests.filter((r) => r.path === "/savings/valuations")[1].body as { date: string }).date).toBe("2030-12-31");
  });

  test("shows the backend message when the server rejects a saving", async ({ page }) => {
    await open(page, { savingsFail: "instrument \"zzz\" does not exist" });
    await savingForm(page).getByLabel("Monto").fill("10");
    await savingForm(page).getByRole("button", { name: "Agregar ahorro" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "instrument \"zzz\" does not exist" })).toBeVisible();
  });

  test("shows the backend message when the server rejects a valuation", async ({ page }) => {
    await open(page, { savingsFail: "value must be greater than zero" }, { form: "valuation" });
    await valuationForm(page).getByLabel("Instrumento a valuar").selectOption("i1");
    await valuationForm(page).getByLabel("Valor actual (MXN)").fill("10");
    await valuationForm(page).getByRole("button", { name: "Registrar valuación" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "value must be greater than zero" })).toBeVisible();
  });

  test("shows an error with retry when the portfolio and the list fail to load", async ({ page }) => {
    await open(page, { portfolioFail: true, savingsListFail: true }, { form: null });
    await expect(page.getByRole("alert").filter({ hasText: "El servidor tuvo un problema" })).toHaveCount(2);
    await expect(page.getByRole("button", { name: "Reintentar" })).toHaveCount(2);
  });

  test("changing the month lists that month's savings", async ({ page }) => {
    await open(page, { savings: [seeded(), seeded({ id: 2, date: "2025-01-10", description: "Ahorro de enero" })] }, { form: null });
    await expect(page.getByText("Ahorro de enero")).toHaveCount(0);
    await page.getByLabel("Mes").fill("2025-01");
    await expect(page.getByText("Ahorro de enero")).toBeVisible();
    await expect(page.getByRole("listitem").filter({ hasText: "Aportación VOO" })).toHaveCount(0);
  });

  test("tells the user when the list is cut at the 200 most recent savings", async ({ page }) => {
    const many = Array.from({ length: 205 }, (_, i) => seeded({ id: i + 1, description: `Aporte ${i + 1}` }));
    await open(page, { savings: many }, { form: null });
    await expect(page.getByText("Mostrando los 200 más recientes")).toBeVisible();
    await expect(page.getByRole("button", { name: /^Editar Aporte/ })).toHaveCount(200);
    // The newest 200 are shown, the oldest 5 are not.
    await expect(page.getByText("Aporte 205", { exact: true })).toBeVisible();
    await expect(page.getByText("Aporte 5", { exact: true })).toHaveCount(0);
  });

  test("shows no cut-off notice below the limit", async ({ page }) => {
    const few = Array.from({ length: 199 }, (_, i) => seeded({ id: i + 1, description: `Aporte ${i + 1}` }));
    await open(page, { savings: few }, { form: null });
    await expect(page.getByRole("button", { name: /^Editar Aporte/ })).toHaveCount(199);
    await expect(page.getByText("Mostrando los 200 más recientes")).toHaveCount(0);
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
      await expectNoHorizontalOverflow(page, `${vp.name}px dialog`);
      await savingForm(page).getByLabel("Monto").fill("-10");
      await savingForm(page).getByRole("button", { name: "Agregar ahorro" }).click();
      await expect(page.getByText("Ahorro guardado.")).toBeVisible();
      await expectNoHorizontalOverflow(page, `${vp.name}px after save`);
      await page.getByRole("button", { name: /^Editar/ }).first().click();
      await page.getByRole("button", { name: /^Eliminar/ }).click();
      await expectNoHorizontalOverflow(page, `${vp.name}px confirm`);
    });
  }
});
