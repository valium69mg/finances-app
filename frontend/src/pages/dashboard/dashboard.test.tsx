import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";
import type { Dashboard, DashboardCategory, DashboardTax, FutureExpenses, RecentMovement, UpcomingBill } from "../../api/dashboard";
import { BudgetTable } from "./BudgetTable";
import { CycleProgress, paceOf } from "./CycleProgress";
import { FutureExpensesCard } from "./FutureExpensesCard";
import { RecentMovements } from "./RecentMovements";
import { UpcomingBills, dueText } from "./UpcomingBills";
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
    cycle: { today: "2026-10-15", day: 16, days: 31 },
    future_expenses: { items: [], target: "0", saved: "0", remaining: "0", suggested_monthly: "0" },
    upcoming_bills: [],
    recent_movements: [],
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

describe("BudgetTable on a phone", () => {
  it("renders one compact card per category with percent, full-width bar, spent of budget and what is left", () => {
    render(<BudgetTable categories={[cat({ category: "Mandado", spent: "250", remaining: "750" })]} />);
    const card = screen.getByTestId("budget-card");
    expect(card).toHaveTextContent("Mandado");
    expect(card).toHaveTextContent("25%");
    expect(card).toHaveTextContent("Gastado $250.00 de $1,000.00");
    expect(card).toHaveTextContent("Restante $750.00");
    expect(within(card).getByRole("progressbar", { name: "Uso del presupuesto de Mandado" })).toHaveAttribute("aria-valuenow", "25");
  });

  it("writes Excedido with the amount over when the budget is exceeded", () => {
    render(<BudgetTable categories={[cat({ category: "Mandado", spent: "1500.50", remaining: "-500.50", over_budget: true })]} />);
    const card = screen.getByTestId("budget-card");
    expect(card).toHaveTextContent("Excedido $500.50");
    expect(card).toHaveTextContent("100%");
    expect(within(card).getByText("Excedido $500.50")).toHaveClass("text-destructive");
  });

  it("shows only the spent amount for a category without budget", () => {
    render(<BudgetTable categories={[cat({ category: "Ocio", spent: "80", budget: null, remaining: null })]} />);
    const card = screen.getByTestId("budget-card");
    expect(card).toHaveTextContent("Sin presupuesto");
    expect(card).toHaveTextContent("Gastado $80.00");
    expect(card).not.toHaveTextContent("Restante");
  });
});

describe("FutureExpensesCard", () => {
  const future: FutureExpenses = {
    items: [
      { name: "Seguro", due_date: "2026-12-15", target: "36000", saved: "9000", remaining: "27000", suggested_monthly: "13500", cycles_left: 2 },
      { name: "Predial", due_date: "2027-01-20", target: "10000", saved: "0", remaining: "10000", suggested_monthly: "2500", cycles_left: 4 },
    ],
    target: "46000",
    saved: "9000",
    remaining: "37000",
    suggested_monthly: "16000",
  };

  it("lists each expense with due date, progress and the suggested monthly amount, and a total row", () => {
    render(<FutureExpensesCard future={future} />);
    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(items[0]).toHaveTextContent("Seguro");
    expect(items[0]).toHaveTextContent("15 de diciembre de 2026");
    expect(items[0]).toHaveTextContent("$9,000.00 de $36,000.00 (25%)");
    expect(items[0]).toHaveTextContent("Aparta $13,500.00 al mes");
    expect(within(items[0]).getByRole("progressbar", { name: "Ahorro para Seguro" })).toHaveAttribute("aria-valuenow", "25");
    expect(items[1]).toHaveTextContent("20 de enero de 2027");
    expect(screen.getByText("Total").nextSibling).toHaveTextContent("$46,000.00");
    expect(screen.getByText("Sugerido al mes").nextSibling).toHaveTextContent("$16,000.00");
  });

  it("says the goal is covered when nothing is left to save", () => {
    const covered = { ...future, items: [{ ...future.items[0], saved: "36000", remaining: "0", suggested_monthly: "0" }] };
    render(<FutureExpensesCard future={covered} />);
    expect(screen.getByText("Meta cubierta.")).toBeInTheDocument();
  });

  it("explains how to add one when there are none", () => {
    render(<FutureExpensesCard future={{ items: [], target: "0", saved: "0", remaining: "0", suggested_monthly: "0" }} />);
    expect(screen.getByText(/Aún no hay gastos futuros/)).toBeInTheDocument();
  });
});

describe("RecentMovements", () => {
  const mv = (over: Partial<RecentMovement>): RecentMovement => ({
    id: 1,
    date: "2026-10-14",
    kind: "Gasto",
    description: "Super",
    category: "Mandado",
    amount_mxn: "350.5",
    ...over,
  });

  it("shows a kind badge, date, description, category and amount for every kind", () => {
    render(
      <RecentMovements
        movements={[mv({}), mv({ id: 2, kind: "Ingreso", description: "Sueldo", category: "Sueldo", amount_mxn: "60000" }), mv({ id: 3, kind: "Ahorro", description: "", category: "Inversiones", amount_mxn: "-200" })]}
      />,
    );
    const rows = screen.getAllByRole("listitem");
    expect(rows).toHaveLength(3);
    expect(rows[0]).toHaveTextContent("Gasto");
    expect(rows[0]).toHaveTextContent("14 de octubre de 2026");
    expect(rows[0]).toHaveTextContent("Super");
    expect(rows[0]).toHaveTextContent("Mandado");
    expect(rows[0]).toHaveTextContent("$350.50");
    expect(rows[1]).toHaveTextContent("Ingreso");
    expect(rows[1]).toHaveTextContent("$60,000.00");
    // Without a description the category is the title, and a withdrawal stays negative.
    expect(rows[2]).toHaveTextContent("Ahorro");
    expect(rows[2]).toHaveTextContent("Inversiones");
    expect(rows[2]).toHaveTextContent("-$200.00");
  });

  it("has an empty state", () => {
    render(<RecentMovements movements={[]} />);
    expect(screen.getByText("Aún no hay movimientos registrados.")).toBeInTheDocument();
  });
});

describe("UpcomingBills", () => {
  const bill = (over: Partial<UpcomingBill>): UpcomingBill => ({
    id: 1,
    name: "Luz",
    category: "Servicios",
    amount: "200",
    currency: "MXN",
    due_date: "2026-10-17",
    days_until_due: 2,
    overdue: false,
    ...over,
  });

  it("words the due state in text", () => {
    expect(dueText({ days_until_due: 0, overdue: false })).toBe("Vence hoy");
    expect(dueText({ days_until_due: 1, overdue: false })).toBe("Vence mañana");
    expect(dueText({ days_until_due: 9, overdue: false })).toBe("Vence en 9 días");
    expect(dueText({ days_until_due: -1, overdue: true })).toBe("Vencida hace 1 día");
  });

  it("lists the bills with amount, variable amounts and the overdue ones in red", () => {
    render(
      <MemoryRouter>
        <UpcomingBills bills={[bill({ id: 1, days_until_due: -3, overdue: true, due_date: "2026-10-12" }), bill({ id: 2, name: "Agua", amount: null })]} />
      </MemoryRouter>,
    );
    const rows = screen.getAllByRole("listitem");
    expect(rows[0]).toHaveTextContent("Vencida hace 3 días");
    expect(rows[0]).toHaveTextContent("$200.00");
    expect(rows[1]).toHaveTextContent("Monto variable");
    expect(screen.getByRole("link", { name: "Ver pagos recurrentes" })).toHaveAttribute("href", "/pagos-recurrentes");
  });

  it("has an empty state", () => {
    render(
      <MemoryRouter>
        <UpcomingBills bills={[]} />
      </MemoryRouter>,
    );
    expect(screen.getByText(/No tienes cuentas por vencer/)).toBeInTheDocument();
  });
});

describe("CycleProgress", () => {
  const base = (over: Partial<Dashboard>) => ({
    cycle: { today: "2026-10-10", day: 10, days: 30 },
    expenses: "1000",
    categories: [cat({ budget: "2000" }), cat({ category: "Mandado", budget: "1000" }), cat({ category: "Ocio", budget: null })],
    ...over,
  });

  it("compares the pace by cross-multiplying", () => {
    expect(paceOf("1000", 300000n, 10, 30)).toBe("within"); // 1000 of 3000 is 33%, exactly day 10 of 30
    expect(paceOf("1000.01", 300000n, 10, 30)).toBe("above");
    expect(paceOf("0", 0n, 10, 30)).toBeNull();
  });

  it("shows the day of the cycle and spending within the pace", () => {
    render(<CycleProgress dashboard={base({})} />);
    expect(screen.getByText("Día 10 de 30")).toBeInTheDocument();
    expect(screen.getByText("Gastado $1,000.00 de $3,000.00 (33%)")).toBeInTheDocument();
    expect(screen.getByText("Vas dentro del ritmo de gasto del ciclo.")).toBeInTheDocument();
    expect(screen.getByRole("progressbar", { name: "Avance del ciclo" })).toHaveAttribute("aria-valuenow", "33");
  });

  it("warns, in text, when spending is ahead of the cycle", () => {
    render(<CycleProgress dashboard={base({ expenses: "2500" })} />);
    expect(screen.getByText("Vas por encima del ritmo de gasto del ciclo.")).toHaveClass("text-destructive");
  });

  it("handles a cycle that has not started and a missing budget", () => {
    render(<CycleProgress dashboard={base({ cycle: { today: "2026-08-01", day: 0, days: 30 }, categories: [cat({ budget: null })] })} />);
    expect(screen.getByText("El periodo aún no empieza")).toBeInTheDocument();
    expect(screen.getByText("Sin presupuesto de gasto para comparar el ritmo.")).toBeInTheDocument();
  });
});
