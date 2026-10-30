import { expect, type Page } from "@playwright/test";

/** Width from which Configuración shows the sidebar tablist instead of the "Sección" select (Tailwind lg). */
export const SIDEBAR_MIN_WIDTH = 1024;

export const hasSidebar = (page: Page) => (page.viewportSize()?.width ?? 0) >= SIDEBAR_MIN_WIDTH;

/** Opens Configuración on a section through its deep link (works at every width). */
export async function gotoSettings(page: Page, section = "general") {
  await page.goto(`/configuracion?seccion=${section}`);
  await expect(page.getByRole("heading", { level: 1, name: "Configuración" })).toBeVisible();
}

/** The control that switches sections: the sidebar tab on wide screens, the "Sección" select below. */
export function sectionControl(page: Page, label?: string) {
  return hasSidebar(page) ? page.getByRole("tab", { name: label ?? "General", exact: true }) : page.getByLabel("Sección");
}

/** Switches section the way a user does at the current width. */
export async function switchSection(page: Page, label: string) {
  if (hasSidebar(page)) await page.getByRole("tab", { name: label, exact: true }).click();
  else await page.getByLabel("Sección").selectOption({ label });
}

/** Asserts which section is current: the selected sidebar tab on wide screens, the chosen option of the select below. */
export async function expectSection(page: Page, label: string) {
  if (hasSidebar(page)) await expect(page.getByRole("tab", { name: label, exact: true })).toHaveAttribute("aria-selected", "true");
  else await expect(page.getByLabel("Sección").locator("option:checked")).toHaveText(label);
}

export async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({ scrollWidth: document.documentElement.scrollWidth, clientWidth: document.documentElement.clientWidth }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}
