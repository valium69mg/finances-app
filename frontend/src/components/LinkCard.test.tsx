import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";
import { CircleDollarSign } from "lucide-react";
import { StatCard } from "./LinkCard";

afterEach(cleanup);

const card = (to?: string) =>
  render(
    <MemoryRouter>
      <dl>
        <StatCard label="Ingresos" icon={CircleDollarSign} tone="income" to={to}>
          $100.00
        </StatCard>
      </dl>
    </MemoryRouter>,
  );

describe("StatCard", () => {
  it("is a real link named by its label when it has a target", () => {
    const { container } = card("/mis-ingresos?mes=2026-10");
    const link = screen.getByRole("link", { name: "Ingresos" });
    expect(link).toHaveAttribute("href", "/mis-ingresos?mes=2026-10");
    // Hover state and the chevron say it is interactive; the chevron is decorative.
    expect(container.firstElementChild?.firstElementChild).toHaveClass("group", "hover:shadow-sm");
    expect(container.querySelector("svg.lucide-chevron-right")).toHaveAttribute("aria-hidden", "true");
    expect(link.className).toContain("focus-visible:after:ring-2");
  });

  it("is plain text without a target: no link, no chevron, no hover state", () => {
    const { container } = card();
    expect(screen.queryByRole("link")).toBeNull();
    expect(screen.getByText("Ingresos")).toBeInTheDocument();
    expect(container.querySelector("svg.lucide-chevron-right")).toBeNull();
    expect(container.firstElementChild?.firstElementChild).not.toHaveClass("group");
  });
});
