import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockExpenseRequest, type MockPersonalFuture, type MockPersonalWish } from "./helpers";

const pad = (n: number) => String(n).padStart(2, "0");

/** Local calendar date `offset` days from today, as YYYY-MM-DD. */
function day(offset = 0) {
  const d = new Date();
  d.setDate(d.getDate() + offset);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

const isDesktop = (width: number | undefined) => (width ?? 0) >= 1024;

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({ scrollWidth: document.documentElement.scrollWidth, clientWidth: document.documentElement.clientWidth }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

/** Obviously fake examples: the repository is public. */
const BOLSA: MockPersonalWish = { id: 1, name: "Bolsa roja", price: "499.90", category: "Ropa", priority: "alta", store_url: "https://tienda.example/bolsa", note: "La de piel" };
const LABIAL: MockPersonalWish = { id: 2, name: "Labial mate", price: "180.00", category: "Maquillaje", priority: "baja" };
const CREMA: MockPersonalWish = { id: 3, name: "Crema facial", price: "320.00", category: "Belleza", priority: "media" };

async function open(page: Page, wishlist: MockPersonalWish[] = [], extra: { expenseRequests?: MockExpenseRequest[]; personalFuture?: MockPersonalFuture[]; expenseRequestsFail?: "rate_limit" } = {}) {
  const api = await mockApi(page, { role: "household", personalWishlist: wishlist, ...extra });
  await seedSession(page, "household");
  await page.goto("/lista-de-deseos");
  await expect(page.getByRole("heading", { level: 1, name: "Lista de deseos" })).toBeVisible();
  return api;
}

const activeList = (page: Page) => page.getByRole("list", { name: "Deseos activos" });
const convertedList = (page: Page) => page.getByRole("list", { name: "Deseos convertidos" });
const cardOf = (list: ReturnType<typeof activeList>, name: string) => list.getByRole("listitem").filter({ hasText: name }).first();
const dialog = (page: Page) => page.getByRole("dialog");

test.describe("household: Lista de deseos", () => {
  test("is a tab of hers and lists active items by priority, with price, category, link and note", async ({ page }) => {
    await open(page, [LABIAL, BOLSA, CREMA]);
    if (!isDesktop(page.viewportSize()?.width)) await page.getByRole("button", { name: "Abrir menú" }).click();
    await expect(page.getByRole("navigation", { name: "Módulos" }).getByRole("link", { name: "Lista de deseos" })).toHaveAttribute("aria-current", "page");
    if (!isDesktop(page.viewportSize()?.width)) await page.keyboard.press("Escape");

    const cards = activeList(page).getByRole("listitem");
    await expect(cards).toHaveCount(3);
    await expect(cards.nth(0)).toContainText("Bolsa roja");
    await expect(cards.nth(1)).toContainText("Crema facial");
    await expect(cards.nth(2)).toContainText("Labial mate");
    await expect(cards.nth(0).getByTestId("wish-priority")).toHaveText("Prioridad alta");
    await expect(cards.nth(2).getByTestId("wish-priority")).toHaveText("Prioridad baja");
    await expect(cards.nth(0)).toContainText("$499.90 MXN · Ropa");
    await expect(cards.nth(0)).toContainText("La de piel");
    const link = cards.nth(0).getByRole("link", { name: /Ver en la tienda/ });
    await expect(link).toHaveAttribute("href", "https://tienda.example/bolsa");
    await expect(link).toHaveAttribute("target", "_blank");
    await expect(link).toHaveAttribute("rel", "noopener noreferrer");
    await expect(page.getByText("Total estimado:")).toContainText("$999.90 MXN");
    await expect(convertedList(page)).toHaveCount(0);
    await expectNoHorizontalOverflow(page, "Lista de deseos");
  });

  test("fits a 375px phone", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await open(page, [BOLSA, LABIAL]);
    await expect(cardOf(activeList(page), "Bolsa roja")).toBeVisible();
    await expectNoHorizontalOverflow(page, "375px Lista de deseos");
  });

  test("creates an item from the collapsed form and checks the link", async ({ page }) => {
    const api = await open(page);
    await expect(page.getByText("Aún no tienes deseos pendientes.")).toBeVisible();
    await expect(page.getByLabel("Nombre")).toHaveCount(0);
    await page.getByRole("button", { name: "Nuevo deseo" }).click();

    await page.getByRole("button", { name: "Agregar deseo" }).click();
    await expect(page.getByText("Escribe qué quieres, por ejemplo Bolsa o Zapatos.")).toBeVisible();
    await page.getByLabel("Nombre").fill("Zapatos blancos");
    await page.getByLabel("Precio estimado (MXN)").fill("1200");
    await page.getByLabel("Categoría").selectOption({ label: "Ropa" });
    await page.getByLabel("Enlace de la tienda (opcional)").fill("javascript:alert(1)");
    await page.getByRole("button", { name: "Agregar deseo" }).click();
    await expect(page.getByText("Escribe un enlace que empiece con http:// o https://.")).toBeVisible();
    await page.getByLabel("Enlace de la tienda (opcional)").fill("https://tienda.example/zapatos");
    await page.getByLabel("Prioridad").selectOption({ label: "Alta" });
    await page.getByLabel("Nota (opcional)").fill("Talla 24");
    await expectNoHorizontalOverflow(page, "wish form");
    await page.getByRole("button", { name: "Agregar deseo" }).click();

    await expect(page.getByText("Zapatos blancos se agregó a tu lista.")).toBeVisible();
    const card = cardOf(activeList(page), "Zapatos blancos");
    await expect(card).toContainText("$1,200.00 MXN · Ropa");
    await expect(card.getByTestId("wish-priority")).toHaveText("Prioridad alta");
    expect((await api).personalWishWrites[0]).toMatchObject({
      method: "POST",
      path: "/personal/wishlist",
      body: { name: "Zapatos blancos", price: "1200", priority: "alta", store_url: "https://tienda.example/zapatos", note: "Talla 24" },
    });
    expect(typeof ((await api).personalWishWrites[0].body as { category_id: unknown }).category_id).toBe("number");
  });

  test("edits an item and deletes another after confirming", async ({ page }) => {
    const api = await open(page, [BOLSA, LABIAL]);
    await page.getByRole("button", { name: "Editar Bolsa roja" }).click();
    await expect(page.getByLabel("Nombre")).toHaveValue("Bolsa roja");
    await page.getByLabel("Precio estimado (MXN)").fill("450");
    await page.getByLabel("Prioridad").selectOption({ label: "Baja" });
    await page.getByRole("button", { name: "Guardar cambios" }).click();
    await expect(page.getByText("Cambios de Bolsa roja guardados.")).toBeVisible();
    await expect(cardOf(activeList(page), "Bolsa roja")).toContainText("$450.00 MXN");
    expect((await api).personalWishWrites.at(-1)).toMatchObject({ method: "PUT", path: "/personal/wishlist/1", body: { price: "450", priority: "baja" } });

    await page.getByRole("button", { name: "Eliminar Labial mate" }).click();
    await expect(page.getByText("¿Eliminar este deseo?")).toBeVisible();
    await page.getByRole("button", { name: "Cancelar", exact: true }).click();
    await expect(cardOf(activeList(page), "Labial mate")).toBeVisible();
    await page.getByRole("button", { name: "Eliminar Labial mate" }).click();
    await page.getByRole("button", { name: "Sí, eliminar" }).click();
    await expect(page.getByText("Labial mate se eliminó de tu lista.")).toBeVisible();
    await expect(activeList(page).getByRole("listitem")).toHaveCount(1);
    expect((await api).personalWishWrites.at(-1)).toMatchObject({ method: "DELETE", path: "/personal/wishlist/2" });
  });

  test("converts to a future expense, which shows on Mis gastos futuros; deleting it brings the item back", async ({ page }) => {
    const api = await open(page, [BOLSA, CREMA]);
    await page.getByRole("button", { name: "Convertir Bolsa roja" }).click();
    await expect(dialog(page).getByRole("radio")).toHaveCount(3);
    await expect(dialog(page).getByLabel("Monto a juntar (MXN)")).toHaveValue("499.90");
    await expectNoHorizontalOverflow(page, "convert dialog");
    await dialog(page).getByRole("button", { name: "Convertir", exact: true }).click();
    await expect(dialog(page).getByText("Elige la fecha de vencimiento.")).toBeVisible();
    await dialog(page).getByLabel("Fecha de vencimiento").fill(day(60));
    await dialog(page).getByRole("button", { name: "Convertir", exact: true }).click();
    await expect(dialog(page)).toHaveCount(0);

    await expect(page.getByText("Bolsa roja se convirtió en un gasto futuro.")).toBeVisible();
    await expect(activeList(page).getByRole("listitem")).toHaveCount(1);
    const card = cardOf(convertedList(page), "Bolsa roja");
    await expect(card).toContainText("Se convirtió en un gasto futuro.");
    await expect(card.getByRole("button", { name: /Convertir/ })).toHaveCount(0);
    expect((await api).personalWishWrites.at(-1)).toMatchObject({
      path: "/personal/wishlist/1/convert/future-expense",
      body: { target_amount: "499.90", due_date: day(60) },
    });

    await card.getByRole("link", { name: "Ver en Mis gastos futuros" }).click();
    await expect(page).toHaveURL(/\/mis-gastos-futuros$/);
    const future = page.getByRole("list", { name: "Gastos futuros activos" }).getByRole("listitem").filter({ hasText: "Bolsa roja" });
    await expect(future).toContainText("$499.90");

    // Deleting the future expense reactivates the wish.
    await future.getByRole("button", { name: /Eliminar/ }).click();
    await page.getByRole("button", { name: "Sí, eliminar" }).click();
    await expect(future).toHaveCount(0);
    await page.goto("/lista-de-deseos");
    await expect(activeList(page).getByRole("listitem")).toHaveCount(2);
    await expect(convertedList(page)).toHaveCount(0);
    await expect(cardOf(activeList(page), "Bolsa roja")).toBeVisible();
  });

  test("converts to her Gasto with her own categories, and deleting the Gasto brings the item back", async ({ page }) => {
    const api = await open(page, [BOLSA]);
    await page.getByRole("button", { name: "Convertir Bolsa roja" }).click();
    await dialog(page).getByRole("radio", { name: /Ya lo compré/ }).check();
    await expect(dialog(page).getByLabel("Monto (MXN)")).toHaveValue("499.90");
    await expect(dialog(page).getByLabel("Fecha")).toHaveValue(day(0));
    await expect(dialog(page).getByLabel("Categoría").locator("option")).toHaveText([
      "Belleza", "Salud", "Tratamientos", "Tienda de conveniencia", "Ropa", "Pintura", "Maquillaje", "Compras en línea",
    ]);
    await expect(dialog(page).getByLabel("Categoría")).toHaveValue(await dialog(page).getByLabel("Categoría").locator("option", { hasText: "Ropa" }).getAttribute("value") ?? "");
    await dialog(page).getByLabel("Monto (MXN)").fill("450");
    await dialog(page).getByRole("button", { name: "Convertir", exact: true }).click();
    await expect(dialog(page)).toHaveCount(0);

    const card = cardOf(convertedList(page), "Bolsa roja");
    await expect(card).toContainText("Ya lo compraste: se registró como gasto.");
    expect((await api).personalWishWrites.at(-1)).toMatchObject({ path: "/personal/wishlist/1/convert/expense", body: { amount: "450", date: day(0) } });

    await card.getByRole("link", { name: "Ver en Mis gastos" }).click();
    await expect(page).toHaveURL(/\/mis-gastos$/);
    const expense = page.getByRole("listitem").filter({ hasText: "Bolsa roja" });
    await expect(expense).toContainText("$450.00");
    await expect(expense).toContainText("Ropa");

    await expense.getByRole("button", { name: "Eliminar Bolsa roja" }).click();
    await page.getByRole("button", { name: "Sí, eliminar" }).click();
    await expect(expense).toHaveCount(0);
    await page.goto("/lista-de-deseos");
    await expect(cardOf(activeList(page), "Bolsa roja")).toBeVisible();
    await expect(convertedList(page)).toHaveCount(0);
  });

  test("asks the owner with an owner category; cancelling the request brings the item back", async ({ page }) => {
    const api = await open(page, [BOLSA]);
    await page.getByRole("button", { name: "Convertir Bolsa roja" }).click();
    await dialog(page).getByRole("radio", { name: /Pedir al titular/ }).check();
    await dialog(page).getByRole("button", { name: "Enviar petición" }).click();
    await expect(dialog(page).getByText("Elige la categoría con la que se pedirá.")).toBeVisible();
    // The owner's categories come from the requests endpoint, not from hers.
    const options = dialog(page).getByLabel("Categoría de la petición").locator("option");
    await expect(options.first()).toHaveText("Elige una categoría");
    await expect(options).not.toContainText(["Belleza"]);
    await dialog(page).getByLabel("Categoría de la petición").selectOption({ index: 1 });
    const chosen = await dialog(page).getByLabel("Categoría de la petición").inputValue();
    await dialog(page).getByRole("button", { name: "Enviar petición" }).click();
    await expect(dialog(page)).toHaveCount(0);

    const card = cardOf(convertedList(page), "Bolsa roja");
    await expect(card).toContainText("Se pidió al titular.");
    await expect(card.getByTestId("request-state")).toHaveText("Solicitada");
    expect((await api).requestWrites.find((w) => w.path === "/expense-requests")).toMatchObject({
      method: "POST",
      body: { amount: "499.90", description: "Bolsa roja", suggested_category: chosen, date: day(0) },
    });

    // The request is really hers on Peticiones; cancelling it there frees the item.
    await card.getByRole("link", { name: "Ver en Peticiones" }).click();
    await expect(page).toHaveURL(/\/peticiones$/);
    await expect(page.getByTestId("request-card").filter({ hasText: "Bolsa roja" })).toContainText("Solicitada");
    await page.getByRole("button", { name: "Cancelar la petición Bolsa roja" }).click();
    await expect(page.getByTestId("request-card").filter({ hasText: "Bolsa roja" })).toContainText("Cancelada");

    await page.goto("/lista-de-deseos");
    await expect(cardOf(activeList(page), "Bolsa roja")).toContainText("Cancelaste tu petición: puedes convertirlo de nuevo.");
    await expect(convertedList(page)).toHaveCount(0);
  });

  test("an item whose request was rejected is active again and can be converted again", async ({ page }) => {
    const api = await open(page, [{ ...BOLSA, request_id: 7 }, { ...CREMA, id: 4, request_id: 8 }], {
      expenseRequests: [
        { id: 7, amount: "499.90", description: "Bolsa roja", status: "rechazada", decision_comment: "Ahora no" },
        { id: 8, amount: "320.00", description: "Crema facial", status: "aprobada", result_kind: "gasto" },
      ],
    });
    const bolsa = cardOf(activeList(page), "Bolsa roja");
    await expect(bolsa).toContainText("El titular rechazó tu petición: puedes convertirlo de nuevo.");
    await expect(cardOf(convertedList(page), "Crema facial").getByTestId("request-state")).toHaveText("Aprobada");

    await bolsa.getByRole("button", { name: "Convertir Bolsa roja" }).click();
    await dialog(page).getByRole("radio", { name: /Ya lo compré/ }).check();
    await dialog(page).getByRole("button", { name: "Convertir", exact: true }).click();
    await expect(dialog(page)).toHaveCount(0);
    await expect(cardOf(convertedList(page), "Bolsa roja")).toContainText("Ya lo compraste");
    expect((await api).personalWishWrites.at(-1)).toMatchObject({ path: "/personal/wishlist/1/convert/expense" });
  });

  test("shows the rate limit of requests in Spanish and keeps the item active", async ({ page }) => {
    await open(page, [BOLSA], { expenseRequestsFail: "rate_limit" });
    await page.getByRole("button", { name: "Convertir Bolsa roja" }).click();
    await dialog(page).getByRole("radio", { name: /Pedir al titular/ }).check();
    await dialog(page).getByLabel("Categoría de la petición").selectOption({ index: 1 });
    await dialog(page).getByRole("button", { name: "Enviar petición" }).click();
    await expect(dialog(page).getByRole("alert")).toContainText("Enviaste demasiadas peticiones seguidas. Espera un rato e intenta de nuevo.");
    await dialog(page).getByRole("button", { name: "Cancelar", exact: true }).click();
    await expect(cardOf(activeList(page), "Bolsa roja")).toBeVisible();
    await expect(convertedList(page)).toHaveCount(0);
  });
});

test.describe("owner session", () => {
  test("does not see Lista de deseos and is redirected from its path", async ({ page }) => {
    await mockApi(page);
    await seedSession(page, "owner");
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 2, name: "Resumen del mes" })).toBeVisible();
    if (!isDesktop(page.viewportSize()?.width)) await page.getByRole("button", { name: "Abrir menú" }).click();
    expect(await page.getByRole("navigation", { name: "Módulos" }).getByRole("link").allInnerTexts()).not.toContain("Lista de deseos");
    await page.goto("/lista-de-deseos");
    await expect(page).toHaveURL(/\/$/);
  });
});
