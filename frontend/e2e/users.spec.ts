import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockUser } from "./helpers";

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

/** Obviously fake accounts: the repository is public. */
const OWNER: MockUser = { id: "u-owner", email: "admin@example.com", role: "owner", active: true, verified: true };
const PENDING: MockUser = { id: "u-her", email: "her@example.com", role: "household", active: true, verified: false };
const VERIFIED: MockUser = { id: "u-done", email: "done@example.com", role: "household", active: true, verified: true };
const INACTIVE: MockUser = { id: "u-off", email: "off@example.com", role: "household", active: false, verified: true };

async function openUsers(page: Page, opts: Parameters<typeof mockApi>[1] = {}) {
  const api = await mockApi(page, { users: [OWNER, PENDING, VERIFIED, INACTIVE], ...opts });
  await seedSession(page, "owner");
  await page.goto("/configuracion");
  await page.getByRole("tab", { name: "Usuarios" }).click();
  await expect(page.getByRole("heading", { level: 2, name: "Usuarios" })).toBeVisible();
  return api;
}

const rowOf = (page: Page, email: string) => page.getByRole("list", { name: "Usuarios" }).getByRole("listitem").filter({ hasText: email });

test.describe("users section (owner)", () => {
  test("is a sub-tab of Configuración that lists email, role, state and verification", async ({ page }) => {
    await openUsers(page);
    await expect(rowOf(page, OWNER.email)).toContainText("Titular");
    await expect(rowOf(page, OWNER.email)).toContainText("Tu cuenta");
    await expect(rowOf(page, OWNER.email).getByRole("button")).toHaveCount(0);
    await expect(rowOf(page, PENDING.email)).toContainText("Familiar");
    await expect(rowOf(page, PENDING.email)).toContainText("Sin verificar");
    await expect(rowOf(page, PENDING.email)).toContainText("Activo");
    await expect(rowOf(page, VERIFIED.email)).toContainText("Verificado");
    await expect(rowOf(page, INACTIVE.email)).toContainText("Inactivo");
    await expect(rowOf(page, VERIFIED.email).getByRole("button", { name: /invitación/ })).toHaveCount(0);
    await expectNoHorizontalOverflow(page, "users list");
  });

  test("creates a household user behind the collapsed form, without a password", async ({ page }) => {
    const api = await openUsers(page);
    await expect(page.getByLabel("Correo electrónico")).toHaveCount(0);
    await page.getByRole("button", { name: "Nuevo usuario" }).click();
    await expectNoHorizontalOverflow(page, "user form");
    await page.getByLabel("Correo electrónico").fill("New.Person@Example.com");
    await page.getByRole("button", { name: "Crear usuario" }).click();

    await expect(page.getByRole("status").filter({ hasText: "se creó sin contraseña" })).toBeVisible();
    await expect(rowOf(page, "new.person@example.com")).toContainText("Sin verificar");
    const post = api.userWrites.find((w) => w.method === "POST" && w.path === "/users");
    expect(post?.body).toEqual({ email: "New.Person@Example.com", role: "household" });
    expect(JSON.stringify(post?.body)).not.toContain("password");
  });

  test("rejects an invalid email locally and a duplicate one with the server's answer", async ({ page }) => {
    const api = await openUsers(page);
    await page.getByRole("button", { name: "Nuevo usuario" }).click();
    await page.getByLabel("Correo electrónico").fill("not-an-email");
    await page.getByRole("button", { name: "Crear usuario" }).click();
    await expect(page.getByText("Escribe un correo electrónico válido.")).toBeVisible();
    expect(api.userWrites).toHaveLength(0);

    await page.getByLabel("Correo electrónico").fill(PENDING.email);
    await page.getByRole("button", { name: "Crear usuario" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "Ya existe una cuenta con ese correo." })).toBeVisible();
  });

  test("invites after a confirmation and then offers to resend", async ({ page }) => {
    const api = await openUsers(page);
    await rowOf(page, PENDING.email).getByRole("button", { name: /Enviar invitación/ }).click();
    const dialog = page.getByRole("dialog", { name: "Enviar invitación" });
    await expect(dialog).toContainText("1 hora");
    expect(api.userWrites).toHaveLength(0);
    await dialog.getByRole("button", { name: "Enviar" }).click();

    await expect(page.getByRole("status").filter({ hasText: "Invitación enviada a her@example.com" })).toBeVisible();
    expect(api.userWrites.map((w) => `${w.method} ${w.path}`)).toEqual(["POST /users/u-her/invite"]);
    await expect(rowOf(page, PENDING.email).getByRole("button", { name: /Reenviar invitación/ })).toBeVisible();
  });

  test("explains a rate limited invitation and keeps the dialog open", async ({ page }) => {
    await openUsers(page, { usersFail: "invite" });
    await rowOf(page, PENDING.email).getByRole("button", { name: /Enviar invitación/ }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Enviar" }).click();
    await expect(page.getByRole("dialog").getByRole("alert")).toContainText("demasiadas invitaciones");
  });

  test("deactivates and reactivates after confirming", async ({ page }) => {
    const api = await openUsers(page);
    await rowOf(page, PENDING.email).getByRole("button", { name: /Desactivar/ }).click();
    const dialog = page.getByRole("dialog", { name: "Desactivar usuario" });
    await expect(dialog).toContainText("se cerrarán sus sesiones");
    await dialog.getByRole("button", { name: "Cancelar" }).click();
    expect(api.userWrites).toHaveLength(0);

    await rowOf(page, PENDING.email).getByRole("button", { name: /Desactivar/ }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Desactivar" }).click();
    await expect(rowOf(page, PENDING.email)).toContainText("Inactivo");
    // An inactive account is not invited: only Activar is offered.
    await expect(rowOf(page, PENDING.email).getByRole("button", { name: /invitación/ })).toHaveCount(0);

    await rowOf(page, PENDING.email).getByRole("button", { name: /Activar/ }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Activar" }).click();
    await expect(rowOf(page, PENDING.email)).toContainText("Activo");
    expect(api.userWrites.map((w) => w.path)).toEqual(["/users/u-her/deactivate", "/users/u-her/activate"]);
    await expectNoHorizontalOverflow(page, "users after actions");
  });

  test("shows an error with a retry when the list cannot be loaded, without crashing", async ({ page }) => {
    await openUsers(page, { usersFail: "list" });
    await expect(page.getByRole("alert")).toBeVisible();
    await expect(page.getByRole("button", { name: "Reintentar" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "General" })).toBeVisible();
  });

  test("the other settings sections still load next to it", async ({ page }) => {
    await openUsers(page);
    await page.getByRole("tab", { name: "General" }).click();
    await expect(page.getByLabel("Inicio del periodo")).toBeVisible();
  });
});
