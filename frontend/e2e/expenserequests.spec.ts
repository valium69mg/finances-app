import { expect, test, type Page } from "@playwright/test";
import { API_ORIGIN, mockApi, seedSession, type MockExpense, type MockExpenseRequest } from "./helpers";

const pad = (n: number) => String(n).padStart(2, "0");
const today = () => {
  const d = new Date();
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
};

const isDesktop = (width: number | undefined) => (width ?? 0) >= 1024;

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

/** A Gasto of the current cycle, so the budget check has something spent to count. */
const spentOn = (id: number, category: string, amount: string): MockExpense => ({
  id,
  date: today(),
  description: `Gasto de ${category}`,
  category,
  payment_method: "Transferencia",
  currency: "MXN",
  amount,
  exchange_rate: null,
  amount_mxn: amount,
});

/** Obviously fake requests: the repository is public. Servicios has a 2,000.00 budget, Suscripciones has none. */
const TACOS: MockExpenseRequest = { id: 1, amount: "500.00", description: "Tacos del sábado", suggested_category: "Servicios", expense_date: today() };
const NETFLIX: MockExpenseRequest = { id: 2, amount: "219.00", description: "Plan de streaming", suggested_category: "Suscripciones", expense_date: today() };
const GIFT: MockExpenseRequest = { id: 3, amount: "1800.00", description: "Regalo de aniversario", suggested_category: null, expense_date: today() };

async function openAsOwner(page: Page, opts: Parameters<typeof mockApi>[1] = {}, path = "/peticiones") {
  const api = await mockApi(page, opts);
  await seedSession(page, "owner");
  await page.goto(path);
  await expect(page.getByRole("heading", { level: 1, name: "Peticiones" })).toBeVisible();
  return api;
}

async function openAsHousehold(page: Page, opts: Parameters<typeof mockApi>[1] = {}) {
  const api = await mockApi(page, { role: "household", ...opts });
  await seedSession(page, "household");
  await page.goto("/peticiones");
  await expect(page.getByRole("heading", { level: 1, name: "Peticiones" })).toBeVisible();
  return api;
}

const cards = (page: Page) => page.getByTestId("request-card");
const cardOf = (page: Page, text: string) => cards(page).filter({ hasText: text });

test.describe("household: expense requests", () => {
  test("creates a request, sees it as Solicitada and cancels it", async ({ page }) => {
    const api = await openAsHousehold(page);
    await expect(page.getByText(/Aún no has hecho ninguna petición/)).toBeVisible();

    const toggle = page.getByRole("button", { name: "Nueva petición" });
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");

    // The form validates before it sends anything.
    await page.getByRole("button", { name: "Enviar petición" }).click();
    await expect(page.getByText(/Escribe un monto mayor a cero/)).toBeVisible();
    expect(api.requestWrites).toHaveLength(0);

    await page.getByLabel("Monto (MXN)").fill("250.50");
    await page.getByLabel("Descripción").fill("Tacos");
    // Only the Gasto category names are offered, with no budgets or amounts.
    const options = await page.getByLabel("Categoría sugerida").locator("option").allInnerTexts();
    expect(options).toEqual(["Sin sugerencia", "Renta", "Comida", "Servicios", "Suscripciones"]);
    await page.getByLabel("Categoría sugerida").selectOption("Comida");
    await page.getByRole("button", { name: "Enviar petición" }).click();

    await expect(page.getByText(/Petición enviada/)).toBeVisible();
    const card = cardOf(page, "Tacos");
    await expect(card).toHaveCount(1);
    await expect(card.getByTestId("request-state")).toHaveText("Solicitada");
    await expect(card).toContainText("$250.50");
    await expect(card).toContainText("Comida");
    expect(api.requestWrites[0].body).toMatchObject({ amount: "250.50", description: "Tacos", suggested_category: "Comida", date: today() });

    await card.getByRole("button", { name: "Cancelar la petición Tacos" }).click();
    await expect(page.getByText(/Cancelaste tu petición/)).toBeVisible();
    await expect(card.getByTestId("request-state")).toHaveText("Cancelada");
    await expect(card.getByRole("button", { name: /Cancelar/ })).toHaveCount(0);
    await expectNoHorizontalOverflow(page, "household requests page");
  });

  test("sees only her own requests, each with its state, the owner comment and what happened", async ({ page }) => {
    await openAsHousehold(page, {
      expenseRequests: [
        { id: 1, amount: "100.00", description: "Pendiente", status: "solicitada" },
        { id: 2, amount: "200.00", description: "Rechazada", status: "rechazada", decision_comment: "Mejor el mes que entra" },
        { id: 3, amount: "300.00", description: "Aprobada como gasto", status: "aprobada", result_kind: "gasto" },
        { id: 4, amount: "400.00", description: "Aprobada a futuro", status: "aprobada", result_kind: "gasto_futuro" },
        { id: 5, amount: "500.00", description: "Cancelada", status: "cancelada" },
        { id: 6, amount: "600.00", description: "De otra persona", requester_email: "someone-else@example.com" },
      ],
    });
    await expect(cards(page)).toHaveCount(5);
    await expect(page.getByText("De otra persona")).toHaveCount(0);
    const states = await cards(page).getByTestId("request-state").allInnerTexts();
    expect(states).toEqual(["Cancelada", "Aprobada", "Aprobada", "Rechazada", "Solicitada"]); // newest first
    await expect(cardOf(page, "Rechazada").first()).toContainText("Comentario: Mejor el mes que entra");
    await expect(cardOf(page, "Aprobada como gasto")).toContainText("Se registró como gasto.");
    await expect(cardOf(page, "Aprobada a futuro")).toContainText("Se movió a gastos futuros.");
    // Cancelar only on the pending one.
    await expect(page.getByRole("button", { name: /^Cancelar la petición/ })).toHaveCount(1);
    await expect(page.getByRole("button", { name: /Aprobar|Rechazar/ })).toHaveCount(0);
  });

  test("the navigation shows Peticiones next to the dashboard and nothing owner-only", async ({ page }) => {
    await openAsHousehold(page);
    if (!isDesktop(page.viewportSize()?.width)) await page.getByRole("button", { name: "Abrir menú" }).click();
    const links = page.getByRole("navigation", { name: "Módulos" }).getByRole("link");
    await expect(links).toHaveText(["Panel", "Peticiones"]);
    await expect(page.getByTestId("pending-requests-badge")).toHaveCount(0);
  });

  test("the server refuses the owner-only routes with 403", async ({ page }) => {
    await openAsHousehold(page, { expenseRequests: [{ id: 1, amount: "100.00", description: "Mía" }] });
    const statuses = await page.evaluate(async (origin) => {
      const call = async (method: string, path: string, body?: unknown) =>
        (await fetch(`${origin}${path}`, { method, headers: { "Content-Type": "application/json" }, body: body ? JSON.stringify(body) : undefined })).status;
      return {
        budgetCheck: await call("GET", "/expense-requests/1/budget-check?category=Servicios"),
        approve: await call("POST", "/expense-requests/1/approve", { destination: "gasto", category: "Servicios" }),
        reject: await call("POST", "/expense-requests/1/reject", { comment: "x" }),
        revert: await call("POST", "/expense-requests/1/revert"),
        settings: await call("GET", "/settings"),
      };
    }, API_ORIGIN);
    expect(statuses).toEqual({ budgetCheck: 403, approve: 403, reject: 403, revert: 403, settings: 403 });
  });

  test("explains the rate limit instead of failing silently", async ({ page }) => {
    await openAsHousehold(page, { expenseRequestsFail: "rate_limit" });
    await page.getByRole("button", { name: "Nueva petición" }).click();
    await page.getByLabel("Monto (MXN)").fill("10");
    await page.getByLabel("Descripción").fill("Algo");
    await page.getByRole("button", { name: "Enviar petición" }).click();
    await expect(page.getByRole("alert")).toContainText("demasiadas peticiones");
  });

  test("a 403 from the server shows a message instead of crashing the page", async ({ page }) => {
    await mockApi(page, { denyAll: true });
    await seedSession(page, "household");
    await page.goto("/peticiones");
    await expect(page.getByRole("heading", { level: 1, name: "Peticiones" })).toBeVisible();
    await expect(page.getByRole("alert").first()).toContainText("no tiene permiso");
  });

  test("has no horizontal overflow at 375px, with the form open", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await openAsHousehold(page, {
      expenseRequests: [
        { id: 1, amount: "99999999.99", description: "Una descripción larguísima ".repeat(4).trim(), status: "rechazada", decision_comment: "Un comentario largo ".repeat(12).trim() },
        { id: 2, amount: "250.50", description: "Tacos", suggested_category: "Suscripciones" },
      ],
    });
    await page.getByRole("button", { name: "Nueva petición" }).click();
    await expectNoHorizontalOverflow(page, "household requests at 375px");
    for (const button of await page.getByRole("button").all()) {
      const box = await button.boundingBox();
      if (box) expect(box.height, `touch target of "${await button.innerText()}"`).toBeGreaterThanOrEqual(43);
    }
  });
});

test.describe("household: a reverted request", () => {
  test("shows the request as Solicitada again, with its history, and she can cancel it", async ({ page }) => {
    await openAsHousehold(page, { expenseRequests: [{ id: 1, amount: "100.00", description: "Mía", revert_count: 1 }] });
    const card = cardOf(page, "Mía");
    await expect(card.getByTestId("request-state")).toHaveText("Solicitada");
    await expect(card.getByTestId("revert-history")).toContainText("la aprobación se deshizo una vez");
    await expect(card.getByRole("button", { name: /^Cancelar la petición/ })).toBeVisible();
    await expect(page.getByRole("button", { name: /Volver a solicitada/ })).toHaveCount(0);
  });
});

test.describe("owner: expense requests", () => {
  test("lists every request with state filters and a pending count in the navigation", async ({ page }) => {
    await openAsOwner(page, {
      expenseRequests: [
        TACOS,
        NETFLIX,
        { id: 3, amount: "300.00", description: "Ya aprobada", status: "aprobada", result_kind: "gasto" },
        { id: 4, amount: "400.00", description: "Ya rechazada", status: "rechazada", decision_comment: "No" },
      ],
    });
    // The default filter is what waits for a decision.
    await expect(page.getByRole("button", { name: "Solicitadas" })).toHaveAttribute("aria-pressed", "true");
    await expect(cards(page)).toHaveCount(2);
    await expect(cardOf(page, "Tacos del sábado")).toContainText("her@example.com");
    await expect(cardOf(page, "Tacos del sábado").getByRole("button", { name: /Aprobar/ })).toBeVisible();
    await expect(cardOf(page, "Tacos del sábado").getByRole("button", { name: /Rechazar/ })).toBeVisible();

    await page.getByRole("button", { name: "Aprobadas" }).click();
    await expect(cards(page)).toHaveCount(1);
    await expect(cards(page).first()).toContainText("Ya aprobada");
    await expect(page.getByRole("button", { name: /Aprobar la petición/ })).toHaveCount(0);
    await page.getByRole("button", { name: "Rechazadas" }).click();
    await expect(cards(page)).toHaveCount(1);
    await page.getByRole("button", { name: "Todas" }).click();
    await expect(cards(page)).toHaveCount(4);

    if (!isDesktop(page.viewportSize()?.width)) await page.getByRole("button", { name: "Abrir menú" }).click();
    await expect(page.getByRole("navigation", { name: "Módulos" }).getByTestId("pending-requests-badge").first()).toHaveText("2");
  });

  test("approves as a Gasto that fits: the bar shows what is spent and the request, then registers the Gasto", async ({ page }) => {
    const api = await openAsOwner(page, { expenseRequests: [TACOS], expenses: [spentOn(1, "Servicios", "1000.00")] });
    await cardOf(page, "Tacos del sábado").getByRole("button", { name: /Aprobar/ }).click();
    const dialog = page.getByRole("dialog", { name: "Aprobar petición" });
    // The suggested category is the default.
    await expect(dialog.getByLabel("Categoría del gasto")).toHaveValue("Servicios");
    const fit = dialog.getByTestId("budget-fit");
    await expect(fit).toContainText("Presupuesto $2,000.00 · Gastado $1,000.00 · Restante $1,000.00");
    await expect(fit).toContainText("Con esta petición: $1,500.00 de $2,000.00");
    await expect(fit).toContainText("Cabe en el presupuesto");
    await expect(fit.getByRole("img")).toHaveAccessibleName(/cabe en el presupuesto/);
    expect(api.budgetChecks.at(-1)).toEqual({ id: 1, category: "Servicios", date: today() });

    await dialog.getByRole("button", { name: "Aprobar" }).click();
    await expect(dialog).toHaveCount(0);
    await expect(page.getByText(/se registró un gasto de \$500\.00 en Servicios/)).toBeVisible();
    expect(api.expenses.at(-1)).toMatchObject({ description: "Tacos del sábado", category: "Servicios", amount: "500.00", date: today() });
    expect(api.requestWrites.at(-1)).toMatchObject({ path: "/expense-requests/1/approve", body: { destination: "gasto", category: "Servicios", date: today() } });

    await page.getByRole("button", { name: "Aprobadas" }).click();
    await expect(cardOf(page, "Tacos del sábado").getByTestId("request-state")).toHaveText("Aprobada");
    await expect(cardOf(page, "Tacos del sábado")).toContainText("Se registró como gasto.");
  });

  test("shows the excess when the request exceeds the budget, and still approves it", async ({ page }) => {
    const api = await openAsOwner(page, { expenseRequests: [TACOS], expenses: [spentOn(1, "Servicios", "1800.00")] });
    await cardOf(page, "Tacos del sábado").getByRole("button", { name: /Aprobar/ }).click();
    const dialog = page.getByRole("dialog", { name: "Aprobar petición" });
    const fit = dialog.getByTestId("budget-fit");
    await expect(fit).toContainText("Presupuesto $2,000.00 · Gastado $1,800.00 · Restante $200.00");
    await expect(fit).toContainText("Con esta petición: $2,300.00 de $2,000.00");
    await expect(fit).toContainText("Excede el presupuesto por $300.00");
    await expect(fit).toContainText("Aun así puedes aprobarla");
    await expect(fit).not.toContainText("Cabe en el presupuesto");
    await expect(fit.getByRole("list", { name: "Leyenda de la barra" })).toContainText("Esta petición");
    await expect(fit.getByRole("list", { name: "Leyenda de la barra" })).toContainText("Excedente");

    // Nothing blocks it.
    await dialog.getByRole("button", { name: "Aprobar" }).click();
    await expect(dialog).toHaveCount(0);
    await expect(page.getByText(/ahora excede su presupuesto/)).toBeVisible();
    expect(api.expenses.at(-1)).toMatchObject({ category: "Servicios", amount: "500.00" });
  });

  test("says there is no budget and re-checks live when the category or the date change", async ({ page }) => {
    const api = await openAsOwner(page, { expenseRequests: [NETFLIX], expenses: [spentOn(1, "Servicios", "1800.00")] });
    await cardOf(page, "Plan de streaming").getByRole("button", { name: /Aprobar/ }).click();
    const dialog = page.getByRole("dialog", { name: "Aprobar petición" });
    await expect(dialog.getByLabel("Categoría del gasto")).toHaveValue("Suscripciones");
    await expect(dialog.getByTestId("budget-fit")).toContainText("Sin presupuesto para esta categoría");
    await expect(dialog.getByTestId("budget-fit")).not.toContainText("Excede");

    await dialog.getByLabel("Categoría del gasto").selectOption("Servicios");
    await expect(dialog.getByTestId("budget-fit")).toContainText("Con esta petición: $2,019.00 de $2,000.00");
    await expect(dialog.getByTestId("budget-fit")).toContainText("Excede el presupuesto por $19.00");
    expect(api.budgetChecks.at(-1)).toMatchObject({ category: "Servicios" });

    // A date in another cycle does not count what was spent in this one.
    await dialog.getByLabel("Fecha del gasto").fill("2030-01-15");
    await expect(dialog.getByTestId("budget-fit")).toContainText("Gastado $0.00");
    await expect(dialog.getByTestId("budget-fit")).toContainText("Cabe en el presupuesto");
    expect(api.budgetChecks.at(-1)).toEqual({ id: 2, category: "Servicios", date: "2030-01-15" });
  });

  test("moves a request to a future expense without any budget check", async ({ page }) => {
    const api = await openAsOwner(page, { expenseRequests: [GIFT] });
    await cardOf(page, "Regalo de aniversario").getByRole("button", { name: /Aprobar/ }).click();
    const dialog = page.getByRole("dialog", { name: "Aprobar petición" });
    await expect(dialog.getByTestId("budget-fit")).toBeVisible();
    const checks = api.budgetChecks.length;

    await dialog.getByRole("radio", { name: /Mover a gasto futuro/ }).check();
    await expect(dialog.getByTestId("budget-fit")).toHaveCount(0);
    await expect(dialog.getByText(/No cuenta contra el presupuesto de este ciclo/)).toBeVisible();

    await dialog.getByRole("button", { name: "Aprobar" }).click();
    await expect(dialog.getByText("Elige la fecha de vencimiento.")).toBeVisible();
    await dialog.getByLabel("Fecha de vencimiento").fill("2026-12-20");
    await dialog.getByRole("button", { name: "Aprobar" }).click();
    await expect(dialog).toHaveCount(0);
    await expect(page.getByText(/se movió a gastos futuros/)).toBeVisible();
    expect(api.requestWrites.at(-1)).toMatchObject({ body: { destination: "gasto_futuro", due_date: "2026-12-20" } });
    expect(api.budgetChecks.length).toBe(checks);
    expect(api.expenses).toHaveLength(0);

    // The item exists in the Future Expenses module.
    await page.goto("/gastos-futuros");
    const row = page.getByRole("list", { name: "Gastos futuros activos" }).getByRole("listitem").filter({ hasText: "Regalo de aniversario" });
    await expect(row).toContainText("$1,800.00");
  });

  test("rejects with a required comment that the requester then reads", async ({ page }) => {
    const api = await openAsOwner(page, { expenseRequests: [TACOS] });
    await cardOf(page, "Tacos del sábado").getByRole("button", { name: /Rechazar/ }).click();
    const dialog = page.getByRole("dialog", { name: "Rechazar petición" });
    await expect(dialog.getByLabel("Comentario")).toBeFocused();
    await dialog.getByRole("button", { name: "Rechazar petición" }).click();
    await expect(dialog.getByText(/Escribe el motivo del rechazo/)).toBeVisible();
    expect(api.requestWrites).toHaveLength(0);

    await dialog.getByLabel("Comentario").fill("Mejor el mes que entra");
    await dialog.getByRole("button", { name: "Rechazar petición" }).click();
    await expect(dialog).toHaveCount(0);
    await expect(page.getByText(/Rechazaste Tacos del sábado/)).toBeVisible();
    expect(api.requestWrites.at(-1)).toMatchObject({ path: "/expense-requests/1/reject", body: { comment: "Mejor el mes que entra" } });
    await page.getByRole("button", { name: "Rechazadas" }).click();
    await expect(cardOf(page, "Tacos del sábado")).toContainText("Comentario: Mejor el mes que entra");
  });

  test("a request decided meanwhile is a conflict, not a second Gasto", async ({ page }) => {
    const api = await openAsOwner(page, { expenseRequests: [TACOS] });
    await cardOf(page, "Tacos del sábado").getByRole("button", { name: /Aprobar/ }).click();
    const dialog = page.getByRole("dialog", { name: "Aprobar petición" });
    await expect(dialog.getByTestId("budget-fit")).toBeVisible();
    api.expenseRequests[0].status = "cancelada"; // the requester cancelled it meanwhile
    await dialog.getByRole("button", { name: "Aprobar" }).click();
    await expect(dialog.getByRole("alert")).toContainText("ya fue resuelta");
    expect(api.expenses).toHaveLength(0);
  });

  test("keeps the page alive when the list fails", async ({ page }) => {
    await openAsOwner(page, { expenseRequests: [TACOS], expenseRequestsFail: "list" });
    await expect(page.getByRole("alert")).toBeVisible();
    await expect(page.getByRole("button", { name: "Reintentar" })).toBeVisible();
  });

  test("reverts a Gasto approval: the dialog says what is deleted and the request goes back to the pending list", async ({ page }) => {
    const api = await openAsOwner(page, {
      expenses: [spentOn(1, "Servicios", "500.00")],
      expenseRequests: [{ ...TACOS, status: "aprobada", result_kind: "gasto", result_movement_id: 1 }, NETFLIX],
    });
    await expect(cards(page)).toHaveCount(1); // only Netflix is pending
    await page.getByRole("button", { name: "Aprobadas" }).click();
    await cardOf(page, "Tacos del sábado").getByRole("button", { name: "Volver a solicitada la petición Tacos del sábado" }).click();
    const dialog = page.getByRole("dialog", { name: "Volver a solicitada" });
    await expect(dialog).toContainText("Se eliminará el gasto registrado de $500.00.");

    // Cancelling the dialog changes nothing.
    await dialog.getByRole("button", { name: "Cancelar" }).click();
    expect(api.requestWrites).toHaveLength(0);
    expect(api.expenses).toHaveLength(1);

    await cardOf(page, "Tacos del sábado").getByRole("button", { name: /Volver a solicitada/ }).click();
    await dialog.getByRole("button", { name: "Volver a solicitada" }).click();
    await expect(dialog).toHaveCount(0);
    await expect(page.getByText(/Tacos del sábado volvió a solicitada/)).toBeVisible();
    expect(api.requestWrites.at(-1)).toMatchObject({ method: "POST", path: "/expense-requests/1/revert" });
    expect(api.expenses).toHaveLength(0);
    await expect(cards(page)).toHaveCount(0); // it left the Aprobadas list

    await page.getByRole("button", { name: "Solicitadas" }).click();
    await expect(cards(page)).toHaveCount(2);
    await expect(cardOf(page, "Tacos del sábado").getByTestId("request-state")).toHaveText("Solicitada");
    await expect(cardOf(page, "Tacos del sábado").getByTestId("revert-history")).toContainText("una vez");
    await expect(cardOf(page, "Tacos del sábado").getByRole("button", { name: /Aprobar/ })).toBeVisible();
    if (!isDesktop(page.viewportSize()?.width)) await page.getByRole("button", { name: "Abrir menú" }).click();
    await expect(page.getByRole("navigation", { name: "Módulos" }).getByTestId("pending-requests-badge").first()).toHaveText("2");
  });

  test("reverts a future expense approval: the item disappears and its savings go back to the free balance", async ({ page }) => {
    const api = await openAsOwner(page, {
      futureExpenses: [{ id: 1, name: "Regalo de aniversario", target_amount: "1800.00", due_date: "2026-12-20", saved: "300.00" }],
      futureFreeBalance: "50.00",
      expenseRequests: [{ ...GIFT, status: "aprobada", result_kind: "gasto_futuro", result_future_expense_id: 1 }],
    });
    await page.getByRole("button", { name: "Aprobadas" }).click();
    await cardOf(page, "Regalo de aniversario").getByRole("button", { name: /Volver a solicitada/ }).click();
    const dialog = page.getByRole("dialog", { name: "Volver a solicitada" });
    await expect(dialog).toContainText("Se eliminará el gasto futuro «Regalo de aniversario»; su ahorro asignado vuelve al saldo libre.");
    await dialog.getByRole("button", { name: "Volver a solicitada" }).click();
    await expect(dialog).toHaveCount(0);
    expect(api.requestWrites.at(-1)).toMatchObject({ path: "/expense-requests/3/revert" });

    await page.goto("/gastos-futuros");
    await expect(page.getByRole("region", { name: "Saldo libre" })).toContainText("$350.00");
    await expect(page.getByRole("list", { name: "Gastos futuros activos" }).getByRole("listitem").filter({ hasText: "Regalo de aniversario" })).toHaveCount(0);
  });

  test("refuses to revert a future expense that was already paid and explains why", async ({ page }) => {
    const api = await openAsOwner(page, {
      futureExpenses: [{ id: 1, name: "Regalo de aniversario", target_amount: "1800.00", due_date: "2026-12-20", status: "paid", paid_at: "2026-12-20", amount_paid: "1800.00" }],
      expenseRequests: [{ ...GIFT, status: "aprobada", result_kind: "gasto_futuro", result_future_expense_id: 1 }],
    });
    await page.getByRole("button", { name: "Aprobadas" }).click();
    await cardOf(page, "Regalo de aniversario").getByRole("button", { name: /Volver a solicitada/ }).click();
    const dialog = page.getByRole("dialog", { name: "Volver a solicitada" });
    await dialog.getByRole("button", { name: "Volver a solicitada" }).click();
    await expect(dialog.getByRole("alert")).toContainText("ese gasto futuro ya se pagó");
    await expect(dialog.getByRole("alert")).toContainText("deshaz ese pago");
    expect(api.expenseRequests[0].status).toBe("aprobada");

    // The page is alive: close the dialog and the request is still approved.
    await dialog.getByRole("button", { name: "Cancelar" }).click();
    await expect(cardOf(page, "Regalo de aniversario").getByTestId("request-state")).toHaveText("Aprobada");
  });

  test("a request that is no longer approved is a conflict, not a crash", async ({ page }) => {
    const api = await openAsOwner(page, { expenseRequests: [{ ...TACOS, status: "aprobada", result_kind: "gasto" }] });
    await page.getByRole("button", { name: "Aprobadas" }).click();
    await cardOf(page, "Tacos del sábado").getByRole("button", { name: /Volver a solicitada/ }).click();
    const dialog = page.getByRole("dialog", { name: "Volver a solicitada" });
    api.expenseRequests[0].status = "solicitada"; // reverted from another tab meanwhile
    await dialog.getByRole("button", { name: "Volver a solicitada" }).click();
    await expect(dialog.getByRole("alert")).toContainText("ya no está aprobada");
    await expect(page.getByRole("heading", { level: 1, name: "Peticiones" })).toBeVisible();
  });

  test("has no horizontal overflow at 375px with the revert dialog open, and keeps 44px touch targets", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    const long = { ...TACOS, description: "Una descripción larguísima ".repeat(4).trim(), amount: "99999999.99", status: "aprobada" as const, result_kind: "gasto" as const };
    await openAsOwner(page, { expenseRequests: [long] });
    await page.getByRole("button", { name: "Aprobadas" }).click();
    await expectNoHorizontalOverflow(page, "approved requests at 375px");
    const action = cards(page).first().getByRole("button", { name: /Volver a solicitada/ });
    expect((await action.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(43);
    await action.click();
    const dialog = page.getByRole("dialog", { name: "Volver a solicitada" });
    await expect(dialog).toBeVisible();
    await expectNoHorizontalOverflow(page, "revert dialog at 375px");
    for (const control of await dialog.getByRole("button").all()) {
      const b = await control.boundingBox();
      if (b) expect(b.height, `touch target of "${await control.innerText()}"`).toBeGreaterThanOrEqual(43);
    }
  });

  test("has no horizontal overflow at 375px, with the approve dialog open", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    const long = { ...TACOS, description: "Una descripción larguísima ".repeat(4).trim(), amount: "99999999.99" };
    await openAsOwner(page, { expenseRequests: [long, NETFLIX], expenses: [spentOn(1, "Servicios", "1800.00")] });
    await expectNoHorizontalOverflow(page, "owner requests at 375px");
    await cards(page).first().getByRole("button", { name: /Aprobar/ }).click();
    const dialog = page.getByRole("dialog", { name: "Aprobar petición" });
    await expect(dialog.getByTestId("budget-fit")).toBeVisible();
    await expectNoHorizontalOverflow(page, "approve dialog at 375px");
    const box = await dialog.getByTestId("budget-fit").boundingBox();
    expect((box?.x ?? 0) + (box?.width ?? 0)).toBeLessThanOrEqual(375);
    for (const control of await dialog.getByRole("button").all()) {
      const b = await control.boundingBox();
      if (b) expect(b.height, `touch target of "${await control.innerText()}"`).toBeGreaterThanOrEqual(43);
    }
  });
});
