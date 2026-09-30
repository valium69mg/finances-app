import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";
import type { Dashboard, DashboardCategory, DashboardTax } from "../../api/dashboard";
import { BudgetTable } from "./BudgetTable";
import { TaxCard } from "./TaxCard";
import { TotalsCards } from "./TotalsCards";

// Vitest runs without globals, so Testing Library does not unmount between tests on its own.
afterEach(cleanup);

const cat = (over: Partial<DashboardCategory>): DashboardCategory => ({
  category: "Renta",
  spent: "0",
  budget: "1000",
  remaining: "1000",
  over_budget: false,
  ...over,
});

describe("BudgetTable", () => {
  it("shows spent, budget, remaining and the usage bar", () => {
    render(<BudgetTable categories={[cat({ spent: "250", remaining: "750" })]} />);
    const row = screen.getByRole("row", { name: /Renta/ });
    expect(row).toHaveTextContent("$250.00");
    expect(row).toHaveTextContent("$1,000.00");
    expect(row).toHaveTextContent("$750.00");
    const bar = within(row).getByRole("progressbar", { name: "Uso del presupuesto de Renta" });
    expect(bar).toHaveAttribute("aria-valuenow", "25");
    expect(row).not.toHaveTextContent("Presupuesto excedido");
  });

  it("flags an exceeded budget in text, with a full bar and a negative remaining", () => {
    render(<BudgetTable categories={[cat({ category: "Mandado", spent: "1500.50", remaining: "-500.50", over_budget: true })]} />);
    const row = screen.getByRole("row", { name: /Mandado/ });
    expect(row).toHaveTextContent("Presupuesto excedido");
    expect(row).toHaveTextContent("-$500.50");
    expect(within(row).getByRole("progressbar")).toHaveAttribute("aria-valuenow", "100");
  });

  it("marks a zero budget with spending as exceeded instead of dividing by zero", () => {
    render(<BudgetTable categories={[cat({ category: "Inversiones", budget: "0", spent: "50", remaining: "-50", over_budget: true })]} />);
    expect(within(screen.getByRole("row", { name: /Inversiones/ })).getByRole("progressbar")).toHaveAttribute("aria-valuenow", "100");
  });

  it("shows no bar for a category without budget", () => {
    render(<BudgetTable categories={[cat({ category: "Ocio", spent: "80", budget: null, remaining: null })]} />);
    const row = screen.getByRole("row", { name: /Ocio/ });
    expect(row).toHaveTextContent("Sin presupuesto");
    expect(row).toHaveTextContent("$80.00");
    expect(within(row).queryByRole("progressbar")).not.toBeInTheDocument();
  });
});

describe("TotalsCards", () => {
  const dashboard = (over: Partial<Dashboard>): Dashboard => ({
    month: "2026-10",
    period_start: "2026-09-30",
    period_end: "2026-10-30",
    categories: [],
    income: "50000",
    expenses: "1700",
    savings: "5000",
    available: "43300",
    emergency: { accumulated: "0", goal: "0" },
    tax: null,
    ...over,
  });

  it("formats the four totals", () => {
    render(<TotalsCards dashboard={dashboard({})} />);
    expect(screen.getByText("Ingresos").nextSibling).toHaveTextContent("$50,000.00");
    expect(screen.getByText("Gastos").nextSibling).toHaveTextContent("$1,700.00");
    expect(screen.getByText("Ahorros").nextSibling).toHaveTextContent("$5,000.00");
    expect(screen.getByText("Disponible").nextSibling).toHaveTextContent("$43,300.00");
  });

  it("highlights a negative available amount", () => {
    render(<TotalsCards dashboard={dashboard({ available: "-300.5" })} />);
    const available = screen.getByText("Disponible").nextSibling as HTMLElement;
    expect(available).toHaveTextContent("-$300.50");
    expect(available.className).toContain("text-destructive");
  });
});

describe("TaxCard", () => {
  const tax = (over: Partial<DashboardTax> = {}): DashboardTax => ({
    rate: "0.015",
    estimated_isr: "900.30405",
    filing_status: "ninguna",
    previous_period: "2026-09",
    previous_period_pending: false,
    ...over,
  });
  const renderCard = (t: DashboardTax | null) =>
    render(
      <MemoryRouter>
        <TaxCard tax={t} />
      </MemoryRouter>,
    );

  it("shows the estimated ISR and the rate", () => {
    renderCard(tax());
    const card = screen.getByRole("region", { name: "ISR RESICO estimado" });
    expect(card).toHaveTextContent("$900.30");
    expect(card).toHaveTextContent("1.5%");
  });

  it.each([
    ["ninguna", "Sin declarar"],
    ["pendiente", "Pago pendiente"],
    ["pagada", "Pagada"],
  ] as const)("shows the filing status %s as text", (status, label) => {
    renderCard(tax({ filing_status: status }));
    expect(screen.getByText("Declaración del mes").nextSibling).toHaveTextContent(label);
  });

  it("warns about the previous period only when it is pending, with a link to the filed records", () => {
    renderCard(tax({ previous_period_pending: true }));
    const alert = screen.getByText(/La declaración o el pago de septiembre de 2026 sigue pendiente/);
    expect(alert).toBeInTheDocument();
    expect(within(alert).getByRole("link", { name: "Ver declaraciones" })).toHaveAttribute("href", "/declaraciones-presentadas");
  });

  it("shows no previous-period warning when nothing is pending", () => {
    renderCard(tax());
    expect(screen.queryByText(/sigue pendiente/)).not.toBeInTheDocument();
  });

  it("explains that the estimate is missing when the settings are incomplete", () => {
    renderCard(null);
    const card = screen.getByRole("region", { name: "ISR RESICO estimado" });
    expect(card).toHaveTextContent("Sin estimación");
    expect(card).not.toHaveTextContent("Declaración del mes");
  });
});
