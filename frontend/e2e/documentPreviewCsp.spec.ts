import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, test, type Page } from "@playwright/test";

// Top level: a browser channel cannot be set inside a describe.
test.use({ channel: "chromium" });

/** A real, minimal one-page PDF. */
const PDF = "%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]>>endobj\ntrailer<</Root 1 0 R/Size 4>>\n%%EOF\n";
// 1x1 PNG.
const PNG = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");

/**
 * The production policy, read from the nginx snippets, applied to a probe page. Uses Playwright's "chromium" channel
 * (new headless mode, with the built-in PDF viewer): the default headless shell has no PDF viewer, so it cannot prove
 * that a PDF renders. A blob: PDF must load in an <iframe> although the policy has object-src 'none',
 * frame-ancestors 'none' and X-Frame-Options DENY; the same policy without frame-src blob: must block it.
 */
test.describe("production CSP", () => {
  const read = (rel: string) => readFileSync(resolve(process.cwd(), "../deploy/nginx/snippets", rel), "utf8");
  const csp = /add_header Content-Security-Policy "([^"]+)"/.exec(read("csp.conf"))![1].replace("; upgrade-insecure-requests", "");
  const xfo = /add_header X-Frame-Options "([^"]+)"/.exec(read("security-headers.conf"))![1];

  async function probe(page: Page, policy: string) {
    await page.route("**/__probe.html", (route) =>
      route.fulfill({
        status: 200,
        headers: { "Content-Security-Policy": policy, "X-Frame-Options": xfo, "Content-Type": "text/html" },
        body: `<!doctype html><title>probe</title><body><script src="/__probe.js"></script></body>`,
      }),
    );
    await page.route("**/__probe.js", (route) =>
      route.fulfill({
        status: 200,
        headers: { "Content-Type": "text/javascript" },
        body: `
          window.__v = [];
          document.addEventListener("securitypolicyviolation", (e) => window.__v.push(e.violatedDirective + " " + e.blockedURI));
          const f = document.createElement("iframe");
          f.id = "pdf"; f.width = 400; f.height = 400; f.title = "pdf";
          f.src = URL.createObjectURL(new Blob([${JSON.stringify(PDF)}], { type: "application/pdf" }));
          document.body.append(f);
          const i = document.createElement("img");
          i.id = "img";
          i.src = URL.createObjectURL(new Blob([Uint8Array.from(atob(${JSON.stringify(PNG.toString("base64"))}), (c) => c.charCodeAt(0))], { type: "image/png" }));
          document.body.append(i);
        `,
      }),
    );
    await page.goto("/__probe.html");
    await page.waitForTimeout(1500);
  }

  test("renders a blob: PDF in an iframe and a blob: image", async ({ page }) => {
    expect(csp).toContain("frame-src blob:");
    expect(csp).toContain("object-src 'none'");
    expect(csp).toContain("frame-ancestors 'none'");
    expect(xfo).toBe("DENY");
    await probe(page, csp);
    expect(await page.evaluate(() => (window as unknown as { __v: string[] }).__v)).toEqual([]);
    expect(await page.locator("#img").evaluate((i: HTMLImageElement) => i.naturalWidth)).toBe(1);
    // The PDF viewer committed a document of type application/pdf from the blob: URL.
    const state = await page.locator("#pdf").evaluate((f: HTMLIFrameElement) => ({ href: f.contentWindow?.location.href, src: f.src, type: f.contentDocument?.contentType }));
    expect(state.type).toBe("application/pdf");
    expect(state.href).toBe(state.src);
    expect(page.frames().some((f) => f.url().startsWith("chrome-extension://") && f.url().endsWith("/index.html"))).toBe(true);
  });

  test("control: without frame-src blob: the same iframe is blocked", async ({ page }) => {
    await probe(page, csp.replace("frame-src blob:; ", ""));
    const violations = await page.evaluate(() => (window as unknown as { __v: string[] }).__v);
    expect(violations.some((v) => v.startsWith("frame-src") || v.startsWith("default-src"))).toBe(true);
    expect(page.frames().some((f) => f.url().startsWith("chrome-extension://"))).toBe(false);
  });
});
