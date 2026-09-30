import { describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient } from "./client";
import { createInvoicesApi } from "./invoices";

const json = (status: number, body?: unknown) =>
  new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

function setup(handler: (url: string, init: RequestInit) => Response) {
  const fetchFn = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => handler(String(url), init ?? {}));
  const client = createApiClient({
    baseUrl: "http://x",
    fetchFn: fetchFn as unknown as typeof fetch,
    store: { getAccess: () => "tok", getRefresh: () => null, set: () => {}, clear: () => {} },
  });
  const invoices = createInvoicesApi(client);
  const call = (i = 0) => {
    const [url, init] = fetchFn.mock.calls[i] as [string, RequestInit];
    return { url, init, headers: (init.headers ?? {}) as Record<string, string> };
  };
  return { invoices, call };
}

describe("invoices api", () => {
  it("prepares with decimal strings untouched", async () => {
    const { invoices, call } = setup(() => json(201, { invoice: { id: 1 }, documents: [], checklist: {}, warnings: [] }));
    await invoices.prepare({ client_id: "usa", date: "2026-10-15", subtotal: "3383.33", exchange_rate: "17.74" });
    const { url, init, headers } = call();
    expect(url).toBe("http://x/invoices");
    expect(init.method).toBe("POST");
    expect(headers["Content-Type"]).toBe("application/json");
    expect(headers.Authorization).toBe("Bearer tok");
    expect(JSON.parse(String(init.body))).toEqual({ client_id: "usa", date: "2026-10-15", subtotal: "3383.33", exchange_rate: "17.74" });
  });

  it("lists with only the filters that are set", async () => {
    const { invoices, call } = setup(() => json(200, []));
    await expect(invoices.list()).resolves.toEqual([]);
    await invoices.list({ period: "2026-10", state: "emitida" });
    await invoices.list({ period: "", state: "" });
    expect(call(0).url).toBe("http://x/invoices");
    expect(call(1).url).toBe("http://x/invoices?period=2026-10&state=emitida");
    expect(call(2).url).toBe("http://x/invoices");
  });

  it("gets one invoice with an optional periodicity", async () => {
    const { invoices, call } = setup(() => json(200, {}));
    await invoices.get(7);
    await invoices.get(7, "quincenal");
    expect(call(0).url).toBe("http://x/invoices/7");
    expect(call(1).url).toBe("http://x/invoices/7?periodicity=quincenal");
  });

  it("issues as multipart without a JSON content type", async () => {
    const { invoices, call } = setup(() => json(200, { invoice: { id: 7 }, documents: [], checklist: {}, warnings: [] }));
    const xml = new File(["<a/>"], "cfdi.xml", { type: "text/xml" });
    const pdf = new File(["%PDF-1"], "f.pdf", { type: "application/pdf" });
    await invoices.issue(7, { xml, pdf });
    const { url, init, headers } = call();
    expect(url).toBe("http://x/invoices/7/issue");
    expect(init.method).toBe("POST");
    expect(headers["Content-Type"]).toBeUndefined();
    expect(init.body).toBeInstanceOf(FormData);
    const form = init.body as FormData;
    expect((form.get("xml") as File).name).toBe("cfdi.xml");
    expect((form.get("pdf") as File).name).toBe("f.pdf");
    expect(form.has("uuid")).toBe(false);
  });

  it("issues with only a manual UUID", async () => {
    const { invoices, call } = setup(() => json(200, {}));
    await invoices.issue(7, { uuid: "6F1C2B3A-4D5E-4F60-8A7B-9C0D1E2F3A4B" });
    const form = call().init.body as FormData;
    expect(form.get("uuid")).toBe("6F1C2B3A-4D5E-4F60-8A7B-9C0D1E2F3A4B");
    expect(form.has("xml")).toBe(false);
    expect(form.has("pdf")).toBe(false);
  });

  it("attaches a document with its kind", async () => {
    const { invoices, call } = setup(() => json(201, { document: { id: 3 }, warnings: [] }));
    await invoices.attach(7, "pdf", new File(["%PDF-1"], "f.pdf"));
    const form = call().init.body as FormData;
    expect(call().url).toBe("http://x/invoices/7/documents");
    expect(form.get("kind")).toBe("pdf");
    expect((form.get("file") as File).name).toBe("f.pdf");
  });

  it("downloads a document as a blob with the bearer token", async () => {
    const { invoices, call } = setup(() => new Response("%PDF-1.7", { status: 200, headers: { "Content-Type": "application/pdf" } }));
    const blob = await invoices.download(7, 3);
    // (jsdom and Node have different Blob classes, so check the behavior, not the prototype.)
    expect(blob.type).toBe("application/pdf");
    expect(await blob.text()).toBe("%PDF-1.7");
    expect(call().url).toBe("http://x/invoices/7/documents/3");
    expect(call().headers.Authorization).toBe("Bearer tok");
  });

  it("cancels through POST without a body", async () => {
    const { invoices, call } = setup(() => json(200, { id: 7, state: "cancelada" }));
    await invoices.cancel(7);
    expect(call().url).toBe("http://x/invoices/7/cancel");
    expect(call().init.method).toBe("POST");
    expect(call().init.body).toBeUndefined();
  });

  it("surfaces the error code and message", async () => {
    const { invoices } = setup(() => json(409, { error: "duplicate_uuid", message: "UUID already used" }));
    const err = await invoices.issue(7, { uuid: "x" }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 409, code: "duplicate_uuid", message: "UUID already used" });
  });

  it("a failed download surfaces the JSON error", async () => {
    const { invoices } = setup(() => json(404, { error: "not_found" }));
    await expect(invoices.download(7, 99)).rejects.toMatchObject({ status: 404, code: "not_found" });
  });
});
