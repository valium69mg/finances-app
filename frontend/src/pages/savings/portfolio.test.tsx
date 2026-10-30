import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Portfolio } from "../../api/savings";
import { PortfolioPanel } from "./PortfolioPanel";

const api = vi.hoisted(() => ({ getPortfolio: vi.fn() }));
vi.mock("../../api/savings", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api/savings")>()), getPortfolio: api.getPortfolio }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const PORTFOLIO: Portfolio = {
  rows: [
    { id: "i1", name: "VOO", type: "ETF", platform: "GBM", contributed: "10000.00", value: "12000.00", value_date: "2026-09-30", unvalued: false, gain: "2000.00", gain_pct: "20.00", pct_of_total: "100.00" },
    { id: "i2", name: "CETES", type: "Renta fija", platform: "", contributed: "20000.00", value: "20000.00", value_date: null, unvalued: true, gain: "0.00", gain_pct: "0.00", pct_of_total: "0.00" },
  ],
  total_contributed: "30000.00",
  total_value: "12000.00",
  by_type: [{ type: "ETF", contributed: "10000.00", value: "12000.00" }],
  by_destination: [{ category: "Inversión ETF", balance: "10000.00" }],
  emergency: { accumulated: "20000.00", goal: "60000.00" },
};

function renderPanel() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <PortfolioPanel />
    </QueryClientProvider>,
  );
}

describe("PortfolioPanel", () => {
  it("leads with the portfolio value, the amount contributed and the emergency fund", async () => {
    api.getPortfolio.mockResolvedValue(PORTFOLIO);
    renderPanel();
    const headline = await screen.findByLabelText("Resumen del portafolio");
    expect(headline).toHaveTextContent("Valor del portafolio");
    expect(headline).toHaveTextContent("$12,000.00");
    expect(headline).toHaveTextContent("$30,000.00");
    expect(screen.getByRole("region", { name: "Fondo de emergencia" })).toHaveTextContent("33.33%");
    expect(screen.getByRole("region", { name: "Subtotales por destino" })).toHaveTextContent("Inversión ETF");
  });

  it("renders every instrument both as a card (phone) and as a table row (md up), CSS decides which one shows", async () => {
    api.getPortfolio.mockResolvedValue(PORTFOLIO);
    renderPanel();
    const cards = await screen.findAllByTestId("instrument-card");
    expect(cards).toHaveLength(2);
    expect(cards[0]).toHaveTextContent("VOO");
    expect(cards[0]).toHaveTextContent("20.00%");
    expect(cards[1]).toHaveTextContent("sin valuar");
    expect(within(cards[1]).getAllByText("—").length).toBeGreaterThan(0);
    expect(screen.getByRole("table", { name: "Portafolio por instrumento" })).toBeInTheDocument();
    expect(screen.getByRole("list", { name: "Instrumentos del portafolio" }).className).toContain("md:hidden");
    expect(screen.getByRole("table").parentElement?.className).toContain("hidden");
  });

  it("explains the empty portfolio", async () => {
    api.getPortfolio.mockResolvedValue({ ...PORTFOLIO, rows: [] });
    renderPanel();
    expect(await screen.findByText(/Aún no hay instrumentos/)).toBeInTheDocument();
  });
});
