import { expect, test, type Page } from "@playwright/test";
import { OWNER_USER } from "./usersMock";
import { expectSection, switchSection } from "./settingsNav";
import { mockApi, seedSession, type MockPersonalFuture, type MockPersonalMovement, type MockPersonalWish, type MockUser } from "./helpers";
import { isDesktop, moduleLabels } from "./nav";

const pad = (n: number) => String(n).padStart(2, "0");
const today = () => {
  const d = new Date();
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
};

const ANA: MockUser = { id: "u-ana", email: "ana@example.com", role: "household", active: true, verified: true };
const LUZ: MockUser = { id: "u-luz", email: "luz@example.com", role: "household", active: true, verified: true };

const MOVEMENTS: MockPersonalMovement[] = [
  { id: 1, kind: "Ingreso", category: "Trabajo", amount: "9000.00", description: "Quincena", date: today() },
  { id: 2, kind: "Gasto", category: "Ropa", amount: "350.00", description: "Blusa", date: today() },
  { id: 3, kind: "Gasto", category: "Salud", amount: "800.50", description: "Consulta", date: today() },
  { id: 4, kind: "Ahorro", category: "Inversión", amount: "1000.00", description: "Aporte", date: today() },
];
const FUTURE: MockPersonalFuture[] = [
  { id: 1, name: "Laptop", target_amount: "8000.00", due_date: "2027-06-20", saved: "2000.00" },
  { id: 2, name: "Teléfono", target_amount: "5000.00", due_date: "2026-09-01", status: "paid", paid_at: "2026-09-02", amount_paid: "4800.00" },
];
const WISHES: MockPersonalWish[] = [
  { id: 1, name: "Bolsa", price: "499.90", category: "Ropa", priority: "alta" },
  { id: 2, name: "Zapatos", price: "900.00", category: "Ropa", future_expense_id: 1 },
];

async function open(page: Page, path: string, users: MockUser[] = [OWNER_USER, ANA]) {
  const api = await mockApi(page, { role: "owner", users, personal: MOVEMENTS, personalFuture: FUTURE, personalWishlist: WISHES });
  await seedSession(page, "owner");
  await page.goto(path);
  return api;
}

/** Nothing that writes is on the page: no form, dialog or control named after a write. */
async function expectReadOnly(page: Page) {
  await expect(page.locator("form")).toHaveCount(0);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const names = await page.getByRole("main").getByRole("button").evaluateAll((els) => els.map((e) => e.getAttribute("aria-label") ?? e.textContent ?? ""));
  for (const name of names) expect(name, `button "${name}"`).not.toMatch(/nuevo|nueva|editar|eliminar|convertir|agregar|asignar|marcar pagado|administrar|guardar/i);
}

test.describe("owner: Hogar", () => {
  test("is listed for the owner, right after Peticiones", async ({ page }) => {
    await open(page, "/");
    const labels = await moduleLabels(page);
    if (isDesktop(page)) expect(labels.slice(0, 3)).toEqual(["Panel", "Peticiones", "Hogar"]);
    else expect(labels).toEqual(expect.arrayContaining(["Panel", "Peticiones", "Hogar"]));
  });

  test("selects the only household user by itself and reads her summary with user_id", async ({ page }) => {
    const api = await open(page, "/hogar");
    await expect(page.getByRole("heading", { level: 1, name: "Finanzas de ana@example.com" })).toBeVisible();
    await expect(page.getByRole("combobox", { name: "Familiar" })).toHaveCount(0);
    await expect(page.getByText("$9,000.00").first()).toBeVisible();
    await expect(page.getByTestId("category-spend")).toHaveCount(2);
    expect(api.personalOwnerReads.length).toBeGreaterThan(0);
    for (const read of api.personalOwnerReads) expect(read, read).toContain("user_id=u-ana");
    await expectReadOnly(page);
  });

  test("asks which person with several users and keeps the choice, the section and the month in the URL", async ({ page }) => {
    const api = await open(page, "/hogar", [OWNER_USER, ANA, LUZ]);
    await expect(page.getByRole("heading", { level: 1, name: "Hogar" })).toBeVisible();
    await expect(page.getByText("Elige a un familiar para ver sus finanzas.")).toBeVisible();
    expect(api.personalOwnerReads).toHaveLength(0);

    await page.getByRole("combobox", { name: "Familiar" }).selectOption({ label: "luz@example.com" });
    await expect(page.getByRole("heading", { level: 1, name: "Finanzas de luz@example.com" })).toBeVisible();
    await expect(page).toHaveURL(/\/hogar\?usuario=u-luz$/);

    await switchSection(page, "Gastos");
    await expect(page).toHaveURL(/usuario=u-luz&seccion=gastos/);
    await page.getByLabel("Mes").fill("2026-09");
    await expect(page).toHaveURL(/seccion=gastos&mes=2026-09/);
    for (const read of api.personalOwnerReads) expect(read, read).toContain("user_id=u-luz");

    await page.reload();
    await expect(page.getByRole("heading", { level: 1, name: "Finanzas de luz@example.com" })).toBeVisible();
    await expect(page.getByLabel("Mes")).toHaveValue("2026-09");
  });

  test("shows an empty note that links to Configuración when there is no active household user", async ({ page }) => {
    await open(page, "/hogar", [OWNER_USER, { ...ANA, active: false }]);
    await expect(page.getByText("Aún no hay familiares con cuenta activa.")).toBeVisible();
    await page.getByRole("link", { name: /Configuración > Usuarios/ }).click();
    await expect(page).toHaveURL(/\/configuracion$/);
  });

  test("Resumen links stay inside Hogar: a category row opens Gastos filtered by it", async ({ page }) => {
    const api = await open(page, "/hogar");
    await page.getByTestId("category-spend").filter({ hasText: "Salud" }).getByRole("link").click();
    await expect(page).toHaveURL(/\/hogar\?usuario=u-ana&seccion=gastos&mes=\d{4}-\d{2}&categoria=\d+/);
    await expect(page.getByText("Categoría: Salud")).toBeVisible();
    await expect(page.getByText("Consulta", { exact: true })).toBeVisible();
    await expect(page.getByText("Blusa", { exact: true })).toHaveCount(0);
    expect(api.personalOwnerReads.some((r) => r.startsWith("/personal/movements?") && r.includes("category_id=") && r.includes("user_id=u-ana"))).toBe(true);
    await expectReadOnly(page);
    await page.getByRole("button", { name: "Quitar filtro de categoría Salud" }).click();
    await expect(page.getByText("Blusa", { exact: true })).toBeVisible();
  });

  test("Ingresos and Ahorros list her rows read-only", async ({ page }) => {
    await open(page, "/hogar?usuario=u-ana&seccion=ingresos");
    await expect(page.getByText("Quincena")).toBeVisible();
    await expect(page.getByTestId("personal-total")).toHaveText("$9,000.00 MXN");
    // The rows are plain: nothing in them opens an editor.
    await expect(page.getByRole("listitem").filter({ hasText: "Quincena" }).getByRole("button")).toHaveCount(0);
    await expectReadOnly(page);
    await switchSection(page, "Ahorros");
    await expect(page.getByText("Aporte")).toBeVisible();
    await expectReadOnly(page);
  });

  test("Gastos futuros shows the plan and the paid history without actions", async ({ page }) => {
    const api = await open(page, "/hogar?usuario=u-ana&seccion=gastos-futuros");
    await expect(page.getByText("Saldo libre")).toBeVisible();
    const active = page.getByRole("list", { name: "Gastos futuros activos" });
    await expect(active.getByText("Laptop")).toBeVisible();
    await expect(active.getByRole("progressbar", { name: "Ahorro para Laptop" })).toBeVisible();
    await expect(page.getByRole("list", { name: "Gastos futuros pagados" }).getByText("Teléfono")).toBeVisible();
    expect(api.personalOwnerReads).toContain("/personal/future-expenses?user_id=u-ana");
    await expectReadOnly(page);
  });

  test("Lista de deseos shows active and converted items with what each became", async ({ page }) => {
    const api = await open(page, "/hogar?usuario=u-ana&seccion=deseos");
    await expect(page.getByRole("list", { name: "Deseos activos" }).getByText("Bolsa")).toBeVisible();
    const converted = page.getByRole("list", { name: "Deseos convertidos" });
    await expect(converted.getByText("Zapatos")).toBeVisible();
    await expect(converted.getByText("Se convirtió en un gasto futuro.")).toBeVisible();
    await converted.getByRole("link", { name: "Ver en Gastos futuros" }).click();
    await expect(page).toHaveURL(/\/hogar\?usuario=u-ana&seccion=gastos-futuros/);
    expect(api.personalOwnerReads).toContain("/personal/wishlist?user_id=u-ana");
    await expectReadOnly(page);
  });

  test("never sends a write on her behalf", async ({ page }) => {
    const api = await open(page, "/hogar");
    for (const section of ["Ingresos", "Gastos", "Ahorros", "Gastos futuros", "Lista de deseos"]) {
      await switchSection(page, section);
      await expectSection(page, section);
    }
    expect(api.personalWrites).toHaveLength(0);
    expect(api.personalFutureWrites).toHaveLength(0);
    expect(api.personalWishWrites).toHaveLength(0);
  });
});

test.describe("household session", () => {
  test("does not list Hogar, redirects /hogar, and her own pages still read her rows without user_id", async ({ page }) => {
    const api = await mockApi(page, { role: "household", personal: MOVEMENTS });
    await seedSession(page, "household");
    await page.goto("/hogar");
    await expect(page).toHaveURL(/\/$/);
    expect(await moduleLabels(page)).not.toContain("Hogar");
    await page.goto("/mis-gastos");
    await expect(page.getByRole("heading", { level: 1, name: "Mis gastos" })).toBeVisible();
    await expect(page.getByText("Blusa", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Nuevo gasto" })).toBeVisible();
    expect(api.personalOwnerReads).toHaveLength(0);
  });
});
