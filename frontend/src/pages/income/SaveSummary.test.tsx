import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { SaveIncomeResult } from "../../api/income";
import { SaveSummary } from "./SaveSummary";

afterEach(cleanup);

const income = {
  id: 1,
  date: "2026-10-01",
  description: "Sueldo",
  category: "Sueldo",
  payment_method: "Transferencia",
  currency: "MXN",
  amount: "1000",
  exchange_rate: null,
  amount_mxn: "1000",
};

describe("SaveSummary", () => {
  it("shows the month total when the summary is available", () => {
    const result: SaveIncomeResult = {
      income,
      summary: { month: "2026-10", month_total_mxn: "1000", resico: null },
      split: null,
    };
    render(<SaveSummary result={result} />);
    expect(screen.getByText(/Total del mes \(2026-10\)/)).toBeInTheDocument();
  });

  it("still confirms the save when the summary could not be computed", () => {
    render(<SaveSummary result={{ income, summary: null, split: null }} />);
    expect(screen.getByText("Ingreso guardado.")).toBeInTheDocument();
    expect(screen.getByText(/no se pudo calcular el resumen del mes/)).toBeInTheDocument();
    expect(screen.queryByText(/Total del mes/)).not.toBeInTheDocument();
  });
});
