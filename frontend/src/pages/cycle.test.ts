import { describe, expect, it } from "vitest";
import { currentCycle, cycleOf, cycleRange, cycleRangeLabel, previousCycle } from "./cycle";

// Same rows as TestCycleOf / TestCycleRange in backend/internal/ledger/domain/cycle_test.go.
const OF: [number, string, string][] = [
  [31, "2026-09-29", "2026-09"],
  [31, "2026-09-30", "2026-10"],
  [31, "2026-10-30", "2026-10"],
  [31, "2026-10-31", "2026-11"],
  [31, "2026-12-31", "2027-01"],
  [31, "2027-01-30", "2027-01"],
  [31, "2027-02-27", "2027-02"],
  [31, "2027-02-28", "2027-03"],
  [31, "2028-02-28", "2028-02"],
  [31, "2028-02-29", "2028-03"],
  [15, "2026-10-14", "2026-10"],
  [15, "2026-10-15", "2026-11"],
  [1, "2026-10-01", "2026-11"],
  [0, "2026-10-31", "2026-10"],
  [0, "2026-10-01", "2026-10"],
  [0, "2028-02-29", "2028-02"],
];

const RANGE: [number, string, string, string][] = [
  [31, "2026-10", "2026-09-30", "2026-10-30"],
  [31, "2026-11", "2026-10-31", "2026-11-29"],
  [31, "2027-01", "2026-12-31", "2027-01-30"],
  [31, "2027-03", "2027-02-28", "2027-03-30"],
  [31, "2028-03", "2028-02-29", "2028-03-30"],
  [31, "2027-02", "2027-01-31", "2027-02-27"],
  [15, "2026-10", "2026-09-15", "2026-10-14"],
  [1, "2026-10", "2026-09-01", "2026-09-30"],
  [0, "2026-02", "2026-02-01", "2026-02-28"],
  [0, "2028-02", "2028-02-01", "2028-02-29"],
  [0, "2026-12", "2026-12-01", "2026-12-31"],
];

describe("cycleOf", () => {
  it.each(OF)("startDay %i: %s -> %s", (start, date, want) => {
    expect(cycleOf(date, start)).toBe(want);
  });

  it("falls back to the YYYY-MM prefix for unparseable dates", () => {
    expect(cycleOf("2026-10", 31)).toBe("2026-10");
  });
});

describe("cycleRange", () => {
  it.each(RANGE)("startDay %i: %s -> %s..%s", (start, label, from, to) => {
    expect(cycleRange(label, start)).toEqual({ from, to });
  });

  it("rejects an invalid label", () => {
    expect(cycleRange("2026-13", 31)).toBeNull();
  });

  it("agrees with cycleOf and tiles the calendar for every start day", () => {
    for (let start = 0; start <= 31; start++) {
      let label = "2025-12";
      let prevTo = "";
      for (let i = 0; i < 26; i++) {
        const r = cycleRange(label, start)!;
        expect(cycleOf(r.from, start)).toBe(label);
        expect(cycleOf(r.to, start)).toBe(label);
        if (prevTo) {
          const next = new Date(`${prevTo}T00:00:00Z`);
          next.setUTCDate(next.getUTCDate() + 1);
          expect(r.from).toBe(next.toISOString().slice(0, 10));
        }
        prevTo = r.to;
        const [y, m] = label.split("-").map(Number);
        label = new Date(Date.UTC(y, m, 1)).toISOString().slice(0, 7);
      }
    }
  });
});

describe("currentCycle and previousCycle", () => {
  it("uses the pay cycle when configured", () => {
    expect(currentCycle("2026-09-30", 31)).toBe("2026-10");
    expect(currentCycle("2026-09-30", 0)).toBe("2026-09");
  });

  it("rolls January back to December", () => {
    expect(previousCycle("2027-01")).toBe("2026-12");
    expect(previousCycle("2026-10")).toBe("2026-09");
  });
});

describe("cycleRangeLabel", () => {
  it("formats the range for display", () => {
    expect(cycleRangeLabel("2026-10", 31)).toBe("30 sep – 30 oct");
    expect(cycleRangeLabel("2026-02", 0)).toBe("1 feb – 28 feb");
    expect(cycleRangeLabel("nope", 31)).toBe("");
  });
});
