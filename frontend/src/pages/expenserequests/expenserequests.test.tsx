import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { AllSettings } from "../../api/settings";
import { tokenStore } from "../../api/tokens";
import { AuthProvider } from "../../auth/AuthContext";
import { ExpenseRequests } from "../ExpenseRequests";
import { describeRequestError } from "./errors";
import { APPROVED_EXPENSE, APPROVED_FUTURE, CANCELLED, CATEGORIES, EXCEEDS, FITS, NO_BUDGET, PENDING, REJECTED } from "./fixtures";
import { emptyDraft, toApproveInput, toNewRequestInput, validateApproval, validateComment, validateRequest } from "./form";

const api = vi.hoisted(() => ({
  listExpenseRequests: vi.fn(),
  getRequestCategories: vi.fn(),
  createExpenseRequest: vi.fn(),
  cancelExpenseRequest: vi.fn(),
  getBudgetCheck: vi.fn(),
  approveExpenseRequest: vi.fn(),
  rejectExpenseRequest: vi.fn(),
  revertExpenseRequest: vi.fn(),
}));
const settingsApi = vi.hoisted(() => ({ getSettings: vi.fn() }));
vi.mock("../../api/expenseRequests", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/expenseRequests")>()), ...api }));
vi.mock("../../api/settings", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/settings")>()), ...settingsApi }));

// Vitest runs without globals, so Testing Library does not unmount between tests on its own.
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  localStorage.clear();
});

beforeEach(() => {
  api.getRequestCategories.mockResolvedValue(CATEGORIES);
  settingsApi.getSettings.mockResolvedValue({ payment_methods: ["Efectivo", "Débito", "Crédito"] } as unknown as AllSettings);
});

function renderPage(role: "owner" | "household", ui: ReactNode = <ExpenseRequests />) {
  localStorage.setItem("access_token", "a");
  localStorage.setItem("refresh_token", "r");
  tokenStore.setRole(role);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <AuthProvider>{ui}</AuthProvider>
    </QueryClientProvider>,
  );
}

describe("form helpers", () => {
  const ok = { amount: "250.50", description: " Tacos ", category: "Comida fuera", date: "2026-10-03" };

  it("validates a new request", () => {
    expect(validateRequest(ok)).toEqual({});
    for (const bad of ["", "0", "-5", "1,600", "1.234", "abc", "1000000000000"]) expect(validateRequest({ ...ok, amount: bad }).amount, bad).toContain("mayor a cero");
    expect(validateRequest({ ...ok, description: "  " }).description).toContain("Describe");
    expect(validateRequest({ ...ok, description: "x".repeat(121) }).description).toContain("120");
    expect(validateRequest({ ...ok, date: "" }).date).toContain("fecha");
  });

  it("builds the body with trimmed text, the typed amount and only a real suggestion", () => {
    expect(toNewRequestInput(ok)).toEqual({ amount: "250.50", description: "Tacos", date: "2026-10-03", suggested_category: "Comida fuera" });
    expect(toNewRequestInput({ ...ok, category: "" })).toEqual({ amount: "250.50", description: "Tacos", date: "2026-10-03" });
    expect(emptyDraft("2026-10-03")).toEqual({ amount: "", description: "", category: "", date: "2026-10-03" });
  });

  it("validates each destination of an approval", () => {
    const draft = { destination: "gasto" as const, category: "Ocio", date: "2026-10-03", paymentMethod: "Débito", dueDate: "" };
    expect(validateApproval(draft)).toEqual({});
    expect(validateApproval({ ...draft, category: "" }).category).toContain("categoría");
    expect(validateApproval({ ...draft, date: "" }).date).toContain("fecha");
    // The due date is only needed (and checked) for the future expense.
    expect(validateApproval({ ...draft, destination: "gasto_futuro" }).dueDate).toContain("vencimiento");
    expect(validateApproval({ ...draft, destination: "gasto_futuro", category: "", dueDate: "2026-12-20" })).toEqual({});
  });

  it("sends only the fields of the chosen destination", () => {
    const draft = { destination: "gasto" as const, category: "Ocio", date: "2026-10-03", paymentMethod: "Efectivo", dueDate: "2026-12-20" };
    expect(toApproveInput(draft)).toEqual({ destination: "gasto", category: "Ocio", date: "2026-10-03", payment_method: "Efectivo" });
    expect(toApproveInput({ ...draft, destination: "gasto_futuro" })).toEqual({ destination: "gasto_futuro", due_date: "2026-12-20" });
  });

  it("requires a bounded comment to reject", () => {
    expect(validateComment("  ")).toContain("motivo");
    expect(validateComment("x".repeat(501))).toContain("500");
    expect(validateComment("No este mes")).toBeNull();
  });
});

describe("describeRequestError", () => {
  it("maps the module codes to Spanish copy", () => {
    expect(describeRequestError(new ApiError(409, "invalid_state"))).toContain("ya fue resuelta");
    expect(describeRequestError(new ApiError(429, "rate_limited"))).toContain("demasiadas");
    expect(describeRequestError(new ApiError(409, "future_expense_paid"))).toContain("ya se pagó");
    expect(describeRequestError(new ApiError(400, "invalid_expense_request", "amount must be greater than zero"))).toBe("Los datos no son válidos: amount must be greater than zero");
    expect(describeRequestError(new ApiError(404, "not_found"))).toContain("ya no existe");
    expect(describeRequestError(new ApiError(403, "forbidden"))).toContain("no tiene permiso");
    expect(describeRequestError(new TypeError("network"))).toContain("No se pudo conectar");
  });
});

describe("household view", () => {
  it("lists her requests with a state badge, the owner comment and what happened", async () => {
    api.listExpenseRequests.mockResolvedValue([PENDING, REJECTED, APPROVED_EXPENSE, APPROVED_FUTURE, CANCELLED]);
    renderPage("household");
    const cards = await screen.findAllByTestId("request-card");
    expect(cards).toHaveLength(5);
    expect(cards.map((c) => within(c).getByTestId("request-state").textContent)).toEqual(["Solicitada", "Rechazada", "Aprobada", "Aprobada", "Cancelada"]);
    expect(cards[1]).toHaveTextContent("Comentario: Mejor el mes que entra");
    expect(cards[2]).toHaveTextContent("Se registró como gasto.");
    expect(cards[3]).toHaveTextContent("Se movió a gastos futuros.");
    // Cancelar only on the pending one; nothing owner-only on the page.
    expect(screen.getAllByRole("button", { name: /^Cancelar la petición/ })).toHaveLength(1);
    expect(screen.queryByRole("button", { name: /Aprobar|Rechazar|Volver a solicitada/ })).toBeNull();
    expect(api.listExpenseRequests).toHaveBeenCalledWith();
    expect(settingsApi.getSettings).not.toHaveBeenCalled();
  });

  it("shows an empty note and the retry button on a failure", async () => {
    api.listExpenseRequests.mockResolvedValueOnce([]);
    renderPage("household");
    expect(await screen.findByText(/Aún no has hecho ninguna petición/)).toBeInTheDocument();
    cleanup();

    api.listExpenseRequests.mockRejectedValueOnce(new ApiError(403, "forbidden")).mockResolvedValueOnce([PENDING]);
    renderPage("household");
    expect(await screen.findByRole("alert")).toHaveTextContent("no tiene permiso");
    fireEvent.click(screen.getByRole("button", { name: "Reintentar" }));
    expect(await screen.findByTestId("request-card")).toBeInTheDocument();
  });

  it("creates a request from the collapsed form", async () => {
    api.listExpenseRequests.mockResolvedValue([]);
    api.createExpenseRequest.mockResolvedValue(PENDING);
    renderPage("household");
    await screen.findByText(/Aún no has hecho ninguna petición/);
    const toggle = screen.getByRole("button", { name: "Nueva petición" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(toggle);

    fireEvent.click(screen.getByRole("button", { name: "Enviar petición" }));
    expect(await screen.findByText(/Escribe un monto mayor a cero/)).toBeInTheDocument();
    expect(api.createExpenseRequest).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText("Monto (MXN)"), { target: { value: "250.50" } });
    fireEvent.change(screen.getByLabelText("Descripción"), { target: { value: "Tacos" } });
    await waitFor(() => expect(screen.getByRole("option", { name: "Comida fuera" })).toBeInTheDocument());
    fireEvent.change(screen.getByLabelText("Categoría sugerida"), { target: { value: "Comida fuera" } });
    fireEvent.change(screen.getByLabelText("Fecha del gasto"), { target: { value: "2026-10-03" } });
    fireEvent.click(screen.getByRole("button", { name: "Enviar petición" }));

    await waitFor(() => expect(api.createExpenseRequest).toHaveBeenCalledTimes(1));
    expect(api.createExpenseRequest.mock.calls[0][0]).toEqual({ amount: "250.50", description: "Tacos", date: "2026-10-03", suggested_category: "Comida fuera" });
    expect(await screen.findByText(/Petición enviada/)).toBeInTheDocument();
  });

  it("shows the rate limit message when the server refuses the request", async () => {
    api.listExpenseRequests.mockResolvedValue([]);
    api.createExpenseRequest.mockRejectedValue(new ApiError(429, "rate_limited"));
    renderPage("household");
    await screen.findByText(/Aún no has hecho/);
    fireEvent.click(screen.getByRole("button", { name: "Nueva petición" }));
    fireEvent.change(screen.getByLabelText("Monto (MXN)"), { target: { value: "10" } });
    fireEvent.change(screen.getByLabelText("Descripción"), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: "Enviar petición" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("demasiadas peticiones");
  });

  it("cancels a pending request and says so", async () => {
    api.listExpenseRequests.mockResolvedValue([PENDING]);
    api.cancelExpenseRequest.mockResolvedValue({ ...PENDING, status: "cancelada" });
    renderPage("household");
    fireEvent.click(await screen.findByRole("button", { name: "Cancelar la petición Tacos" }));
    await waitFor(() => expect(api.cancelExpenseRequest).toHaveBeenCalledWith(1));
    expect(await screen.findByText(/Cancelaste tu petición/)).toBeInTheDocument();
  });

  it("explains a request that was decided meanwhile", async () => {
    api.listExpenseRequests.mockResolvedValue([PENDING]);
    api.cancelExpenseRequest.mockRejectedValue(new ApiError(409, "invalid_state"));
    renderPage("household");
    fireEvent.click(await screen.findByRole("button", { name: "Cancelar la petición Tacos" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("ya fue resuelta");
  });
});

describe("owner view", () => {
  it("lists the pending requests first and filters by state", async () => {
    api.listExpenseRequests.mockImplementation(async (status?: string) => (status === "rechazada" ? [REJECTED] : status === "" ? [PENDING, REJECTED] : [PENDING]));
    renderPage("owner");
    expect(await screen.findByText("Tacos")).toBeInTheDocument();
    expect(api.listExpenseRequests).toHaveBeenCalledWith("solicitada");
    expect(screen.getByRole("button", { name: "Solicitadas" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("spouse@example.com")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Aprobar la petición Tacos" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Rechazadas" }));
    expect(await screen.findByText("Zapatos")).toBeInTheDocument();
    expect(api.listExpenseRequests).toHaveBeenCalledWith("rechazada");
    expect(screen.queryByRole("button", { name: /Aprobar la petición/ })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Todas" }));
    await waitFor(() => expect(screen.getAllByTestId("request-card")).toHaveLength(2));
  });

  it("shows an empty note for no pending requests and a message on a 403", async () => {
    api.listExpenseRequests.mockResolvedValueOnce([]);
    renderPage("owner");
    expect(await screen.findByText("No hay peticiones pendientes.")).toBeInTheDocument();
    cleanup();
    api.listExpenseRequests.mockRejectedValueOnce(new ApiError(403, "forbidden"));
    renderPage("owner");
    expect(await screen.findByRole("alert")).toHaveTextContent("no tiene permiso");
  });

  async function openApprove() {
    api.listExpenseRequests.mockResolvedValue([PENDING]);
    renderPage("owner");
    fireEvent.click(await screen.findByRole("button", { name: "Aprobar la petición Tacos" }));
    return await screen.findByRole("dialog", { name: "Aprobar petición" });
  }

  it("defaults the category to the suggestion and checks the budget live as it changes", async () => {
    api.getBudgetCheck.mockImplementation(async (_id: number, category: string) => (category === "Ocio" ? NO_BUDGET : FITS));
    const dialog = await openApprove();
    await waitFor(() => expect(screen.getByLabelText("Categoría del gasto")).toHaveValue("Comida fuera"));
    expect(await within(dialog).findByTestId("budget-fit")).toHaveTextContent("Cabe en el presupuesto");
    expect(api.getBudgetCheck).toHaveBeenCalledWith(1, "Comida fuera", "2026-10-03");

    fireEvent.change(screen.getByLabelText("Categoría del gasto"), { target: { value: "Ocio" } });
    await waitFor(() => expect(within(dialog).getByTestId("budget-fit")).toHaveTextContent("Sin presupuesto para esta categoría"));
    expect(api.getBudgetCheck).toHaveBeenLastCalledWith(1, "Ocio", "2026-10-03");

    fireEvent.change(screen.getByLabelText("Fecha del gasto"), { target: { value: "2026-11-20" } });
    await waitFor(() => expect(api.getBudgetCheck).toHaveBeenLastCalledWith(1, "Ocio", "2026-11-20"));
  });

  it("shows the excess but still approves a request that does not fit", async () => {
    api.getBudgetCheck.mockResolvedValue(EXCEEDS);
    api.approveExpenseRequest.mockResolvedValue({
      request: { ...PENDING, status: "aprobada", result_kind: "gasto", result_movement_id: 40 },
      budget: { month: "2026-10", category: "Comida fuera", budget: "1000", spent: "1150.5", remaining: "-150.5", over_budget: true },
    });
    const dialog = await openApprove();
    expect(await within(dialog).findByText(/Excede el presupuesto por \$150\.50/)).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Método de pago"), { target: { value: "Efectivo" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Aprobar" }));
    await waitFor(() => expect(api.approveExpenseRequest).toHaveBeenCalledTimes(1));
    expect(api.approveExpenseRequest).toHaveBeenCalledWith(1, { destination: "gasto", category: "Comida fuera", date: "2026-10-03", payment_method: "Efectivo" });
    expect(await screen.findByText(/ahora excede su presupuesto/)).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("moves to a future expense without ever running the budget check", async () => {
    api.getBudgetCheck.mockResolvedValue(FITS);
    api.approveExpenseRequest.mockResolvedValue({ request: { ...PENDING, status: "aprobada", result_kind: "gasto_futuro", result_future_expense_id: 9 }, budget: null });
    const dialog = await openApprove();
    await within(dialog).findByTestId("budget-fit");
    const checks = api.getBudgetCheck.mock.calls.length;

    fireEvent.click(within(dialog).getByRole("radio", { name: /Mover a gasto futuro/ }));
    expect(within(dialog).queryByTestId("budget-fit")).toBeNull();
    expect(within(dialog).getByText(/No cuenta contra el presupuesto de este ciclo/)).toBeInTheDocument();

    fireEvent.click(within(dialog).getByRole("button", { name: "Aprobar" }));
    expect(await within(dialog).findByText("Elige la fecha de vencimiento.")).toBeInTheDocument();
    expect(api.approveExpenseRequest).not.toHaveBeenCalled();

    fireEvent.change(within(dialog).getByLabelText("Fecha de vencimiento"), { target: { value: "2026-12-20" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Aprobar" }));
    await waitFor(() => expect(api.approveExpenseRequest).toHaveBeenCalledWith(1, { destination: "gasto_futuro", due_date: "2026-12-20" }));
    expect(api.getBudgetCheck.mock.calls.length).toBe(checks);
    expect(await screen.findByText(/se movió a gastos futuros/)).toBeInTheDocument();
  });

  it("keeps the approval possible when the budget check fails", async () => {
    api.getBudgetCheck.mockRejectedValue(new ApiError(500, "internal_error"));
    const dialog = await openApprove();
    expect(await within(dialog).findByText(/Aun así puedes aprobar la petición/)).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Aprobar" })).toBeEnabled();
  });

  it("shows the conflict when the request was decided meanwhile", async () => {
    api.getBudgetCheck.mockResolvedValue(FITS);
    api.approveExpenseRequest.mockRejectedValue(new ApiError(409, "invalid_state"));
    const dialog = await openApprove();
    await within(dialog).findByTestId("budget-fit");
    fireEvent.click(within(dialog).getByRole("button", { name: "Aprobar" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("ya fue resuelta");
  });

  it("rejects only with a comment and sends it", async () => {
    api.listExpenseRequests.mockResolvedValue([PENDING]);
    api.rejectExpenseRequest.mockResolvedValue({ ...PENDING, status: "rechazada", decision_comment: "No este mes" });
    renderPage("owner");
    fireEvent.click(await screen.findByRole("button", { name: "Rechazar la petición Tacos" }));
    const dialog = await screen.findByRole("dialog", { name: "Rechazar petición" });
    expect(dialog.querySelector("textarea")).toHaveFocus();

    fireEvent.click(within(dialog).getByRole("button", { name: "Rechazar petición" }));
    expect(await within(dialog).findByText(/Escribe el motivo del rechazo/)).toBeInTheDocument();
    expect(api.rejectExpenseRequest).not.toHaveBeenCalled();

    fireEvent.change(within(dialog).getByLabelText("Comentario"), { target: { value: "  No este mes " } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Rechazar petición" }));
    await waitFor(() => expect(api.rejectExpenseRequest).toHaveBeenCalledWith(1, "No este mes"));
    expect(await screen.findByText(/Rechazaste Tacos/)).toBeInTheDocument();
  });
});

describe("reverting an approval (owner)", () => {
  async function openRevert(name: string) {
    fireEvent.click(await screen.findByRole("button", { name: `Volver a solicitada la petición ${name}` }));
    return screen.findByRole("dialog", { name: "Volver a solicitada" });
  }

  it("offers the action only on approved requests", async () => {
    api.listExpenseRequests.mockResolvedValue([PENDING, REJECTED, APPROVED_EXPENSE, CANCELLED]);
    renderPage("owner");
    await screen.findAllByTestId("request-card");
    expect(screen.getAllByRole("button", { name: /^Volver a solicitada la petición/ })).toHaveLength(1);
  });

  it("says exactly what a Gasto approval will undo and reverts it", async () => {
    api.listExpenseRequests.mockResolvedValue([APPROVED_EXPENSE]);
    api.revertExpenseRequest.mockResolvedValue({ ...PENDING, id: 3, description: "Gasolina", revert_count: 1, reverted_at: "2026-10-05T12:00:00Z" });
    renderPage("owner");
    const dialog = await openRevert("Gasolina");
    expect(dialog).toHaveTextContent("Se eliminará el gasto registrado de $250.50.");
    fireEvent.click(within(dialog).getByRole("button", { name: "Volver a solicitada" }));
    await waitFor(() => expect(api.revertExpenseRequest).toHaveBeenCalledWith(3));
    expect(await screen.findByText(/Gasolina volvió a solicitada/)).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("says that the savings of a future expense go back to the free balance", async () => {
    api.listExpenseRequests.mockResolvedValue([APPROVED_FUTURE]);
    renderPage("owner");
    const dialog = await openRevert("Regalo");
    expect(dialog).toHaveTextContent("Se eliminará el gasto futuro «Regalo»; su ahorro asignado vuelve al saldo libre.");
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancelar" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(api.revertExpenseRequest).not.toHaveBeenCalled();
  });

  it("keeps the dialog open with a clear message on 409 and 403", async () => {
    api.listExpenseRequests.mockResolvedValue([APPROVED_FUTURE]);
    renderPage("owner");
    const dialog = await openRevert("Regalo");

    api.revertExpenseRequest.mockRejectedValueOnce(new ApiError(409, "future_expense_paid"));
    fireEvent.click(within(dialog).getByRole("button", { name: "Volver a solicitada" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("ya se pagó");

    api.revertExpenseRequest.mockRejectedValueOnce(new ApiError(409, "invalid_state"));
    fireEvent.click(within(dialog).getByRole("button", { name: "Volver a solicitada" }));
    await waitFor(() => expect(within(dialog).getByRole("alert")).toHaveTextContent("ya no está aprobada"));

    api.revertExpenseRequest.mockRejectedValueOnce(new ApiError(403, "forbidden"));
    fireEvent.click(within(dialog).getByRole("button", { name: "Volver a solicitada" }));
    await waitFor(() => expect(within(dialog).getByRole("alert")).toHaveTextContent("no tiene permiso"));
    expect(screen.getByRole("button", { name: "Volver a solicitada la petición Regalo" })).toBeInTheDocument();
  });

  it("shows the revert history on a pending request, for the owner and the requester", async () => {
    api.listExpenseRequests.mockResolvedValue([{ ...PENDING, revert_count: 2, reverted_at: "2026-10-05T12:00:00Z" }, PENDING]);
    renderPage("household");
    const cards = await screen.findAllByTestId("request-card");
    expect(within(cards[0]).getByTestId("revert-history")).toHaveTextContent("la aprobación se deshizo 2 veces");
    expect(within(cards[1]).queryByTestId("revert-history")).toBeNull();
    expect(within(cards[0]).getByTestId("request-state")).toHaveTextContent("Solicitada");
  });
});
