import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PersonalMovement } from "../../api/personal";
import { PERSONAL_PAGES } from "./config";
import { MovementList } from "./MovementList";

const api = vi.hoisted(() => ({ listPersonalMovements: vi.fn(), deletePersonalMovement: vi.fn() }));
vi.mock("../../api/personal", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api/personal")>()),
  listPersonalMovements: api.listPersonalMovements,
  deletePersonalMovement: api.deletePersonalMovement,
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const row = (over: Partial<PersonalMovement>): PersonalMovement => ({
  id: 1,
  kind: "Ahorro",
  category_id: 9,
  category: "Gastos futuros",
  amount: "300.00",
  description: "Ahorro",
  date: "2026-10-05",
  future_expense_id: null,
  transfer_id: null,
  created_at: "2026-10-05T10:00:00Z",
  updated_at: "2026-10-05T10:00:00Z",
  ...over,
});
const OUT = row({ id: 5, description: "Asignado a Viaje", amount: "-120.00", transfer_id: "t1" });
const IN = row({ id: 6, description: "Asignado a Viaje", amount: "120.00", transfer_id: "t1", future_expense_id: 2 });

describe("personal savings list", () => {
  it("shows an assignment as one positive row and deletes both legs from its dialog", async () => {
    api.listPersonalMovements.mockResolvedValue([IN, OUT, row({ id: 7, description: "Aporte", amount: "50.00" })]);
    api.deletePersonalMovement.mockResolvedValue(undefined);
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={qc}>
        <MovementList config={PERSONAL_PAGES.Ahorro} month="2026-10" onEdit={() => {}} />
      </QueryClientProvider>,
    );
    const assignment = await screen.findByRole("button", { name: "Ver Asignado a Viaje" });
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    expect(assignment).toHaveTextContent("Asignación");
    expect(assignment).toHaveTextContent("120.00");
    expect(assignment).not.toHaveTextContent("Retiro");
    fireEvent.click(assignment);
    expect(screen.getByRole("dialog", { name: "Asignación" })).toHaveTextContent("saldo libre");
    fireEvent.click(screen.getByRole("button", { name: /^Eliminar asignación/ }));
    fireEvent.click(screen.getByRole("button", { name: "Sí, eliminar" }));
    await waitFor(() => expect(api.deletePersonalMovement).toHaveBeenCalledWith(6));
  });
});
