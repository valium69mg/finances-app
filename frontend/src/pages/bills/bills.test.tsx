import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { AllSettings } from "../../api/settings";
import { BillForm } from "./BillForm";
import { BillList } from "./BillList";
import { DueBadge } from "./DueBadge";
import { HistoryPanel } from "./HistoryPanel";
import { PayDialog } from "./PayDialog";
import { describeBillsError } from "./errors";
import { DETAIL, FUTURE_BILL, INACTIVE_BILL, OVERDUE_BILL, VARIABLE_BILL } from "./fixtures";
import { defaultPayDraft, draftOf, emptyDraft, expenseCategories, isBillAmount, toBillInput, toPayInput, validateBill, validatePay } from "./form";
import { dueState, leadLabel } from "./labels";

const api = vi.hoisted(() => ({
  listBills: vi.fn(),
  getBill: vi.fn(),
  createBill: vi.fn(),
  updateBill: vi.fn(),
  deactivateBill: vi.fn(),
  payBill: vi.fn(),
  skipBill: vi.fn(),
}));
vi.mock("../../api/bills", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/bills")>()), ...api }));

// Vitest runs without globals, so Testing Library does not unmount between tests on its own.
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function wrap(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

const CATEGORIES = ["Servicios", "Suscripciones", "Hogar"];

describe("expenseCategories", () => {
  it("keeps only the Gasto categories", () => {
    const settings = {
      categories: [
        { name: "Servicios", kind: "Gasto" },
        { name: "Sueldo", kind: "Ingreso" },
        { name: "Inversiones", kind: "Ahorro" },
        { name: "Hogar", kind: "Gasto" },
      ],
    } as unknown as AllSettings;
    expect(expenseCategories(settings)).toEqual(["Servicios", "Hogar"]);
    expect(expenseCategories(undefined)).toEqual([]);
  });
});

describe("bill form helpers", () => {
  const ok = { ...emptyDraft("2026-10-01", "Servicios"), name: "Megacable", amount: "550" };

  it("starts monthly, fixed, three days before and due today", () => {
    expect(emptyDraft("2026-10-01", "Servicios")).toEqual({
      name: "",
      category: "Servicios",
      variable: false,
      amount: "",
      currency: "MXN",
      recurrence: "monthly",
      nextDueDate: "2026-10-01",
      leadDays: "3",
      notes: "",
    });
  });

  it("accepts an amount of at least 0.01 with at most 2 decimals", () => {
    for (const good of ["0.01", "550", "550.5", "999999999999.99"]) expect(isBillAmount(good), good).toBe(true);
    for (const bad of ["", "0", "0.00", "-1", "1,600", "1.234", "abc", "1000000000000"]) expect(isBillAmount(bad), bad).toBe(false);
  });

  it("validates a fixed bill", () => {
    expect(validateBill(ok)).toEqual({});
    expect(validateBill({ ...ok, name: "  " }).name).toContain("nombre");
    expect(validateBill({ ...ok, name: "x".repeat(121) }).name).toContain("120");
    expect(validateBill({ ...ok, category: "" }).category).toContain("categoría");
    expect(validateBill({ ...ok, amount: "0" }).amount).toContain("mayor a cero");
    expect(validateBill({ ...ok, nextDueDate: "" }).date).toContain("próximo vencimiento");
    expect(validateBill({ ...ok, leadDays: "-1" }).lead).toContain("entre 0 y 365");
    expect(validateBill({ ...ok, leadDays: "366" }).lead).toBeDefined();
    expect(validateBill({ ...ok, leadDays: "0" })).toEqual({});
  });

  it("does not need an amount for a variable bill", () => {
    expect(validateBill({ ...ok, variable: true, amount: "" })).toEqual({});
  });

  it("builds the request, with a null amount for a variable bill", () => {
    expect(toBillInput({ ...ok, name: " Megacable ", amount: " 550 ", notes: " n " }, true)).toEqual({
      name: "Megacable",
      category: "Servicios",
      amount: "550",
      currency: "MXN",
      recurrence: "monthly",
      next_due_date: "2026-10-01",
      reminder_lead_days: 3,
      active: true,
      notes: "n",
    });
    expect(toBillInput({ ...ok, variable: true, amount: "999" }, false)).toMatchObject({ amount: null, active: false });
  });

  it("round-trips an existing bill through the draft", () => {
    expect(draftOf(VARIABLE_BILL)).toMatchObject({ variable: true, amount: "", recurrence: "bimonthly", nextDueDate: "2026-10-12", leadDays: "3", notes: "Recibo CFE" });
    expect(toBillInput(draftOf(OVERDUE_BILL), true)).toMatchObject({ amount: "550.00", next_due_date: "2026-10-01", reminder_lead_days: 3 });
  });
});

describe("payment helpers", () => {
  it("proposes the bill's amount, today and its category", () => {
    expect(defaultPayDraft(OVERDUE_BILL, "2026-10-10")).toEqual({ amount: "550.00", date: "2026-10-10", category: "Servicios", description: "" });
    expect(defaultPayDraft(VARIABLE_BILL, "2026-10-10").amount).toBe("");
  });

  it("requires an amount, a date and a category", () => {
    const ok = { amount: "10", date: "2026-10-10", category: "Servicios", description: "" };
    expect(validatePay(ok)).toEqual({});
    expect(validatePay({ ...ok, amount: "" }).amount).toContain("mayor a cero");
    expect(validatePay({ ...ok, date: "" }).date).toContain("fecha del pago");
    expect(validatePay({ ...ok, category: "" }).category).toContain("categoría");
  });

  it("only sends the description when written", () => {
    expect(toPayInput({ amount: " 10 ", date: "2026-10-10", category: "Hogar", description: " " })).toEqual({ amount: "10", date: "2026-10-10", category: "Hogar" });
    expect(toPayInput({ amount: "10", date: "2026-10-10", category: "Hogar", description: " recibo " })).toMatchObject({ description: "recibo" });
  });
});

describe("labels", () => {
  it("derives the attention state from the API flags, overdue first", () => {
    expect(dueState(OVERDUE_BILL)).toEqual({ kind: "overdue", label: "Vencido hace 9 días" });
    expect(dueState({ ...OVERDUE_BILL, days_until_due: -1 })).toEqual({ kind: "overdue", label: "Vencido hace 1 día" });
    expect(dueState(VARIABLE_BILL)).toEqual({ kind: "soon", label: "Vence en 2 días" });
    expect(dueState({ ...VARIABLE_BILL, days_until_due: 1 })).toEqual({ kind: "soon", label: "Vence mañana" });
    expect(dueState({ ...VARIABLE_BILL, days_until_due: 0 })).toEqual({ kind: "today", label: "Vence hoy" });
    expect(dueState(FUTURE_BILL)).toEqual({ kind: "none", label: "" });
    expect(dueState({ ...OVERDUE_BILL, overdue: true, due_soon: true })).toMatchObject({ kind: "overdue" });
  });

  it("raises no badge for an inactive bill or one without an occurrence", () => {
    expect(dueState({ ...OVERDUE_BILL, active: false }).kind).toBe("none");
    expect(dueState({ ...OVERDUE_BILL, days_until_due: null }).kind).toBe("none");
  });

  it("describes the reminder lead time", () => {
    expect(leadLabel(0)).toBe("Avisar el mismo día");
    expect(leadLabel(1)).toBe("Avisar 1 día antes");
    expect(leadLabel(7)).toBe("Avisar 7 días antes");
  });
});

describe("describeBillsError", () => {
  const err = (status: number, code: string, message?: string) => new ApiError(status, code, message);

  it("maps every known code to Spanish", () => {
    const cases: [ApiError, string][] = [
      [err(400, "invalid_bill", "name is required"), "Los datos no son válidos: name is required"],
      [err(400, "invalid_expense", "amount must be greater than zero"), "No se pudo registrar el gasto: amount must be greater than zero"],
      [err(409, "occurrence_resolved"), "ya se pagó u omitió"],
      [err(409, "bill_inactive"), "está desactivado"],
      [err(404, "not_found"), "ya no existe"],
    ];
    for (const [e, text] of cases) expect(describeBillsError(e)).toContain(text);
  });

  it("falls back to the generic save error for the rest", () => {
    expect(describeBillsError(err(500, "internal_error"))).toContain("El servidor tuvo un problema");
    expect(describeBillsError(err(401, "unauthorized"))).toContain("sesión expiró");
    expect(describeBillsError(new TypeError("Failed to fetch"))).toContain("No se pudo conectar");
  });
});

describe("DueBadge", () => {
  it("shows the state as text and nothing when no attention is needed", () => {
    const { container } = wrap(<DueBadge bill={FUTURE_BILL} />);
    expect(container).toBeEmptyDOMElement();
    cleanup();
    wrap(
      <>
        <DueBadge bill={OVERDUE_BILL} />
        <DueBadge bill={VARIABLE_BILL} />
      </>,
    );
    expect(screen.getByText("Vencido hace 9 días")).toBeInTheDocument();
    expect(screen.getByText("Vence en 2 días")).toBeInTheDocument();
  });
});

describe("BillForm", () => {
  it("creates a fixed monthly bill", async () => {
    api.createBill.mockResolvedValue(OVERDUE_BILL);
    const onSaved = vi.fn();
    wrap(<BillForm categories={CATEGORIES} editing={null} onSaved={onSaved} onCancelEdit={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Nombre"), { target: { value: " Megacable " } });
    fireEvent.change(screen.getByLabelText("Monto"), { target: { value: "550" } });
    fireEvent.change(screen.getByLabelText("Próximo vencimiento"), { target: { value: "2026-11-01" } });
    fireEvent.click(screen.getByRole("button", { name: "Agregar pago recurrente" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(OVERDUE_BILL, false));
    expect(api.createBill).toHaveBeenCalledWith({
      name: "Megacable",
      category: "Servicios",
      amount: "550",
      currency: "MXN",
      recurrence: "monthly",
      next_due_date: "2026-11-01",
      reminder_lead_days: 3,
      active: true,
      notes: "",
    });
  });

  it("creates a variable bill with a null amount and a custom reminder", async () => {
    api.createBill.mockResolvedValue(VARIABLE_BILL);
    wrap(<BillForm categories={CATEGORIES} editing={null} onSaved={vi.fn()} onCancelEdit={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Nombre"), { target: { value: "Luz" } });
    fireEvent.click(screen.getByRole("checkbox", { name: "Monto variable (sin monto fijo)" }));
    expect(screen.getByLabelText("Monto")).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Recurrencia"), { target: { value: "bimonthly" } });
    fireEvent.change(screen.getByLabelText("Avisar con (días de anticipación)"), { target: { value: "7" } });
    fireEvent.click(screen.getByRole("button", { name: "Agregar pago recurrente" }));

    await waitFor(() => expect(api.createBill).toHaveBeenCalled());
    expect(api.createBill.mock.calls[0][0]).toMatchObject({ name: "Luz", amount: null, recurrence: "bimonthly", reminder_lead_days: 7 });
  });

  it("shows the field errors and does not call the API", () => {
    wrap(<BillForm categories={CATEGORIES} editing={null} onSaved={vi.fn()} onCancelEdit={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Avisar con (días de anticipación)"), { target: { value: "400" } });
    fireEvent.click(screen.getByRole("button", { name: "Agregar pago recurrente" }));
    expect(screen.getByText(/nombre del pago/)).toBeInTheDocument();
    expect(screen.getByText(/mayor a cero/)).toBeInTheDocument();
    expect(screen.getByText(/entre 0 y 365/)).toBeInTheDocument();
    expect(api.createBill).not.toHaveBeenCalled();
  });

  it("edits a bill, keeping it inactive if it was", async () => {
    api.updateBill.mockResolvedValue(INACTIVE_BILL);
    const onSaved = vi.fn();
    wrap(<BillForm categories={CATEGORIES} editing={INACTIVE_BILL} onSaved={onSaved} onCancelEdit={vi.fn()} />);
    expect(screen.getByLabelText("Nombre")).toHaveValue("Disney+");
    fireEvent.change(screen.getByLabelText("Monto"), { target: { value: "150" } });
    fireEvent.click(screen.getByRole("button", { name: "Guardar cambios" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(INACTIVE_BILL, true));
    expect(api.updateBill).toHaveBeenCalledWith(4, expect.objectContaining({ amount: "150", active: false, next_due_date: "2026-11-20" }));
  });

  it("shows the API error", async () => {
    api.createBill.mockRejectedValue(new ApiError(400, "invalid_bill", "name is required"));
    wrap(<BillForm categories={CATEGORIES} editing={null} onSaved={vi.fn()} onCancelEdit={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Nombre"), { target: { value: "x" } });
    fireEvent.change(screen.getByLabelText("Monto"), { target: { value: "1" } });
    fireEvent.click(screen.getByRole("button", { name: "Agregar pago recurrente" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Los datos no son válidos: name is required");
  });
});

describe("PayDialog", () => {
  const result = { bill: { ...OVERDUE_BILL, next_due_date: "2026-11-01" }, paid_occurrence: DETAIL.history[0], expense: { id: 5, date: "2026-10-10", description: "Megacable", category: "Servicios", currency: "MXN", amount: "550.00", amount_mxn: "550.00" } };

  it("prefills the bill's amount and category and pays with them", async () => {
    api.payBill.mockResolvedValue(result);
    const onPaid = vi.fn();
    wrap(<PayDialog bill={OVERDUE_BILL} categories={CATEGORIES} onClose={vi.fn()} onPaid={onPaid} />);
    const dialog = screen.getByRole("dialog", { name: "Pagar Megacable" });
    expect(within(dialog).getByLabelText("Monto (MXN)")).toHaveValue("550.00");
    expect(within(dialog).getByLabelText("Categoría")).toHaveValue("Servicios");
    fireEvent.change(within(dialog).getByLabelText("Fecha de pago"), { target: { value: "2026-10-10" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Registrar pago" }));

    await waitFor(() => expect(onPaid).toHaveBeenCalledWith(result));
    expect(api.payBill).toHaveBeenCalledWith(1, { date: "2026-10-10", amount: "550.00", category: "Servicios" });
  });

  it("lets the amount, category and description be overridden", async () => {
    api.payBill.mockResolvedValue(result);
    wrap(<PayDialog bill={OVERDUE_BILL} categories={CATEGORIES} onClose={vi.fn()} onPaid={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Monto (MXN)"), { target: { value: "499.99" } });
    fireEvent.change(screen.getByLabelText("Categoría"), { target: { value: "Hogar" } });
    fireEvent.change(screen.getByLabelText("Descripción (opcional)"), { target: { value: "descuento" } });
    fireEvent.change(screen.getByLabelText("Fecha de pago"), { target: { value: "2026-10-03" } });
    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));
    await waitFor(() => expect(api.payBill).toHaveBeenCalled());
    expect(api.payBill).toHaveBeenCalledWith(1, { date: "2026-10-03", amount: "499.99", category: "Hogar", description: "descuento" });
  });

  it("requires the amount of a variable bill", async () => {
    api.payBill.mockResolvedValue(result);
    wrap(<PayDialog bill={VARIABLE_BILL} categories={CATEGORIES} onClose={vi.fn()} onPaid={vi.fn()} />);
    expect(screen.getByLabelText("Monto (MXN)")).toHaveValue("");
    expect(screen.getByText(/no tiene monto fijo/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));
    expect(screen.getByText(/mayor a cero/)).toBeInTheDocument();
    expect(api.payBill).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText("Monto (MXN)"), { target: { value: "312.10" } });
    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));
    await waitFor(() => expect(api.payBill).toHaveBeenCalled());
    expect(api.payBill.mock.calls[0][1]).toMatchObject({ amount: "312.10" });
  });

  it("keeps a category that is not in the list selectable", () => {
    wrap(<PayDialog bill={{ ...OVERDUE_BILL, category: "Vieja" }} categories={CATEGORIES} onClose={vi.fn()} onPaid={vi.fn()} />);
    expect(screen.getByLabelText("Categoría")).toHaveValue("Vieja");
  });

  it("closes with Escape and the buttons", () => {
    const onClose = vi.fn();
    wrap(<PayDialog bill={OVERDUE_BILL} categories={CATEGORIES} onClose={onClose} onPaid={vi.fn()} />);
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Cancelar" }));
    fireEvent.click(screen.getByRole("button", { name: "Cerrar" }));
    expect(onClose).toHaveBeenCalledTimes(3);
  });

  it("shows the API error and stays open", async () => {
    api.payBill.mockRejectedValue(new ApiError(409, "occurrence_resolved"));
    const onPaid = vi.fn();
    wrap(<PayDialog bill={OVERDUE_BILL} categories={CATEGORIES} onClose={vi.fn()} onPaid={onPaid} />);
    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("ya se pagó u omitió");
    expect(onPaid).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});

describe("BillList", () => {
  const props = { includeInactive: false, editingId: null, onPay: vi.fn(), onEdit: vi.fn(), onNotice: vi.fn(), onChanged: vi.fn() };

  it("lists the bills with amount or variable, recurrence, next due date and badges", async () => {
    api.listBills.mockResolvedValue([OVERDUE_BILL, VARIABLE_BILL, FUTURE_BILL]);
    wrap(<BillList {...props} />);
    const items = await screen.findAllByRole("listitem");
    expect(items).toHaveLength(3);
    expect(items[0]).toHaveTextContent("Megacable");
    expect(items[0]).toHaveTextContent("$550.00 MXN");
    expect(items[0]).toHaveTextContent("Mensual");
    expect(items[0]).toHaveTextContent("1 de octubre de 2026");
    expect(items[0]).toHaveTextContent("Vencido hace 9 días");
    expect(items[1]).toHaveTextContent("Monto variable");
    expect(items[1]).toHaveTextContent("Bimestral");
    expect(items[1]).toHaveTextContent("Vence en 2 días");
    expect(items[1]).toHaveTextContent("Recibo CFE");
    expect(items[2]).not.toHaveTextContent("Vence");
    expect(api.listBills).toHaveBeenCalledWith(false);
  });

  it("asks for the inactive bills when requested and offers to reactivate them", async () => {
    api.listBills.mockResolvedValue([FUTURE_BILL, INACTIVE_BILL]);
    api.updateBill.mockResolvedValue({ ...INACTIVE_BILL, active: true });
    const onNotice = vi.fn();
    wrap(<BillList {...props} includeInactive onNotice={onNotice} />);
    const row = (await screen.findByText("Disney+")).closest("li") as HTMLElement;
    expect(row).toHaveTextContent("Desactivado");
    expect(within(row).queryByRole("button", { name: "Pagar Disney+" })).not.toBeInTheDocument();
    expect(api.listBills).toHaveBeenCalledWith(true);
    fireEvent.click(within(row).getByRole("button", { name: "Reactivar Disney+" }));
    await waitFor(() => expect(onNotice).toHaveBeenCalledWith("Disney+ se reactivó."));
    expect(api.updateBill).toHaveBeenCalledWith(4, expect.objectContaining({ active: true }));
  });

  it("explains the empty state", async () => {
    api.listBills.mockResolvedValue([]);
    wrap(<BillList {...props} />);
    expect(await screen.findByText(/Aún no tienes pagos recurrentes/)).toBeInTheDocument();
  });

  it("shows the error with a retry", async () => {
    api.listBills.mockRejectedValueOnce(new ApiError(500, "internal_error")).mockResolvedValue([FUTURE_BILL]);
    wrap(<BillList {...props} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("El servidor tuvo un problema");
    fireEvent.click(screen.getByRole("button", { name: "Reintentar" }));
    expect(await screen.findByText("Netflix")).toBeInTheDocument();
  });

  it("opens the payment and the edit of a bill", async () => {
    api.listBills.mockResolvedValue([OVERDUE_BILL]);
    const onPay = vi.fn();
    const onEdit = vi.fn();
    wrap(<BillList {...props} onPay={onPay} onEdit={onEdit} />);
    fireEvent.click(await screen.findByRole("button", { name: "Pagar Megacable" }));
    fireEvent.click(screen.getByRole("button", { name: "Editar Megacable" }));
    expect(onPay).toHaveBeenCalledWith(OVERDUE_BILL);
    expect(onEdit).toHaveBeenCalledWith(OVERDUE_BILL);
  });

  it("skips only after confirming, and says no expense was registered", async () => {
    api.listBills.mockResolvedValue([OVERDUE_BILL]);
    api.skipBill.mockResolvedValue({ ...OVERDUE_BILL, next_due_date: "2026-11-01" });
    const onNotice = vi.fn();
    const onChanged = vi.fn();
    wrap(<BillList {...props} onNotice={onNotice} onChanged={onChanged} />);
    fireEvent.click(await screen.findByRole("button", { name: "Omitir Megacable" }));
    const group = screen.getByRole("group", { name: "Confirmar omitir Megacable" });
    expect(group).toHaveTextContent("No se registra ningún gasto");
    expect(api.skipBill).not.toHaveBeenCalled();

    fireEvent.click(within(group).getByRole("button", { name: "Cancelar" }));
    expect(screen.queryByRole("group")).not.toBeInTheDocument();
    expect(api.skipBill).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Omitir Megacable" }));
    fireEvent.click(screen.getByRole("button", { name: "Sí, omitir" }));
    await waitFor(() => expect(api.skipBill).toHaveBeenCalledWith(1));
    await waitFor(() => expect(onNotice).toHaveBeenCalled());
    expect(onNotice.mock.calls[0][0]).toContain("No se registró ningún gasto");
    expect(onNotice.mock.calls[0][0]).toContain("1 de noviembre de 2026");
    expect(onChanged).toHaveBeenCalledWith(1);
  });

  it("deactivates only after confirming", async () => {
    api.listBills.mockResolvedValue([FUTURE_BILL]);
    api.deactivateBill.mockResolvedValue(undefined);
    const onNotice = vi.fn();
    wrap(<BillList {...props} onNotice={onNotice} />);
    fireEvent.click(await screen.findByRole("button", { name: "Desactivar Netflix" }));
    expect(api.deactivateBill).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Sí, desactivar" }));
    await waitFor(() => expect(api.deactivateBill).toHaveBeenCalledWith(3));
    await waitFor(() => expect(onNotice).toHaveBeenCalledWith(expect.stringContaining("historial y sus gastos se conservan")));
  });

  it("shows the API error of a failed skip", async () => {
    api.listBills.mockResolvedValue([OVERDUE_BILL]);
    api.skipBill.mockRejectedValue(new ApiError(409, "occurrence_resolved"));
    wrap(<BillList {...props} />);
    fireEvent.click(await screen.findByRole("button", { name: "Omitir Megacable" }));
    fireEvent.click(screen.getByRole("button", { name: "Sí, omitir" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("ya se pagó u omitió");
  });

  it("expands the history of a bill", async () => {
    api.listBills.mockResolvedValue([OVERDUE_BILL]);
    api.getBill.mockResolvedValue(DETAIL);
    wrap(<BillList {...props} />);
    const toggle = await screen.findByRole("button", { name: "Ver historial de Megacable" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(toggle);
    const history = await screen.findByRole("list", { name: "Historial de Megacable" });
    expect(history).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Ocultar historial de Megacable" })).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(screen.getByRole("button", { name: "Ocultar historial de Megacable" }));
    expect(screen.queryByRole("list", { name: "Historial de Megacable" })).not.toBeInTheDocument();
  });
});

describe("HistoryPanel", () => {
  it("shows paid and skipped occurrences, and a deleted expense", async () => {
    api.getBill.mockResolvedValue(DETAIL);
    wrap(<HistoryPanel billId={1} name="Megacable" />);
    const items = await screen.findAllByRole("listitem");
    expect(items).toHaveLength(3);
    expect(items[0]).toHaveTextContent("2026-09-01");
    expect(items[0]).toHaveTextContent("Pagado el 2026-09-03 · $499.50 MXN · gasto #42");
    expect(items[1]).toHaveTextContent("Omitido: no se registró ningún gasto");
    expect(items[2]).toHaveTextContent("el gasto ya no existe");
  });

  it("explains an empty history", async () => {
    api.getBill.mockResolvedValue({ ...DETAIL, history: [] });
    wrap(<HistoryPanel billId={1} name="Megacable" />);
    expect(await screen.findByText(/Aún no hay pagos ni omisiones/)).toBeInTheDocument();
  });

  it("shows the error with a retry", async () => {
    api.getBill.mockRejectedValueOnce(new ApiError(404, "not_found")).mockResolvedValue({ ...DETAIL, history: [] });
    wrap(<HistoryPanel billId={1} name="Megacable" />);
    expect(await screen.findByRole("alert")).toHaveTextContent("ya no existe");
    fireEvent.click(screen.getByRole("button", { name: "Reintentar" }));
    expect(await screen.findByText(/Aún no hay pagos/)).toBeInTheDocument();
  });
});
