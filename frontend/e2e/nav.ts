import { expect, type Locator, type Page } from "@playwright/test";

/** The sidebar layout starts at 1024px; below it the bottom dock and the "Más" sheet replace it. */
export const isDesktop = (page: Page): boolean => (page.viewportSize()?.width ?? 0) >= 1024;

export const sidebar = (page: Page): Locator => page.getByRole("navigation", { name: "Módulos" });
export const dock = (page: Page): Locator => page.getByRole("navigation", { name: "Navegación principal" });
export const moreButton = (page: Page): Locator => page.getByRole("button", { name: "Más", exact: true });
export const moreSheet = (page: Page): Locator => page.getByRole("dialog", { name: "Más", exact: true });
export const moreNav = (page: Page): Locator => page.getByRole("navigation", { name: "Más módulos" });
export const quickExpenseButton = (page: Page): Locator => page.getByRole("button", { name: "Registrar gasto rápido" });

export async function openMore(page: Page): Promise<Locator> {
  await moreButton(page).click();
  await expect(moreSheet(page)).toBeVisible();
  return moreNav(page);
}

export async function closeMore(page: Page): Promise<void> {
  if (await moreSheet(page).isVisible()) {
    await page.keyboard.press("Escape");
    await expect(moreSheet(page)).toBeHidden();
  }
}

/**
 * The link of a module: in the sidebar on desktop; below lg in the dock when it has a slot there, otherwise in the
 * "Más" sheet, which this opens (close it with closeMore).
 */
export async function moduleLink(page: Page, label: string): Promise<Locator> {
  if (isDesktop(page)) return sidebar(page).getByRole("link", { name: label, exact: true });
  const inDock = dock(page).getByRole("link", { name: label, exact: true });
  if ((await inDock.count()) > 0) return inDock;
  const nav = await openMore(page);
  return nav.getByRole("link", { name: label, exact: true });
}

/** Labels of every module link the session can reach: the sidebar (in order) or the dock then the "Más" sheet. */
export async function moduleLabels(page: Page): Promise<string[]> {
  if (isDesktop(page)) return sidebar(page).getByRole("link").allInnerTexts();
  const docked = await dock(page).getByRole("link").allInnerTexts();
  const nav = await openMore(page);
  const more = await nav.getByRole("link").allInnerTexts();
  await closeMore(page);
  return [...docked, ...more];
}

/** Ordered list on desktop; the same set (dock slots first, then the sheet) below lg. */
export async function expectModules(page: Page, expected: string[]): Promise<void> {
  const labels = await moduleLabels(page);
  if (isDesktop(page)) expect(labels).toEqual(expected);
  else expect([...labels].sort()).toEqual([...expected].sort());
}

/** Opens the page that lists the pending-requests badge (the sidebar, or the "Más" sheet below lg). */
export async function pendingBadge(page: Page): Promise<Locator> {
  if (!isDesktop(page)) await openMore(page);
  return page.getByTestId("pending-requests-badge").locator("visible=true").first();
}
