import { describe, expect, it } from "vitest";
import { linkWithMonth, parseCategoryParam, parseMonthParam } from "./monthParam";

describe("parseMonthParam", () => {
  it("accepts a YYYY-MM label", () => {
    expect(parseMonthParam("2026-10")).toBe("2026-10");
    expect(parseMonthParam("2026-01")).toBe("2026-01");
  });

  it.each([null, undefined, "", "2026-13", "2026-00", "2026-1", "26-10", "2026-10-01", "oct 2026", " 2026-10"])("ignores %j", (raw) => {
    expect(parseMonthParam(raw)).toBe("");
  });
});

describe("parseCategoryParam", () => {
  it("accepts a positive integer", () => {
    expect(parseCategoryParam("5")).toBe(5);
    expect(parseCategoryParam("120")).toBe(120);
  });

  it.each([null, undefined, "", "0", "-3", "1.5", "abc", "07", "5x", "99999999999"])("ignores %j", (raw) => {
    expect(parseCategoryParam(raw)).toBe(0);
  });
});

describe("linkWithMonth", () => {
  it("carries the month and the category", () => {
    expect(linkWithMonth("/mis-gastos", "2026-10")).toBe("/mis-gastos?mes=2026-10");
    expect(linkWithMonth("/mis-gastos", "2026-10", { categoria: 5 })).toBe("/mis-gastos?mes=2026-10&categoria=5");
  });

  it("leaves out what is empty", () => {
    expect(linkWithMonth("/peticiones", "")).toBe("/peticiones");
    expect(linkWithMonth("/mis-gastos", "", { categoria: 5 })).toBe("/mis-gastos?categoria=5");
  });
});
