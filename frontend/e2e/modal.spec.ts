import { expect, test, type Locator, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockFutureExpense } from "./helpers";
import { gotoSettings } from "./settingsNav";
import { quickExpenseButton } from "./nav";

// A short phone: long dialogs must still open at their title, whatever the page scroll was.
test.use({ viewport: { width: 375, height: 667 } });

const pad = (n: number) => String(n).padStart(2, "0");
const due = (() => {
  const d = new Date();
  d.setDate(d.getDate() + 90);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
})();
const LAPTOP: MockFutureExpense = { id: 1, name: "Laptop", target_amount: "8000.00", due_date: due, saved: "2000.00" };

async function expectOpenAtTitle(page: Page, dialog: Locator) {
  await expect(dialog).toBeVisible();
  const title = dialog.getByRole("heading", { level: 2 });
  await expect(title).toBeInViewport({ ratio: 1 });
  const top = (await title.boundingBox())!.y;
  expect(top).toBeGreaterThanOrEqual(0);
  const scrolls = await dialog.evaluate((el) => [...el.querySelectorAll<HTMLElement>("*")].filter((n) => n.scrollHeight > n.clientHeight && n.scrollTop !== 0).length);
  expect(scrolls).toBe(0);
  const panel = await dialog.locator("div.relative").first().evaluate((el) => el.scrollTop);
  expect(panel).toBe(0);
}

test.describe("modals open at their title", () => {

  test("future expense detail, after scrolling the page", async ({ page }) => {
    await mockApi(page, { futureExpenses: [LAPTOP] });
    await seedSession(page);
    await page.goto("/gastos-futuros");
    await expect(page.getByRole("heading", { level: 1, name: "Gastos futuros" })).toBeVisible();
    await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
    await page.getByRole("list", { name: "Gastos futuros activos" }).getByRole("button", { name: "Ver Laptop" }).click();
    await expectOpenAtTitle(page, page.getByRole("dialog", { name: "Laptop" }));
  });

  test("category edit", async ({ page }) => {
    await mockApi(page);
    await seedSession(page);
    await gotoSettings(page, "categorias");
    await page.evaluate(() => window.scrollTo(0, 400));
    await page.getByRole("button", { name: /^Renta/ }).click();
    await expectOpenAtTitle(page, page.getByRole("dialog", { name: "Editar Renta" }));
  });

  test("quick expense", async ({ page }) => {
    await mockApi(page);
    await seedSession(page);
    await page.goto("/configuracion");
    await page.evaluate(() => window.scrollTo(0, 600));
    await quickExpenseButton(page).click();
    await expectOpenAtTitle(page, page.getByRole("dialog", { name: "Gasto rápido" }));
  });
});
