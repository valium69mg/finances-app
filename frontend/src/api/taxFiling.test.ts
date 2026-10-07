import { describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient } from "./client";
import { createTaxFilingApi } from "./taxFiling";

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
  const taxFiling = createTaxFilingApi(client);
  const call = (i = 0) => {
    const [url, init] = fetchFn.mock.calls[i] as [string, RequestInit];
    return { url, init, headers: (init.headers ?? {}) as Record<string, string> };
  };
  return { taxFiling, call };
}

describe("tax filing api", () => {
  it("previews a period, sending the creditable IVA only when set", async () => {
    const { taxFiling, call } = setup(() => json(200, { period: "2026-10" }));
    await taxFiling.preview("2026-10");
    await taxFiling.preview("2026-10", "800.50");
    expect(call(0).url).toBe("http://x/tax-filing/preview?period=2026-10");
    expect(call(0).init.method).toBe("GET");
    expect(call(0).headers.Authorization).toBe("Bearer tok");
    expect(call(1).url).toBe("http://x/tax-filing/preview?period=2026-10&iva_acreditable=800.50");
  });

  it("registers with decimal strings untouched", async () => {
    const { taxFiling, call } = setup(() => json(201, { filing: { period: "2026-10" }, warnings: [] }));
    await taxFiling.register({
      period: "2026-10",
      filing_date: "2026-11-05",
      folio: "ACUSE-1",
      iva_acreditable: "800.50",
      payment: { isr_paid: "1845.25", iva_paid: "4827.59", record_expense: true },
    });
    const { url, init, headers } = call();
    expect(url).toBe("http://x/tax-filing");
    expect(init.method).toBe("POST");
    expect(headers["Content-Type"]).toBe("application/json");
    expect(JSON.parse(String(init.body))).toEqual({
      period: "2026-10",
      filing_date: "2026-11-05",
      folio: "ACUSE-1",
      iva_acreditable: "800.50",
      payment: { isr_paid: "1845.25", iva_paid: "4827.59", record_expense: true },
    });
  });

  it("lists with only the filters that are set", async () => {
    const { taxFiling, call } = setup(() => json(200, []));
    await expect(taxFiling.list()).resolves.toEqual([]);
    await taxFiling.list({ year: "2026", status: "pendiente" });
    await taxFiling.list({ year: "", status: "" });
    expect(call(0).url).toBe("http://x/tax-filing");
    expect(call(1).url).toBe("http://x/tax-filing?year=2026&status=pendiente");
    expect(call(2).url).toBe("http://x/tax-filing");
  });

  it("gets a filing by period", async () => {
    const { taxFiling, call } = setup(() => json(200, {}));
    await taxFiling.get("2026-10");
    expect(call().url).toBe("http://x/tax-filing/2026-10");
  });

  it("marks a filing as paid", async () => {
    const { taxFiling, call } = setup(() => json(200, { filing: {}, warnings: [] }));
    await taxFiling.pay("2026-10", { date: "2026-11-12", isr_paid: "10", iva_paid: "0" });
    const { url, init } = call();
    expect(url).toBe("http://x/tax-filing/2026-10/payment");
    expect(init.method).toBe("POST");
    // record_expense is not sent unless the caller asks for it.
    expect(JSON.parse(String(init.body))).toEqual({ date: "2026-11-12", isr_paid: "10", iva_paid: "0" });
  });

  it("lists the invoices left out of a filed period", async () => {
    const { taxFiling, call } = setup(() => json(200, [{ id: 9, period: "2026-10" }]));
    await expect(taxFiling.unfiledInvoices()).resolves.toHaveLength(1);
    expect(call().url).toBe("http://x/tax-filing/unfiled-invoices");
  });

  it("lists the pending periods", async () => {
    const { taxFiling, call } = setup(() => json(200, [{ period: "2026-10", due_date: "2026-11-17", overdue: false }]));
    await expect(taxFiling.pending()).resolves.toHaveLength(1);
    expect(call().url).toBe("http://x/tax-filing/pending-periods");
  });

  it("deletes a filing and accepts the empty response", async () => {
    const { taxFiling, call } = setup(() => json(204));
    await expect(taxFiling.remove("2026-10")).resolves.toBeUndefined();
    expect(call().url).toBe("http://x/tax-filing/2026-10");
    expect(call().init.method).toBe("DELETE");
  });

  it("surfaces the API error code", async () => {
    const { taxFiling } = setup(() => json(409, { error: "already_filed", message: "period already filed: 2026-10" }));
    await expect(taxFiling.register({ period: "2026-10" })).rejects.toMatchObject({ status: 409, code: "already_filed" } satisfies Partial<ApiError>);
  });

  it("uploads a document with PUT and a multipart body, never JSON", async () => {
    const { taxFiling, call } = setup(() => json(200, { period: "2026-10", documents: [] }));
    const file = new File(["%PDF-1.4"], "acuse.pdf", { type: "application/pdf" });
    await taxFiling.attachDocument("2026-10", "acuse", file);
    const { url, init, headers } = call();
    expect(url).toBe("http://x/tax-filing/2026-10/documents/acuse");
    expect(init.method).toBe("PUT");
    expect(init.body).toBeInstanceOf(FormData);
    expect((init.body as FormData).get("file")).toBeInstanceOf(File);
    expect(((init.body as FormData).get("file") as File).name).toBe("acuse.pdf");
    expect(headers["Content-Type"]).toBeUndefined();
    expect(headers.Authorization).toBe("Bearer tok");
  });

  it("downloads a document as a blob through the authenticated API", async () => {
    const { taxFiling, call } = setup(() => new Response("%PDF-1.4", { status: 200, headers: { "Content-Type": "application/pdf" } }));
    const blob = await taxFiling.downloadDocument("2026-10", "comprobante");
    expect(blob.size).toBe(8);
    expect(blob.type).toBe("application/pdf");
    expect(call().url).toBe("http://x/tax-filing/2026-10/documents/comprobante");
    expect(call().init.method).toBe("GET");
    expect(call().headers.Authorization).toBe("Bearer tok");
  });

  it("surfaces the upload error codes", async () => {
    const { taxFiling } = setup(() => json(400, { error: "invalid_document", message: "the acuse must be a PDF" }));
    await expect(taxFiling.attachDocument("2026-10", "acuse", new File(["x"], "a.pdf"))).rejects.toMatchObject({ status: 400, code: "invalid_document" } satisfies Partial<ApiError>);
  });
});
