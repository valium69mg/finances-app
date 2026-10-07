import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { PendingPeriod } from "../../api/taxFiling";
import { Breakdown } from "./Breakdown";
import { FILING, PAID_FILING, PREVIEW } from "./fixtures";
import { FilingHistory } from "./FilingHistory";
import { InvoiceRefs, TaxWarnings } from "./InvoiceRefs";
import { PaymentDialog } from "./PaymentDialog";
import { PendingPeriods } from "./PendingPeriods";
import { RegisterForm } from "./RegisterForm";
import { UnfiledInvoices } from "./UnfiledInvoices";
import { FilingStatusBadge } from "./StatusBadge";
import { describeTaxFilingError } from "./errors";
import { dateLabel, describeTaxWarning, periodLabel, previousPeriod } from "./labels";
import { amountToPay, defaultPaymentDraft, isPaidAmount, toPaymentInput, validatePayment } from "./payment";

const api = vi.hoisted(() => ({
  registerTaxFiling: vi.fn(),
  payTaxFiling: vi.fn(),
  listTaxFilings: vi.fn(),
  listPendingPeriods: vi.fn(),
  listUnfiledInvoices: vi.fn(),
}));
vi.mock("../../api/taxFiling", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/taxFiling")>()), ...api }));

// Vitest runs without globals, so Testing Library does not unmount between tests on its own.
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function wrap(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("payment helpers", () => {
  it("accepts zero or more with at most 2 decimals", () => {
    for (const ok of ["0", "0.00", "591.25", "1600", "999999999999.99"]) expect(isPaidAmount(ok), ok).toBe(true);
    for (const bad of ["", "-1", "1,600", "1.234", "abc", "1e3", "1000000000000", " "]) expect(isPaidAmount(bad), bad).toBe(false);
  });

  it("defaults the amounts to what is due, with 0 for an IVA balance in favor", () => {
    expect(amountToPay("1600.00")).toBe("1600.00");
    expect(amountToPay("-172.41")).toBe("0.00");
    expect(defaultPaymentDraft("591.25", "-10.00", "2026-11-12")).toEqual({ date: "2026-11-12", isr: "591.25", iva: "0.00", recordExpense: false });
  });

  it("validates the date, the amounts and the expense", () => {
    const ok = { date: "2026-11-12", isr: "1", iva: "0", recordExpense: false };
    expect(validatePayment(ok)).toEqual({});
    expect(validatePayment({ ...ok, date: "" }).date).toContain("fecha de pago");
    expect(validatePayment({ ...ok, isr: "x" }).isr).toContain("monto de cero o más");
    expect(validatePayment({ ...ok, iva: "-3" }).iva).toContain("monto de cero o más");
    expect(validatePayment({ ...ok, isr: "0", recordExpense: true }).expense).toContain("mayor a cero");
    expect(validatePayment({ ...ok, isr: "0", iva: "10", recordExpense: true })).toEqual({});
    // A bad amount is reported on its field, not as an expense problem.
    expect(validatePayment({ ...ok, isr: "x", recordExpense: true }).expense).toBeUndefined();
  });

  it("only sends record_expense when it is on", () => {
    expect(toPaymentInput({ date: "2026-11-12", isr: " 10 ", iva: "0", recordExpense: false })).toEqual({ date: "2026-11-12", isr_paid: "10", iva_paid: "0" });
    expect(toPaymentInput({ date: "2026-11-12", isr: "10", iva: "0", recordExpense: true })).toEqual({
      date: "2026-11-12",
      isr_paid: "10",
      iva_paid: "0",
      record_expense: true,
    });
  });
});

describe("labels", () => {
  it("formats periods and dates in Spanish", () => {
    expect(periodLabel("2026-10")).toBe("octubre de 2026");
    expect(periodLabel("2026-13")).toBe("2026-13");
    expect(dateLabel("2026-11-17")).toBe("17 de noviembre de 2026");
    expect(dateLabel("hoy")).toBe("hoy");
  });

  it("finds the previous period across a year boundary", () => {
    expect(previousPeriod("2026-10")).toBe("2026-09");
    expect(previousPeriod("2026-01")).toBe("2025-12");
  });

  it("describes the warnings and falls back to the backend message", () => {
    expect(describeTaxWarning({ code: "prepared_invoices", message: "m", invoice_ids: [3, 5] })).toContain("(#3, #5)");
    expect(describeTaxWarning({ code: "already_filed", message: "m" })).toContain("ya fue declarado");
    expect(describeTaxWarning({ code: "otra", message: "mensaje del servidor" })).toBe("mensaje del servidor");
  });
});

describe("describeTaxFilingError", () => {
  const err = (status: number, code: string, message?: string) => new ApiError(status, code, message);

  it("maps every known code to Spanish", () => {
    const cases: [ApiError, string][] = [
      [err(400, "invalid_filing", "creditable IVA cannot be negative"), "Los datos no son válidos: creditable IVA cannot be negative"],
      [err(400, "invalid_expense", "amount must be greater than zero"), "No se pudo registrar el gasto de Impuestos: amount must be greater than zero"],
      [err(409, "already_filed"), "ya está declarado"],
      [err(409, "already_paid"), "pago de esta declaración ya está registrado"],
      [err(409, "filing_paid"), "no se puede eliminar"],
      [err(409, "invoices_changed"), "Vuelve a calcular"],
      [err(404, "not_found"), "ya no existe"],
    ];
    for (const [e, text] of cases) expect(describeTaxFilingError(e)).toContain(text);
  });

  it("falls back to the generic save error for the rest", () => {
    expect(describeTaxFilingError(err(422, "settings_incomplete"))).toContain("Faltan parámetros fiscales");
    expect(describeTaxFilingError(err(500, "internal_error"))).toContain("El servidor tuvo un problema");
    expect(describeTaxFilingError(err(401, "unauthorized"))).toContain("sesión expiró");
    expect(describeTaxFilingError(new TypeError("Failed to fetch"))).toContain("No se pudo conectar");
  });
});

describe("FilingStatusBadge", () => {
  it("shows the status as text", () => {
    wrap(
      <>
        <FilingStatusBadge status="ninguna" />
        <FilingStatusBadge status="pendiente" />
        <FilingStatusBadge status="pagada" />
      </>,
    );
    expect(screen.getByText("Sin declarar")).toBeInTheDocument();
    expect(screen.getByText("Pago pendiente")).toBeInTheDocument();
    expect(screen.getByText("Pagada")).toBeInTheDocument();
  });
});

describe("Breakdown", () => {
  it("shows the declaration as the SAT portal asks for it", () => {
    render(<Breakdown amounts={PREVIEW} />);
    const income = screen.getByRole("region", { name: "Ingresos e ISR" });
    expect(income).toHaveTextContent("Ingresos cobrados (sin IVA)$53,750.00");
    expect(income).toHaveTextContent("Tasa RESICO aplicable1.1%");
    expect(income).toHaveTextContent("ISR a pagar$591.25");
    const iva = screen.getByRole("region", { name: "IVA" });
    expect(iva).toHaveTextContent("IVA a cargo$1,600.00");
    expect(iva).toHaveTextContent("Actos a tasa 0% (exportación, Cliente USA)$43,750.00");
    expect(screen.getByText("$2,191.25")).toBeInTheDocument();
    expect(screen.getByText("17 de noviembre de 2026")).toBeInTheDocument();
  });

  it("shows an IVA balance in favor without the minus sign", () => {
    render(<Breakdown amounts={{ ...PREVIEW, iva_due: "-172.41", total_to_pay: "591.25" }} />);
    expect(screen.getByRole("region", { name: "IVA" })).toHaveTextContent("IVA a favor (saldo a favor)$172.41");
  });

  it("omits the export base for a registered filing", () => {
    render(<Breakdown amounts={FILING} />);
    expect(screen.queryByText(/Actos a tasa 0%/)).not.toBeInTheDocument();
  });
});

describe("InvoiceRefs and TaxWarnings", () => {
  const clients = [{ id: "usa", name: "Acme Inc." }] as never[];

  it("lists the invoices with the client name, or the id when it is unknown", () => {
    render(<InvoiceRefs invoices={PREVIEW.invoices} clients={clients} />);
    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(items[0]).toHaveTextContent("Factura #1");
    expect(items[0]).toHaveTextContent("Acme Inc.");
    expect(items[0]).toHaveTextContent("$43,750.00 sin IVA");
    expect(items[1]).toHaveTextContent("· cobrada el 2026-10-20");
    expect(items[1]).toHaveTextContent("b ·");
  });

  it("explains an empty period", () => {
    render(<InvoiceRefs invoices={[]} clients={[]} />);
    expect(screen.getByText(/queda en ceros/)).toBeInTheDocument();
  });

  it("renders nothing without warnings and one entry per warning", () => {
    const { container } = render(<TaxWarnings warnings={[]} />);
    expect(container).toBeEmptyDOMElement();
    cleanup();
    render(<TaxWarnings warnings={[{ code: "prepared_invoices", message: "m", invoice_ids: [3] }]} />);
    expect(screen.getByRole("status", { name: "Advertencias" })).toHaveTextContent("(#3)");
  });
});

describe("RegisterForm", () => {
  const registered = { filing: { ...FILING }, warnings: [] };

  it("registers a pending filing with only the folio and the date", async () => {
    api.registerTaxFiling.mockResolvedValue(registered);
    const onRegistered = vi.fn();
    wrap(<RegisterForm preview={PREVIEW} onRegistered={onRegistered} />);
    fireEvent.change(screen.getByLabelText("Folio del acuse (opcional)"), { target: { value: "  ACUSE-1 " } });
    fireEvent.change(screen.getByLabelText("Fecha de presentación"), { target: { value: "2026-11-05" } });
    fireEvent.click(screen.getByRole("button", { name: "Registrar declaración" }));

    await waitFor(() => expect(onRegistered).toHaveBeenCalledWith(registered, []));
    expect(api.registerTaxFiling).toHaveBeenCalledWith({ period: "2026-10", filing_date: "2026-11-05", folio: "ACUSE-1" });
  });

  it("sends the creditable IVA used in the calculation", async () => {
    api.registerTaxFiling.mockResolvedValue(registered);
    wrap(<RegisterForm preview={{ ...PREVIEW, iva_acreditable: "500.00" }} onRegistered={vi.fn()} />);
    expect(screen.getByText(/IVA acreditable de \$500.00/)).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Fecha de presentación"), { target: { value: "2026-11-05" } });
    fireEvent.click(screen.getByRole("button", { name: "Registrar declaración" }));
    await waitFor(() => expect(api.registerTaxFiling).toHaveBeenCalled());
    expect(api.registerTaxFiling.mock.calls[0][0]).toMatchObject({ iva_acreditable: "500.00" });
  });

  it("does not create an expense unless asked and defaults the paid amounts to the amounts due", async () => {
    api.registerTaxFiling.mockResolvedValue(registered);
    wrap(<RegisterForm preview={PREVIEW} onRegistered={vi.fn()} />);
    expect(screen.queryByLabelText("ISR pagado (MXN)")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("checkbox", { name: "Ya pagué esta declaración al SAT" }));
    expect(screen.getByLabelText("ISR pagado (MXN)")).toHaveValue("591.25");
    expect(screen.getByLabelText("IVA pagado (MXN)")).toHaveValue("1600.00");
    expect(screen.getByRole("checkbox", { name: /Registrar el pago como gasto/ })).not.toBeChecked();
    fireEvent.change(screen.getByLabelText("Fecha de presentación"), { target: { value: "2026-11-05" } });
    fireEvent.change(screen.getByLabelText("Fecha de pago"), { target: { value: "2026-11-12" } });
    fireEvent.click(screen.getByRole("button", { name: "Registrar declaración" }));

    await waitFor(() => expect(api.registerTaxFiling).toHaveBeenCalled());
    expect(api.registerTaxFiling.mock.calls[0][0].payment).toEqual({ date: "2026-11-12", isr_paid: "591.25", iva_paid: "1600.00" });
  });

  it("shows the field errors and does not call the API", () => {
    wrap(<RegisterForm preview={PREVIEW} onRegistered={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Folio del acuse (opcional)"), { target: { value: "x".repeat(65) } });
    fireEvent.click(screen.getByRole("checkbox", { name: "Ya pagué esta declaración al SAT" }));
    fireEvent.change(screen.getByLabelText("ISR pagado (MXN)"), { target: { value: "1.234" } });
    fireEvent.click(screen.getByRole("button", { name: "Registrar declaración" }));
    expect(screen.getByText("El folio puede tener hasta 64 caracteres.")).toBeInTheDocument();
    expect(screen.getByText(/monto de cero o más/)).toBeInTheDocument();
    expect(api.registerTaxFiling).not.toHaveBeenCalled();
  });

  it("shows the API error", async () => {
    api.registerTaxFiling.mockRejectedValue(new ApiError(409, "already_filed"));
    wrap(<RegisterForm preview={PREVIEW} onRegistered={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Fecha de presentación"), { target: { value: "2026-11-05" } });
    fireEvent.click(screen.getByRole("button", { name: "Registrar declaración" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Este periodo ya está declarado");
  });
});

describe("PaymentDialog", () => {
  it("pays without an expense by default and reports the result", async () => {
    const result = { filing: { ...FILING, status: "pagada" as const }, warnings: [] };
    api.payTaxFiling.mockResolvedValue(result);
    const onPaid = vi.fn();
    wrap(<PaymentDialog filing={FILING} onClose={vi.fn()} onPaid={onPaid} />);
    const dialog = screen.getByRole("dialog", { name: "Registrar pago de octubre de 2026" });
    expect(within(dialog).getByLabelText("ISR pagado (MXN)")).toHaveValue("591.25");
    expect(within(dialog).getByLabelText("IVA pagado (MXN)")).toHaveValue("1600.00");
    fireEvent.change(within(dialog).getByLabelText("Fecha de pago"), { target: { value: "2026-11-12" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Registrar pago" }));

    await waitFor(() => expect(onPaid).toHaveBeenCalledWith(result, []));
    expect(api.payTaxFiling).toHaveBeenCalledWith("2026-10", { date: "2026-11-12", isr_paid: "591.25", iva_paid: "1600.00" });
  });

  it("asks for the expense only when the box is checked", async () => {
    api.payTaxFiling.mockResolvedValue({ filing: FILING, warnings: [] });
    wrap(<PaymentDialog filing={FILING} onClose={vi.fn()} onPaid={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Fecha de pago"), { target: { value: "2026-11-12" } });
    fireEvent.click(screen.getByRole("checkbox", { name: /Registrar el pago como gasto/ }));
    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));
    await waitFor(() => expect(api.payTaxFiling).toHaveBeenCalled());
    expect(api.payTaxFiling.mock.calls[0][1]).toMatchObject({ record_expense: true });
  });

  it("uses 0 as the IVA default when the balance is in favor", () => {
    wrap(<PaymentDialog filing={{ ...FILING, iva_due: "-172.41" }} onClose={vi.fn()} onPaid={vi.fn()} />);
    expect(screen.getByLabelText("IVA pagado (MXN)")).toHaveValue("0.00");
  });

  it("validates before calling the API and closes with Escape and the buttons", () => {
    const onClose = vi.fn();
    wrap(<PaymentDialog filing={FILING} onClose={onClose} onPaid={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("IVA pagado (MXN)"), { target: { value: "-1" } });
    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));
    expect(screen.getByText(/monto de cero o más/)).toBeInTheDocument();
    expect(api.payTaxFiling).not.toHaveBeenCalled();

    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Cancelar" }));
    fireEvent.click(screen.getByRole("button", { name: "Cerrar" }));
    expect(onClose).toHaveBeenCalledTimes(3);
  });

  it("shows the API error and stays open", async () => {
    api.payTaxFiling.mockRejectedValue(new ApiError(409, "already_paid"));
    const onPaid = vi.fn();
    wrap(<PaymentDialog filing={FILING} onClose={vi.fn()} onPaid={onPaid} />);
    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("pago de esta declaración ya está registrado");
    expect(onPaid).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});

describe("UnfiledInvoices", () => {
  it("alerts about issued invoices left out of an already filed period", async () => {
    api.listUnfiledInvoices.mockResolvedValue([
      { id: 9, client_id: "b", collection_date: "2026-10-31", currency: "MXN", subtotal_mxn: "30172.41", state: "emitida", uuid: null, period: "2026-10" },
    ]);
    wrap(<UnfiledInvoices />);
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Facturas emitidas en un periodo ya declarado");
    expect(alert).toHaveTextContent("Factura #9");
    expect(alert).toHaveTextContent("$30,172.41 MXN");
    expect(alert).toHaveTextContent("octubre de 2026");
    expect(within(alert).getByRole("link", { name: "Factura #9" })).toHaveAttribute("href", "/facturas");
  });

  it("renders nothing when there are none and does not break the page on an error", async () => {
    api.listUnfiledInvoices.mockResolvedValueOnce([]);
    const { container } = wrap(<UnfiledInvoices />);
    await waitFor(() => expect(api.listUnfiledInvoices).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
    cleanup();

    api.listUnfiledInvoices.mockRejectedValueOnce(new ApiError(500, "internal_error"));
    const failed = wrap(<UnfiledInvoices />);
    await waitFor(() => expect(api.listUnfiledInvoices).toHaveBeenCalledTimes(2));
    expect(failed.container).toBeEmptyDOMElement();
  });
});

describe("PendingPeriods", () => {
  it("lists the periods with the overdue flag and a link to declare each one", async () => {
    const pending: PendingPeriod[] = [
      { period: "2026-08", due_date: "2026-09-17", overdue: true },
      { period: "2026-10", due_date: "2026-11-17", overdue: false },
    ];
    api.listPendingPeriods.mockResolvedValue(pending);
    wrap(<PendingPeriods />);
    const items = await screen.findAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(items[0]).toHaveTextContent("agosto de 2026");
    expect(items[0]).toHaveTextContent("Vencido");
    expect(items[0]).toHaveTextContent("17 de septiembre de 2026");
    expect(within(items[0]).getByRole("link", { name: "Declarar agosto de 2026" })).toHaveAttribute("href", "/declaracion?period=2026-08");
    expect(items[1]).not.toHaveTextContent("Vencido");
  });

  it("shows the empty state and the error with a retry", async () => {
    api.listPendingPeriods.mockResolvedValueOnce([]);
    wrap(<PendingPeriods />);
    expect(await screen.findByText(/No tienes periodos pendientes de declarar/)).toBeInTheDocument();
    cleanup();

    api.listPendingPeriods.mockRejectedValueOnce(new ApiError(500, "internal_error")).mockResolvedValueOnce([]);
    wrap(<PendingPeriods />);
    expect(await screen.findByRole("alert")).toHaveTextContent("El servidor tuvo un problema");
    fireEvent.click(screen.getByRole("button", { name: "Reintentar" }));
    expect(await screen.findByText(/No tienes periodos pendientes de declarar/)).toBeInTheDocument();
  });
});

describe("FilingHistory", () => {
  it("shows the status badges, the amounts and the actions per filing", async () => {
    api.listTaxFilings.mockResolvedValue([FILING, PAID_FILING]);
    const onSelect = vi.fn();
    const onPay = vi.fn();
    wrap(<FilingHistory filter={{}} selectedPeriod={null} onSelect={onSelect} onPay={onPay} />);
    const pending = await screen.findByRole("row", { name: /octubre de 2026/ });
    expect(pending).toHaveTextContent("Pago pendiente");
    expect(pending).toHaveTextContent("ACUSE-1");
    expect(pending).toHaveTextContent("$591.25");
    expect(pending).toHaveTextContent("$1,600.00");
    const paid = screen.getByRole("row", { name: /septiembre de 2026/ });
    expect(paid).toHaveTextContent("Pagada");
    expect(paid).toHaveTextContent("$600.00");
    expect(within(paid).queryByRole("button", { name: /Registrar pago/ })).not.toBeInTheDocument();

    fireEvent.click(within(pending).getByRole("button", { name: "Registrar pago de octubre de 2026" }));
    expect(onPay).toHaveBeenCalledWith(FILING);
    fireEvent.click(within(paid).getByRole("button", { name: "Ver declaración de septiembre de 2026" }));
    expect(onSelect).toHaveBeenCalledWith("2026-09");
  });

  it("shows an IVA balance in favor", async () => {
    api.listTaxFilings.mockResolvedValue([{ ...FILING, iva_due: "-172.41" }]);
    wrap(<FilingHistory filter={{}} selectedPeriod={null} onSelect={vi.fn()} onPay={vi.fn()} />);
    expect(await screen.findByRole("row", { name: /octubre de 2026/ })).toHaveTextContent("A favor $172.41");
  });

  it("passes the filters to the API and explains the empty results", async () => {
    api.listTaxFilings.mockResolvedValue([]);
    wrap(<FilingHistory filter={{ year: "2026", status: "pendiente" }} selectedPeriod={null} onSelect={vi.fn()} onPay={vi.fn()} />);
    expect(await screen.findByText("No hay declaraciones con estos filtros.")).toBeInTheDocument();
    expect(api.listTaxFilings).toHaveBeenCalledWith({ year: "2026", status: "pendiente" });
    cleanup();

    wrap(<FilingHistory filter={{}} selectedPeriod={null} onSelect={vi.fn()} onPay={vi.fn()} />);
    expect(await screen.findByText(/Aún no hay declaraciones registradas/)).toBeInTheDocument();
  });

  it("shows the error with a retry", async () => {
    api.listTaxFilings.mockRejectedValueOnce(new ApiError(500, "internal_error")).mockResolvedValueOnce([FILING]);
    wrap(<FilingHistory filter={{}} selectedPeriod={null} onSelect={vi.fn()} onPay={vi.fn()} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("El servidor tuvo un problema");
    fireEvent.click(screen.getByRole("button", { name: "Reintentar" }));
    expect(await screen.findByRole("row", { name: /octubre de 2026/ })).toBeInTheDocument();
  });
});
