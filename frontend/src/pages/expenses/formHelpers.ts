import { useEffect, useState } from "react";

/** Shared by the expense and income forms. */
export const DEBOUNCE_MS = 400;

export function useDebounced<T>(value: T, ms: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return debounced;
}

/** Keeps a current value selectable even when it is not in the configured list. */
export function withValue(list: string[], value: string): string[] {
  return value && !list.includes(value) ? [...list, value] : list;
}
