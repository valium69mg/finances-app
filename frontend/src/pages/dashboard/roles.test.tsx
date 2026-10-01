import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { BudgetDashboard } from "../../api/dashboard";
import { AuthProvider } from "../../auth/AuthContext";
import { Dashboard } from "../Dashboard";

const api = vi.hoisted(() => ({ getDashboard: vi.fn(), getBudgetDashboard: vi.fn() }));
const settingsApi = vi.hoisted(() => ({ getSettings: vi.fn() }));
vi.mock("../../api/dashboard", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/dashboard")>()), ...api }));
vi.mock("../../api/settings", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/settings")>()), ...settingsApi }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
beforeEach(() => localStorage.clear());

const REDUCED: BudgetDashboard = {
  month: "2026-10",
  period_start: "2026-10-01",
  period_end: "2026-10-31",
  categories: [
    { category: "Mandado", spent: "1500", budget: "1000", remaining: "-500", over_budget: true },
    { category: "Ocio", spent: "200", budget: null, remaining: null, over_budget: false },
  ],
};

function open(role: "owner" | "household") {
  localStorage.setItem("role", role);
  localStorage.setItem("refresh_token", "r");
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <AuthProvider>
          <Dashboard />
        </AuthProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("Dashboard for the household role", () => {
  it("renders only the budget by category from the reduced payload", async () => {
    api.getBudgetDashboard.mockResolvedValue(REDUCED);
    open("household");
    expect(await screen.findByRole("heading", { name: "Presupuesto por categoría" })).toBeInTheDocument();
    expect(screen.getAllByTestId("budget-card")).toHaveLength(2);
    expect(screen.getByRole("row", { name: /Mandado/ })).toHaveTextContent("Presupuesto excedido");

    // Nothing else of the owner's dashboard, and no link to a module this role cannot open.
    for (const name of ["Resumen del mes", "Disponible", "Ingresos", "Fondo de emergencia", "ISR", "Movimientos recientes", "Gastos futuros", "Próximos pagos"]) {
      expect(screen.queryByText(name, { exact: false }), name).toBeNull();
    }
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("asks the server for the current cycle and never reads the settings", async () => {
    api.getBudgetDashboard.mockResolvedValue(REDUCED);
    open("household");
    await screen.findByRole("heading", { name: "Presupuesto por categoría" });
    expect(api.getBudgetDashboard).toHaveBeenCalledWith("");
    expect(api.getDashboard).not.toHaveBeenCalled();
    expect(settingsApi.getSettings).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Mes")).toHaveValue("2026-10");
  });

  it("asks for the picked month", async () => {
    api.getBudgetDashboard.mockResolvedValue(REDUCED);
    open("household");
    await screen.findByRole("heading", { name: "Presupuesto por categoría" });
    fireEvent.change(screen.getByLabelText("Mes"), { target: { value: "2026-09" } });
    await waitFor(() => expect(api.getBudgetDashboard).toHaveBeenCalledWith("2026-09"));
  });

  it("shows a friendly message and a retry on a 403 instead of crashing", async () => {
    api.getBudgetDashboard.mockRejectedValue(new ApiError(403, "forbidden"));
    open("household");
    expect(await screen.findByRole("alert")).toHaveTextContent("no tiene permiso");
    expect(screen.getByRole("button", { name: "Reintentar" })).toBeInTheDocument();
  });

  it("explains an empty budget without sending the household user to Configuración", async () => {
    api.getBudgetDashboard.mockResolvedValue({ ...REDUCED, categories: [] });
    open("household");
    expect(await screen.findByText(/Aún no hay categorías con presupuesto/)).toBeInTheDocument();
    expect(screen.queryByText(/Configuración/)).toBeNull();
  });
});

describe("Dashboard for the owner", () => {
  beforeEach(() => {
    settingsApi.getSettings.mockResolvedValue({ general: { cycle_start_day: 0 } });
  });

  it("tolerates a reduced payload (the role changed since the last refresh) by showing the budget only", async () => {
    api.getDashboard.mockResolvedValue(REDUCED);
    open("owner");
    expect(await screen.findByRole("heading", { name: "Presupuesto por categoría" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Resumen del mes" })).toBeNull();
  });
});
