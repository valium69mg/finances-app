import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { AllSettings } from "../../api/settings";
import { FutureExpenses } from "../FutureExpenses";
import { AmountDialog } from "./AmountDialog";
import { describeFutureError } from "./errors";
import { LAPTOP, LIST, PAID } from "./fixtures";
import { defaultPayDraft, draftOf, emptyDraft, isEmptyBalance, isGreater, toItemInput, toPayInput, validateAmount, validateItem } from "./form";
import { ItemForm } from "./ItemForm";
import { PayDialog } from "./PayDialog";

const api = vi.hoisted(() => ({
  listFutureExpenses: vi.fn(),
  createFutureExpense: vi.fn(),
  updateFutureExpense: vi.fn(),
  deleteFutureExpense: vi.fn(),
  addFutureSaving: vi.fn(),
  assignFutureSaving: vi.fn(),
  payFutureExpense: vi.fn(),
}));
const settingsApi = vi.hoisted(() => ({ getSettings: vi.fn() }));
vi.mock("../../api/settings", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/settings")>()), ...settingsApi }));
vi.mock("../../api/futureExpenses", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/futureExpenses")>()), ...api }));

// Vitest runs without globals, so Testing Library does not unmount between tests on its own.
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function wrap(ui: ReactNode) {
  settingsApi.getSettings.mockResolvedValue(SETTINGS);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

const SETTINGS = {
  categories: [
    { name: "Ocio", kind: "Gasto" },
    { name: "Otros", kind: "Gasto" },
    { name: "Sueldo", kind: "Ingreso" },
  ],
} as unknown as AllSettings;

describe("form helpers", () => {
  const ok = { name: "Laptop", target: "8000", dueDate: "2027-01-20" };

  it("validates an item", () => {
    expect(validateItem(ok)).toEqual({});
    expect(validateItem({ ...ok, name: "  " }).name).toContain("nombre");
    expect(validateItem({ ...ok, name: "x".repeat(121) }).name).toContain("120");
    for (const bad of ["", "0", "-5", "1,600", "1.234", "abc"]) expect(validateItem({ ...ok, target: bad }).target, bad).toContain("mayor a cero");
    expect(validateItem({ ...ok, dueDate: "" }).date).toContain("vencimiento");
  });

  it("builds the request with trimmed text and the amount as typed", () => {
    expect(toItemInput({ name: "  Laptop ", target: " 8000.50 ", dueDate: "2027-01-20" })).toEqual({ name: "Laptop", target_amount: "8000.50", due_date: "2027-01-20" });
    expect(draftOf(LAPTOP)).toEqual({ name: "Laptop", target: "8000.00", dueDate: "2027-01-20" });
    expect(emptyDraft()).toEqual({ name: "", target: "", dueDate: "" });
  });

  it("validates an amount and a date", () => {
    expect(validateAmount({ amount: "10.5", date: "2026-10-01" })).toEqual({});
    expect(validateAmount({ amount: "0", date: "" })).toEqual({ amount: expect.any(String), date: expect.any(String) });
  });

  it("starts a payment at the target and leaves the category to the backend", () => {
    const draft = defaultPayDraft(LAPTOP, "2026-10-05");
    expect(draft).toEqual({ amount: "8000.00", date: "2026-10-05", category: "" });
    expect(toPayInput(draft)).toEqual({ amount: "8000.00", date: "2026-10-05" });
    expect(toPayInput({ ...draft, category: "Ocio", amount: " 7900 " })).toEqual({ amount: "7900", date: "2026-10-05", category: "Ocio" });
  });

  it("compares amounts without floats", () => {
    expect(isGreater("350.26", "350.25")).toBe(true);
    expect(isGreater("350.25", "350.25")).toBe(false);
    expect(isGreater("0.10", "0.09")).toBe(true);
    expect(isEmptyBalance("0")).toBe(true);
    expect(isEmptyBalance("0.00")).toBe(true);
    expect(isEmptyBalance("-3")).toBe(true);
    expect(isEmptyBalance("0.01")).toBe(false);
  });

  it("maps failures to Spanish copy", () => {
    expect(describeFutureError(new ApiError(400, "invalid_future_expense", "name is required"))).toBe("Los datos no son válidos: name is required");
    expect(describeFutureError(new ApiError(409, "already_paid"))).toContain("ya está pagado");
    expect(describeFutureError(new ApiError(409, "insufficient_free_balance"))).toContain("saldo libre");
    expect(describeFutureError(new ApiError(404, "not_found"))).toContain("ya no existe");
    expect(describeFutureError(new ApiError(500, "internal_error"))).toContain("servidor");
  });
});

describe("ItemForm", () => {
  it("blocks an invalid form and focuses the first bad field", () => {
    wrap(<ItemForm editing={null} onSaved={vi.fn()} onCancel={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Agregar gasto futuro" }));
    expect(screen.getByLabelText("Nombre")).toHaveFocus();
    expect(screen.getAllByRole("alert")).toHaveLength(3);
    expect(api.createFutureExpense).not.toHaveBeenCalled();
  });

  it("creates an item and reports it", async () => {
    api.createFutureExpense.mockResolvedValue(LAPTOP);
    const onSaved = vi.fn();
    wrap(<ItemForm editing={null} onSaved={onSaved} onCancel={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Nombre"), { target: { value: "Laptop" } });
    fireEvent.change(screen.getByLabelText("Monto a juntar (MXN)"), { target: { value: "8000" } });
    fireEvent.change(screen.getByLabelText("Fecha de vencimiento"), { target: { value: "2027-01-20" } });
    fireEvent.click(screen.getByRole("button", { name: "Agregar gasto futuro" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(LAPTOP, false));
    expect(api.createFutureExpense).toHaveBeenCalledWith({ name: "Laptop", target_amount: "8000", due_date: "2027-01-20" });
  });

  it("edits an item with its current values", async () => {
    api.updateFutureExpense.mockResolvedValue(LAPTOP);
    const onSaved = vi.fn();
    wrap(<ItemForm editing={LAPTOP} onSaved={onSaved} onCancel={vi.fn()} />);
    expect(screen.getByLabelText("Nombre")).toHaveValue("Laptop");
    fireEvent.change(screen.getByLabelText("Monto a juntar (MXN)"), { target: { value: "9000" } });
    fireEvent.click(screen.getByRole("button", { name: "Guardar cambios" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(LAPTOP, true));
    expect(api.updateFutureExpense).toHaveBeenCalledWith(1, { name: "Laptop", target_amount: "9000", due_date: "2027-01-20" });
  });

  it("shows the backend message when the server rejects it", async () => {
    api.createFutureExpense.mockRejectedValue(new ApiError(400, "invalid_future_expense", "name is required"));
    wrap(<ItemForm editing={null} onSaved={vi.fn()} onCancel={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Nombre"), { target: { value: "x" } });
    fireEvent.change(screen.getByLabelText("Monto a juntar (MXN)"), { target: { value: "1" } });
    fireEvent.change(screen.getByLabelText("Fecha de vencimiento"), { target: { value: "2027-01-20" } });
    fireEvent.click(screen.getByRole("button", { name: "Agregar gasto futuro" }));
    expect(await screen.findByText("Los datos no son válidos: name is required")).toBeInTheDocument();
  });
});

describe("AmountDialog", () => {
  it("adds a saving", async () => {
    api.addFutureSaving.mockResolvedValue({});
    const onDone = vi.fn();
    wrap(<AmountDialog item={LAPTOP} mode="saving" freeBalance="350.25" onClose={vi.fn()} onDone={onDone} />);
    fireEvent.change(screen.getByLabelText("Monto (MXN)"), { target: { value: "500" } });
    fireEvent.click(screen.getByRole("button", { name: "Agregar ahorro" }));
    await waitFor(() => expect(onDone).toHaveBeenCalledWith("Se agregaron $500.00 de ahorro a Laptop."));
    expect(api.addFutureSaving).toHaveBeenCalledWith(1, { amount: "500", date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/) });
  });

  it("refuses to assign more than the free balance and sends nothing", () => {
    wrap(<AmountDialog item={LAPTOP} mode="assign" freeBalance="350.25" onClose={vi.fn()} onDone={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Monto (MXN)"), { target: { value: "350.26" } });
    fireEvent.click(screen.getByRole("button", { name: "Asignar" }));
    expect(screen.getByRole("alert")).toHaveTextContent("El saldo libre es $350.25");
    expect(api.assignFutureSaving).not.toHaveBeenCalled();
  });

  it("assigns part of the free balance", async () => {
    api.assignFutureSaving.mockResolvedValue(LAPTOP);
    const onDone = vi.fn();
    wrap(<AmountDialog item={LAPTOP} mode="assign" freeBalance="350.25" onClose={vi.fn()} onDone={onDone} />);
    fireEvent.change(screen.getByLabelText("Monto (MXN)"), { target: { value: "350.25" } });
    fireEvent.click(screen.getByRole("button", { name: "Asignar" }));
    await waitFor(() => expect(onDone).toHaveBeenCalledWith("Se asignaron $350.25 del saldo libre a Laptop."));
  });
});

describe("PayDialog", () => {
  it("confirms with the target by default and lets the category be automatic", async () => {
    api.payFutureExpense.mockResolvedValue({ item: PAID, expense: { id: 9, category: "Otros", amount: "8000.00" } });
    const onPaid = vi.fn();
    wrap(<PayDialog item={LAPTOP} categories={[{ value: "Ocio", label: "Ocio" }, { value: "Otros", label: "Otros" }]} onClose={vi.fn()} onPaid={onPaid} />);
    expect(screen.getByLabelText("Monto pagado (MXN)")).toHaveValue("8000.00");
    expect(screen.getByLabelText("Categoría del gasto")).toHaveValue("");
    fireEvent.click(screen.getByRole("button", { name: "Confirmar pago" }));
    await waitFor(() => expect(onPaid).toHaveBeenCalled());
    expect(api.payFutureExpense).toHaveBeenCalledWith(1, { amount: "8000.00", date: expect.any(String) });
  });

  it("sends the real amount and the chosen category", async () => {
    api.payFutureExpense.mockResolvedValue({ item: PAID, expense: { id: 9, category: "Ocio", amount: "7900" } });
    wrap(<PayDialog item={LAPTOP} categories={[{ value: "Ocio", label: "Ocio" }, { value: "Otros", label: "Otros" }]} onClose={vi.fn()} onPaid={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Monto pagado (MXN)"), { target: { value: "7900" } });
    fireEvent.change(screen.getByLabelText("Categoría del gasto"), { target: { value: "Ocio" } });
    fireEvent.click(screen.getByRole("button", { name: "Confirmar pago" }));
    await waitFor(() => expect(api.payFutureExpense).toHaveBeenCalledWith(1, { amount: "7900", date: expect.any(String), category: "Ocio" }));
  });

  it("blocks an invalid amount and says a double payment is rejected", async () => {
    wrap(<PayDialog item={LAPTOP} categories={[]} onClose={vi.fn()} onPaid={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Monto pagado (MXN)"), { target: { value: "0" } });
    fireEvent.click(screen.getByRole("button", { name: "Confirmar pago" }));
    expect(screen.getByLabelText("Monto pagado (MXN)")).toHaveFocus();
    expect(api.payFutureExpense).not.toHaveBeenCalled();

    api.payFutureExpense.mockRejectedValue(new ApiError(409, "already_paid"));
    fireEvent.change(screen.getByLabelText("Monto pagado (MXN)"), { target: { value: "100" } });
    fireEvent.click(screen.getByRole("button", { name: "Confirmar pago" }));
    expect(await screen.findByText(/ya está pagado/)).toBeInTheDocument();
  });
});

describe("FutureExpenses page", () => {
  it("shows the free balance, the active items, the totals and the paid history", async () => {
    api.listFutureExpenses.mockResolvedValue(LIST);
    wrap(<FutureExpenses />);
    expect(await screen.findByRole("list", { name: "Gastos futuros activos" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Saldo libre" })).toHaveTextContent("$350.25");
    const active = within(screen.getByRole("list", { name: "Gastos futuros activos" })).getAllByRole("listitem");
    expect(active).toHaveLength(2);
    expect(active[1]).toHaveTextContent("$2,000.00 de $8,000.00 (25%)");
    expect(active[1]).toHaveTextContent("Aparta $1,500.00 al mes (4 ciclos restantes)");
    expect(active[0]).toHaveTextContent("Meta cubierta");
    expect(screen.getByText("Sugerido al mes").nextSibling).toHaveTextContent("$1,500.00");
    expect(within(screen.getByRole("list", { name: "Gastos futuros pagados" })).getByRole("listitem")).toHaveTextContent("Pagado el 2 de septiembre de 2026 · $4,800.00");
  });

  it("shows each item as one summary card, without per-row action buttons", async () => {
    api.listFutureExpenses.mockResolvedValue(LIST);
    wrap(<FutureExpenses />);
    const list = await screen.findByRole("list", { name: "Gastos futuros activos" });
    for (const card of within(list).getAllByRole("listitem")) {
      expect(within(card).getAllByRole("button")).toHaveLength(1);
      expect(within(card).getByRole("progressbar")).toBeInTheDocument();
    }
    expect(screen.queryByRole("button", { name: /Marcar pagado|Editar|Eliminar|Agregar ahorro/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Nuevo gasto futuro" })).toBeInTheDocument();
  });

  it("opens the detail with every action of the item", async () => {
    api.listFutureExpenses.mockResolvedValue(LIST);
    wrap(<FutureExpenses />);
    await screen.findByRole("list", { name: "Gastos futuros activos" });
    const card = screen.getByRole("button", { name: "Ver Laptop" });
    card.focus();
    fireEvent.click(card);
    const detail = screen.getByRole("dialog", { name: "Laptop" });
    for (const name of ["Agregar ahorro a Laptop", "Asignar saldo libre a Laptop", "Marcar pagado Laptop", "Editar Laptop", "Eliminar Laptop"]) {
      expect(within(detail).getByRole("button", { name })).toBeInTheDocument();
    }
    fireEvent.keyDown(detail, { key: "Escape" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Ver Laptop" })).toHaveFocus();
  });

  it("hides the assign action when there is no free balance", async () => {
    api.listFutureExpenses.mockResolvedValue({ ...LIST, free_balance: "0.00" });
    wrap(<FutureExpenses />);
    await screen.findByRole("list", { name: "Gastos futuros activos" });
    fireEvent.click(screen.getByRole("button", { name: "Ver Laptop" }));
    expect(screen.getByRole("button", { name: "Marcar pagado Laptop" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Asignar saldo libre/ })).not.toBeInTheDocument();
  });

  it("hands the detail over to the edit, add-saving and pay dialogs", async () => {
    api.listFutureExpenses.mockResolvedValue(LIST);
    wrap(<FutureExpenses />);
    await screen.findByRole("list", { name: "Gastos futuros activos" });
    fireEvent.click(screen.getByRole("button", { name: "Ver Laptop" }));
    fireEvent.click(screen.getByRole("button", { name: "Editar Laptop" }));
    expect(screen.getByRole("dialog", { name: "Editar Laptop" })).toBeInTheDocument();
    expect(screen.getByLabelText("Nombre")).toHaveValue("Laptop");
    fireEvent.click(screen.getByRole("button", { name: "Cancelar" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Ver Laptop" }));
    fireEvent.click(screen.getByRole("button", { name: "Agregar ahorro a Laptop" }));
    expect(screen.getByLabelText("Monto (MXN)")).toBeInTheDocument();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "Cancelar" }));

    fireEvent.click(screen.getByRole("button", { name: "Ver Laptop" }));
    fireEvent.click(screen.getByRole("button", { name: "Marcar pagado Laptop" }));
    expect(screen.getByLabelText("Monto pagado (MXN)")).toBeInTheDocument();
  });

  it("creates a new one from the header button, in a dialog", async () => {
    api.listFutureExpenses.mockResolvedValue(LIST);
    api.createFutureExpense.mockResolvedValue(LAPTOP);
    wrap(<FutureExpenses />);
    await screen.findByRole("list", { name: "Gastos futuros activos" });
    fireEvent.click(screen.getByRole("button", { name: "Nuevo gasto futuro" }));
    const dialog = screen.getByRole("dialog", { name: "Nuevo gasto futuro" });
    fireEvent.change(within(dialog).getByLabelText("Nombre"), { target: { value: "Laptop" } });
    fireEvent.change(within(dialog).getByLabelText("Monto a juntar (MXN)"), { target: { value: "8000" } });
    fireEvent.change(within(dialog).getByLabelText("Fecha de vencimiento"), { target: { value: "2027-01-20" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Agregar gasto futuro" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.getByRole("status")).toHaveTextContent("Laptop agregado");
  });

  it("asks before deleting, from the detail, and explains the savings are kept", async () => {
    api.listFutureExpenses.mockResolvedValue(LIST);
    api.deleteFutureExpense.mockResolvedValue(undefined);
    wrap(<FutureExpenses />);
    await screen.findByRole("list", { name: "Gastos futuros activos" });
    fireEvent.click(screen.getByRole("button", { name: "Ver Laptop" }));
    fireEvent.click(screen.getByRole("button", { name: "Eliminar Laptop" }));
    expect(screen.getByText(/vuelve al saldo libre/)).toBeInTheDocument();
    expect(api.deleteFutureExpense).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Sí, eliminar" }));
    await waitFor(() => expect(api.deleteFutureExpense).toHaveBeenCalledWith(1));
    expect(await screen.findByText(/Laptop se eliminó/)).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("removes a paid item from the history after confirming", async () => {
    api.listFutureExpenses.mockResolvedValue(LIST);
    api.deleteFutureExpense.mockResolvedValue(undefined);
    wrap(<FutureExpenses />);
    await screen.findByRole("list", { name: "Gastos futuros pagados" });
    fireEvent.click(screen.getByRole("button", { name: "Ver Teléfono" }));
    expect(screen.getByRole("dialog", { name: "Teléfono" })).toHaveTextContent("$4,800.00");
    fireEvent.click(screen.getByRole("button", { name: "Eliminar Teléfono" }));
    fireEvent.click(screen.getByRole("button", { name: "Sí, eliminar" }));
    await waitFor(() => expect(api.deleteFutureExpense).toHaveBeenCalledWith(3));
    expect(await screen.findByText(/Teléfono se quitó del historial/)).toBeInTheDocument();
  });

  it("explains an empty state and offers a retry when loading fails", async () => {
    api.listFutureExpenses.mockResolvedValueOnce({ active: [], paid: [], totals: LIST.totals, free_balance: "0" });
    const { unmount } = wrap(<FutureExpenses />);
    expect(await screen.findByText(/Aún no tienes gastos futuros/)).toBeInTheDocument();
    unmount();
    api.listFutureExpenses.mockRejectedValueOnce(new ApiError(500, "internal_error"));
    wrap(<FutureExpenses />);
    expect(await screen.findByRole("button", { name: "Reintentar" })).toBeInTheDocument();
  });
});
