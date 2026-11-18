import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { FUTURE_BILL, INACTIVE_BILL, OVERDUE_BILL, VARIABLE_BILL } from "../pages/bills/fixtures";
import { QuickBills, pendingBills } from "./QuickBills";
import { Tabs } from "./Tabs";

const api = vi.hoisted(() => ({ listBills: vi.fn(), payBill: vi.fn() }));
vi.mock("../api/bills", async (importOriginal) => ({ ...(await importOriginal<typeof import("../api/bills")>()), ...api }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function show(onPaid = vi.fn(), onClose = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <QuickBills categories={["Servicios", "Suscripciones"]} onClose={onClose} onPaid={onPaid} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { onPaid, onClose };
}

describe("pendingBills", () => {
  it("keeps the active bills with a pending occurrence, soonest first", () => {
    const noPending = { ...FUTURE_BILL, id: 9, pending_occurrence: null };
    expect(pendingBills([FUTURE_BILL, INACTIVE_BILL, VARIABLE_BILL, noPending, OVERDUE_BILL]).map((b) => b.id)).toEqual([OVERDUE_BILL.id, VARIABLE_BILL.id, FUTURE_BILL.id]);
  });
});

describe("QuickBills", () => {
  it("lists the bills with amount or Variable, and pays a fixed one with the amount prefilled", async () => {
    api.listBills.mockResolvedValue([VARIABLE_BILL, OVERDUE_BILL]);
    api.payBill.mockResolvedValue({
      bill: { ...OVERDUE_BILL, next_due_date: "2026-11-01" },
      paid_occurrence: {},
      expense: { id: 1, date: "2026-10-10", description: "Megacable", category: "Servicios", currency: "MXN", amount: "550.00", amount_mxn: "550.00" },
    });
    const { onPaid } = show();
    const radios = await screen.findAllByRole("radio");
    expect(radios.map((r) => r.closest("li")?.textContent)).toEqual([expect.stringContaining("Megacable"), expect.stringContaining("Luz")]);
    expect(radios[1].closest("li")).toHaveTextContent("Variable");

    fireEvent.click(radios[0]);
    expect(screen.getByLabelText("Monto (MXN)")).toHaveValue("550.00");
    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));
    await waitFor(() => expect(onPaid).toHaveBeenCalledWith(expect.stringContaining("Pago de Megacable registrado: $550.00 MXN en Servicios.")));
    expect(api.payBill).toHaveBeenCalledWith(OVERDUE_BILL.id, expect.objectContaining({ amount: "550.00", category: "Servicios" }));
  });

  it("asks for the amount of a variable bill before calling the API", async () => {
    api.listBills.mockResolvedValue([VARIABLE_BILL]);
    show();
    fireEvent.click(await screen.findByRole("radio"));
    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));
    expect(await screen.findByText(/Escribe un monto mayor a cero/)).toBeInTheDocument();
    expect(api.payBill).not.toHaveBeenCalled();
  });

  it("shows the API error and does not report a payment", async () => {
    api.listBills.mockResolvedValue([OVERDUE_BILL]);
    api.payBill.mockRejectedValue(new ApiError(409, "occurrence_resolved", "done"));
    const { onPaid } = show();
    fireEvent.click(await screen.findByRole("radio"));
    fireEvent.click(screen.getByRole("button", { name: "Registrar pago" }));
    expect(await screen.findByText(/ya se pagó u omitió/)).toBeInTheDocument();
    expect(onPaid).not.toHaveBeenCalled();
  });

  it("shows the empty state with a link to Pagos recurrentes", async () => {
    api.listBills.mockResolvedValue([INACTIVE_BILL]);
    show();
    expect(await screen.findByText("No tienes pagos recurrentes pendientes.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Ir a Pagos recurrentes" })).toHaveAttribute("href", "/pagos-recurrentes");
  });
});

describe("Tabs", () => {
  it("selects with the arrow keys, wrapping, and uses a roving tabindex", () => {
    const onSelect = vi.fn();
    render(<Tabs items={[{ id: "a", label: "Uno" }, { id: "b", label: "Dos" }]} active="a" onSelect={onSelect} label="Tipos" idPrefix="t" />);
    const [uno, dos] = screen.getAllByRole("tab");
    expect(uno).toHaveAttribute("tabindex", "0");
    expect(dos).toHaveAttribute("tabindex", "-1");
    fireEvent.keyDown(uno, { key: "ArrowRight" });
    expect(onSelect).toHaveBeenLastCalledWith("b");
    fireEvent.keyDown(uno, { key: "ArrowLeft" });
    expect(onSelect).toHaveBeenLastCalledWith("b");
    fireEvent.keyDown(dos, { key: "Home" });
    expect(onSelect).toHaveBeenLastCalledWith("a");
  });
});
