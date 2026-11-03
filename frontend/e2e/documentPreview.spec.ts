import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockFiling, type MockInvoice } from "./helpers";

const PERIOD = "2026-10";
const UUID = "6F1C2B3A-4D5E-4F60-8A7B-9C0D1E2F3A4B";

/** A real, minimal one-page PDF (the xref offsets are not needed: viewers rebuild them). */
const PDF = "%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]>>endobj\ntrailer<</Root 1 0 R/Size 4>>\n%%EOF\n";
// 1x1 PNG.
const PNG = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");

const CFDI = `<?xml version="1.0" encoding="UTF-8"?><cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Version="4.0" Serie="A" Folio="12" Fecha="2026-10-02T10:00:00" SubTotal="2500.00" Moneda="USD" Total="2500.00"><cfdi:Emisor Rfc="AAA010101AAA" Nombre="Juan Pérez"/><cfdi:Receptor Rfc="BBB020202BBB" Nombre="Cliente LLC"/><cfdi:Complemento><tfd:TimbreFiscalDigital xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" UUID="${UUID}"/></cfdi:Complemento></cfdi:Comprobante>`;

const invoice = (): MockInvoice => ({
  id: 1,
  client_id: "usa",
  collection_date: "2026-10-01",
  period: PERIOD,
  currency: "USD",
  exchange_rate: "17.50",
  subtotal: "2500.00",
  subtotal_mxn: "43750.00",
  iva: "0.00",
  isr_withheld: "0.00",
  iva_withheld: "0.00",
  total: "2500.00",
  expected_deposit_mxn: "43750.00",
  state: "emitida",
  uuid: UUID,
  movement_id: null,
  declaration_period: null,
  created_at: "2026-10-01T12:00:00Z",
  documents: [
    { id: 1, kind: "xml", name: "cfdi.xml", content_type: "application/xml", size: CFDI.length, sha256: "0".repeat(64), uploaded_at: "2026-10-02T10:00:00Z", content: CFDI },
    { id: 2, kind: "pdf", name: "factura.pdf", content_type: "application/pdf", size: PDF.length, sha256: "0".repeat(64), uploaded_at: "2026-10-02T10:00:00Z", content: PDF },
  ],
});

const filing = (): MockFiling => ({
  period: PERIOD,
  filing_date: "2026-11-05",
  due_date: "2026-11-17",
  income_collected: "43750.00",
  isr_rate: "0.011",
  isr_accrued: "481.25",
  isr_withheld: "0.00",
  isr_due: "481.25",
  iva_transferred: "0.00",
  iva_withheld: "0.00",
  iva_acreditable: "0.00",
  iva_due: "0.00",
  total_to_pay: "481.25",
  folio: "ACUSE-2026-10",
  status: "pendiente",
  payment: null,
  expense_movement_id: null,
  invoice_ids: [1],
  created_at: "2026-11-05T12:00:00Z",
  documents: [{ kind: "comprobante", filename: "IMG_0042.PNG", content_type: "image/png", size: PNG.length, uploaded_at: "2026-11-06T09:00:00Z", content: "" }],
});

/** Records every CSP violation the page reports. */
async function watchCsp(page: Page) {
  await page.addInitScript(() => {
    (window as unknown as { __csp: string[] }).__csp = [];
    document.addEventListener("securitypolicyviolation", (e) => (window as unknown as { __csp: string[] }).__csp.push(`${e.violatedDirective} ${e.blockedURI}`));
  });
  const messages: string[] = [];
  page.on("console", (m) => {
    if (/Content Security Policy|Refused to/i.test(m.text())) messages.push(m.text());
  });
  return {
    violations: () => page.evaluate(() => (window as unknown as { __csp: string[] }).__csp),
    messages,
  };
}

test.describe("document preview", () => {
  test("previews the PDF and the CFDI XML of an invoice in the same page", async ({ page }) => {
    const csp = await watchCsp(page);
    await mockApi(page, { invoices: [invoice()] });
    await seedSession(page);
    await page.goto("/facturas");
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    const url = page.url();
    const popups: unknown[] = [];
    page.context().on("page", (p) => popups.push(p));

    await page.getByRole("button", { name: "Ver factura.pdf" }).click();
    const dialog = page.getByRole("dialog", { name: "factura.pdf" });
    const frame = dialog.locator("iframe");
    await expect(frame).toBeVisible();
    await expect(frame).toHaveAttribute("title", "Vista previa de factura.pdf");
    await expect(frame).toHaveAttribute("src", /^blob:/);
    const box = await frame.boundingBox();
    expect(box!.height).toBeGreaterThan(300);
    await expect(dialog.getByRole("button", { name: "Descargar" })).toBeVisible();
    // The download button saves the same file.
    const [download] = await Promise.all([page.waitForEvent("download"), dialog.getByRole("button", { name: "Descargar" }).click()]);
    expect(download.suggestedFilename()).toBe("factura.pdf");
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);

    await page.getByRole("button", { name: "Ver cfdi.xml" }).click();
    const xml = page.getByRole("dialog", { name: "cfdi.xml" });
    await expect(xml.getByText("Juan Pérez · AAA010101AAA")).toBeVisible();
    await expect(xml.getByText("Cliente LLC · BBB020202BBB")).toBeVisible();
    await expect(xml.getByText("A 12")).toBeVisible();
    await expect(xml.getByText("$2,500.00").first()).toBeVisible();
    await expect(xml.locator("dd", { hasText: UUID })).toBeVisible();
    await xml.getByText("Ver XML").click();
    await expect(xml.locator("pre")).toContainText("<cfdi:Emisor");
    await expect(xml.locator("pre")).toBeVisible();

    expect(popups).toHaveLength(0);
    expect(page.url()).toBe(url);
    expect(await csp.violations()).toEqual([]);
    expect(csp.messages).toEqual([]);
  });

  test("previews the image of a tax filing and falls back when it cannot be decoded", async ({ page }) => {
    const csp = await watchCsp(page);
    await mockApi(page, { invoices: [invoice()], filings: [filing()] });
    let body: Buffer = PNG;
    await page.route(/\/tax-filing\/[\d-]+\/documents\/comprobante$/, (route) =>
      route.request().method() === "GET"
        ? route.fulfill({ status: 200, headers: { "Access-Control-Allow-Origin": "*", "Content-Type": "image/png" }, body })
        : route.fallback(),
    );
    await seedSession(page);
    await page.goto("/declaraciones-presentadas");
    await page.getByRole("row", { name: /octubre de 2026/i }).getByRole("button", { name: /Ver declaración de octubre de 2026/ }).click();
    const docs = page.getByRole("region", { name: "Documentos" });
    await docs.getByRole("button", { name: "Ver comprobante" }).click();
    const dialog = page.getByRole("dialog", { name: "IMG_0042.PNG" });
    const img = dialog.getByRole("img", { name: "IMG_0042.PNG" });
    await expect(img).toHaveAttribute("src", /^blob:/);
    await expect.poll(() => img.evaluate((i: HTMLImageElement) => i.naturalWidth)).toBe(1);
    await expect(dialog.getByRole("button", { name: "Descargar" })).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);

    // Bytes the browser cannot decode (what an unsupported HEIC looks like) show the fallback.
    body = Buffer.from("not an image");
    await docs.getByRole("button", { name: "Ver comprobante" }).click();
    await expect(dialog.getByText("Vista previa no disponible en este navegador.")).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Descargar" })).toBeVisible();
    expect(await csp.violations()).toEqual([]);
    expect(csp.messages).toEqual([]);
  });
});
