import { expect, test } from "@playwright/test";

// Minimal PWA: manifest + icons + iOS meta tags, deliberately no service worker.
test("the page links the manifest, the apple touch icon and the iOS meta tags", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator('link[rel="manifest"]')).toHaveAttribute("href", "/manifest.webmanifest");
  await expect(page.locator('link[rel="apple-touch-icon"]')).toHaveAttribute("href", "/apple-touch-icon.png");
  await expect(page.locator('meta[name="apple-mobile-web-app-capable"]')).toHaveAttribute("content", "yes");
  await expect(page.locator('meta[name="mobile-web-app-capable"]')).toHaveAttribute("content", "yes");
  await expect(page.locator('meta[name="apple-mobile-web-app-title"]')).toHaveAttribute("content", "Finanzas");
  await expect(page.locator('meta[name="apple-mobile-web-app-status-bar-style"]')).toHaveAttribute("content", "default");
  await expect(page.locator('meta[name="theme-color"][media="(prefers-color-scheme: light)"]')).toHaveCount(1);
  await expect(page.locator('meta[name="theme-color"][media="(prefers-color-scheme: dark)"]')).toHaveCount(1);
  await expect(page.locator('meta[name="viewport"]')).toHaveAttribute("content", /viewport-fit=cover/);
});

test("the manifest is valid and every icon resolves to a PNG", async ({ request }) => {
  const res = await request.get("/manifest.webmanifest");
  expect(res.status()).toBe(200);
  const manifest = await res.json();
  expect(manifest).toMatchObject({
    name: "Finanzas",
    short_name: "Finanzas",
    lang: "es",
    start_url: "/",
    scope: "/",
    display: "standalone",
  });
  expect(manifest.background_color).toMatch(/^#[0-9a-fA-F]{6}$/);
  expect(manifest.theme_color).toMatch(/^#[0-9a-fA-F]{6}$/);

  const sizes = manifest.icons.map((i: { sizes: string }) => i.sizes);
  expect(sizes).toEqual(expect.arrayContaining(["192x192", "512x512"]));
  expect(manifest.icons.some((i: { purpose: string }) => i.purpose === "maskable")).toBe(true);

  for (const src of [...manifest.icons.map((i: { src: string }) => i.src), "/apple-touch-icon.png"]) {
    const icon = await request.get(src);
    expect(icon.status(), src).toBe(200);
    expect(icon.headers()["content-type"], src).toContain("image/png");
  }
});
