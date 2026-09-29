import { expect, test, type Page } from "@playwright/test";
import { goToPasswordStep, mockApi } from "./helpers";

const EMAIL = "admin@example.com";
const RATE_LIMITED = "Demasiados intentos. Inténtalo de nuevo en unos minutos.";

const emailField = (page: Page) => page.getByLabel("Correo electrónico");
const passwordField = (page: Page) => page.getByLabel("Contraseña", { exact: true });
const stepLabel = (page: Page) => page.getByTestId("login-step");

async function submitEmail(page: Page, email = EMAIL) {
  await emailField(page).fill(email);
  await page.getByRole("button", { name: "Continuar" }).click();
}

async function expectNoHorizontalOverflow(page: Page) {
  const m = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(m.scrollWidth).toBeLessThanOrEqual(m.clientWidth);
}

test.describe("login, step 1 (email)", () => {
  test("shows only the email form with a visible step indicator", async ({ page }) => {
    await mockApi(page);
    await page.goto("/login");
    await expect(page.getByRole("heading", { level: 1, name: "Iniciar sesión" })).toBeVisible();
    await expect(stepLabel(page)).toHaveText("Paso 1 de 2");
    await expect(emailField(page)).toBeVisible();
    await expect(emailField(page)).toBeFocused();
    await expect(passwordField(page)).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Continuar" })).toBeVisible();
  });

  test("a verified email advances to the password step", async ({ page }) => {
    const api = await mockApi(page, { identify: "password_required" });
    await page.goto("/login");
    await submitEmail(page, "  Admin@Example.com ");
    await expect(stepLabel(page)).toHaveText("Paso 2 de 2");
    await expect(passwordField(page)).toBeVisible();
    await expect(page.getByRole("button", { name: "Iniciar sesión" })).toBeVisible();
    expect(api.requests).toEqual([{ path: "/auth/identify", body: { email: "Admin@Example.com" } }]);
  });

  test("an unverified or unknown email shows the confirmation panel", async ({ page }) => {
    await mockApi(page, { identify: "verification_sent" });
    await page.goto("/login");
    await submitEmail(page, "new@example.com");
    await expect(page.getByRole("status")).toContainText(
      "Te enviamos un correo para verificar tu cuenta. Revisa tu bandeja y sigue el enlace.",
    );
    await expect(page.getByRole("button", { name: "Usar otro correo" })).toBeVisible();
    await expect(passwordField(page)).toHaveCount(0);
    await expect(page).toHaveURL(/\/login$/);
  });

  test("an empty email is rejected without calling the API", async ({ page }) => {
    const api = await mockApi(page);
    await page.goto("/login");
    await page.getByRole("button", { name: "Continuar" }).click();
    await expect(page.getByRole("alert")).toHaveText("Ingresa tu correo electrónico.");
    await expect(emailField(page)).toBeFocused();
    await expect(emailField(page)).toHaveAttribute("aria-invalid", "true");
    expect(api.requests).toHaveLength(0);
  });

  test("an invalid email (400) shows a message and focuses the field", async ({ page }) => {
    await mockApi(page, { identify: "invalid_email" });
    await page.goto("/login");
    await submitEmail(page, "not-an-email");
    await expect(page.getByRole("alert")).toHaveText("Ingresa un correo electrónico válido.");
    await expect(emailField(page)).toBeFocused();
    await expect(emailField(page)).toHaveAttribute("aria-invalid", "true");
    await expect(stepLabel(page)).toHaveText("Paso 1 de 2");
  });

  test("rate limiting (429) shows a clear message", async ({ page }) => {
    await mockApi(page, { identify: "rate_limited" });
    await page.goto("/login");
    await submitEmail(page);
    await expect(page.getByRole("alert")).toHaveText(RATE_LIMITED);
    await expect(emailField(page)).toBeFocused();
  });

  test("a network error shows a connection message", async ({ page }) => {
    await mockApi(page, { identify: "network_error" });
    await page.goto("/login");
    await submitEmail(page);
    await expect(page.getByRole("alert")).toContainText("No se pudo conectar con el servidor");
    await expect(page.getByRole("button", { name: "Continuar" })).toBeEnabled();
  });

  test("a server error shows a generic message", async ({ page }) => {
    await mockApi(page, { identify: "server_error" });
    await page.goto("/login");
    await submitEmail(page);
    await expect(page.getByRole("alert")).toHaveText("Algo salió mal. Inténtalo de nuevo.");
  });

  test("the error clears when submitting again successfully", async ({ page }) => {
    await mockApi(page, { identify: "password_required" });
    await page.goto("/login");
    await page.getByRole("button", { name: "Continuar" }).click();
    await expect(page.getByRole("alert")).toBeVisible();
    await submitEmail(page);
    await expect(passwordField(page)).toBeVisible();
    await expect(page.getByRole("alert")).toHaveCount(0);
  });
});

test.describe("login, step 2 (password)", () => {
  test("shows the email read-only, the password toggle and the primary button", async ({ page }) => {
    await mockApi(page);
    await goToPasswordStep(page);
    await expect(emailField(page)).toHaveValue(EMAIL);
    await expect(emailField(page)).toHaveAttribute("readonly", "");
    await expect(page.getByRole("button", { name: "Cambiar correo" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Mostrar contraseña" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Iniciar sesión" })).toBeVisible();
  });

  test("logs in with valid credentials and lands on the panel", async ({ page }) => {
    const api = await mockApi(page, { login: "ok" });
    await goToPasswordStep(page);
    await passwordField(page).fill("correct horse battery");
    await page.getByRole("button", { name: "Iniciar sesión" }).click();
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByRole("heading", { level: 1, name: "Panel" })).toBeVisible();
    expect(api.requests.at(-1)).toEqual({
      path: "/auth/login",
      body: { email: EMAIL, password: "correct horse battery" },
    });
  });

  test("shows a message below the field on 401 and focuses the password", async ({ page }) => {
    await mockApi(page, { login: "unauthorized" });
    await goToPasswordStep(page);
    await passwordField(page).fill("wrong password");
    await page.getByRole("button", { name: "Iniciar sesión" }).click();
    const alert = page.getByRole("alert");
    await expect(alert).toHaveText("Correo o contraseña incorrectos.");
    await expect(passwordField(page)).toBeFocused();
    await expect(passwordField(page)).toHaveAttribute("aria-invalid", "true");
    await expect(passwordField(page)).toHaveAttribute("aria-describedby", /.+/);
    await expect(page).toHaveURL(/\/login$/);

    const below = await page.evaluate(() => {
      const input = document.querySelector<HTMLInputElement>('input[name="password"]')!;
      const alertEl = document.querySelector('[role="alert"]')!;
      return alertEl.getBoundingClientRect().top >= input.getBoundingClientRect().bottom;
    });
    expect(below).toBe(true);
  });

  test("refocuses the password on every repeated failure", async ({ page }) => {
    await mockApi(page, { login: "unauthorized" });
    await goToPasswordStep(page);
    for (let i = 0; i < 2; i++) {
      await passwordField(page).fill(`wrong ${i}`);
      await page.getByRole("button", { name: "Iniciar sesión" }).click();
      await expect(page.getByRole("alert")).toBeVisible();
      await expect(passwordField(page)).toBeFocused();
      await page.getByRole("button", { name: "Mostrar contraseña" }).focus();
    }
  });

  test("rate limiting (429) shows a clear message", async ({ page }) => {
    await mockApi(page, { login: "rate_limited" });
    await goToPasswordStep(page);
    await passwordField(page).fill("whatever password");
    await page.getByRole("button", { name: "Iniciar sesión" }).click();
    await expect(page.getByRole("alert")).toHaveText(RATE_LIMITED);
    await expect(passwordField(page)).toBeFocused();
  });

  test("a network error shows a connection message", async ({ page }) => {
    await mockApi(page, { login: "network_error" });
    await goToPasswordStep(page);
    await passwordField(page).fill("whatever password");
    await page.getByRole("button", { name: "Iniciar sesión" }).click();
    await expect(page.getByRole("alert")).toContainText("No se pudo conectar con el servidor");
  });

  test("an empty password is rejected without calling the API", async ({ page }) => {
    const api = await mockApi(page);
    await goToPasswordStep(page);
    const before = api.requests.length;
    await page.getByRole("button", { name: "Iniciar sesión" }).click();
    await expect(page.getByRole("alert")).toHaveText("Ingresa tu contraseña.");
    await expect(passwordField(page)).toBeFocused();
    expect(api.requests).toHaveLength(before);
  });

  test("password toggle switches the input type", async ({ page }) => {
    await mockApi(page);
    await goToPasswordStep(page);
    await expect(passwordField(page)).toHaveAttribute("type", "password");
    await page.getByRole("button", { name: "Mostrar contraseña" }).click();
    await expect(passwordField(page)).toHaveAttribute("type", "text");
  });
});

test.describe("login, navigation between steps", () => {
  test('"Cambiar correo" returns to step 1 keeping the email and dropping the password', async ({ page }) => {
    await mockApi(page);
    await goToPasswordStep(page);
    await passwordField(page).fill("secret value");
    await page.getByRole("button", { name: "Cambiar correo" }).click();
    await expect(stepLabel(page)).toHaveText("Paso 1 de 2");
    await expect(emailField(page)).toHaveValue(EMAIL);
    await expect(emailField(page)).toBeEnabled();
    await expect(emailField(page)).toBeFocused();
    await expect(passwordField(page)).toHaveCount(0);

    await page.getByRole("button", { name: "Continuar" }).click();
    await expect(passwordField(page)).toHaveValue("");
  });

  test('"Usar otro correo" returns to step 1', async ({ page }) => {
    await mockApi(page, { identify: "verification_sent" });
    await page.goto("/login");
    await submitEmail(page, "new@example.com");
    await page.getByRole("button", { name: "Usar otro correo" }).click();
    await expect(stepLabel(page)).toHaveText("Paso 1 de 2");
    await expect(emailField(page)).toHaveValue("new@example.com");
    await expect(emailField(page)).toBeFocused();
    await expect(page.getByRole("status")).toHaveCount(0);
  });

  test("a different email can be tried after going back", async ({ page }) => {
    const api = await mockApi(page);
    await goToPasswordStep(page);
    await page.getByRole("button", { name: "Cambiar correo" }).click();
    await submitEmail(page, "other@example.com");
    await expect(emailField(page)).toHaveValue("other@example.com");
    expect(api.requests.filter((r) => r.path === "/auth/identify").map((r) => r.body)).toEqual([
      { email: EMAIL },
      { email: "other@example.com" },
    ]);
  });
});

test.describe("login, keyboard", () => {
  test("the whole flow works with the keyboard only", async ({ page }) => {
    await mockApi(page, { identify: "password_required", login: "ok" });
    await page.goto("/login");
    await expect(emailField(page)).toBeFocused();
    await page.keyboard.type(EMAIL);
    await page.keyboard.press("Enter");

    await expect(stepLabel(page)).toHaveText("Paso 2 de 2");
    await expect(passwordField(page)).toBeFocused();
    await page.keyboard.type("correct horse battery");
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/\/$/);
  });

  test("focus moves to the confirmation panel and Tab reaches the action", async ({ page }) => {
    await mockApi(page, { identify: "verification_sent" });
    await page.goto("/login");
    await page.keyboard.type("new@example.com");
    await page.keyboard.press("Enter");
    await expect(page.getByRole("status")).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(page.getByRole("button", { name: "Usar otro correo" })).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(emailField(page)).toBeFocused();
  });

  test("Cambiar correo is reachable by keyboard from the read-only email", async ({ page }) => {
    await mockApi(page);
    await goToPasswordStep(page);
    await page.keyboard.press("Shift+Tab"); // password field -> "Cambiar correo"
    await expect(page.getByRole("button", { name: "Cambiar correo" })).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(emailField(page)).toBeFocused();
  });
});

test.describe("login, layout", () => {
  test("has no horizontal overflow on any step", async ({ page }) => {
    await mockApi(page, { identify: "password_required", login: "unauthorized" });
    await page.goto("/login");
    await expectNoHorizontalOverflow(page);

    await page.getByRole("button", { name: "Continuar" }).click(); // empty-email error state
    await expect(page.getByRole("alert")).toBeVisible();
    await expectNoHorizontalOverflow(page);

    await submitEmail(page, "a-very-long-email-address-to-stress-the-layout@subdomain.example-company.com");
    await expect(passwordField(page)).toBeVisible();
    await expectNoHorizontalOverflow(page);

    await passwordField(page).fill("wrong");
    await page.getByRole("button", { name: "Iniciar sesión" }).click();
    await expect(page.getByRole("alert")).toBeVisible();
    await expectNoHorizontalOverflow(page);

  });

  test("the confirmation panel has no horizontal overflow", async ({ page }) => {
    await mockApi(page, { identify: "verification_sent" });
    await page.goto("/login");
    await submitEmail(page, "a-very-long-email-address-to-stress-the-layout@subdomain.example-company.com");
    await expect(page.getByRole("heading", { level: 1, name: "Revisa tu correo" })).toBeVisible();
    await expectNoHorizontalOverflow(page);
  });
});
