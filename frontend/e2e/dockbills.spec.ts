import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockBill, type MockRole } from "./helpers";
import { isDesktop, quickExpenseButton } from "./nav";

const pad = (n: number) => String(n).padStart(2, "0");
const day = (offset = 0) => {
  const d = new Date();
  d.setDate(d.getDate() + offset);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
};

const bill = (over: Partial<MockBill> = {}): MockBill => ({
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
  ...over,
});

/** Created out of order on purpose: the list must sort by due date, the overdue one first. */
const BILLS: MockBill[] = [
  bill({ id: 3, name: "Netflix", category: "Suscripciones", amount: "139.00", next_due_date: day(40) }),
  bill({ id: 2, name: "Luz", amount: null, recurrence: "bimonthly", next_due_date: day(2) }),
  bill(),
];

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({ scrollWidth: document.documentElement.scrollWidth, clientWidth: document.documentElement.clientWidth }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

async function open(page: Page, role: MockRole, path = "/", opts: Parameters<typeof mockApi>[1] = {}) {
  const api = await mockApi(page, { role, ...opts });
  await seedSession(page, role);
  await page.goto(path);
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  return api;
}

async function openRecurring(page: Page) {
  await quickExpenseButton(page).click();
  const dialog = page.getByRole("dialog", { name: "Gasto rápido" });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("tab", { name: "Recurrente" }).click();
  await expect(page.getByRole("dialog", { name: "Pago recurrente" })).toBeVisible();
  // The panel, not the dialog: the Gasto form stays mounted (hidden) with fields of the same name.
  return page.getByRole("dialog", { name: "Pago recurrente" }).getByRole("tabpanel", { name: "Recurrente" });
}

test.describe("dock quick dialog: Recurrente tab", () => {
  test.beforeEach(({ page }) => {
    test.skip(isDesktop(page), "The dock only exists below 1024px");
  });

  test("opens on Gasto with accessible tabs that move with the arrow keys", async ({ page }) => {
    await open(page, "owner", "/", { bills: BILLS });
    await quickExpenseButton(page).click();
    await expect(page.getByRole("dialog", { name: "Gasto rápido" })).toBeVisible();
    // The dialog's name follows the tab, so it is located without one from here on.
    const dialog = page.getByRole("dialog");
    const gasto = dialog.getByRole("tab", { name: "Gasto" });
    const recurrente = dialog.getByRole("tab", { name: "Recurrente" });
    await expect(dialog.getByRole("tablist", { name: "Tipo de registro" })).toBeVisible();
    await expect(gasto).toHaveAttribute("aria-selected", "true");
    await expect(recurrente).toHaveAttribute("aria-selected", "false");
    await expect(dialog.getByRole("tabpanel", { name: "Gasto" })).toBeVisible();
    await expect(dialog.getByLabel("Monto (MXN)")).toBeFocused();
    expect((await gasto.boundingBox())!.height).toBeGreaterThanOrEqual(44);

    await gasto.focus();
    await page.keyboard.press("ArrowRight");
    await expect(recurrente).toBeFocused();
    await expect(recurrente).toHaveAttribute("aria-selected", "true");
    await expect(dialog.getByRole("tabpanel", { name: "Recurrente" })).toBeVisible();
    await page.keyboard.press("ArrowLeft");
    await expect(gasto).toHaveAttribute("aria-selected", "true");
    await expectNoHorizontalOverflow(page, "tabs");
  });

  test("lists the pending bills soonest first and pays a fixed bill with its amount prefilled", async ({ page }) => {
    const api = await open(page, "owner", "/pagos-recurrentes", { bills: BILLS });
    const dialog = await openRecurring(page);
    const radios = dialog.getByRole("radio");
    await expect(radios).toHaveCount(3);
    await expect(dialog.getByRole("listitem").nth(0)).toContainText("Megacable");
    await expect(dialog.getByRole("listitem").nth(0)).toContainText("$550.00 MXN");
    await expect(dialog.getByRole("listitem").nth(0)).toContainText("Vencido hace 5 días");
    await expect(dialog.getByRole("listitem").nth(1)).toContainText("Luz");
    await expect(dialog.getByRole("listitem").nth(1)).toContainText("Variable");
    await expect(dialog.getByRole("listitem").nth(2)).toContainText("Netflix");
    await expect(dialog.getByRole("button", { name: "Registrar pago" })).toHaveCount(0);
    await expectNoHorizontalOverflow(page, "recurring list");

    await dialog.getByRole("radio", { name: /Megacable/ }).check();
    await expect(dialog.getByLabel("Monto (MXN)")).toHaveValue("550.00");
    await expect(dialog.getByLabel("Fecha de pago")).toHaveValue(day(0));
    await expectNoHorizontalOverflow(page, "recurring form");
    await dialog.getByRole("button", { name: "Registrar pago" }).click();

    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByTestId("quick-expense-notice")).toContainText("Pago de Megacable registrado: $550.00 MXN en Servicios.");
    const write = api.billWrites.find((w) => w.method === "POST" && w.path === "/bills/1/pay");
    expect(write?.body).toMatchObject({ amount: "550.00", date: day(0), category: "Servicios" });
    // The open Pagos recurrentes page already shows the next due date.
    await expect(page.getByRole("list", { name: "Pagos recurrentes" }).getByRole("listitem").filter({ hasText: "Megacable" })).not.toContainText("Vencido");
  });

  test("a variable bill requires the amount", async ({ page }) => {
    const api = await open(page, "owner", "/", { bills: BILLS });
    const dialog = await openRecurring(page);
    await dialog.getByRole("radio", { name: /Luz/ }).check();
    await expect(dialog.getByLabel("Monto (MXN)")).toHaveValue("");
    await dialog.getByRole("button", { name: "Registrar pago" }).click();
    await expect(dialog.getByText("Escribe un monto mayor a cero")).toBeVisible();
    await expect(dialog.getByLabel("Monto (MXN)")).toBeFocused();
    expect(api.billWrites.filter((w) => w.path === "/bills/2/pay")).toHaveLength(0);

    await dialog.getByLabel("Monto (MXN)").fill("412.30");
    await dialog.getByRole("button", { name: "Registrar pago" }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByTestId("quick-expense-notice")).toContainText("Pago de Luz registrado: $412.30 MXN");
    expect(api.billWrites.find((w) => w.path === "/bills/2/pay")?.body).toMatchObject({ amount: "412.30" });
  });

  test("shows the empty state with a link to Pagos recurrentes", async ({ page }) => {
    await open(page, "owner", "/", { bills: [] });
    const dialog = await openRecurring(page);
    await expect(dialog.getByText("No tienes pagos recurrentes pendientes.")).toBeVisible();
    await dialog.getByRole("link", { name: "Ir a Pagos recurrentes" }).click();
    await expect(page).toHaveURL(/\/pagos-recurrentes$/);
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });

  test("keeps the dialog open and shows the error when the payment fails", async ({ page }) => {
    await open(page, "owner", "/", { bills: BILLS, billsFail: "pay" });
    const dialog = await openRecurring(page);
    await dialog.getByRole("radio", { name: /Megacable/ }).check();
    await dialog.getByRole("button", { name: "Registrar pago" }).click();
    await expect(dialog.getByRole("alert")).toBeVisible();
    await expect(dialog).toBeVisible();
    await expect(page.getByTestId("quick-expense-notice")).toHaveCount(0);
  });

  test("shows an error with a retry when the bills cannot be loaded", async ({ page }) => {
    await open(page, "owner", "/", { bills: BILLS, billsFail: "list" });
    const dialog = await openRecurring(page);
    await expect(dialog.getByRole("alert")).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Reintentar" })).toBeVisible();
  });

  test("household keeps her dialog without tabs", async ({ page }) => {
    await open(page, "household", "/mis-gastos");
    await quickExpenseButton(page).click();
    await expect(page.getByRole("dialog", { name: "Nuevo gasto" })).toBeVisible();
    await expect(page.getByRole("tablist")).toHaveCount(0);
    await expect(page.getByRole("tab", { name: "Recurrente" })).toHaveCount(0);
  });
});
