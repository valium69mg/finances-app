import { describe, expect, it } from "vitest";
import { linkWithMonth, parseCategoryNameParam, parseCategoryParam, parseIdParam, parseMonthParam } from "./monthParam";

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

describe("parseIdParam", () => {
  it("is the id rule of the deep links", () => {
    expect(parseIdParam("42")).toBe(42);
    expect(parseIdParam("0")).toBe(0);
    expect(parseIdParam("abc")).toBe(0);
    expect(parseIdParam(null)).toBe(0);
  });
});

describe("parseCategoryNameParam", () => {
  it("keeps a trimmed name", () => {
    expect(parseCategoryNameParam("Fondo de emergencia")).toBe("Fondo de emergencia");
    expect(parseCategoryNameParam("  Servicios ")).toBe("Servicios");
  });

  it.each([null, undefined, "", "   ", "x".repeat(101)])("ignores %j", (raw) => {
    expect(parseCategoryNameParam(raw)).toBe("");
  });
});

describe("linkWithMonth for the owner", () => {
  it("encodes a category name and carries the deep link ids", () => {
    expect(linkWithMonth("/ahorros", "2026-10", { categoria: "Fondo de emergencia" })).toBe("/ahorros?mes=2026-10&categoria=Fondo+de+emergencia");
    expect(linkWithMonth("/gastos", "2026-09", { editar: 7 })).toBe("/gastos?mes=2026-09&editar=7");
    expect(linkWithMonth("/pagos-recurrentes", "", { pago: 3 })).toBe("/pagos-recurrentes?pago=3");
  });
});
