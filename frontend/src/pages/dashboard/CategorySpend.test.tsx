import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";
import { CategorySpend } from "./CategorySpend";

afterEach(cleanup);

const CATS = [
  { category_id: 6, category: "Salud", spent: "900" },
  { category_id: 5, category: "Ropa", spent: "400.50" },
];

const show = (props: Partial<Parameters<typeof CategorySpend>[0]> = {}) =>
  render(
    <MemoryRouter>
      <CategorySpend categories={CATS} total="1300.50" {...props} />
    </MemoryRouter>,
  );

describe("CategorySpend", () => {
  it("lists the amount and the share of the total of each category", () => {
    show();
    const rows = screen.getAllByTestId("category-spend");
    expect(within(rows[0]).getByText("Salud")).toBeInTheDocument();
    expect(rows[0]).toHaveTextContent("$900.00");
    expect(rows[0]).toHaveTextContent("69%");
    expect(within(rows[0]).getByRole("progressbar")).toHaveAttribute("aria-valuenow", "69");
    expect(rows[1]).toHaveTextContent("$400.50");
    expect(rows[1]).toHaveTextContent("31%");
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("makes a row a link only when linkFor gives it a target", () => {
    show({ linkFor: (c) => (c.category === "Salud" ? `/mis-gastos?mes=2026-10&categoria=${c.category_id}` : undefined) });
    expect(screen.getByRole("link", { name: "Salud" })).toHaveAttribute("href", "/mis-gastos?mes=2026-10&categoria=6");
    expect(screen.queryByRole("link", { name: "Ropa" })).toBeNull();
  });

  it("shows a share of zero for a zero total instead of dividing by it", () => {
    show({ total: "0" });
    expect(screen.getAllByTestId("category-spend")[0]).toHaveTextContent("0%");
  });

  it("says there is no spending when the list is empty", () => {
    show({ categories: [], total: "0" });
    expect(screen.getByText("Aún no hay gastos en este periodo.")).toBeInTheDocument();
  });
});
