import { expect, test, type Page } from "@playwright/test";
import { mockApi, seedSession, type MockInvoice } from "./helpers";

function today() {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

const UUID_A = "6F1C2B3A-4D5E-4F60-8A7B-9C0D1E2F3A4B";
const UUID_B = "11111111-2222-3333-4444-555555555555";

const seeded = (over: Partial<MockInvoice> = {}): MockInvoice => ({
  id: 1,
  client_id: "usa",
  collection_date: today(),
  period: today().slice(0, 7),
  currency: "USD",
  exchange_rate: "17.50",
  subtotal: "2500.00",
  subtotal_mxn: "43750.00",
  iva: "0.00",
  isr_withheld: "0.00",
  iva_withheld: "0.00",
  total: "2500.00",
  expected_deposit_mxn: "43750.00",
  state: "preparada",
  uuid: null,
  movement_id: null,
  declaration_period: null,
  created_at: "2026-10-01T12:00:00Z",
  ...over,
});

const issued = (over: Partial<MockInvoice> = {}) =>
  seeded({
    state: "emitida",
    uuid: UUID_A,
    documents: [
      { id: 1, kind: "xml", name: "cfdi.xml", content_type: "application/xml", size: 1200, sha256: "0".repeat(64), uploaded_at: "2026-10-02T10:00:00Z", content: "<cfdi/>" },
      { id: 2, kind: "pdf", name: "factura.pdf", content_type: "application/pdf", size: 52000, sha256: "0".repeat(64), uploaded_at: "2026-10-02T10:00:00Z", content: "%PDF-1.7 hello" },
    ],
    ...over,
  });

const cfdi = (uuid: string, total: string) =>
  `<?xml version="1.0"?><cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Total="${total}" SubTotal="${total}" Moneda="USD"><cfdi:Complemento><tfd:TimbreFiscalDigital xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" UUID="${uuid}"/></cfdi:Complemento></cfdi:Comprobante>`;

const xmlFile = (uuid = UUID_A, total = "2500.00", name = "cfdi.xml") => ({ name, mimeType: "text/xml", buffer: Buffer.from(cfdi(uuid, total)) });
const pdfFile = (name = "factura.pdf") => ({ name, mimeType: "application/pdf", buffer: Buffer.from("%PDF-1.7 hello") });

async function expectNoHorizontalOverflow(page: Page, where: string) {
  const size = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(size.scrollWidth, `page overflows on ${where}`).toBeLessThanOrEqual(size.clientWidth);
}

async function open(page: Page, opts: Parameters<typeof mockApi>[1] = {}) {
  const api = await mockApi(page, opts);
  await seedSession(page);
  await page.goto("/facturas");
  await expect(page.getByRole("heading", { level: 1, name: "Facturas" })).toBeVisible();
  await expect(page.getByLabel("Cliente")).toBeVisible();
  return api;
}

const detail = (page: Page, id = 1) => page.getByRole("region", { name: `Detalle de la Factura #${id}` });
const listItem = (page: Page, id: number) => page.getByRole("listitem").filter({ hasText: `Factura #${id}` });

const issuer = { rfc: "AAA010101AAA", name: "Juan Pérez", regimen: "626", postal_code: "64000", note: "" };

// A client that withholds taxes (settings patch) and the stamped XML of its 7,318.18 payment.
const ibl = {
  id: "ibl", name: "IBL", type: "nacional", currency: "MXN", iva_rate: "0.16", rfc: "IBL121029ED3", regimen: "601", uso_cfdi: "G03",
  ret_isr_rate: "0.0125", ret_iva_rate: "0.106667", concepto: "Servicios de software", clave_prod_serv: "81111500", clave_unidad: "E48",
  address: "", tax_residence: "MX", contract: "", real_payer: "", postal_code: "06600",
};
const withIbl = (extra?: (c: typeof ibl) => void) => (settings: { clients: unknown[] }) => {
  const client = { ...ibl };
  extra?.(client);
  settings.clients.push(client);
};
const iblXml = (uuid: string) =>
  `<?xml version="1.0"?><cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Total="7318.18" SubTotal="7031.08" Moneda="MXN"><cfdi:Conceptos><cfdi:Concepto><cfdi:Impuestos><cfdi:Traslados><cfdi:Traslado Impuesto="002" Importe="999.99"/></cfdi:Traslados></cfdi:Impuestos></cfdi:Concepto></cfdi:Conceptos><cfdi:Impuestos><cfdi:Retenciones><cfdi:Retencion Impuesto="001" Importe="87.89"/><cfdi:Retencion Impuesto="002" Importe="749.98"/></cfdi:Retenciones><cfdi:Traslados><cfdi:Traslado Impuesto="002" Importe="1124.97"/></cfdi:Traslados></cfdi:Impuestos><cfdi:Complemento><tfd:TimbreFiscalDigital xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" UUID="${uuid}"/></cfdi:Complemento></cfdi:Comprobante>`;
// What the app stored before the fix: no withholdings and the wrong subtotal.
const staleIbl = (over: Partial<MockInvoice> = {}) =>
  issued({
    client_id: "ibl", currency: "MXN", exchange_rate: null, subtotal: "6308.78", subtotal_mxn: "6308.78", iva: "1009.40",
    total: "7318.18", expected_deposit_mxn: "7318.18",
    documents: [{ id: 1, kind: "xml", name: "cfdi.xml", content_type: "application/xml", size: 1200, sha256: "0".repeat(64), uploaded_at: "2026-10-02T10:00:00Z", content: iblXml(UUID_A) }],
    ...over,
  });

test.describe("invoices page", () => {
  test("prepares an invoice for a client that withholds taxes", async ({ page }) => {
    const api = await open(page, { issuer, settingsPatch: withIbl() });
    await page.getByLabel("Cliente").selectOption({ label: "IBL" });
    await expect(page.getByLabel("Periodicidad (factura global)")).toHaveCount(0);
    await expect(page.getByText(/retención de ISR 1.25% y la retención de IVA 10.6667%/)).toBeVisible();
    await page.getByLabel("Monto neto recibido").fill("7318.18");
    await page.getByRole("button", { name: "Preparar factura" }).click();

    const body = api.invoiceCalls.find((c) => c.path === "/invoices")?.fields as Record<string, unknown>;
    expect(body).toMatchObject({ client_id: "ibl", amount: "7318.18" });
    expect(body).not.toHaveProperty("periodicity");

    const d = detail(page);
    const taxes = d.getByRole("region", { name: "Impuestos" });
    await expect(taxes).toContainText("$1,124.97 MXN");
    await expect(taxes).toContainText("Retención de ISR");
    await expect(taxes).toContainText("$87.89 MXN");
    await expect(taxes).toContainText("Retención de IVA");
    await expect(taxes).toContainText("$749.98 MXN");
    await expect(taxes).not.toContainText("Ninguna");
    await expect(d.getByRole("region", { name: "Totales" })).toContainText("$7,318.18 MXN");
    await expect(d.getByRole("region", { name: "Receptor" })).toContainText("06600");
    await expect(d.getByRole("region", { name: "Comprobante" })).not.toContainText("InformacionGlobal");
    await expect(d.getByLabel("Periodicidad (factura global)")).toHaveCount(0);
  });

  test("asks for the client postal code when it is missing", async ({ page }) => {
    await open(page, { issuer, invoices: [seeded({ client_id: "ibl", currency: "MXN", exchange_rate: null })], settingsPatch: withIbl((c) => (c.postal_code = "")) });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    await expect(detail(page).getByRole("alert").filter({ hasText: "Faltan datos" })).toContainText("código postal del cliente");
  });

  test("syncs an issued invoice with its stored XML", async ({ page }) => {
    const api = await open(page, { invoices: [staleIbl()], settingsPatch: withIbl() });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    const d = detail(page);
    await expect(d.getByText("$6,308.78 MXN").first()).toBeVisible();
    await expect(d.getByRole("region", { name: "Impuestos" })).toContainText("Ninguna");

    await d.getByRole("button", { name: "Sincronizar con XML" }).click();
    const warnings = d.getByRole("status", { name: "Advertencias" });
    await expect(warnings).toContainText("se tomaron del XML timbrado");
    await expect(warnings).toContainText("Subtotal: $6,308.78 MXN → $7,031.08 MXN");
    await expect(warnings).toContainText("Retención de IVA: $0.00 MXN → $749.98 MXN");
    await expect(d.getByText("$7,031.08 MXN").first()).toBeVisible();
    await expect(d.getByRole("region", { name: "Impuestos" })).toContainText("$87.89 MXN");
    await expect(d.getByRole("region", { name: "Impuestos" })).not.toContainText("Ninguna");
    expect(api.invoiceCalls.some((c) => c.method === "POST" && c.path === "/invoices/1/resync")).toBe(true);
  });

  test("a declared invoice cannot be synced", async ({ page }) => {
    const api = await open(page, { invoices: [staleIbl({ declaration_period: "2026-10" })], settingsPatch: withIbl() });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    const button = detail(page).getByRole("button", { name: "Sincronizar con XML" });
    await expect(button).toBeDisabled();
    await expect(detail(page).getByText(/No se puede sincronizar: esta factura está incluida en la declaración de octubre de 2026/)).toBeVisible();
    expect(api.invoiceCalls.some((c) => c.path === "/invoices/1/resync")).toBe(false);
  });

  test("shows the reason when the stored XML belongs to another invoice", async ({ page }) => {
    await open(page, {
      invoices: [staleIbl({ documents: [{ id: 1, kind: "xml", name: "cfdi.xml", content_type: "application/xml", size: 1, sha256: "0".repeat(64), uploaded_at: "2026-10-02T10:00:00Z", content: iblXml(UUID_B) }] })],
      settingsPatch: withIbl(),
    });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    await detail(page).getByRole("button", { name: "Sincronizar con XML" }).click();
    await expect(detail(page).getByRole("alert").filter({ hasText: "UUID del XML guardado no coincide" })).toBeVisible();
  });

  test("offers no sync without a stored XML", async ({ page }) => {
    await open(page, { invoices: [staleIbl({ documents: [] })], settingsPatch: withIbl() });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    await expect(detail(page).getByRole("button", { name: "Cancelar factura" })).toBeVisible();
    await expect(detail(page).getByRole("button", { name: "Sincronizar con XML" })).toHaveCount(0);
  });


  test("shows the empty state before any invoice exists", async ({ page }) => {
    await open(page);
    await expect(page.getByText("Aún no hay facturas.")).toBeVisible();
    await expect(page.getByRole("heading", { level: 2, name: "Preparar factura" })).toBeVisible();
  });

  test("prepares a USA invoice with the configured defaults and shows its checklist", async ({ page }) => {
    const api = await open(page, { issuer });
    await page.getByLabel("Cliente").selectOption({ label: "Acme Inc." });
    await expect(page.getByLabel("Subtotal (USD)")).toBeVisible();
    await expect(page.getByText("sueldo configurado ($2,500.00 USD)")).toBeVisible();
    await page.getByRole("button", { name: "Preparar factura" }).click();

    const body = api.invoiceCalls.find((c) => c.path === "/invoices")?.fields as Record<string, unknown>;
    expect(body.client_id).toBe("usa");
    expect(body.date).toBe(today());
    expect(body).not.toHaveProperty("subtotal");
    expect(body).not.toHaveProperty("amount");

    const d = detail(page);
    await expect(d).toBeVisible();
    await expect(d.getByText("Preparada", { exact: true })).toBeVisible();
    await expect(d.getByText("$43,750.00 MXN").first()).toBeVisible();
    const checklist = d.getByRole("heading", { name: "Checklist para el portal del SAT" });
    await expect(checklist).toBeVisible();
    await expect(d.getByRole("region", { name: "Emisor" })).toContainText("AAA010101AAA");
    await expect(d.getByRole("region", { name: "Comprobante" })).toContainText("Clave de exportación, venta a tasa 0%");
    await expect(d.getByRole("region", { name: "Comprobante" })).toContainText("confirmar con contador");
    await expect(d.getByRole("region", { name: "Impuestos" })).toContainText("Tasa 0% = $0.00");
    await expect(d.getByText("Fecha límite de declaración de ese periodo:")).toBeVisible();
    await expect(d.getByRole("alert").filter({ hasText: "Faltan datos" })).toHaveCount(0);
    await expect(listItem(page, 1)).toContainText("Preparada");
  });

  test("flags a missing issuer in the checklist", async ({ page }) => {
    await open(page);
    await page.getByLabel("Cliente").selectOption({ label: "Acme Inc." });
    await page.getByLabel("Subtotal (USD)").fill("3383.33");
    await page.getByLabel("Tipo de cambio (opcional)").fill("17.74");
    await page.getByRole("button", { name: "Preparar factura" }).click();
    await expect(detail(page).getByRole("alert").filter({ hasText: "Faltan datos" })).toContainText("RFC del emisor");
    await expect(detail(page).getByRole("region", { name: "Emisor" })).toContainText("Pendiente");
  });

  test("prepares a public-in-general invoice from the total received", async ({ page }) => {
    const api = await open(page, { issuer });
    await page.getByLabel("Cliente").selectOption({ label: "Público en general" });
    await expect(page.getByLabel("Subtotal (USD)")).toHaveCount(0);
    await page.getByLabel("Monto neto recibido").fill("35000");
    await page.getByLabel("Periodicidad (factura global)").first().selectOption("quincenal");
    await page.getByRole("button", { name: "Preparar factura" }).click();

    const body = api.invoiceCalls.find((c) => c.path === "/invoices")?.fields as Record<string, unknown>;
    expect(body).toMatchObject({ client_id: "b", amount: "35000", periodicity: "quincenal" });
    expect(typeof body.amount).toBe("string");

    const d = detail(page);
    await expect(d.getByRole("region", { name: "Impuestos" })).toContainText("$4,827.59 MXN");
    await expect(d.getByRole("region", { name: "Totales" })).toContainText("$35,000.00 MXN");
    await expect(d.getByRole("region", { name: "Comprobante" })).toContainText("03 Quincenal");
    await expect(d.getByRole("region", { name: "Receptor" })).toContainText("Empresa pagadora");
    await expect(d.getByRole("region", { name: "Receptor" })).toContainText("(no va en el CFDI)");
  });

  test("warns about a possible duplicate but still prepares the invoice", async ({ page }) => {
    await open(page, { invoices: [seeded()] });
    await page.getByLabel("Cliente").selectOption({ label: "Acme Inc." });
    await page.getByRole("button", { name: "Preparar factura" }).click();
    const warning = detail(page, 2).getByRole("status", { name: "Advertencias" });
    await expect(warning).toContainText("Ya existe una factura de este cliente con la misma fecha de cobro (#1)");
    await expect(listItem(page, 2)).toBeVisible();
  });

  test("blocks invalid input and sends nothing", async ({ page }) => {
    const api = await open(page);
    await page.getByRole("button", { name: "Preparar factura" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "Elige un cliente." })).toBeVisible();

    await page.getByLabel("Cliente").selectOption({ label: "Público en general" });
    await page.getByLabel("Monto neto recibido").fill("12,5");
    await page.getByRole("button", { name: "Preparar factura" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "monto neto recibido" })).toBeVisible();
    await expect(page.getByLabel("Monto neto recibido")).toBeFocused();

    await page.getByLabel("Cliente").selectOption({ label: "Acme Inc." });
    await page.getByLabel("Subtotal (USD)").fill("0");
    await page.getByRole("button", { name: "Preparar factura" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "subtotal mayor a cero" })).toBeVisible();
    expect(api.invoiceCalls).toHaveLength(0);
  });

  test("shows the backend message when preparing fails", async ({ page }) => {
    await open(page, { invoicesFail: "prepare" });
    await page.getByLabel("Cliente").selectOption({ label: "Acme Inc." });
    await page.getByRole("button", { name: "Preparar factura" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "amount must be greater than zero" })).toBeVisible();
  });

  test("opening an invoice detail scrolls to it and flashes the highlight", async ({ page }) => {
    await open(page, { invoices: [seeded()] });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    const d = detail(page);
    await expect(d).toBeInViewport();
    const panel = d.locator("xpath=..");
    await expect(panel).toHaveClass(/(^|\s)edit-highlight(\s|$)/);
    await expect(panel).not.toHaveClass(/(^|\s)edit-highlight(\s|$)/, { timeout: 4_000 });
  });

  test("marks an invoice as issued from the XML and a PDF", async ({ page }) => {
    const api = await open(page, { invoices: [seeded()] });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    const d = detail(page);
    await expect(d.getByText("Aún sin UUID")).toBeVisible();

    await d.getByLabel("XML del CFDI").setInputFiles(xmlFile());
    await d.getByLabel("PDF (opcional)").setInputFiles(pdfFile());
    await expect(d.getByLabel("UUID manual")).toBeDisabled();
    await d.getByRole("button", { name: "Marcar como emitida" }).click();

    await expect(d.getByText("Emitida", { exact: true })).toBeVisible();
    await expect(d.getByText(UUID_A).first()).toBeVisible();
    await expect(d.getByRole("status", { name: "Advertencias" })).toHaveCount(0);
    await expect(d.getByRole("button", { name: "Descargar cfdi.xml" })).toBeVisible();
    await expect(d.getByRole("button", { name: "Descargar factura.pdf" })).toBeVisible();
    await expect(d.getByRole("heading", { name: "Marcar como emitida" })).toHaveCount(0);
    await expect(listItem(page, 1)).toContainText("Emitida");

    const upload = api.invoiceCalls.find((c) => c.path === "/invoices/1/issue");
    expect(upload?.files.xml.name).toBe("cfdi.xml");
    expect(upload?.files.pdf.name).toBe("factura.pdf");
    expect(upload?.fields).not.toHaveProperty("uuid");
  });

  test("cannot cancel an invoice that a tax filing includes and explains why", async ({ page }) => {
    const api = await open(page, { invoices: [issued({ declaration_period: "2026-10", period: "2026-10" })] });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    const cancel = detail(page).getByRole("button", { name: "Cancelar factura" });
    await expect(cancel).toBeDisabled();
    await expect(detail(page).getByText("No se puede cancelar: esta factura está incluida en la declaración de octubre de 2026")).toBeVisible();
    expect(api.invoiceCalls.find((c) => c.path === "/invoices/1/cancel")).toBeUndefined();
  });

  test("takes the XML amounts when issuing and lists what changed", async ({ page }) => {
    await open(page, { invoices: [seeded()] });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    await detail(page).getByLabel("XML del CFDI").setInputFiles(xmlFile(UUID_A, "2400.00"));
    await detail(page).getByRole("button", { name: "Marcar como emitida" }).click();

    await expect(detail(page).getByText("Emitida", { exact: true })).toBeVisible();
    const warning = detail(page).getByRole("status", { name: "Advertencias" });
    await expect(warning).toContainText("se tomaron del XML timbrado");
    await expect(warning).toContainText("Total: $2,500.00 USD → $2,400.00 USD");
    await expect(detail(page).getByText("$2,400.00 USD").first()).toBeVisible();
  });

  test("marks an invoice as issued with a manual UUID and validates its format", async ({ page }) => {
    const api = await open(page, { invoices: [seeded()] });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    const d = detail(page);

    await d.getByRole("button", { name: "Marcar como emitida" }).click();
    await expect(d.getByRole("alert").filter({ hasText: "Sube el XML del CFDI o escribe el UUID" })).toBeVisible();

    await d.getByLabel("UUID manual").fill("not-a-uuid");
    await d.getByRole("button", { name: "Marcar como emitida" }).click();
    await expect(d.getByRole("alert").filter({ hasText: "formato 8-4-4-4-12" })).toBeVisible();
    expect(api.invoiceCalls).toHaveLength(0);

    await d.getByLabel("UUID manual").fill(UUID_A.toLowerCase());
    await d.getByRole("button", { name: "Marcar como emitida" }).click();
    await expect(d.getByText("Emitida", { exact: true })).toBeVisible();
    await expect(d.getByText(UUID_A).first()).toBeVisible();
    await expect(d.getByText("Esta factura aún no tiene archivos guardados.")).toBeVisible();
    expect(api.invoiceCalls.find((c) => c.path === "/invoices/1/issue")?.fields.uuid).toBe(UUID_A.toLowerCase());
  });

  test("rejects a wrong file type before uploading", async ({ page }) => {
    const api = await open(page, { invoices: [seeded()] });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    await detail(page).getByLabel("XML del CFDI").setInputFiles({ name: "cfdi.txt", mimeType: "text/plain", buffer: Buffer.from("x") });
    await detail(page).getByRole("button", { name: "Marcar como emitida" }).click();
    await expect(detail(page).getByRole("alert").filter({ hasText: "extensión .xml" })).toBeVisible();
    expect(api.invoiceCalls).toHaveLength(0);
  });

  test("shows the server error for a UUID already used by another invoice", async ({ page }) => {
    await open(page, { invoices: [seeded(), issued({ id: 2 })] });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    await detail(page).getByLabel("UUID manual").fill(UUID_A);
    await detail(page).getByRole("button", { name: "Marcar como emitida" }).click();
    await expect(detail(page).getByRole("alert").filter({ hasText: "Ese UUID ya está registrado en otra factura." })).toBeVisible();
    await expect(detail(page).getByText("Preparada", { exact: true })).toBeVisible();
  });

  test("shows the server error for an XML that is not stamped", async ({ page }) => {
    await open(page, { invoices: [seeded()] });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    await detail(page).getByLabel("XML del CFDI").setInputFiles({ name: "cfdi.xml", mimeType: "text/xml", buffer: Buffer.from("<cfdi:Comprobante/>") });
    await detail(page).getByRole("button", { name: "Marcar como emitida" }).click();
    await expect(detail(page).getByRole("alert").filter({ hasText: "El XML no está timbrado" })).toBeVisible();
  });

  test("shows an error when document storage is unavailable", async ({ page }) => {
    await open(page, { invoices: [seeded()], invoicesFail: "storage" });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    await detail(page).getByLabel("XML del CFDI").setInputFiles(xmlFile());
    await detail(page).getByRole("button", { name: "Marcar como emitida" }).click();
    await expect(detail(page).getByRole("alert").filter({ hasText: "almacenamiento de documentos no está disponible" })).toBeVisible();
    await expect(detail(page).getByText("Preparada", { exact: true })).toBeVisible();
  });

  test("downloads a stored document through the API", async ({ page }) => {
    await open(page, { invoices: [issued()] });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    const downloadPromise = page.waitForEvent("download");
    await detail(page).getByRole("button", { name: "Descargar factura.pdf" }).click();
    const download = await downloadPromise;
    expect(download.suggestedFilename()).toBe("factura.pdf");
    const stream = await download.createReadStream();
    const chunks: Buffer[] = [];
    for await (const chunk of stream) chunks.push(chunk as Buffer);
    expect(Buffer.concat(chunks).toString()).toBe("%PDF-1.7 hello");
  });

  test("shows an error when a download fails", async ({ page }) => {
    await open(page, { invoices: [issued()], invoicesFail: "download" });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    await detail(page).getByRole("button", { name: "Descargar cfdi.xml" }).click();
    await expect(detail(page).getByRole("alert").filter({ hasText: "almacenamiento de documentos no está disponible" })).toBeVisible();
  });

  test("attaches a new PDF to an issued invoice, replacing the old one", async ({ page }) => {
    const api = await open(page, { invoices: [issued()] });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    const d = detail(page);
    await expect(d.getByText("Ya hay un PDF: el archivo nuevo lo reemplaza.")).toBeVisible();

    await d.getByRole("button", { name: "Subir documento" }).click();
    await expect(d.getByRole("alert").filter({ hasText: "Elige el archivo" })).toBeVisible();

    await d.getByLabel("Archivo").setInputFiles(pdfFile("factura-v2.pdf"));
    await d.getByRole("button", { name: "Subir documento" }).click();
    await expect(d.getByText("Documento guardado.")).toBeVisible();
    await expect(d.getByRole("button", { name: "Descargar factura-v2.pdf" })).toBeVisible();
    await expect(d.getByRole("button", { name: "Descargar factura.pdf" })).toHaveCount(0);
    const upload = api.invoiceCalls.find((c) => c.path === "/invoices/1/documents");
    expect(upload?.fields.kind).toBe("pdf");
    expect(upload?.files.file.name).toBe("factura-v2.pdf");
  });

  test("an XML attached later warns when its total differs", async ({ page }) => {
    await open(page, { invoices: [issued()] });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    const d = detail(page);
    await d.getByLabel("Tipo de documento").selectOption("xml");
    await d.getByLabel("Archivo").setInputFiles(xmlFile(UUID_A, "2300.00", "nuevo.xml"));
    await d.getByRole("button", { name: "Subir documento" }).click();
    await expect(d.getByRole("status", { name: "Advertencias" })).toContainText("El total del XML ($2,300.00 USD)");
    await expect(d.getByRole("button", { name: "Descargar nuevo.xml" })).toBeVisible();
  });

  test("cancels an issued invoice after an inline confirmation", async ({ page }) => {
    page.on("dialog", (dlg) => {
      throw new Error(`unexpected native dialog: ${dlg.message()}`);
    });
    const api = await open(page, { invoices: [issued()] });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    const d = detail(page);

    await d.getByRole("button", { name: "Cancelar factura" }).click();
    await expect(d.getByText("¿Cancelar esta factura?")).toBeVisible();
    await expect(d.getByText("cancélala también en el portal del SAT")).toBeVisible();
    await d.getByRole("button", { name: "No, conservarla" }).click();
    await expect(d.getByRole("button", { name: "Cancelar factura" })).toBeVisible();
    expect(api.invoiceCalls).toHaveLength(0);

    await d.getByRole("button", { name: "Cancelar factura" }).click();
    await d.getByRole("button", { name: "Sí, cancelar factura" }).click();
    await expect(d.getByText("Cancelada", { exact: true })).toBeVisible();
    await expect(d.getByText("Esta factura está cancelada")).toBeVisible();
    await expect(d.getByRole("button", { name: "Cancelar factura" })).toHaveCount(0);
    await expect(d.getByRole("button", { name: "Subir documento" })).toHaveCount(0);
    // Its files remain downloadable.
    await expect(d.getByRole("button", { name: "Descargar factura.pdf" })).toBeVisible();
    await expect(listItem(page, 1)).toContainText("Cancelada");
    expect(api.invoiceCalls.some((c) => c.method === "POST" && c.path === "/invoices/1/cancel")).toBe(true);
  });

  test("a cancelled invoice cannot be issued", async ({ page }) => {
    await open(page, { invoices: [seeded({ state: "cancelada" })] });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    await expect(detail(page).getByText("Cancelada", { exact: true })).toBeVisible();
    await expect(detail(page).getByRole("heading", { name: "Marcar como emitida" })).toHaveCount(0);
    await expect(detail(page).getByRole("button", { name: "Cancelar factura" })).toHaveCount(0);
  });

  test("filters the list by period and by state", async ({ page }) => {
    await open(page, {
      invoices: [
        seeded({ id: 1, collection_date: "2025-01-10", period: "2025-01" }),
        issued({ id: 2, collection_date: "2025-02-10", period: "2025-02" }),
        seeded({ id: 3, collection_date: "2025-02-20", period: "2025-02", state: "cancelada" }),
      ],
    });
    await expect(listItem(page, 1)).toBeVisible();
    await expect(listItem(page, 2)).toBeVisible();
    await expect(listItem(page, 3)).toBeVisible();

    await page.getByLabel("Periodo").fill("2025-02");
    await expect(listItem(page, 1)).toHaveCount(0);
    await expect(listItem(page, 2)).toBeVisible();
    await expect(listItem(page, 3)).toBeVisible();

    await page.getByLabel("Estado").selectOption("cancelada");
    await expect(listItem(page, 2)).toHaveCount(0);
    await expect(listItem(page, 3)).toContainText("Cancelada");

    await page.getByLabel("Estado").selectOption("preparada");
    await expect(page.getByText("No hay facturas con estos filtros.")).toBeVisible();

    await page.getByRole("button", { name: "Quitar filtros" }).click();
    await expect(listItem(page, 1)).toBeVisible();
  });

  test("shows an error with a retry when the list fails", async ({ page }) => {
    await open(page, { invoicesFail: "list" });
    await expect(page.getByRole("alert").filter({ hasText: "El servidor tuvo un problema" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Reintentar" })).toBeVisible();
  });

  test("shows an error with a retry when the detail fails", async ({ page }) => {
    await open(page, { invoices: [seeded()], invoicesFail: "detail" });
    await page.getByRole("button", { name: "Ver factura #1" }).click();
    await expect(detail(page).getByRole("alert").filter({ hasText: "El servidor tuvo un problema" })).toBeVisible();
    await detail(page).getByRole("button", { name: "Cerrar detalle" }).click();
    await expect(detail(page)).toHaveCount(0);
  });

  for (const vp of [
    { name: "375", width: 375, height: 800 },
    { name: "768", width: 768, height: 900 },
    { name: "1440", width: 1440, height: 900 },
  ]) {
    test(`has no horizontal overflow at ${vp.name}px`, async ({ page }) => {
      await page.setViewportSize({ width: vp.width, height: vp.height });
      await open(page, {
        issuer,
        invoices: [
          issued({
            documents: [
              { id: 1, kind: "xml", name: "un-nombre-de-archivo-extremadamente-largo-sin-espacios-".repeat(3) + ".xml", content_type: "application/xml", size: 1200, sha256: "0".repeat(64), uploaded_at: "2026-10-02T10:00:00Z" },
            ],
          }),
          seeded({ id: 2, client_id: "b", currency: "MXN", exchange_rate: null, subtotal: "30172.41", subtotal_mxn: "30172.41", iva: "4827.59", total: "35000.00", expected_deposit_mxn: "35000.00" }),
        ],
      });
      await expectNoHorizontalOverflow(page, `${vp.name}px list`);
      await page.getByRole("button", { name: "Ver factura #1" }).click();
      await expect(detail(page).getByRole("heading", { name: "Checklist para el portal del SAT" })).toBeVisible();
      await expectNoHorizontalOverflow(page, `${vp.name}px issued detail`);
      await page.getByRole("button", { name: "Ver factura #2" }).click();
      await expect(detail(page, 2).getByRole("region", { name: "Impuestos" })).toBeVisible();
      await expectNoHorizontalOverflow(page, `${vp.name}px prepared detail`);
      await page.getByLabel("Cliente").first().selectOption({ label: "Público en general" });
      await page.getByRole("button", { name: "Preparar factura" }).click();
      await expectNoHorizontalOverflow(page, `${vp.name}px form errors`);
      await detail(page, 2).getByRole("button", { name: "Cancelar factura" }).click();
      await expectNoHorizontalOverflow(page, `${vp.name}px cancel confirm`);
    });
  }
});
