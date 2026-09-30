import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, systemStatus } from "./helpers";

const GB = 1024 ** 3;

async function open(page: Page, opts: Parameters<typeof mockApi>[1] = {}) {
  const api = await mockApi(page, opts);
  await seedSession(page);
  await page.goto("/sistema");
  await expect(page.getByRole("heading", { level: 1, name: "Sistema" })).toBeVisible();
  return api;
}

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

/** A status whose three gauges sit at the given percentages. */
function usage(cpu: number, memory: number, disk: number) {
  return systemStatus({
    cpu_percent: cpu,
    memory: { total_bytes: 2 * GB, used_bytes: (2 * GB * memory) / 100, available_bytes: (2 * GB * (100 - memory)) / 100, used_percent: memory },
    disk: { total_bytes: 50 * GB, used_bytes: (50 * GB * disk) / 100, used_percent: disk, path_monitored: true },
  });
}

test.describe("system page", () => {
  test("is a tab before Configuración and shows the three cards with bars and numbers", async ({ page }) => {
    await open(page);

    const tabs = page.getByRole("navigation", { name: "Módulos" });
    if (!(await tabs.isVisible())) await page.getByRole("button", { name: "Abrir menú" }).click();
    const labels = await tabs.getByRole("link").allInnerTexts();
    expect(labels.indexOf("Sistema")).toBe(labels.indexOf("Configuración") - 1);

    for (const [name, value] of [
      ["Uso de CPU", "12.5"],
      ["Uso de memoria", "25"],
      ["Uso del disco de datos", "20"],
    ] as const) {
      const bar = page.getByRole("progressbar", { name });
      await expect(bar).toHaveAttribute("aria-valuemin", "0");
      await expect(bar).toHaveAttribute("aria-valuemax", "100");
      await expect(bar).toHaveAttribute("aria-valuenow", value);
    }
    await expect(page.getByTestId("cpu-card")).toContainText("12.5%");
    await expect(page.getByTestId("memory-card")).toContainText("0.5 GB usados de 2.0 GB");
    await expect(page.getByTestId("memory-card")).toContainText("1.5 GB disponibles");
    await expect(page.getByTestId("memory-card")).toContainText("Swap: 0.1 GB de 1.0 GB");
    await expect(page.getByTestId("disk-card")).toContainText("10.0 GB usados de 50.0 GB");
    await expect(page.getByTestId("disk-card")).toContainText("Se envía un aviso por correo al llegar a 80%");
    await expect(page.getByTestId("cpu-card")).toContainText("0.52 · 0.58 · 0.59");
    await expect(page.getByText("1 día 1 h")).toBeVisible();
    await expect(page.getByTestId("system-age")).toHaveText(/^Actualizado hace \d+ s$/);
  });

  test("hides the e-mail notice when reminders are off and swap when there is none", async ({ page }) => {
    await open(page, { system: systemStatus({ reminders_enabled: false, swap: null }) });
    await expect(page.getByTestId("disk-card")).toContainText("usados de 50.0 GB");
    await expect(page.getByText(/Se envía un aviso/)).toHaveCount(0);
    await expect(page.getByText(/Swap:/)).toHaveCount(0);
  });

  test("switches level and color class at 60 and 80", async ({ page }) => {
    const api = await open(page, { system: usage(59.9, 59.9, 59.9) });
    const refresh = page.getByRole("button", { name: "Actualizar" });

    async function expectLevel(level: string, fill: string) {
      for (const id of ["cpu", "memory", "disk"]) {
        const card = page.getByTestId(`${id}-card`);
        await expect(card, id).toHaveAttribute("data-level", level);
        await expect(card.getByRole("progressbar").locator("div"), id).toHaveClass(new RegExp(`\\b${fill}\\b`));
      }
    }

    await expectLevel("normal", "bg-accent");
    api.system.current = usage(60, 60, 60);
    await refresh.click();
    await expect(page.getByTestId("cpu-card")).toContainText("60.0%");
    await expectLevel("warning", "bg-warning");
    api.system.current = usage(79.9, 79.9, 79.9);
    await refresh.click();
    await expect(page.getByTestId("cpu-card")).toContainText("79.9%");
    await expectLevel("warning", "bg-warning");
    api.system.current = usage(80, 80, 80);
    await refresh.click();
    await expect(page.getByTestId("cpu-card")).toContainText("80.0%");
    await expectLevel("danger", "bg-destructive");
    await expect(page.getByTestId("disk-card")).toContainText("Crítico");
  });

  test("the Actualizar button fetches again and shows the new values", async ({ page }) => {
    const api = await open(page);
    await expect(page.getByTestId("cpu-card")).toContainText("12.5%");
    const before = api.system.calls;
    api.system.current = usage(33.3, 25, 20);
    await page.getByRole("button", { name: "Actualizar" }).click();
    await expect(page.getByTestId("cpu-card")).toContainText("33.3%");
    expect(api.system.calls).toBeGreaterThan(before);
  });

  test("refreshes by itself every 10 seconds while the tab is visible", async ({ page }) => {
    await page.clock.install();
    const api = await open(page);
    await expect(page.getByTestId("cpu-card")).toContainText("12.5%");
    const before = api.system.calls;
    api.system.current = usage(41, 25, 20);
    await page.clock.runFor(10_500);
    await expect(page.getByTestId("cpu-card")).toContainText("41.0%");
    expect(api.system.calls).toBeGreaterThan(before);
  });

  test("shows a Spanish error when the server cannot read its state (503) and recovers", async ({ page }) => {
    const api = await open(page, { systemFail: "unavailable" });
    await expect(page.getByRole("alert")).toHaveText("No se pudo leer el estado del servidor");
    await expect(page.getByRole("progressbar")).toHaveCount(0);
    await expectNoHorizontalOverflow(page, "error state");

    api.system.fail = undefined;
    await page.getByRole("button", { name: "Actualizar" }).click();
    await expect(page.getByRole("alert")).toHaveCount(0);
    await expect(page.getByRole("progressbar", { name: "Uso de CPU" })).toBeVisible();
  });

  test("shows the generic server error for a 500", async ({ page }) => {
    await open(page, { systemFail: "server" });
    await expect(page.getByRole("alert")).toContainText("El servidor tuvo un problema");
  });

  test("says there is no disk measurement when disk is null, and the rest still shows", async ({ page }) => {
    await open(page, { system: systemStatus({ disk: null }) });
    await expect(page.getByTestId("disk-card")).toContainText("Sin medición del disco en este entorno");
    await expect(page.getByRole("progressbar", { name: "Uso del disco de datos" })).toHaveCount(0);
    await expect(page.getByRole("progressbar", { name: "Uso de CPU" })).toBeVisible();
    await expect(page.getByRole("progressbar", { name: "Uso de memoria" })).toBeVisible();
  });

  for (const width of [390, 320]) {
    test(`has no horizontal overflow at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 800 });
      await open(page, { system: usage(85, 65, 91) });
      await expect(page.getByTestId("disk-card")).toContainText("Crítico");
      await expectNoHorizontalOverflow(page, `${width}px with data`);
    });

    test(`has no horizontal overflow at ${width}px without disk`, async ({ page }) => {
      await page.setViewportSize({ width, height: 800 });
      await open(page, { system: systemStatus({ disk: null, swap: null }) });
      await expectNoHorizontalOverflow(page, `${width}px without disk`);
    });
  }
});
