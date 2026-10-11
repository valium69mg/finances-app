import { useCallback } from "react";
import { useSearchParams } from "react-router-dom";
import { CATEGORY_PARAM, MONTH_PARAM, parseCategoryNameParam, parseCategoryParam, parseMonthParam } from "../lib/monthParam";

/**
 * The month of a page, kept in `?mes=YYYY-MM`. "" means none (or a malformed value, which is ignored) and lets the
 * page ask for the current cycle. Changing it replaces the history entry and keeps every other parameter.
 */
export function useMonthParam(): [string, (month: string) => void] {
  const [params, setParams] = useSearchParams();
  const month = parseMonthParam(params.get(MONTH_PARAM));
  const setMonth = useCallback(
    (next: string) =>
      setParams(
        (prev) => {
          const out = new URLSearchParams(prev);
          if (next) out.set(MONTH_PARAM, next);
          else out.delete(MONTH_PARAM);
          return out;
        },
        { replace: true },
      ),
    [setParams],
  );
  return [month, setMonth];
}

/** The category filter of a page, kept in `?categoria=<id>`. 0 means none (or a malformed value). */
export function useCategoryParam(): [number, (id: number) => void] {
  const [params, setParams] = useSearchParams();
  const id = parseCategoryParam(params.get(CATEGORY_PARAM));
  const setId = useCallback(
    (next: number) =>
      setParams(
        (prev) => {
          const out = new URLSearchParams(prev);
          if (next > 0) out.set(CATEGORY_PARAM, String(next));
          else out.delete(CATEGORY_PARAM);
          return out;
        },
        { replace: true },
      ),
    [setParams],
  );
  return [id, setId];
}

/**
 * The owner's category filter of a page, kept in `?categoria=<name>` (owner categories are names from the settings).
 * "" means none. Whether the name is a real category is up to the page, which ignores an unknown one.
 */
export function useCategoryNameParam(): [string, (name: string) => void] {
  const [params, setParams] = useSearchParams();
  const name = parseCategoryNameParam(params.get(CATEGORY_PARAM));
  const setName = useCallback(
    (next: string) =>
      setParams(
        (prev) => {
          const out = new URLSearchParams(prev);
          if (next) out.set(CATEGORY_PARAM, next);
          else out.delete(CATEGORY_PARAM);
          return out;
        },
        { replace: true },
      ),
    [setParams],
  );
  return [name, setName];
}
