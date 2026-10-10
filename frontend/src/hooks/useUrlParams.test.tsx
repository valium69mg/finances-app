import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { useCategoryParam, useMonthParam } from "./useUrlParams";

function setup(url: string) {
  const wrapper = ({ children }: { children: ReactNode }) => <MemoryRouter initialEntries={[url]}>{children}</MemoryRouter>;
  return renderHook(
    () => {
      const [month, setMonth] = useMonthParam();
      const [category, setCategory] = useCategoryParam();
      return { month, setMonth, category, setCategory, search: useLocation().search };
    },
    { wrapper },
  );
}

describe("url params", () => {
  it("reads the month and the category", () => {
    const { result } = setup("/mis-gastos?mes=2026-09&categoria=4");
    expect(result.current.month).toBe("2026-09");
    expect(result.current.category).toBe(4);
  });

  it("ignores malformed values", () => {
    const { result } = setup("/mis-gastos?mes=oct&categoria=abc");
    expect(result.current.month).toBe("");
    expect(result.current.category).toBe(0);
  });

  it("writes the month and the category without losing the other one", () => {
    const { result } = setup("/mis-gastos?categoria=4");
    act(() => result.current.setMonth("2026-08"));
    expect(result.current.search).toBe("?categoria=4&mes=2026-08");
    expect(result.current.month).toBe("2026-08");
    act(() => result.current.setCategory(0));
    expect(result.current.search).toBe("?mes=2026-08");
    act(() => result.current.setMonth(""));
    expect(result.current.search).toBe("");
  });
});
