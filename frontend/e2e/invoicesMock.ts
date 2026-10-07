import type { Request, Route } from "@playwright/test";

/** In-memory stand-in for the /invoices endpoints, used by helpers.ts. */

export interface MockDocument {
  id: number;
  kind: "xml" | "pdf";
  name: string;
  content_type: string;
  size: number;
  sha256: string;
  uploaded_at: string;
  /** File content served back by the download endpoint. */
  content?: string;
}

export interface MockInvoice {
  id: number;
  client_id: string;
  collection_date: string;
  period: string;
  currency: string;
  exchange_rate: string | null;
  subtotal: string;
  subtotal_mxn: string;
  iva: string;
  isr_withheld: string;
  iva_withheld: string;
  total: string;
  expected_deposit_mxn: string;
  state: "preparada" | "emitida" | "cancelada";
  uuid: string | null;
  movement_id: number | null;
  declaration_period: string | null;
  created_at: string;
  documents?: MockDocument[];
}

/** What the API received in a multipart upload (file names and text content). */
export interface MockUpload {
  method: string;
  path: string;
  fields: Record<string, string>;
  files: Record<string, { name: string; content: string }>;
}

export type InvoicesFail = "list" | "detail" | "prepare" | "storage" | "download";

export interface Part {
  name: string;
  filename?: string;
  content: string;
}

/** Minimal multipart/form-data reader; enough for the text files the tests upload. */
export function parseMultipart(request: Request): Part[] {
  const type = request.headers()["content-type"] ?? "";
  const boundary = /boundary=(.+)$/.exec(type)?.[1];
  const body = request.postDataBuffer()?.toString("utf8") ?? "";
  if (!boundary) return [];
  const parts: Part[] = [];
  for (const chunk of body.split(`--${boundary}`)) {
    const m = /^\r\n([\s\S]*?)\r\n\r\n([\s\S]*?)\r\n$/.exec(chunk);
    if (!m) continue;
    const name = /name="([^"]*)"/.exec(m[1])?.[1];
    if (name === undefined) continue;
    parts.push({ name, filename: /filename="([^"]*)"/.exec(m[1])?.[1], content: m[2] });
  }
  return parts;
}

const money = (n: number) => n.toFixed(2);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Settings = any;

function buildChecklist(inv: MockInvoice, settings: Settings, periodicity: string) {
  const client = settings.clients.find((c: { id: string }) => c.id === inv.client_id);
  const issuer = settings.issuer ?? { rfc: "", name: "", regimen: "", postal_code: "" };
  const usa = inv.client_id === "usa";
  const missing: string[] = [];
  if (!issuer.rfc) missing.push("issuer.rfc");
  if (!issuer.name) missing.push("issuer.name");
  if (!issuer.postal_code) missing.push("issuer.postal_code");
  const [year, month] = inv.period.split("-");
  const confirm = ["prod_serv_key"];
  if (inv.currency === "USD") confirm.unshift("fx_rate_dof");
  if (usa) confirm.push("export_key", "tax_object");
  else confirm.push("global_info", "unit_key");
  const dueMonth = Number(month) === 12 ? `${Number(year) + 1}-01` : `${year}-${String(Number(month) + 1).padStart(2, "0")}`;
  return {
    issuer: { rfc: issuer.rfc, name: issuer.name, regimen: issuer.regimen || "626", postal_code: issuer.postal_code },
    receiver: {
      rfc: client?.rfc ?? "",
      name: client?.name ?? "",
      regimen: client?.regimen ?? "",
      postal_code: issuer.postal_code,
      uso_cfdi: client?.uso_cfdi ?? "",
      ...(client?.real_payer ? { internal_note: client.real_payer } : {}),
    },
    voucher: {
      type: "I",
      currency: inv.currency,
      exchange_rate: inv.exchange_rate,
      payment_form: "03",
      payment_method: "PUE",
      export: usa,
      global: usa
        ? null
        : { periodicity: periodicity || "mensual", code: periodicity === "quincenal" ? "03" : "04", months: month, year },
    },
    concept: {
      prod_serv_key: client?.clave_prod_serv ?? "",
      unit_key: client?.clave_unidad ?? "",
      description: client?.concepto ?? "",
      quantity: 1,
      unit_value: inv.subtotal,
    },
    taxes: { iva_included: !usa, iva: inv.iva },
    totals: { currency: inv.currency, subtotal: inv.subtotal, total: inv.total, expected_deposit_mxn: inv.expected_deposit_mxn },
    period: inv.period,
    due_date: `${dueMonth}-17`,
    missing_config: missing,
    to_confirm: confirm,
  };
}

function toDTO(inv: MockInvoice) {
  const { documents: _documents, ...rest } = inv;
  void _documents;
  return rest;
}

export function createInvoicesMock(
  seed: MockInvoice[],
  fail: InvoicesFail | undefined,
  json: (route: Route, status: number, body?: unknown) => Promise<void>,
) {
  const invoices: MockInvoice[] = structuredClone(seed);
  const uploads: MockUpload[] = [];
  let nextId = invoices.reduce((max, i) => Math.max(max, i.id), 0) + 1;
  let nextDocId = invoices.flatMap((i) => i.documents ?? []).reduce((max, d) => Math.max(max, d.id), 0) + 1;

  const detail = (inv: MockInvoice, settings: Settings, periodicity: string, warnings: unknown[] = []) => ({
    invoice: toDTO(inv),
    documents: (inv.documents ?? []).map(({ content: _c, ...d }) => {
      void _c;
      return d;
    }),
    checklist: buildChecklist(inv, settings, periodicity),
    warnings,
  });

  const storeDoc = (inv: MockInvoice, kind: "xml" | "pdf", file: { name: string; content: string }): MockDocument => {
    const docs = (inv.documents ??= []);
    const existing = docs.find((d) => d.kind === kind);
    const doc: MockDocument = {
      id: existing?.id ?? nextDocId++,
      kind,
      name: file.name,
      content_type: kind === "xml" ? "application/xml" : "application/pdf",
      size: file.content.length,
      sha256: "0".repeat(64),
      uploaded_at: "2026-10-20T12:00:00Z",
      content: file.content,
    };
    if (existing) docs[docs.indexOf(existing)] = doc;
    else docs.push(doc);
    docs.sort((a, b) => (a.kind === b.kind ? 0 : a.kind === "xml" ? -1 : 1));
    return doc;
  };

  function cfdiWarnings(inv: MockInvoice, xml: string) {
    const total = /Total="([^"]*)"/.exec(xml)?.[1] ?? "";
    const warnings = [];
    if (total && Number(total) !== Number(inv.total)) {
      warnings.push({ code: "total_mismatch", message: "the XML total differs from the prepared invoice total", expected: inv.total, actual: total });
    }
    return warnings;
  }

  async function handle(route: Route, request: Request, pathname: string, params: URLSearchParams, settings: Settings): Promise<boolean> {
    if (!pathname.startsWith("/invoices")) return false;
    const method = request.method();

    if (pathname === "/invoices" && method === "GET") {
      if (fail === "list") return json(route, 500, { error: "internal_error" }).then(() => true);
      const period = params.get("period") ?? "";
      const state = params.get("state") ?? "";
      const rows = invoices.filter((i) => (!period || i.period === period) && (!state || i.state === state)).sort((a, b) => b.id - a.id);
      await json(route, 200, rows.map(toDTO));
      return true;
    }

    if (pathname === "/invoices" && method === "POST") {
      const body = request.postDataJSON();
      uploads.push({ method, path: pathname, fields: body, files: {} });
      if (fail === "prepare") {
        await json(route, 400, { error: "invalid_invoice", message: "invalid invoice input: amount must be greater than zero" });
        return true;
      }
      const client = settings.clients.find((c: { id: string }) => c.id === body.client_id);
      if (!client) {
        await json(route, 400, { error: "unknown_client", message: `unknown client: "${body.client_id}"` });
        return true;
      }
      const date: string = body.date;
      const usa = body.client_id === "usa";
      const rate = usa ? (body.exchange_rate ?? settings.general.fx_rate_applied) : null;
      const subtotalUsd = usa ? Number(body.subtotal ?? settings.general.salary_usd) : 0;
      const total = usa ? subtotalUsd : Number(body.amount);
      const subtotal = usa ? subtotalUsd : total / (1 + Number(client.iva_rate));
      const dup = invoices.filter((i) => i.client_id === body.client_id && i.collection_date === date && i.state !== "cancelada").map((i) => i.id);
      const inv: MockInvoice = {
        id: nextId++,
        client_id: body.client_id,
        collection_date: date,
        period: date.slice(0, 7),
        currency: usa ? "USD" : "MXN",
        exchange_rate: rate,
        subtotal: money(subtotal),
        subtotal_mxn: money(usa ? subtotal * Number(rate) : subtotal),
        iva: money(usa ? 0 : total - Number(money(subtotal))),
        isr_withheld: "0.00",
        iva_withheld: "0.00",
        total: money(total),
        expected_deposit_mxn: money(usa ? subtotal * Number(rate) : total),
        state: "preparada",
        uuid: null,
        movement_id: body.movement_id ?? null,
        declaration_period: null,
        created_at: "2026-10-20T12:00:00Z",
      };
      invoices.push(inv);
      const warnings = dup.length ? [{ code: "possible_duplicate", message: "an invoice for the same client and collection date already exists", invoice_ids: dup }] : [];
      await json(route, 201, detail(inv, settings, body.periodicity ?? "", warnings));
      return true;
    }

    const m = /^\/invoices\/(\d+)(?:\/(issue|documents|cancel)(?:\/(\d+))?)?$/.exec(pathname);
    if (!m) return false;
    const inv = invoices.find((i) => i.id === Number(m[1]));
    const action = m[2];
    if (!inv) {
      await json(route, 404, { error: "not_found" });
      return true;
    }

    if (!action && method === "GET") {
      if (fail === "detail") {
        await json(route, 500, { error: "internal_error" });
        return true;
      }
      await json(route, 200, detail(inv, settings, params.get("periodicity") ?? ""));
      return true;
    }

    if (action === "cancel" && method === "POST") {
      uploads.push({ method, path: pathname, fields: {}, files: {} });
      if (inv.state === "cancelada") await json(route, 409, { error: "invoice_cancelled", message: "invoice is cancelled" });
      else if (inv.declaration_period) {
        await json(route, 409, { error: "invoice_declared", message: `invoice is included in a filed tax declaration: #${inv.id} is part of the filing of ${inv.declaration_period}` });
      } else {
        inv.state = "cancelada";
        await json(route, 200, toDTO(inv));
      }
      return true;
    }

    if (action === "documents" && m[3] && method === "GET") {
      const doc = (inv.documents ?? []).find((d) => d.id === Number(m[3]));
      if (!doc) await json(route, 404, { error: "not_found" });
      else if (fail === "download") await json(route, 503, { error: "storage_unavailable" });
      else {
        await route.fulfill({
          status: 200,
          headers: {
            "Access-Control-Allow-Origin": "*",
            "Content-Type": doc.content_type,
            "Content-Disposition": `attachment; filename="${doc.name}"`,
          },
          body: doc.content ?? "",
        });
      }
      return true;
    }

    if ((action === "issue" || action === "documents") && method === "POST") {
      const parts = parseMultipart(request);
      const upload: MockUpload = { method, path: pathname, fields: {}, files: {} };
      for (const p of parts) {
        if (p.filename !== undefined) upload.files[p.name] = { name: p.filename, content: p.content };
        else upload.fields[p.name] = p.content;
      }
      uploads.push(upload);
      if (fail === "storage") {
        await json(route, 503, { error: "storage_unavailable", message: "document storage unavailable" });
        return true;
      }
      if (inv.state === "cancelada") {
        await json(route, 409, { error: "invoice_cancelled", message: "invoice is cancelled" });
        return true;
      }

      if (action === "issue") {
        if (inv.state === "emitida") {
          await json(route, 409, { error: "invoice_already_issued", message: "invoice is already issued" });
          return true;
        }
        const xml = upload.files.xml;
        const manual = upload.fields.uuid;
        if (!xml && !manual) {
          await json(route, 400, { error: "xml_or_uuid_required", message: "issue requires an XML or a UUID" });
          return true;
        }
        let uuid = manual?.toUpperCase();
        let warnings: unknown[] = [];
        if (xml) {
          uuid = /UUID="([^"]*)"/.exec(xml.content)?.[1]?.toUpperCase();
          if (!uuid) {
            await json(route, 422, { error: "cfdi_not_stamped", message: "CFDI XML has no TimbreFiscalDigital" });
            return true;
          }
          warnings = cfdiWarnings(inv, xml.content);
        }
        if (invoices.some((i) => i.id !== inv.id && i.uuid === uuid)) {
          await json(route, 409, { error: "duplicate_uuid", message: "UUID already used by another invoice" });
          return true;
        }
        inv.state = "emitida";
        inv.uuid = uuid ?? null;
        if (xml) storeDoc(inv, "xml", xml);
        if (upload.files.pdf) storeDoc(inv, "pdf", upload.files.pdf);
        await json(route, 200, detail(inv, settings, "", warnings));
        return true;
      }

      // POST /invoices/{id}/documents
      if (inv.state !== "emitida") {
        await json(route, 409, { error: "invoice_not_issued", message: "invoice is not issued" });
        return true;
      }
      const kind = upload.fields.kind as "xml" | "pdf";
      const file = upload.files.file;
      const doc = storeDoc(inv, kind, file);
      const { content: _c, ...dto } = doc;
      void _c;
      await json(route, 201, { document: dto, warnings: kind === "xml" ? cfdiWarnings(inv, file.content) : [] });
      return true;
    }
    return false;
  }

  return { invoices, uploads, handle };
}
