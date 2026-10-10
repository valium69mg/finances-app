import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MisGastosFuturos } from "../MisGastosFuturos";
import { FutureBackendProvider, personalFutureBackend } from "./backend";
import { LAPTOP, LIST, PAID } from "./fixtures";
import { validatePay } from "./form";
import { PayDialog } from "./PayDialog";

const api = vi.hoisted(() => ({
  listPersonalFutureExpenses: vi.fn(),
  createPersonalFutureExpense: vi.fn(),
  updatePersonalFutureExpense: vi.fn(),
  deletePersonalFutureExpense: vi.fn(),
  addPersonalFutureSaving: vi.fn(),
  assignPersonalFutureSaving: vi.fn(),
  payPersonalFutureExpense: vi.fn(),
  listPersonalCategories: vi.fn(),
}));
vi.mock("../../api/personal", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/personal")>()), ...api }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const CATEGORIES = [
  { id: 4, kind: "Gasto", name: "Ropa", fixed: false },
  { id: 5, kind: "Gasto", name: "Salud", fixed: false },
];

function wrap(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <FutureBackendProvider value={personalFutureBackend}>{ui}</FutureBackendProvider>
    </QueryClientProvider>,
  );
}

describe("validatePay", () => {
  const draft = { amount: "100", date: "2026-10-10", category: "" };
  it("requires the category only when asked to", () => {
    expect(validatePay(draft, false)).toEqual({});
    expect(validatePay(draft, true).category).toBe("Elige la categoría del gasto.");
    expect(validatePay({ ...draft, category: "5" }, true)).toEqual({});
  });
});

describe("PayDialog over her ledger", () => {
  const options = [
    { value: "4", label: "Ropa" },
    { value: "5", label: "Salud" },
  ];

  it("does not offer an automatic category and blocks payment until one is chosen", async () => {
    wrap(<PayDialog item={LAPTOP} categories={options} categoryRequired onClose={vi.fn()} onPaid={vi.fn()} />);
    expect(screen.queryByRole("option", { name: "Automática" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Confirmar pago" }));
    expect(await screen.findByText("Elige la categoría del gasto.")).toBeInTheDocument();
    expect(api.payPersonalFutureExpense).not.toHaveBeenCalled();
  });

  it("sends the id of the chosen category", async () => {
    api.payPersonalFutureExpense.mockResolvedValue({ item: PAID, expense: { amount: "7900", category: "Salud" } });
    const onPaid = vi.fn();
    wrap(<PayDialog item={LAPTOP} categories={options} categoryRequired onClose={vi.fn()} onPaid={onPaid} />);
    fireEvent.change(screen.getByLabelText("Monto pagado (MXN)"), { target: { value: "7900" } });
    fireEvent.change(screen.getByLabelText("Categoría del gasto"), { target: { value: "5" } });
    fireEvent.click(screen.getByRole("button", { name: "Confirmar pago" }));
    await waitFor(() => expect(onPaid).toHaveBeenCalled());
    expect(api.payPersonalFutureExpense).toHaveBeenCalledWith(1, { amount: "7900", date: expect.any(String), category_id: 5 });
  });
});

describe("MisGastosFuturos", () => {
  it("renders her items from her endpoint and offers only her Gasto categories when paying", async () => {
    api.listPersonalFutureExpenses.mockResolvedValue(LIST);
    api.listPersonalCategories.mockResolvedValue(CATEGORIES);
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={qc}>
        <MisGastosFuturos />
      </QueryClientProvider>,
    );
    expect(await screen.findByRole("heading", { level: 1, name: "Mis gastos futuros" })).toBeInTheDocument();
    expect(await screen.findByText("Laptop")).toBeInTheDocument();
    expect(api.listPersonalCategories).toHaveBeenCalledWith("Gasto");
    fireEvent.click(screen.getByRole("button", { name: "Marcar pagado Laptop" }));
    expect(await screen.findByRole("option", { name: "Ropa" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Salud" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "Automática" })).not.toBeInTheDocument();
  });
});
