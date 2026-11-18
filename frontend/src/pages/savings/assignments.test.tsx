import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Saving } from "../../api/savings";
import { AssignmentLegDialog } from "./SavingDialogs";
import { SavingsList } from "./SavingsList";
import { isAssignment } from "./assignment";

const api = vi.hoisted(() => ({ listSavings: vi.fn(), deleteSaving: vi.fn() }));
vi.mock("../../api/savings", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/savings")>()), listSavings: api.listSavings, deleteSaving: api.deleteSaving }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const base: Saving = {
  id: 1,
  date: "2026-10-05",
  description: "Aportación VOO",
  category: "Inversión ETF",
  instrument: "i1",
  payment_method: "Transferencia",
  currency: "MXN",
  amount: "100.00",
  exchange_rate: null,
  amount_mxn: "100.00",
  transfer_id: null,
  future_expense_id: null,
};
const OUT: Saving = { ...base, id: 5, description: "Asignado a Viaje", category: "Gastos futuros", amount: "-6000.00", amount_mxn: "-6000.00", transfer_id: "t1" };
const IN: Saving = { ...OUT, id: 6, amount: "6000.00", amount_mxn: "6000.00", future_expense_id: 3 };
const LEG: Saving = { ...base, id: 7, description: "Traspaso a -> b", amount: "-50.00", transfer_id: "t2" };

function wrap(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe("isAssignment", () => {
  it("needs a transfer id and either an item link or the Asignado a description", () => {
    expect(isAssignment(IN)).toBe(true);
    expect(isAssignment(OUT)).toBe(true);
    expect(isAssignment(LEG)).toBe(false);
    expect(isAssignment(base)).toBe(false);
    expect(isAssignment({ ...IN, transfer_id: null })).toBe(false);
  });
});

describe("SavingsList assignments", () => {
  it("collapses the pair into one positive, badged row and leaves traspasos alone", async () => {
    api.listSavings.mockResolvedValue([IN, OUT, LEG, base]);
    const onOpen = vi.fn();
    wrap(<SavingsList month="2026-10" instruments={[]} onOpen={onOpen} />);
    const rows = await screen.findAllByRole("listitem");
    expect(rows).toHaveLength(3);
    const row = screen.getByRole("button", { name: "Ver Asignado a Viaje" });
    expect(row).toHaveTextContent("Asignación");
    expect(row).toHaveTextContent("$6,000.00 MXN");
    expect(row).not.toHaveTextContent("-$6,000.00");
    expect(row).not.toHaveTextContent("Retiro");
    expect(row).toHaveTextContent("2026-10-05 · Gastos futuros");
    expect(row.querySelector(".text-destructive")).toBeNull();
    const leg = screen.getByRole("button", { name: "Ver Traspaso a -> b" });
    expect(leg).toHaveTextContent("Traspaso");
    expect(leg).toHaveTextContent("Retiro");
    fireEvent.click(row);
    expect(onOpen).toHaveBeenCalledWith(IN);
  });
});

describe("AssignmentLegDialog", () => {
  it("explains the saldo libre and deletes the assignment after a confirmation", async () => {
    api.deleteSaving.mockResolvedValue(undefined);
    const onDeleted = vi.fn();
    wrap(<AssignmentLegDialog saving={IN} onClose={() => {}} onDeleted={onDeleted} />);
    expect(screen.getByRole("dialog", { name: "Asignación" })).toHaveTextContent("saldo libre");
    fireEvent.click(screen.getByRole("button", { name: /^Eliminar asignación/ }));
    fireEvent.click(screen.getByRole("button", { name: "Sí, eliminar" }));
    await waitFor(() => expect(onDeleted).toHaveBeenCalled());
    expect(api.deleteSaving).toHaveBeenCalledWith(6);
  });
});
