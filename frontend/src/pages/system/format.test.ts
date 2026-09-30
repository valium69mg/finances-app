import { describe, expect, it } from "vitest";
import { clampPercent, formatAge, formatGB, formatLoad, formatPercent, formatUptime, usageLevel } from "./format";

describe("usageLevel", () => {
  it("switches at 60 and 80", () => {
    expect(usageLevel(0)).toBe("normal");
    expect(usageLevel(59.9)).toBe("normal");
    expect(usageLevel(60)).toBe("warning");
    expect(usageLevel(79.9)).toBe("warning");
    expect(usageLevel(80)).toBe("danger");
    expect(usageLevel(100)).toBe("danger");
  });
});

describe("formatGB", () => {
  it("converts bytes to binary GB with one decimal, es-MX", () => {
    expect(formatGB(0)).toBe("0.0 GB");
    expect(formatGB(1024 ** 3)).toBe("1.0 GB");
    expect(formatGB(2005412 * 1024)).toBe("1.9 GB");
    expect(formatGB(50 * 1024 ** 3)).toBe("50.0 GB");
    expect(formatGB(1234.56 * 1024 ** 3)).toBe("1,234.6 GB");
  });
});

describe("formatPercent and formatLoad", () => {
  it("uses fixed decimals", () => {
    expect(formatPercent(23.5)).toBe("23.5%");
    expect(formatPercent(100)).toBe("100.0%");
    expect(formatLoad(0.5)).toBe("0.50");
    expect(formatLoad(12.345)).toBe("12.35");
  });
});

describe("clampPercent", () => {
  it("keeps bars inside 0..100 and ignores garbage", () => {
    expect(clampPercent(-3)).toBe(0);
    expect(clampPercent(140)).toBe(100);
    expect(clampPercent(42.5)).toBe(42.5);
    expect(clampPercent(Number.NaN)).toBe(0);
  });
});

describe("formatUptime", () => {
  it("shows days and hours, hours and minutes, or minutes", () => {
    expect(formatUptime(90061)).toBe("1 día 1 h");
    expect(formatUptime(1234567)).toBe("14 días 6 h");
    expect(formatUptime(3 * 3600 + 120)).toBe("3 h 2 min");
    expect(formatUptime(59)).toBe("0 min");
    expect(formatUptime(600)).toBe("10 min");
    expect(formatUptime(-5)).toBe("0 min");
  });
});

describe("formatAge", () => {
  it("counts seconds, then minutes", () => {
    expect(formatAge(0)).toBe("Actualizado hace 0 s");
    expect(formatAge(9.7)).toBe("Actualizado hace 9 s");
    expect(formatAge(59)).toBe("Actualizado hace 59 s");
    expect(formatAge(125)).toBe("Actualizado hace 2 min");
    expect(formatAge(-2)).toBe("Actualizado hace 0 s");
  });
});
