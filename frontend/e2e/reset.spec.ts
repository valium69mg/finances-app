import { expect, test, type Page } from "@playwright/test";
import { goToPasswordStep, mockApi } from "./helpers";

const EMAIL = "admin@example.com";
const NEUTRAL =
  "Si la cuenta existe, te enviamos un correo con un enlace para restablecer tu contraseña. El enlace vence en 1 hora.";

const link = (page: Page) => page.getByRole("button", { name: "¿Olvidaste tu contraseña?" });
const newPassword = (page: Page) => page.getByLabel("Nueva contraseña");
const confirmPassword = (page: Page) => page.getByLabel("Confirmar contraseña");
const save = (page: Page) => page.getByRole("button", { name: "Guardar contraseña" });

test.describe("forgot password link (login, password step)", () => {
  test("is shown on the password step only", async ({ page }) => {
    await mockApi(page);
    await page.goto("/login");
    await expect(link(page)).toHaveCount(0);
    await goToPasswordStep(page);
    await expect(link(page)).toBeVisible();
  });

  test("requests a reset for the email already entered and shows the neutral message", async ({ page }) => {
    const api = await mockApi(page);
    await goToPasswordStep(page);
    await link(page).click();
    await expect(page.getByRole("heading", { level: 1, name: "Revisa tu correo" })).toBeVisible();
    await expect(page.getByRole("status")).toContainText(NEUTRAL);
    expect(api.requests.at(-1)).toEqual({ path: "/auth/forgot", body: { email: EMAIL } });
    await expect(page.getByLabel("Contraseña", { exact: true })).toHaveCount(0);
  });

  test("the panel returns to the login keeping the email", async ({ page }) => {
    await mockApi(page);
    await goToPasswordStep(page);
    await link(page).click();
    await page.getByRole("button", { name: "Volver a iniciar sesión" }).click();
    await expect(page.getByLabel("Correo electrónico")).toHaveValue(EMAIL);
    await expect(page.getByTestId("login-step")).toHaveText("Paso 1 de 2");
  });

  test("rate limiting (429) shows a message and stays on the password step", async ({ page }) => {
    await mockApi(page, { forgot: "rate_limited" });
    await goToPasswordStep(page);
    await link(page).click();
    await expect(page.getByRole("alert")).toHaveText("Demasiados intentos. Inténtalo de nuevo en unos minutos.");
    await expect(page.getByTestId("login-step")).toHaveText("Paso 2 de 2");
    await expect(link(page)).toBeEnabled();
  });

  test("a server error shows a generic message", async ({ page }) => {
    await mockApi(page, { forgot: "server_error" });
    await goToPasswordStep(page);
    await link(page).click();
    await expect(page.getByRole("alert")).toHaveText("Algo salió mal. Inténtalo de nuevo.");
  });
});

test.describe("reset page (/restablecer)", () => {
  test("without a token it says the link is not valid", async ({ page }) => {
    await mockApi(page);
    await page.goto("/restablecer");
    await expect(page.getByRole("alert")).toHaveText("El enlace para restablecer la contraseña no es válido.");
    await expect(page.getByRole("link", { name: "Ir a iniciar sesión" })).toBeVisible();
  });

  test("sets the new password, confirms it and leads to the login", async ({ page }) => {
    const api = await mockApi(page);
    await page.goto("/restablecer?token=abc");
    await expect(page.getByRole("heading", { level: 1, name: "Restablecer contraseña" })).toBeVisible();
    await newPassword(page).fill("a brand new password");
    await confirmPassword(page).fill("a brand new password");
    await save(page).click();
    await expect(page.getByRole("heading", { level: 1, name: "Contraseña restablecida" })).toBeVisible();
    await expect(page.getByRole("status")).toContainText("Ya puedes iniciar sesión");
    expect(api.requests.at(-1)).toEqual({ path: "/auth/reset", body: { token: "abc", password: "a brand new password" } });
    await page.getByRole("link", { name: "Ir a iniciar sesión" }).click();
    await expect(page).toHaveURL(/\/login$/);
  });

  test("full path: forgot link, reset, then login with the new password", async ({ page }) => {
    const api = await mockApi(page);
    await goToPasswordStep(page);
    await link(page).click();
    await expect(page.getByRole("status")).toContainText(NEUTRAL);
    await page.goto("/restablecer?token=from-email");
    await newPassword(page).fill("a brand new password");
    await confirmPassword(page).fill("a brand new password");
    await save(page).click();
    await page.getByRole("link", { name: "Ir a iniciar sesión" }).click();
    await goToPasswordStep(page);
    await page.getByLabel("Contraseña", { exact: true }).fill("a brand new password");
    await page.getByRole("button", { name: "Iniciar sesión" }).click();
    await expect(page.getByRole("heading", { level: 1, name: "Panel" })).toBeVisible();
    expect(api.requests.at(-1)).toEqual({ path: "/auth/login", body: { email: EMAIL, password: "a brand new password" } });
  });

  test("a short password is rejected without calling the API", async ({ page }) => {
    const api = await mockApi(page);
    await page.goto("/restablecer?token=abc");
    await newPassword(page).fill("short");
    await confirmPassword(page).fill("short");
    await save(page).click();
    await expect(page.getByRole("alert")).toHaveText("La contraseña debe tener al menos 12 caracteres.");
    expect(api.requests).toHaveLength(0);
  });

  test("a weak password answered by the API keeps the form so the token can be retried", async ({ page }) => {
    await mockApi(page, { reset: "weak_password" });
    await page.goto("/restablecer?token=abc");
    await newPassword(page).fill("123456789012");
    await confirmPassword(page).fill("123456789012");
    await save(page).click();
    await expect(page.getByRole("alert")).toHaveText("La contraseña debe tener al menos 12 caracteres.");
    await expect(save(page)).toBeEnabled();
  });

  test("mismatching confirmation is rejected", async ({ page }) => {
    const api = await mockApi(page);
    await page.goto("/restablecer?token=abc");
    await newPassword(page).fill("a brand new password");
    await confirmPassword(page).fill("another password!");
    await save(page).click();
    await expect(page.getByRole("alert")).toHaveText("Las contraseñas no coinciden.");
    expect(api.requests).toHaveLength(0);
  });

  test("an expired or used token shows the expiry message", async ({ page }) => {
    await mockApi(page, { reset: "invalid_token" });
    await page.goto("/restablecer?token=old");
    await newPassword(page).fill("a brand new password");
    await confirmPassword(page).fill("a brand new password");
    await save(page).click();
    await expect(page.getByRole("alert")).toContainText("El enlace no es válido o ya expiró");
    await expect(page.getByRole("alert")).toContainText("¿Olvidaste tu contraseña?");
  });

  test("rate limiting (429) shows the generic failure", async ({ page }) => {
    await mockApi(page, { reset: "rate_limited" });
    await page.goto("/restablecer?token=abc");
    await newPassword(page).fill("a brand new password");
    await confirmPassword(page).fill("a brand new password");
    await save(page).click();
    await expect(page.getByRole("alert")).toHaveText("No se pudo restablecer la contraseña. Inténtalo de nuevo.");
  });

  test("the verification page keeps its own copy", async ({ page }) => {
    await mockApi(page);
    await page.goto("/verify?token=abc");
    await expect(page.getByRole("heading", { level: 1, name: "Verificar cuenta" })).toBeVisible();
  });
});
