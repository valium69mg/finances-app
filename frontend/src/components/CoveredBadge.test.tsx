import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { CoveredBadge, coveredText } from "./CoveredBadge";

afterEach(cleanup);

describe("coveredText", () => {
  it("is empty when savings paid nothing", () => {
    expect(coveredText(undefined, "500.00")).toBeNull();
    expect(coveredText("0", "500.00")).toBeNull();
  });
  it("is the plain badge when savings paid all of it", () => {
    expect(coveredText("500.00", "500.00")).toBe("Pagado con ahorro");
    expect(coveredText("800.00", "500.00")).toBe("Pagado con ahorro");
  });
  it("shows the covered amount when only part was paid from savings", () => {
    expect(coveredText("300.00", "500.00")).toBe("Pagado con ahorro: $300.00");
  });
});

describe("CoveredBadge", () => {
  it("renders the badge text", () => {
    render(<CoveredBadge covered="300.00" amount="500.00" />);
    expect(screen.getByText("Pagado con ahorro: $300.00")).toBeInTheDocument();
  });
  it("renders nothing without coverage", () => {
    const { container } = render(<CoveredBadge covered={undefined} amount="500.00" />);
    expect(container).toBeEmptyDOMElement();
  });
});
