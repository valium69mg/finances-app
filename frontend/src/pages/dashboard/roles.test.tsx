import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import type { BudgetDashboard } from "../../api/dashboard";
import type { PersonalDashboard } from "../../api/personal";
import { AuthProvider } from "../../auth/AuthContext";
import { Dashboard } from "../Dashboard";

const api = vi.hoisted(() => ({ getDashboard: vi.fn(), getBudgetDashboard: vi.fn() }));
const settingsApi = vi.hoisted(() => ({ getSettings: vi.fn() }));
const personalApi = vi.hoisted(() => ({ getPersonalDashboard: vi.fn() }));
vi.mock("../../api/dashboard", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/dashboard")>()), ...api }));
vi.mock("../../api/personal", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/personal")>()), ...personalApi }));
vi.mock("../../api/settings", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/settings")>()), ...settingsApi }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
beforeEach(() => {
  localStorage.clear();
  personalApi.getPersonalDashboard.mockResolvedValue(MINE);
});

const REDUCED: BudgetDashboard = {
  month: "2026-10",
  period_start: "2026-10-01",
  period_end: "2026-10-31",
  categories: [
    { category: "Mandado", spent: "1500", budget: "1000", remaining: "-500", over_budget: true },
    { category: "Ocio", spent: "200", budget: null, remaining: null, over_budget: false },
  ],
};

const MINE: PersonalDashboard = {
  month: "2026-10",
  period_start: "2026-09-15",
  period_end: "2026-10-14",
  income: "10000",
  expenses: "1300.50",
  savings: "1800",
  available: "6899.50",
  categories: [
    { category_id: 6, category: "Salud", spent: "900" },
    { category_id: 5, category: "Ropa", spent: "400.50" },
  ],
};

function open(role: "owner" | "household", url = "/") {
  localStorage.setItem("role", role);
  localStorage.setItem("refresh_token", "r");
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>
        <AuthProvider>
          <Dashboard />
        </AuthProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("Dashboard for the household role", () => {
  it("renders her summary and the family budget by category", async () => {
    api.getBudgetDashboard.mockResolvedValue(REDUCED);
    open("household");
    expect(await screen.findByRole("heading", { name: "Presupuesto por categoría" })).toBeInTheDocument();
    expect(screen.getAllByTestId("budget-card")).toHaveLength(2);
    expect(screen.getByRole("row", { name: /Mandado/ })).toHaveTextContent("Presupuesto excedido");

    expect(await screen.findByText("$10,000.00")).toBeInTheDocument();
    expect(screen.getByText("$1,300.50")).toBeInTheDocument();
    expect(screen.getByText("$1,800.00")).toBeInTheDocument();
    expect(screen.getByText("$6,899.50")).toBeInTheDocument();
    const rows = screen.getAllByTestId("category-spend");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("Salud");
    expect(rows[0]).toHaveTextContent("69%");
    expect(rows[1]).toHaveTextContent("Ropa");
    expect(rows[1]).toHaveTextContent("31%");

    // Nothing of the owner's dashboard.
    for (const name of ["Resumen del mes", "Fondo de emergencia", "ISR", "Movimientos recientes", "Próximos pagos"]) {
      expect(screen.queryByText(name, { exact: false }), name).toBeNull();
    }
  });

  it("links every card and row to a page her role can open, carrying the month", async () => {
    api.getBudgetDashboard.mockResolvedValue(REDUCED);
    open("household");
    await screen.findByText("$10,000.00");
    // The budget has a card layout and a table layout in the DOM (CSS hides one), so a row may have two links.
    const href = (name: string | RegExp) => {
      const hrefs = new Set(screen.getAllByRole("link", { name }).map((a) => a.getAttribute("href")));
      expect(hrefs.size, String(name)).toBe(1);
      return [...hrefs][0];
    };
    expect(href("Ingresos")).toBe("/mis-ingresos?mes=2026-10");
    expect(href("Gastos")).toBe("/mis-gastos?mes=2026-10");
    expect(href("Ahorros")).toBe("/mis-ahorros?mes=2026-10");
    expect(href("Salud")).toBe("/mis-gastos?mes=2026-10&categoria=6");
    expect(href("Ropa")).toBe("/mis-gastos?mes=2026-10&categoria=5");
    expect(href("Mandado")).toBe("/peticiones");
    expect(href("Ocio")).toBe("/peticiones");
    // Disponible only informs.
    expect(screen.queryByRole("link", { name: "Disponible" })).toBeNull();
    expect(screen.getByText("Disponible")).toBeInTheDocument();
  });

  it("asks the server for the current cycle and never reads the settings", async () => {
    api.getBudgetDashboard.mockResolvedValue(REDUCED);
    open("household");
    await screen.findByRole("heading", { name: "Presupuesto por categoría" });
    expect(api.getBudgetDashboard).toHaveBeenCalledWith("");
    expect(personalApi.getPersonalDashboard).toHaveBeenCalledWith("");
    expect(api.getDashboard).not.toHaveBeenCalled();
    expect(settingsApi.getSettings).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Mes")).toHaveValue("2026-10");
    // The range comes from her own payload.
    expect(await screen.findByText("15 sep – 14 oct")).toBeInTheDocument();
  });

  it("opens on the month of the URL and keeps the picked one there", async () => {
    api.getBudgetDashboard.mockResolvedValue(REDUCED);
    open("household", "/?mes=2026-08");
    await screen.findByRole("heading", { name: "Presupuesto por categoría" });
    expect(api.getBudgetDashboard).toHaveBeenCalledWith("2026-08");
    expect(personalApi.getPersonalDashboard).toHaveBeenCalledWith("2026-08");
    fireEvent.change(screen.getByLabelText("Mes"), { target: { value: "2026-09" } });
    await waitFor(() => expect(personalApi.getPersonalDashboard).toHaveBeenCalledWith("2026-09"));
    expect(api.getBudgetDashboard).toHaveBeenCalledWith("2026-09");
  });

  it("ignores a malformed month in the URL", async () => {
    api.getBudgetDashboard.mockResolvedValue(REDUCED);
    open("household", "/?mes=octubre");
    await screen.findByRole("heading", { name: "Presupuesto por categoría" });
    expect(api.getBudgetDashboard).toHaveBeenCalledWith("");
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
    const alerts = await screen.findAllByRole("alert");
    expect(alerts[0]).toHaveTextContent("no tiene permiso");
    expect(screen.getAllByRole("button", { name: "Reintentar" }).length).toBeGreaterThan(0);
  });

  it("keeps the budget when her summary fails, and the summary when the budget fails", async () => {
    api.getBudgetDashboard.mockResolvedValue(REDUCED);
    personalApi.getPersonalDashboard.mockRejectedValue(new ApiError(500, "internal_error"));
    open("household");
    expect(await screen.findByRole("heading", { name: "Presupuesto por categoría" })).toBeInTheDocument();
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    cleanup();

    api.getBudgetDashboard.mockRejectedValue(new ApiError(500, "internal_error"));
    personalApi.getPersonalDashboard.mockResolvedValue(MINE);
    open("household");
    expect(await screen.findByText("$10,000.00")).toBeInTheDocument();
    expect(await screen.findByRole("alert")).toBeInTheDocument();
  });

  it("explains an empty budget without sending the household user to Configuración", async () => {
    api.getBudgetDashboard.mockResolvedValue({ ...REDUCED, categories: [] });
    open("household");
    expect(await screen.findByText(/Aún no hay categorías con presupuesto/)).toBeInTheDocument();
    expect(screen.queryByText(/Configuración/)).toBeNull();
  });

  it("says there is no spending yet when no category has any", async () => {
    api.getBudgetDashboard.mockResolvedValue(REDUCED);
    personalApi.getPersonalDashboard.mockResolvedValue({ ...MINE, expenses: "0", categories: [] });
    open("household");
    expect(await screen.findByText("Aún no hay gastos en este periodo.")).toBeInTheDocument();
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
