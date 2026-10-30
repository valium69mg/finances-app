import { expect, test, type Page } from "@playwright/test";
import { OWNER_USER } from "./usersMock";
import { mockApi, seedSession, type MockClose, type MockPersonalMovement, type MockUser } from "./helpers";

// Screenshots for visual review: set SHOTS_DIR to write them (never committed). Light scheme, 1280 and 375 wide.
const shots = process.env.SHOTS_DIR;

const pad = (n: number) => String(n).padStart(2, "0");
const today = () => {
  const d = new Date();
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
};
const day = (offset: number) => {
  const d = new Date();
  d.setDate(d.getDate() + offset);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
};
const PREV = (() => {
  const d = new Date();
  const first = new Date(d.getFullYear(), d.getMonth() - 1, 1);
  return `${first.getFullYear()}-${pad(first.getMonth() + 1)}`;
})();
const MONTHS = ["enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"];

const ANA: MockUser = { id: "u-ana", email: "ana@example.com", role: "household", active: true, verified: true };
const PERSONAL: MockPersonalMovement[] = [
  { id: 1, kind: "Ingreso", category: "Trabajo", amount: "9000.00", description: "Quincena", date: today() },
  { id: 2, kind: "Gasto", category: "Ropa", amount: "350.00", description: "Blusa", date: today() },
  { id: 3, kind: "Gasto", category: "Salud", amount: "800.50", description: "Consulta", date: today() },
  { id: 4, kind: "Ahorro", category: "Inversión", amount: "1000.00", description: "Aporte", date: today() },
];

const CLOSE: MockClose = {
  period: PREV,
  closed_at: `${PREV}-28T18:30:00.000Z`,
  categories: [
    { category: "Renta", spent: "12000.50", budget: "12000.50", remaining: "0.00", over_budget: false },
    { category: "Comida", spent: "9000.00", budget: "6000", remaining: "-3000.00", over_budget: true },
  ],
  income: "30000.00",
  expenses: "21000.50",
  savings: "1000.00",
  available: "7999.50",
  emergency: { accumulated: "20000.00", goal: "60000.00" },
  suggestion: { to_emergency_fund: "7999.50", to_investments: "0", to_future_expenses: "0", investments_paused: false },
  adjustments: [{ category: "Comida", kind: "Gasto", budget: "6000", real: "9000.00", deviation_pct: "50.00" }],
  tax_filing_status: "pendiente",
};

for (const width of [1280, 375]) {
  test(`screenshots round 2 ${width}px`, async ({ page }, info) => {
    test.skip(!shots || info.project.name !== "mobile", "only with SHOTS_DIR, once");
    await page.setViewportSize({ width, height: width === 375 ? 900 : 1000 });
    await page.emulateMedia({ colorScheme: "light" });
    const snap = (name: string) => page.screenshot({ path: `${shots}/${name}-${width}.png`, fullPage: true });

    await mockApi(page, {
      role: "owner",
      users: [OWNER_USER, ANA],
      personal: PERSONAL,
      personalFuture: [{ id: 1, name: "Laptop", target_amount: "8000.00", due_date: "2027-06-20", saved: "2000.00" }],
      personalWishlist: [{ id: 1, name: "Bolsa", price: "499.90", category: "Ropa", priority: "alta" }],
      futureExpenses: [
        { id: 1, name: "Laptop", target_amount: "8000.00", due_date: day(120), saved: "2000.00" },
        { id: 2, name: "Viaje", target_amount: "24000.00", due_date: day(60), saved: "24000.00" },
      ],
      futureFreeBalance: "350.25",
      closes: [CLOSE],
    });
    await seedSession(page, "owner");

    await page.goto("/hogar?usuario=u-ana");
    await expect(page.getByRole("heading", { level: 1, name: "Finanzas de ana@example.com" })).toBeVisible();
    await expect(page.getByTestId("category-spend").first()).toBeVisible();
    await snap("hogar-resumen");

    await page.goto("/gastos-futuros");
    await page.getByRole("button", { name: "Ver Laptop" }).click();
    await expect(page.getByRole("dialog", { name: "Laptop" })).toBeVisible();
    await page.screenshot({ path: `${shots}/gasto-futuro-detalle-${width}.png` });
    await page.keyboard.press("Escape");

    await page.goto("/ahorros");
    await expect(page.getByRole("heading", { level: 1, name: "Ahorros" })).toBeVisible();
    await expect(page.getByText("Valor del portafolio")).toBeVisible();
    await snap("ahorros");

    await page.goto("/cierre-de-mes");
    await page.getByRole("button", { name: `Ver cierre de ${MONTHS[Number(PREV.slice(5)) - 1]} de ${PREV.slice(0, 4)}` }).click();
    await expect(page.getByRole("region", { name: /^Detalle del cierre/ })).toBeVisible();
    await snap("cierre-de-mes");
  });
}

test("screenshot: Mis gastos (household)", async ({ page }, info) => {
  test.skip(!shots || info.project.name !== "mobile", "only with SHOTS_DIR, once");
  for (const width of [1280, 375]) {
    await page.setViewportSize({ width, height: width === 375 ? 900 : 1000 });
    await page.emulateMedia({ colorScheme: "light" });
    await mockApi(page, { role: "household", personal: PERSONAL });
    await seedSession(page, "household");
    await page.goto("/mis-gastos");
    await expect(page.getByRole("button", { name: "Editar Blusa" })).toBeVisible();
    await page.screenshot({ path: `${shots}/mis-gastos-${width}.png`, fullPage: true });
  }
});

// Round 3: the mobile dock, the quick expense, the Más sheet and the theme selector, in both color schemes.
test("screenshots round 3 (dock, quick expense, Más, theme selector)", async ({ page }, info) => {
  test.skip(!shots || info.project.name !== "mobile", "only with SHOTS_DIR, once");
  const EXPENSES = [
    { id: 1, date: today(), description: "Renta", category: "Renta", payment_method: "Transferencia", currency: "MXN", amount: "12000.00", exchange_rate: null, amount_mxn: "12000.00" },
    { id: 2, date: today(), description: "Súper", category: "Comida", payment_method: "Débito", currency: "MXN", amount: "1450.00", exchange_rate: null, amount_mxn: "1450.00" },
  ];
  await mockApi(page, { role: "owner", expenses: EXPENSES });
  await seedSession(page, "owner");
  for (const scheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: scheme });

    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 2, name: "Resumen del mes" })).toBeVisible();
    await page.screenshot({ path: `${shots}/panel-dock-${scheme}-375.png` });

    await page.getByRole("button", { name: "Registrar gasto rápido" }).click();
    const quick = page.getByRole("dialog", { name: "Gasto rápido" });
    await quick.getByLabel("Monto (MXN)").fill("250.50");
    await quick.getByLabel("Descripción").fill("Tacos");
    await page.waitForTimeout(400);
    await page.screenshot({ path: `${shots}/gasto-rapido-${scheme}-375.png` });
    await page.keyboard.press("Escape");

    await page.getByRole("button", { name: "Más", exact: true }).click();
    await expect(page.getByRole("dialog", { name: "Más", exact: true })).toBeVisible();
    await page.waitForTimeout(400);
    await page.screenshot({ path: `${shots}/mas-${scheme}-375.png` });
    await page.keyboard.press("Escape");

    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 2, name: "Resumen del mes" })).toBeVisible();
    await page.screenshot({ path: `${shots}/escritorio-tema-${scheme}-1280.png` });
  }
});
