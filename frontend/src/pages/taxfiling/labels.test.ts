import { describe, expect, it } from "vitest";
import { periodLabel, periodTitle } from "./labels";

describe("period labels", () => {
  it("keeps the label lowercase inside a sentence and capitalizes only the first letter standalone", () => {
    expect(periodLabel("2026-09")).toBe("septiembre de 2026");
    expect(periodTitle("2026-09")).toBe("Septiembre de 2026");
  });

  it("returns an unknown value untouched", () => {
    expect(periodTitle("n/a")).toBe("N/a");
  });
});
