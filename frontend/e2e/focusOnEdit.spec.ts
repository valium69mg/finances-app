import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockBill, type MockIncome } from "./helpers";

const pad = (n: number) => String(n).padStart(2, "0");

function day(offset = 0) {
  const d = new Date();
  d.setDate(d.getDate() + offset);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** A long list, so the edit form (at the top of the page) is far away from the last row. */
const INCOME: MockIncome[] = Array.from({ length: 80 }, (_, i) => ({
  id: i + 1,
  date: day(),
  description: `Sueldo ${i + 1}`,
  category: "Sueldo",
  payment_method: "Transferencia",
  currency: "MXN",
  amount: "20000.00",
  exchange_rate: null,
  amount_mxn: "20000.00",
}));

const BILL: MockBill = {
  id: 1,
  name: "Megacable",
  category: "Servicios",
  amount: "550.00",
  currency: "MXN",
  recurrence: "monthly",
  next_due_date: day(-5),
  reminder_lead_days: 3,
  active: true,
  notes: "",
};

async function openPage(page: Page, path: string, opts: Parameters<typeof mockApi>[1]) {
  await mockApi(page, opts);
  await seedSession(page);
  await page.goto(path);
}

const HIGHLIGHT = /(^|\s)edit-highlight(\s|$)/;

test.describe("focus on edit (inline form)", () => {
  test("editing the last income scrolls the form into view, focuses it and flashes green", async ({ page }) => {
    await openPage(page, "/ingresos", { income: INCOME });
    const edit = page.getByRole("button", { name: "Editar Sueldo 80" });
    await expect(edit).toBeVisible();
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    const form = page.getByRole("form", { name: "Nuevo ingreso" });
    await expect(form).not.toBeInViewport();
    expect(await page.evaluate(() => window.scrollY)).toBeGreaterThan(300);

    await edit.click();

    const editForm = page.getByRole("form", { name: "Editar ingreso" });
    await expect(editForm).toBeInViewport({ ratio: 0.9 });
    await expect(editForm).toHaveClass(HIGHLIGHT);
    await expect(editForm.getByLabel("Descripción")).toBeFocused();
    // The highlight fades away after about two seconds.
    await expect(editForm).not.toHaveClass(HIGHLIGHT, { timeout: 4_000 });
  });
});

test.describe("focus on edit (modal form)", () => {
  test("the payment dialog is in view and shows the highlight without scrolling the page", async ({ page }) => {
    await openPage(page, "/pagos-recurrentes", { bills: [BILL] });
    await page.getByRole("button", { name: "Pagar Megacable" }).click();

    const dialog = page.getByRole("dialog", { name: "Pagar Megacable" });
    await expect(dialog).toBeInViewport();
    const body = dialog.locator("form").locator("xpath=..");
    await expect(body).toHaveClass(HIGHLIGHT);
    await expect(body).not.toHaveClass(HIGHLIGHT, { timeout: 4_000 });
  });
});

for (const [width, height] of [
  [390, 844],
  [320, 640],
] as const) {
  test.describe(`phone width ${width}px`, () => {
    test.use({ viewport: { width, height } });

    for (const [name, path] of [
      ["Panel", "/"],
      ["Ingresos", "/ingresos"],
      ["Gastos", "/gastos"],
      ["Ahorro", "/ahorros"],
      ["Cierre de mes", "/cierre-de-mes"],
    ] as const) {
      test(`${name} does not overflow and the month banner stays inside the viewport`, async ({ page }) => {
        await openPage(page, path, { income: INCOME });
        await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
        const month = page.locator('input[type="month"]').first();
        await expect(month).toBeVisible();

        const size = await page.evaluate(() => ({ scrollWidth: document.documentElement.scrollWidth, clientWidth: document.documentElement.clientWidth }));
        expect(size.scrollWidth, `${name} overflows horizontally`).toBeLessThanOrEqual(size.clientWidth);
        const box = await month.boundingBox();
        expect(box).not.toBeNull();
        expect(box!.x).toBeGreaterThanOrEqual(0);
        expect(box!.x + box!.width).toBeLessThanOrEqual(width);
      });
    }
  });
}
