import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession } from "./helpers";

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

test.describe("settings page", () => {
  test("edits a category budget, saves it as a decimal string and shows the saved value", async ({ page }) => {
    const api = await mockApi(page);
    await seedSession(page);
    await page.goto("/configuracion");

    await expect(page.getByRole("heading", { level: 1, name: "Configuración" })).toBeVisible();
    await page.getByRole("tab", { name: "Categorías y presupuestos" }).click();

    const budget = page.getByLabel("Presupuesto de Renta");
    await expect(budget).toHaveValue("12000.50");
    await budget.fill("13500.75");
    await expectNoHorizontalOverflow(page, "categories (edited)");

    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByText("Cambios guardados.")).toBeVisible();

    const put = api.writes.find((w) => w.method === "PUT" && w.path === "/settings/categories");
    expect(put, "PUT /settings/categories was sent").toBeTruthy();
    const body = put?.body as { name: string; budget: string | null }[];
    expect(body.find((c) => c.name === "Renta")?.budget).toBe("13500.75");
    expect(body.find((c) => c.name === "Inversiones")?.budget).toBeNull();

    await page.reload();
    await page.getByRole("tab", { name: "Categorías y presupuestos" }).click();
    await expect(page.getByLabel("Presupuesto de Renta")).toHaveValue("13500.75");
  });

  test("blocks an invalid budget, focuses the field and sends nothing", async ({ page }) => {
    const api = await mockApi(page);
    await seedSession(page);
    await page.goto("/configuracion");
    await page.getByRole("tab", { name: "Categorías y presupuestos" }).click();

    const budget = page.getByLabel("Presupuesto de Comida");
    await budget.fill("12,5");
    await page.getByRole("button", { name: "Guardar cambios" }).click();

    await expect(page.getByRole("alert").filter({ hasText: "número válido" })).toBeVisible();
    await expect(budget).toBeFocused();
    expect(api.writes).toHaveLength(0);
  });

  test("shows the backend message when the server rejects the settings", async ({ page }) => {
    await mockApi(page, { settingsFail: "budget for Renta exceeds income" });
    await seedSession(page);
    await page.goto("/configuracion");
    await page.getByRole("tab", { name: "Categorías y presupuestos" }).click();
    await page.getByLabel("Presupuesto de Renta").fill("99999");
    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "budget for Renta exceeds income" })).toBeVisible();
  });

  test("cancels the investment pause after confirming", async ({ page }) => {
    const api = await mockApi(page);
    await seedSession(page);
    await page.goto("/configuracion");
    await page.getByRole("tab", { name: "Pausa de inversiones" }).click();

    await expect(page.getByLabel("Meses en pausa")).toHaveValue("2026-10, 2026-11");
    await page.getByRole("button", { name: "Cancelar pausa" }).click();
    await page.getByRole("button", { name: "Sí, cancelar pausa" }).click();

    await expect(page.getByText("No hay una pausa activa.")).toBeVisible();
    expect(api.writes.some((w) => w.method === "DELETE" && w.path === "/settings/investment-pause")).toBe(true);
  });

  test("has no horizontal overflow in any section", async ({ page }) => {
    await mockApi(page);
    await seedSession(page);
    await page.goto("/configuracion");
    for (const name of ["General", "Categorías y presupuestos", "Clientes", "Instrumentos", "Rangos de RESICO", "Pausa de inversiones"]) {
      await page.getByRole("tab", { name, exact: true }).click();
      await expect(page.getByRole("tab", { name, exact: true })).toHaveAttribute("aria-selected", "true");
      await expect(page.getByRole("tabpanel").getByRole("heading", { level: 2 }).first()).toBeVisible();
      await expectNoHorizontalOverflow(page, name);
    }
  });
});
