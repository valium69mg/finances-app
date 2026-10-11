import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockExpense, type MockPersonalMovement } from "./helpers";

/** Her interactive Panel (phase 12 step 6): the month travels in ?mes=, every link goes to a page her role can open. */

const pad = (n: number) => String(n).padStart(2, "0");
const now = new Date();
const thisMonth = `${now.getFullYear()}-${pad(now.getMonth() + 1)}`;
const today = `${thisMonth}-${pad(now.getDate())}`;
/** A fixed past month (calendar cycle in the mock) so the data never depends on the day the suite runs. */
const AUG = "2026-08";

const row = (id: number, kind: MockPersonalMovement["kind"], category: string, amount: string, date: string, description = ""): MockPersonalMovement => ({
  id,
  kind,
  category,
  amount,
  description: description || `${category} ${id}`,
  date,
});

const HERS: MockPersonalMovement[] = [
  row(1, "Ingreso", "Trabajo", "10000.00", "2026-08-02", "Quincena"),
  row(2, "Gasto", "Ropa", "300.50", "2026-08-03", "Blusa"),
  row(3, "Gasto", "Salud", "900.00", "2026-08-04", "Consulta"),
  row(4, "Gasto", "Ropa", "100.00", "2026-08-05", "Falda"),
  row(5, "Ahorro", "Ahorro", "2000.00", "2026-08-06", "Aportación"),
  row(6, "Ahorro", "Ahorro", "-500.00", "2026-08-07", "Retiro"),
  // Outside August: must not count.
  row(7, "Gasto", "Ropa", "9999.00", "2026-07-31", "Julio"),
  row(8, "Ingreso", "Trabajo", "777.00", "2026-09-01", "Septiembre"),
  // The current cycle.
  row(9, "Gasto", "Belleza", "250.00", today, "Corte"),
];

const FAMILY: MockExpense = {
  id: 1,
  date: today,
  description: "Renta",
  category: "Renta",
  payment_method: "Transferencia",
  currency: "MXN",
  amount: "13000.00",
  exchange_rate: null,
  amount_mxn: "13000.00",
};

async function open(page: Page, path: string, personal: MockPersonalMovement[] = HERS) {
  const api = await mockApi(page, { role: "household", personal, expenses: [FAMILY] });
  await seedSession(page, "household");
  await page.goto(path);
  return api;
}

const mainLinks = (page: Page) => page.locator("main").getByRole("link");
const monthOf = (page: Page) => new URL(page.url()).searchParams.get("mes");
const categoryOf = (page: Page) => new URL(page.url()).searchParams.get("categoria");

test.describe("her Panel: summary", () => {
  test("shows her cycle totals, the available money and the spending by category", async ({ page }) => {
    const api = await open(page, `/?mes=${AUG}`);
    await expect(page.getByRole("heading", { level: 2, name: "Mi resumen" })).toBeVisible();
    const cards = page.locator("dl");
    await expect(cards.locator("div", { has: page.getByText("Ingresos", { exact: true }) }).last()).toContainText("$10,000.00");
    await expect(cards.locator("div", { has: page.getByText("Gastos", { exact: true }) }).last()).toContainText("$1,300.50");
    await expect(cards.locator("div", { has: page.getByText("Ahorros", { exact: true }) }).last()).toContainText("$1,500.00");
    // 10,000.00 - 1,300.50 - 1,500.00; a withdrawal came off the savings.
    await expect(cards.locator("div", { has: page.getByText("Disponible", { exact: true }) }).last()).toContainText("$7,199.50");

    const spent = page.getByTestId("category-spend");
    await expect(spent).toHaveCount(2);
    await expect(spent.first()).toContainText("Salud");
    await expect(spent.first()).toContainText("$900.00");
    await expect(spent.first()).toContainText("69%");
    await expect(spent.last()).toContainText("Ropa");
    await expect(spent.last()).toContainText("$400.50");
    await expect(spent.last()).toContainText("31%");

    // The range hint comes from her own payload, and the family budget stays below.
    await expect(page.getByLabel("Mes")).toHaveValue(AUG);
    await expect(page.getByText("1 ago – 31 ago")).toBeVisible();
    await expect(page.getByRole("heading", { level: 2, name: "Presupuesto por categoría" })).toBeVisible();
    expect(api.personalDashboardMonths).toEqual([AUG]);
  });

  test("opens on the current cycle without a month and says when nothing was spent", async ({ page }) => {
    const api = await open(page, "/", []);
    await expect(page.getByLabel("Mes")).toHaveValue(thisMonth);
    await expect(page.getByText("Aún no hay gastos en este periodo.")).toBeVisible();
    expect(api.personalDashboardMonths).toEqual([""]);
    // The links still carry the month the figures are for.
    await expect(page.getByRole("link", { name: "Ingresos", exact: true })).toHaveAttribute("href", `/mis-ingresos?mes=${thisMonth}`);
  });

  test("changing the month updates the URL and both sections", async ({ page }) => {
    const api = await open(page, "/");
    await page.getByLabel("Mes").fill(AUG);
    await expect(page.getByTestId("category-spend")).toHaveCount(2);
    expect(monthOf(page)).toBe(AUG);
    expect(api.personalDashboardMonths).toContain(AUG);
    expect(api.dashboardMonths).toContain(AUG);
  });

  test("ignores a malformed month in the URL", async ({ page }) => {
    const api = await open(page, "/?mes=octubre");
    await expect(page.getByLabel("Mes")).toHaveValue(thisMonth);
    expect(api.personalDashboardMonths).toEqual([""]);
  });
});

test.describe("her Panel: links", () => {
  test("Ingresos, Gastos and Ahorros open their page on the Panel's month", async ({ page }) => {
    await open(page, `/?mes=${AUG}`);
    const targets: [string, string, string, string][] = [
      ["Ingresos", "/mis-ingresos", "Mis ingresos", "$10,000.00 MXN"],
      ["Gastos", "/mis-gastos", "Mis gastos", "$1,300.50 MXN"],
      ["Ahorros", "/mis-ahorros", "Mis ahorros", "$1,500.00 MXN"],
    ];
    for (const [card, path, title, total] of targets) {
      await page.goto(`/?mes=${AUG}`);
      await page.getByRole("link", { name: card, exact: true }).click();
      await expect(page).toHaveURL(new RegExp(`${path}\\?mes=${AUG}$`));
      await expect(page.getByRole("heading", { level: 1, name: title })).toBeVisible();
      await expect(page.getByLabel("Mes")).toHaveValue(AUG);
      await expect(page.getByTestId("personal-total")).toHaveText(total);
    }
  });

  test("a category row opens Mis gastos filtered by that category", async ({ page }) => {
    await open(page, `/?mes=${AUG}`);
    await page.getByRole("link", { name: "Salud", exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/mis-gastos\\?mes=${AUG}&categoria=\\d+$`));
    await expect(page.getByRole("heading", { level: 1, name: "Mis gastos" })).toBeVisible();
    await expect(page.getByText("Categoría: Salud")).toBeVisible();
    await expect(page.getByTestId("personal-total")).toHaveText("$900.00 MXN");
    await expect(page.getByRole("listitem").filter({ hasText: "Consulta" })).toBeVisible();
    await expect(page.getByRole("listitem").filter({ hasText: "Blusa" })).toHaveCount(0);
  });

  test("a family budget row goes to Peticiones", async ({ page }) => {
    await open(page, `/?mes=${thisMonth}`);
    await page.getByRole("link", { name: "Renta", exact: true }).click();
    await expect(page).toHaveURL(/\/peticiones$/);
    await expect(page.getByRole("heading", { level: 1, name: "Peticiones" })).toBeVisible();
  });

  test("Disponible is informative: not a link, and it does not look like one", async ({ page }) => {
    await open(page, `/?mes=${AUG}`);
    await expect(page.getByRole("link", { name: "Disponible", exact: true })).toHaveCount(0);
    const available = page.locator("dl > div", { has: page.getByText("Disponible", { exact: true }) });
    await expect(available).toBeVisible();
    await expect(available.locator("svg.lucide-chevron-right")).toHaveCount(0);
    await expect(available).not.toHaveClass(/\bgroup\b/);
    // The linked cards do carry the chevron.
    await expect(page.locator("dl > div", { has: page.getByRole("link", { name: "Ingresos", exact: true }) }).locator("svg.lucide-chevron-right")).toHaveCount(1);
  });

  test("the links are keyboard accessible and show focus", async ({ page }) => {
    await open(page, `/?mes=${AUG}`);
    const link = page.getByRole("link", { name: "Gastos", exact: true });
    await link.focus();
    await expect(link).toBeFocused();
    // The focus ring is drawn on the layer stretched over the whole card.
    const ring = await link.evaluate((el) => getComputedStyle(el, "::after").boxShadow);
    expect(ring).not.toBe("none");
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(new RegExp(`/mis-gastos\\?mes=${AUG}$`));
  });

  test("every link on the Panel opens a page her role can reach", async ({ page }) => {
    await open(page, `/?mes=${AUG}`);
    await expect(page.getByTestId("category-spend")).toHaveCount(2);
    await expect(page.getByRole("link", { name: "Renta", exact: true })).toBeVisible();
    const hrefs = await mainLinks(page).evaluateAll((links) => links.map((a) => a.getAttribute("href") ?? ""));
    expect(hrefs.length).toBeGreaterThanOrEqual(6);
    for (const href of hrefs) expect(href, href).toMatch(/^\/(peticiones|mis-ingresos|mis-gastos|mis-ahorros)(\?|$)/);
  });
});

test.describe("her pages: ?mes", () => {
  for (const [path, title] of [
    ["/mis-ingresos", "Mis ingresos"],
    ["/mis-gastos", "Mis gastos"],
    ["/mis-ahorros", "Mis ahorros"],
  ] as const) {
    test(`${title} opens on the month of the URL, shows its range and keeps the picked month in the URL`, async ({ page }) => {
      const api = await open(page, `${path}?mes=${AUG}`);
      await expect(page.getByRole("heading", { level: 1, name: title })).toBeVisible();
      await expect(page.getByLabel("Mes")).toHaveValue(AUG);
      await expect(page.getByText("1 ago – 31 ago")).toBeVisible();
      expect(api.personalMonths[0]).toBe(AUG);
      expect(api.personalPeriodMonths[0]).toBe(AUG);

      await page.getByLabel("Mes").fill("2026-09");
      await expect(page.getByText("1 sep – 30 sep")).toBeVisible();
      expect(monthOf(page)).toBe("2026-09");
      await page.reload();
      await expect(page.getByLabel("Mes")).toHaveValue("2026-09");
    });
  }

  test("a malformed month is ignored and the current cycle is shown", async ({ page }) => {
    const api = await open(page, "/mis-ingresos?mes=2026-13");
    await expect(page.getByLabel("Mes")).toHaveValue(thisMonth);
    expect(api.personalMonths[0]).toBe("");
  });
});

test.describe("Mis gastos: category filter", () => {
  test("applies the filter from the URL, shows a clearable chip and keeps the month", async ({ page }) => {
    const api = await open(page, `/mis-gastos?mes=${AUG}`);
    await expect(page.getByRole("listitem").filter({ hasText: "Consulta" })).toBeVisible();
    await expect(page.getByText(/^Categoría: /)).toHaveCount(0);

    // Apply it through the Panel link the way the user does, then clear it on the page.
    await page.goto(`/?mes=${AUG}`);
    await page.getByRole("link", { name: "Ropa", exact: true }).click();
    const chip = page.getByText("Categoría: Ropa");
    await expect(chip).toBeVisible();
    expect(categoryOf(page)).not.toBeNull();
    await expect(page.getByTestId("personal-total")).toHaveText("$400.50 MXN");
    await expect(page.getByRole("listitem").filter({ hasText: "Consulta" })).toHaveCount(0);
    expect(api.personalMovementQueries.at(-1)).toMatch(/category_id=\d+/);

    await page.getByRole("button", { name: "Quitar filtro de categoría Ropa" }).click();
    await expect(chip).toHaveCount(0);
    expect(categoryOf(page)).toBeNull();
    expect(monthOf(page)).toBe(AUG);
    await expect(page.getByTestId("personal-total")).toHaveText("$1,300.50 MXN");
    await expect(page.getByRole("listitem").filter({ hasText: "Consulta" })).toBeVisible();
  });

  test("keeps the filter when the month changes", async ({ page }) => {
    await open(page, `/mis-gastos?mes=${AUG}`);
    await page.goto(`/?mes=${AUG}`);
    await page.getByRole("link", { name: "Ropa", exact: true }).click();
    await page.getByLabel("Mes").fill("2026-07");
    await expect(page.getByText("Categoría: Ropa")).toBeVisible();
    expect(categoryOf(page)).not.toBeNull();
    await expect(page.getByTestId("personal-total")).toHaveText("$9,999.00 MXN");
  });

  test("explains an empty filtered list", async ({ page }) => {
    await open(page, `/mis-gastos?mes=2026-06`);
    await page.goto(`/?mes=${thisMonth}`);
    await page.getByRole("link", { name: "Belleza", exact: true }).click();
    await page.getByLabel("Mes").fill("2026-06");
    await expect(page.getByText("No hay movimientos de la categoría Belleza en este periodo.")).toBeVisible();
  });

  test("ignores a category that is not a valid id or not one of hers", async ({ page }) => {
    for (const bad of ["abc", "0", "-2", "9999"]) {
      const api = await open(page, `/mis-gastos?mes=${AUG}&categoria=${bad}`);
      await expect(page.getByRole("heading", { level: 1, name: "Mis gastos" })).toBeVisible();
      await expect(page.getByTestId("personal-total")).toHaveText("$1,300.50 MXN");
      await expect(page.getByText(/^Categoría: /)).toHaveCount(0);
      expect(api.personalMovementQueries.every((q) => !q.includes("category_id")), bad).toBe(true);
    }
  });
});

test.describe("owner Panel and her Panel stay apart", () => {
  test("the owner's Panel links its cards with the month and shows none of her summary", async ({ page }) => {
    await mockApi(page, { expenses: [FAMILY] });
    await seedSession(page, "owner");
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 2, name: "Resumen del mes" })).toBeVisible();
    await expect(page.locator("dl").first().getByRole("link")).toHaveCount(3);
    await expect(page.getByRole("link", { name: "Gastos", exact: true }).last()).toHaveAttribute("href", new RegExp(`^/gastos\\?mes=\\d{4}-\\d{2}$`));
    await expect(page.getByRole("heading", { level: 2, name: "Presupuesto por categoría" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Mi resumen" })).toHaveCount(0);
    await expect(page.getByTestId("category-spend")).toHaveCount(0);
  });
});
