import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockPersonalMovement, type MockRole } from "./helpers";
import { dock, isDesktop, moreButton, moreNav, moreSheet, openMore, quickExpenseButton } from "./nav";

const pad = (n: number) => String(n).padStart(2, "0");
const today = () => {
  const d = new Date();
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
};

const OWNER_MORE = ["Peticiones", "Hogar", "Ahorros", "Gastos futuros", "Facturas", "Declaración", "Declaraciones presentadas", "Cierre de mes", "Pagos recurrentes", "Sistema", "Configuración"];
const HOUSEHOLD_MORE = ["Peticiones", "Mis ahorros", "Mis gastos futuros", "Lista de deseos"];

async function open(page: Page, role: MockRole, path = "/", opts: Parameters<typeof mockApi>[1] = {}) {
  const api = await mockApi(page, { role, ...opts });
  await seedSession(page, role);
  await page.goto(path);
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  return api;
}

test.describe("desktop", () => {
  test("keeps the sidebar and has no dock", async ({ page }) => {
    test.skip(!isDesktop(page), "Sidebar layout starts at 1024px");
    await open(page, "owner");
    await expect(page.getByRole("navigation", { name: "Módulos" }).getByRole("link")).toHaveCount(14);
    await expect(dock(page)).toBeHidden();
    await expect(quickExpenseButton(page)).toBeHidden();
    await expect(moreButton(page)).toBeHidden();
  });
});

test.describe("bottom dock", () => {
  test.beforeEach(({ page }) => {
    test.skip(isDesktop(page), "The dock only exists below 1024px");
  });

  test("owner: Panel, Gastos, the quick expense button, Ingresos and Más", async ({ page }) => {
    await open(page, "owner");
    await expect(dock(page)).toBeVisible();
    await expect(dock(page).getByRole("link")).toHaveText(["Panel", "Gastos", "Ingresos"]);
    await expect(quickExpenseButton(page)).toBeVisible();
    await expect(moreButton(page)).toBeVisible();
    await expect(dock(page).getByRole("listitem")).toHaveCount(5);
    await expect(page.getByRole("button", { name: "Abrir menú" })).toHaveCount(0);
  });

  test("household: Panel, Mis gastos, the quick expense button, Mis ingresos and Más", async ({ page }) => {
    await open(page, "household");
    await expect(dock(page).getByRole("link")).toHaveText(["Panel", "Mis gastos", "Mis ingresos"]);
    await expect(quickExpenseButton(page)).toBeVisible();
    await expect(moreButton(page)).toBeVisible();
  });

  test("every slot is a big tap target with the icon above the label", async ({ page }) => {
    await open(page, "owner");
    const slots = [...(await dock(page).getByRole("link").all()), moreButton(page), quickExpenseButton(page)];
    for (const slot of slots) {
      const box = await slot.boundingBox();
      expect(box!.height).toBeGreaterThanOrEqual(56);
      expect(box!.width).toBeGreaterThanOrEqual(44);
    }
    const link = dock(page).getByRole("link", { name: "Gastos" });
    const icon = (await link.locator("svg").boundingBox())!;
    const label = (await link.locator("span").boundingBox())!;
    expect(icon.width).toBeGreaterThanOrEqual(24);
    expect(label.y).toBeGreaterThanOrEqual(icon.y + icon.height - 1);
  });

  test("navigates, marks the current page and moves focus to the content", async ({ page }) => {
    await open(page, "owner");
    await expect(dock(page).getByRole("link", { name: "Panel" })).toHaveAttribute("aria-current", "page");
    await dock(page).getByRole("link", { name: "Gastos" }).click();
    await expect(page).toHaveURL(/\/gastos$/);
    await expect(dock(page).getByRole("link", { name: "Gastos" })).toHaveAttribute("aria-current", "page");
    await expect(dock(page).getByRole("link", { name: "Panel" })).not.toHaveAttribute("aria-current", "page");
    await expect(page.locator("main")).toBeFocused();
    await dock(page).getByRole("link", { name: "Ingresos" }).click();
    await expect(page).toHaveURL(/\/ingresos$/);
  });

  test("the content is not hidden behind the dock", async ({ page }) => {
    await open(page, "owner", "/configuracion");
    await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
    const gap = await page.evaluate(() => {
      const last = document.querySelector("main")!.lastElementChild!.getBoundingClientRect();
      const bar = document.querySelector('nav[aria-label="Navegación principal"]')!.getBoundingClientRect();
      return bar.top - last.bottom;
    });
    expect(gap).toBeGreaterThanOrEqual(0);
  });

  test("the top bar keeps only the brand", async ({ page }) => {
    await open(page, "owner");
    const header = page.locator("header");
    await expect(header.getByText("Finanzas")).toBeVisible();
    await expect(header.getByRole("button")).toHaveCount(0);
  });

  test.describe("Más sheet", () => {
    test("owner: lists every module the dock does not show, the theme selector, the session and closes with Escape", async ({ page }) => {
      await open(page, "owner");
      const nav = await openMore(page);
      await expect(nav.getByRole("link")).toHaveText(OWNER_MORE);
      await expect(moreSheet(page).getByRole("radiogroup", { name: "Tema" })).toBeVisible();
      await expect(moreSheet(page).getByText("admin@example.com")).toBeVisible();
      await expect(moreSheet(page).getByRole("button", { name: "Cerrar sesión" })).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(moreSheet(page)).toBeHidden();
      await expect(moreButton(page)).toBeFocused();
    });

    test("household: lists her remaining modules", async ({ page }) => {
      await open(page, "household");
      await openMore(page);
      await expect(moreNav(page).getByRole("link")).toHaveText(HOUSEHOLD_MORE);
    });

    test("closes with the scrim and the close button, and keeps Tab inside", async ({ page }) => {
      await open(page, "owner");
      await openMore(page);
      for (let i = 0; i < 30; i++) await page.keyboard.press("Tab");
      expect(await page.evaluate(() => !!document.activeElement?.closest('[role="dialog"]'))).toBe(true);
      await page.mouse.click(5, 5);
      await expect(moreSheet(page)).toBeHidden();
      await openMore(page);
      await moreSheet(page).getByRole("button", { name: "Cerrar", exact: true }).click();
      await expect(moreSheet(page)).toBeHidden();
    });

    test("opening a module closes the sheet and moves focus to the content", async ({ page }) => {
      await open(page, "owner");
      const nav = await openMore(page);
      await nav.getByRole("link", { name: "Facturas" }).click();
      await expect(page).toHaveURL(/\/facturas$/);
      await expect(moreSheet(page)).toBeHidden();
      await expect(page.locator("main")).toBeFocused();
      const current = await openMore(page);
      await expect(current.getByRole("link", { name: "Facturas" })).toHaveAttribute("aria-current", "page");
    });

    test("is anchored at the bottom and stays inside the screen", async ({ page }) => {
      await open(page, "owner");
      await openMore(page);
      const vh = page.viewportSize()!.height;
      const panel = moreSheet(page).locator("div.relative");
      // Wait for the slide-in to settle.
      await expect.poll(async () => { const b = (await panel.boundingBox())!; return Math.round(b.y + b.height); }).toBe(vh);
      expect((await panel.boundingBox())!.y).toBeGreaterThanOrEqual(0);
    });
  });

  test.describe("quick expense", () => {
    test("owner: validates, saves the expense with the chosen values and refreshes the open page", async ({ page }) => {
      const api = await open(page, "owner", "/gastos");
      await quickExpenseButton(page).click();
      const dialog = page.getByRole("dialog", { name: "Gasto rápido" });
      await expect(dialog).toBeVisible();
      await expect(dialog.getByLabel("Monto (MXN)")).toBeFocused();
      await expect(dialog.getByLabel("Fecha")).toHaveValue(today());

      await dialog.getByRole("button", { name: "Agregar gasto" }).click();
      await expect(dialog.getByText("Escribe un monto mayor a cero")).toBeVisible();
      await expect(dialog.getByLabel("Monto (MXN)")).toBeFocused();

      await dialog.getByLabel("Monto (MXN)").fill("250.50");
      await dialog.getByLabel("Categoría").selectOption("Comida");
      await dialog.getByLabel("Descripción").fill("Tacos quick");
      await dialog.getByRole("button", { name: "Agregar gasto" }).click();

      await expect(dialog).toBeHidden();
      await expect(page.getByTestId("quick-expense-notice")).toContainText("Gasto guardado.");
      const posts = api.requests.filter((r) => r.path === "/expenses");
      expect(posts).toHaveLength(1);
      expect(posts[0].body).toMatchObject({ amount: "250.50", currency: "MXN", date: today(), description: "Tacos quick", category: "Comida" });
      expect(typeof (posts[0].body as { payment_method: unknown }).payment_method).toBe("string");
      // The open Gastos page shows it without a reload.
      await expect(page.getByText("Tacos quick")).toBeVisible();
      await expect(quickExpenseButton(page)).toBeFocused();
    });

    test("owner: Escape closes it without saving", async ({ page }) => {
      const api = await open(page, "owner");
      await quickExpenseButton(page).click();
      await expect(page.getByRole("dialog", { name: "Gasto rápido" })).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(page.getByRole("dialog")).toHaveCount(0);
      expect(api.requests.filter((r) => r.path === "/expenses")).toHaveLength(0);
    });

    test("household: saves a personal Gasto and refreshes her page", async ({ page }) => {
      const personal: MockPersonalMovement[] = [];
      const api = await open(page, "household", "/mis-gastos", { personal });
      await quickExpenseButton(page).click();
      const dialog = page.getByRole("dialog", { name: "Nuevo gasto" });
      await expect(dialog).toBeVisible();
      await dialog.getByLabel("Descripción").fill("Labial quick");
      await dialog.getByLabel("Monto (MXN)").fill("199.90");
      await dialog.getByRole("button", { name: "Guardar" }).click();
      await expect(dialog).toBeHidden();
      await expect(page.getByTestId("quick-expense-notice")).toContainText("Gasto guardado.");
      expect(api.personalWrites[0].body).toMatchObject({ kind: "Gasto", amount: "199.90", description: "Labial quick", date: today() });
      await expect(page.getByText("Labial quick")).toBeVisible();
    });
  });
});
