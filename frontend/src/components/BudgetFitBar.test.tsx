import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { EXCEEDS, FITS, NO_BUDGET } from "../pages/expenserequests/fixtures";
import { BudgetFitBar } from "./BudgetFitBar";

afterEach(cleanup);

const widthOf = (segment: string) => (document.querySelector(`[data-segment="${segment}"]`) as HTMLElement).style.width;

describe("BudgetFitBar", () => {
  it("says what fits in plain Spanish, with the figures", () => {
    render(<BudgetFitBar check={FITS} />);
    const box = screen.getByTestId("budget-fit");
    expect(box).toHaveTextContent("Presupuesto $1,000.00 · Gastado $700.00 · Restante $300.00");
    expect(box).toHaveTextContent("Con esta petición: $900.00 de $1,000.00");
    expect(box).toHaveTextContent("Cabe en el presupuesto");
    expect(box).not.toHaveTextContent("Excede");
    expect(box).toHaveAttribute("aria-live", "polite");
  });

  it("draws the spent part and the request as separate segments inside the budget", () => {
    render(<BudgetFitBar check={FITS} />);
    expect(widthOf("spent")).toBe("70%");
    expect(widthOf("request")).toBe("20%");
    expect(widthOf("request-over")).toBe("0%");
    expect(document.querySelector('[data-segment="limit"]')).toBeNull();
  });

  it("writes the excess explicitly and marks the limit when the request does not fit", () => {
    render(<BudgetFitBar check={EXCEEDS} />);
    const box = screen.getByTestId("budget-fit");
    expect(box).toHaveTextContent("Excede el presupuesto por $150.50");
    expect(box).toHaveTextContent("Aun así puedes aprobarla");
    expect(box).toHaveTextContent("Con esta petición: $1,150.50 de $1,000.00");
    expect(box).not.toHaveTextContent("Cabe en el presupuesto");
    expect(box).toHaveTextContent("Restante $100.00");
    expect(document.querySelector('[data-segment="limit"]')).not.toBeNull();
    expect(parseFloat(widthOf("request-over"))).toBeGreaterThan(0);
  });

  it("never relies on color: the request has its own legend entry and the bar has a text alternative", () => {
    render(<BudgetFitBar check={EXCEEDS} />);
    const legend = screen.getByRole("list", { name: "Leyenda de la barra" });
    for (const text of ["Gastado", "Esta petición", "Excedente", "Límite del presupuesto"]) expect(within(legend).getByText(text)).toBeInTheDocument();
    expect(screen.getByRole("img")).toHaveAccessibleName(/excede el presupuesto por \$150\.50/);
  });

  it("shows a negative remaining in text when the category was already over", () => {
    render(<BudgetFitBar check={{ ...EXCEEDS, spent: "1100.00", remaining: "-100.00", projected_spent: "1350.50", over_by: "350.50" }} />);
    expect(screen.getByTestId("budget-fit")).toHaveTextContent("Restante -$100.00");
    expect(widthOf("spent-over")).not.toBe("0%");
  });

  it("says there is no budget instead of inventing a bar", () => {
    render(<BudgetFitBar check={NO_BUDGET} />);
    const box = screen.getByTestId("budget-fit");
    expect(box).toHaveTextContent("Sin presupuesto para esta categoría");
    expect(box).toHaveTextContent("Gastado $40.00 · Con esta petición: $290.50");
    expect(box).not.toHaveTextContent("Cabe");
    expect(screen.queryByRole("img")).toBeNull();
  });
});
