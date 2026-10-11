import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { parseIdParam } from "../lib/monthParam";

interface Options<T extends { id: number }> {
  /** URL parameter that carries the record id (`editar`, `pago`). */
  param: string;
  /** The records of the loaded list; undefined while it loads or when it failed. */
  items: T[] | undefined;
  /** The record the page currently has open (its edit form or dialog), or null. */
  open: T | null;
  /** Opens the record. Called once per link, after the list loaded. */
  onOpen: (item: T) => void;
  /** A record found by id may still not be openable (e.g. a transfer cannot be edited). Defaults to true. */
  canOpen?: (item: T) => boolean;
}

/**
 * Deep link to one record of a page: `?<param>=<id>` opens that record's edit form or dialog once the list is loaded.
 * Closing it (or opening another one) removes the parameter; every other parameter stays. An id that is not in the
 * list leaves the page as it is and reports `missing`, so the page can say so. Each link is handled once.
 */
export function useDeepLink<T extends { id: number }>({ param, items, open, onOpen, canOpen }: Options<T>): { missing: boolean } {
  const [params, setParams] = useSearchParams();
  const id = parseIdParam(params.get(param));
  const handled = useRef(0);
  const shown = useRef(false);
  const [missing, setMissing] = useState(false);

  const drop = () =>
    setParams(
      (prev) => {
        const out = new URLSearchParams(prev);
        out.delete(param);
        return out;
      },
      { replace: true },
    );

  useEffect(() => {
    if (id === 0) {
      handled.current = 0;
      shown.current = false;
      return;
    }
    if (!items || handled.current === id) return;
    handled.current = id;
    const item = items.find((i) => i.id === id);
    if (item && (canOpen?.(item) ?? true)) {
      setMissing(false);
      onOpen(item);
      return;
    }
    // Not in the loaded list: show the page normally and forget the link.
    setMissing(true);
    drop();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, items]);

  // Once the record was opened from the link, closing it (or switching to another) drops the parameter.
  useEffect(() => {
    if (id === 0 || handled.current !== id || missing) return;
    if (open?.id === id) shown.current = true;
    else if (shown.current) {
      shown.current = false;
      drop();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, id]);

  return { missing };
}
