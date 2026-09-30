import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { Checklist, InvoiceDetail } from "../../api/invoices";
import { SETTINGS } from "./fixtures";
import { ChecklistView } from "./ChecklistView";
import { ClientSelect } from "./ClientSelect";
import { IssuePanel } from "./IssuePanel";
import { PrepareForm } from "./PrepareForm";
import { StateBadge } from "./StateBadge";
import { WarningsList } from "./WarningsList";
import { describeInvoiceError } from "./errors";
import { formatBytes, isValidUuid, validateFile } from "./files";
import { describeWarning } from "./labels";

const api = vi.hoisted(() => ({ prepareInvoice: vi.fn(), issueInvoice: vi.fn() }));
vi.mock("../../api/invoices", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/invoices")>()), ...api }));

// Vitest runs without globals, so Testing Library does not unmount between tests on its own.
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function wrap(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

const UUID = "6F1C2B3A-4D5E-4F60-8A7B-9C0D1E2F3A4B";

describe("describeInvoiceError", () => {
  const err = (status: number, code: string, message?: string) => new ApiError(status, code, message);

  it("maps every known code to Spanish", () => {
    const cases: [ApiError, string][] = [
      [err(400, "unknown_client"), "El cliente no existe en Configuración"],
      [err(400, "invalid_invoice", "amount must be greater than zero"), "Los datos no son válidos: amount must be greater than zero"],
      [err(400, "xml_or_uuid_required"), "Sube el XML del CFDI o escribe el UUID"],
      [err(400, "invalid_uuid"), "8-4-4-4-12"],
      [err(400, "invalid_document", "the file is not a PDF"), "El archivo no es válido: the file is not a PDF"],
      [err(422, "invalid_cfdi"), "no es un CFDI válido"],
      [err(422, "cfdi_not_stamped"), "no está timbrado"],
      [err(422, "uuid_mismatch"), "no coincide"],
      [err(409, "duplicate_uuid"), "ya está registrado en otra factura"],
      [err(409, "invoice_cancelled"), "está cancelada"],
      [err(409, "invoice_already_issued"), "ya fue marcada como emitida"],
      [err(409, "invoice_not_issued"), "Primero marca la factura como emitida"],
      [err(409, "invoice_state_changed"), "cambió de estado"],
      [err(413, "request_too_large"), "demasiado grandes"],
      [err(503, "storage_unavailable"), "almacenamiento de documentos no está disponible"],
      [err(404, "not_found"), "ya no existe"],
    ];
    for (const [e, text] of cases) expect(describeInvoiceError(e)).toContain(text);
  });

  it("falls back to the generic save error for the rest", () => {
    expect(describeInvoiceError(err(500, "internal_error"))).toContain("El servidor tuvo un problema");
    expect(describeInvoiceError(err(401, "unauthorized"))).toContain("sesión expiró");
    expect(describeInvoiceError(err(422, "settings_incomplete"))).toContain("Faltan parámetros fiscales");
    expect(describeInvoiceError(new TypeError("Failed to fetch"))).toContain("No se pudo conectar");
  });
});

describe("file helpers", () => {
  const file = (name: string, size: number) => new File([new Uint8Array(size)], name);

  it("validates extension, emptiness and size per kind", () => {
    expect(validateFile(file("cfdi.XML", 10), "xml")).toBeNull();
    expect(validateFile(file("f.pdf", 10), "pdf")).toBeNull();
    expect(validateFile(file("f.txt", 10), "xml")).toContain(".xml");
    expect(validateFile(file("f.xml", 10), "pdf")).toContain(".pdf");
    expect(validateFile(file("f.xml", 0), "xml")).toContain("vacío");
    expect(validateFile(file("f.xml", (1 << 20) + 1), "xml")).toContain("máximo es 1.0 MB");
    expect(validateFile(file("f.pdf", (10 << 20) + 1), "pdf")).toContain("máximo es 10.0 MB");
  });

  it("formats sizes", () => {
    expect(formatBytes(12)).toBe("12 B");
    expect(formatBytes(2048)).toBe("2 KB");
    expect(formatBytes(1_572_864)).toBe("1.5 MB");
  });

  it("checks the UUID format case-insensitively", () => {
    expect(isValidUuid(UUID)).toBe(true);
    expect(isValidUuid(` ${UUID.toLowerCase()} `)).toBe(true);
    for (const bad of ["", "abc", UUID + "0", UUID.replace("F", "G")]) expect(isValidUuid(bad)).toBe(false);
  });
});

describe("describeWarning", () => {
  it("formats amounts with the invoice currency", () => {
    const text = describeWarning({ code: "total_mismatch", message: "x", expected: "2500.00", actual: "2400" }, "USD");
    expect(text).toContain("$2,400.00 USD");
    expect(text).toContain("$2,500.00 USD");
  });

  it("lists the duplicated invoices and keeps unknown codes readable", () => {
    expect(describeWarning({ code: "possible_duplicate", message: "x", invoice_ids: [3, 5] })).toContain("(#3, #5)");
    expect(describeWarning({ code: "future_code", message: "backend text" })).toBe("backend text");
  });
});

describe("small components", () => {
  it("shows the state as text", () => {
    render(
      <>
        <StateBadge state="preparada" />
        <StateBadge state="emitida" />
        <StateBadge state="cancelada" />
      </>,
    );
    for (const label of ["Preparada", "Emitida", "Cancelada"]) expect(screen.getByText(label)).toBeInTheDocument();
  });

  it("renders nothing without warnings and a list with them", () => {
    const { container, rerender } = render(<WarningsList warnings={[]} currency="USD" />);
    expect(container).toBeEmptyDOMElement();
    rerender(<WarningsList warnings={[{ code: "possible_duplicate", message: "x", invoice_ids: [2] }]} currency="USD" />);
    expect(screen.getByRole("status", { name: "Advertencias" })).toHaveTextContent("(#2)");
  });

  it("keeps a client that is no longer configured selectable", () => {
    render(<ClientSelect label="Cliente" value="old" onChange={() => {}} clients={SETTINGS.clients} emptyLabel="Elige un cliente" />);
    expect(screen.getByRole("option", { name: "old" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Acme Inc." })).toBeInTheDocument();
  });
});

describe("ChecklistView", () => {
  const checklist = (over: Partial<Checklist> = {}): Checklist => ({
    issuer: { rfc: "AAA010101AAA", name: "Juan", regimen: "626", postal_code: "64000" },
    receiver: { rfc: "XEXX010101000", name: "Acme", regimen: "616", postal_code: "64000", uso_cfdi: "S01" },
    voucher: { type: "I", currency: "USD", exchange_rate: "17.50", payment_form: "03", payment_method: "PUE", export: true, global: null },
    concept: { prod_serv_key: "81111500", unit_key: "E48", description: "Servicios", quantity: 1, unit_value: "2500.00" },
    taxes: { iva_included: false, iva: "0.00" },
    totals: { currency: "USD", subtotal: "2500.00", total: "2500.00", expected_deposit_mxn: "43750.00" },
    period: "2026-10",
    due_date: "2026-11-17",
    missing_config: [],
    to_confirm: ["fx_rate_dof", "prod_serv_key", "export_key"],
    ...over,
  });

  it("shows the export invoice with the confirm tags", () => {
    render(<ChecklistView checklist={checklist()} />);
    expect(screen.getByRole("region", { name: "Impuestos" })).toHaveTextContent("Tasa 0% = $0.00");
    expect(screen.getByRole("region", { name: "Totales" })).toHaveTextContent("$43,750.00 MXN");
    expect(within(screen.getByRole("region", { name: "Comprobante" })).getAllByText("confirmar con contador")).toHaveLength(2);
    expect(within(screen.getByRole("region", { name: "Concepto" })).getAllByText("confirmar con contador")).toHaveLength(1);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByText("2026-11-17")).toBeInTheDocument();
  });

  it("shows the global information and IVA of a public-in-general invoice", () => {
    render(
      <ChecklistView
        checklist={checklist({
          voucher: { type: "I", currency: "MXN", exchange_rate: null, payment_form: "03", payment_method: "PUE", export: false, global: { periodicity: "quincenal", code: "03", months: "10", year: "2026" } },
          taxes: { iva_included: true, iva: "4827.59" },
          receiver: { rfc: "XAXX010101000", name: "Público", regimen: "616", postal_code: "64000", uso_cfdi: "S01", internal_note: "Empresa pagadora" },
          to_confirm: ["global_info"],
        })}
      />,
    );
    expect(screen.getByRole("region", { name: "Comprobante" })).toHaveTextContent("03 Quincenal");
    expect(screen.getByRole("region", { name: "Impuestos" })).toHaveTextContent("$4,827.59 MXN");
    expect(screen.getByRole("region", { name: "Receptor" })).toHaveTextContent("Empresa pagadora");
    expect(screen.getByRole("region", { name: "Comprobante" })).not.toHaveTextContent("Clave de exportación");
  });

  it("flags empty issuer fields as pending and lists the missing settings", () => {
    render(<ChecklistView checklist={checklist({ issuer: { rfc: "", name: "", regimen: "626", postal_code: "" }, missing_config: ["issuer.rfc", "issuer.postal_code"] })} />);
    expect(screen.getByRole("alert")).toHaveTextContent("RFC del emisor, código postal del emisor");
    expect(within(screen.getByRole("region", { name: "Emisor" })).getAllByText(/Pendiente/)).toHaveLength(3);
  });
});

describe("PrepareForm", () => {
  const detail = { invoice: { id: 9 }, documents: [], checklist: {}, warnings: [{ code: "possible_duplicate", message: "x", invoice_ids: [2] }] } as unknown as InvoiceDetail;

  function setup() {
    api.prepareInvoice.mockResolvedValue(detail);
    const onPrepared = vi.fn();
    wrap(<PrepareForm settings={SETTINGS} onPrepared={onPrepared} />);
    return onPrepared;
  }
  const choose = (name: string) => fireEvent.change(screen.getByLabelText("Cliente"), { target: { value: name } });
  const submit = () => fireEvent.click(screen.getByRole("button", { name: "Preparar factura" }));

  it("asks for a client first", () => {
    setup();
    submit();
    expect(screen.getByRole("alert")).toHaveTextContent("Elige un cliente.");
    expect(api.prepareInvoice).not.toHaveBeenCalled();
  });

  it("sends only what the USA client typed and keeps amounts as strings", async () => {
    const onPrepared = setup();
    choose("usa");
    fireEvent.change(screen.getByLabelText("Subtotal (USD)"), { target: { value: "3383.33" } });
    fireEvent.change(screen.getByLabelText("Tipo de cambio (opcional)"), { target: { value: "17.74" } });
    fireEvent.change(screen.getByLabelText("Fecha de cobro"), { target: { value: "2026-10-15" } });
    submit();
    await waitFor(() => expect(onPrepared).toHaveBeenCalledWith(detail, "mensual"));
    expect(api.prepareInvoice).toHaveBeenCalledWith({ client_id: "usa", date: "2026-10-15", subtotal: "3383.33", exchange_rate: "17.74" });
  });

  it("omits empty USA fields so the backend applies the configured defaults", async () => {
    setup();
    choose("usa");
    fireEvent.change(screen.getByLabelText("Fecha de cobro"), { target: { value: "2026-10-15" } });
    submit();
    await waitFor(() => expect(api.prepareInvoice).toHaveBeenCalled());
    expect(api.prepareInvoice.mock.calls[0][0]).toEqual({ client_id: "usa", date: "2026-10-15" });
  });

  it("requires the total received for any other client and sends the periodicity", async () => {
    const onPrepared = setup();
    choose("b");
    expect(screen.queryByLabelText("Subtotal (USD)")).not.toBeInTheDocument();
    submit();
    expect(screen.getByRole("alert")).toHaveTextContent("total recibido");
    expect(api.prepareInvoice).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText("Total recibido (IVA incluido)"), { target: { value: "35000" } });
    fireEvent.change(screen.getByLabelText("Periodicidad (factura global)"), { target: { value: "quincenal" } });
    fireEvent.change(screen.getByLabelText("Fecha de cobro"), { target: { value: "2026-10-31" } });
    submit();
    await waitFor(() => expect(onPrepared).toHaveBeenCalledWith(detail, "quincenal"));
    expect(api.prepareInvoice).toHaveBeenCalledWith({ client_id: "b", date: "2026-10-31", amount: "35000", periodicity: "quincenal" });
  });

  it("rejects non-positive or malformed numbers", () => {
    setup();
    choose("usa");
    fireEvent.change(screen.getByLabelText("Subtotal (USD)"), { target: { value: "0" } });
    fireEvent.change(screen.getByLabelText("Tipo de cambio (opcional)"), { target: { value: "17,5" } });
    submit();
    expect(screen.getAllByRole("alert").map((a) => a.textContent).join(" ")).toMatch(/subtotal mayor a cero.*tipo de cambio válido/);
    expect(api.prepareInvoice).not.toHaveBeenCalled();
  });

  it("shows the mapped API error", async () => {
    api.prepareInvoice.mockRejectedValue(new ApiError(400, "unknown_client"));
    wrap(<PrepareForm settings={SETTINGS} onPrepared={() => {}} />);
    choose("b");
    fireEvent.change(screen.getByLabelText("Total recibido (IVA incluido)"), { target: { value: "10" } });
    submit();
    expect(await screen.findByText(/El cliente no existe en Configuración/)).toBeInTheDocument();
  });

  it("explains there are no clients", () => {
    wrap(<PrepareForm settings={{ ...SETTINGS, clients: [] }} onPrepared={() => {}} />);
    expect(screen.getByText(/Aún no hay clientes/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Preparar factura" })).not.toBeInTheDocument();
  });
});

describe("IssuePanel", () => {
  const xml = new File(["<a/>"], "cfdi.xml", { type: "text/xml" });
  const pick = (label: string, file: File) => fireEvent.change(screen.getByLabelText(label), { target: { files: [file] } });
  const submit = () => fireEvent.click(screen.getByRole("button", { name: "Marcar como emitida" }));

  it("requires an XML or a UUID", () => {
    wrap(<IssuePanel invoiceId={4} onIssued={() => {}} />);
    submit();
    expect(screen.getByRole("alert")).toHaveTextContent("Sube el XML del CFDI o escribe el UUID");
    expect(api.issueInvoice).not.toHaveBeenCalled();
  });

  it("validates the UUID format and the PDF extension", () => {
    wrap(<IssuePanel invoiceId={4} onIssued={() => {}} />);
    fireEvent.change(screen.getByLabelText("UUID manual"), { target: { value: "nope" } });
    pick("PDF (opcional)", new File(["x"], "f.txt"));
    submit();
    const alerts = screen.getAllByRole("alert").map((a) => a.textContent);
    expect(alerts.join(" ")).toMatch(/8-4-4-4-12/);
    expect(alerts.join(" ")).toMatch(/extensión \.pdf/);
    expect(api.issueInvoice).not.toHaveBeenCalled();
  });

  it("sends the XML and ignores the manual UUID field", async () => {
    const issued = { invoice: { id: 4 }, documents: [], checklist: {}, warnings: [{ code: "total_mismatch", message: "x", expected: "1", actual: "2" }] } as unknown as InvoiceDetail;
    api.issueInvoice.mockResolvedValue(issued);
    const onIssued = vi.fn();
    wrap(<IssuePanel invoiceId={4} onIssued={onIssued} />);
    fireEvent.change(screen.getByLabelText("UUID manual"), { target: { value: UUID } });
    pick("XML del CFDI", xml);
    expect(screen.getByLabelText("UUID manual")).toBeDisabled();
    submit();
    await waitFor(() => expect(onIssued).toHaveBeenCalledWith(issued));
    expect(api.issueInvoice).toHaveBeenCalledWith(4, { xml, pdf: null, uuid: undefined });
  });

  it("sends a valid manual UUID alone and shows a mapped server error", async () => {
    api.issueInvoice.mockRejectedValue(new ApiError(409, "duplicate_uuid", "UUID already used by another invoice"));
    wrap(<IssuePanel invoiceId={4} onIssued={() => {}} />);
    fireEvent.change(screen.getByLabelText("UUID manual"), { target: { value: `  ${UUID.toLowerCase()} ` } });
    submit();
    expect(await screen.findByText("Ese UUID ya está registrado en otra factura.")).toBeInTheDocument();
    expect(api.issueInvoice).toHaveBeenCalledWith(4, { xml: null, pdf: null, uuid: UUID.toLowerCase() });
  });
});
