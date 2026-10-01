import { describe, expect, it } from "vitest";
import { fitSegments } from "./budgetFit";

describe("fitSegments", () => {
  it("shows what is spent and the request inside the budget when it fits", () => {
    expect(fitSegments("1000", "500", "200")).toEqual({ spentWithin: 50, requestWithin: 20, spentOver: 0, requestOver: 0, budgetMarker: null });
  });

  it("fills the bar exactly when the request just fits", () => {
    const s = fitSegments("1000.00", "700.00", "300.00");
    expect(s).toMatchObject({ spentWithin: 70, requestWithin: 30, requestOver: 0, budgetMarker: null });
  });

  it("puts the excess past the budget marker, scaled to the projected total", () => {
    // budget 1000, spent 800, request 400: projected 1200, the bar is 1200 wide.
    const s = fitSegments("1000", "800", "400");
    expect(s?.spentWithin).toBeCloseTo(66.66, 1);
    expect(s?.requestWithin).toBeCloseTo(16.66, 1);
    expect(s?.requestOver).toBeCloseTo(16.66, 1);
    expect(s?.spentOver).toBe(0);
    expect(s?.budgetMarker).toBeCloseTo(83.33, 1);
  });

  it("separates what was already over the budget from the request", () => {
    // budget 1000, spent 1100, request 100: projected 1200.
    const s = fitSegments("1000", "1100", "100");
    expect(s?.spentWithin).toBeCloseTo(83.33, 1);
    expect(s?.requestWithin).toBe(0);
    expect(s?.spentOver).toBeCloseTo(8.33, 1);
    expect(s?.requestOver).toBeCloseTo(8.33, 1);
  });

  it("a cent over is already past the marker", () => {
    expect(fitSegments("1000", "700", "300.01")?.budgetMarker).not.toBeNull();
  });

  it("never splits floats: a huge amount stays exact", () => {
    const s = fitSegments("999999999999.99", "0", "999999999999.99");
    expect(s?.requestWithin).toBe(100);
  });

  it("returns null without a usable budget or with a malformed figure", () => {
    expect(fitSegments("0", "10", "10")).toBeNull();
    expect(fitSegments("abc", "10", "10")).toBeNull();
    expect(fitSegments("100", "x", "10")).toBeNull();
    expect(fitSegments("100", "10", "-5")).toBeNull();
  });
});
