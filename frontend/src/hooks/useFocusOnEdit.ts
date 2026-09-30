import { useEffect, useRef } from "react";

/** Class that paints the temporary green highlight (defined in index.css). */
export const EDIT_HIGHLIGHT_CLASS = "edit-highlight";
/** Same highlight drawn inside the element, for containers that clip overflow (modals). */
export const EDIT_HIGHLIGHT_INSET_CLASS = "edit-highlight-inset";
export const EDIT_HIGHLIGHT_MS = 2000;

const FIRST_FIELD = "input:not([disabled]):not([type=hidden]), select:not([disabled]), textarea:not([disabled])";

interface Options {
  /** Scroll the element into view. Turn it off when the element is already in view (modals). */
  scroll?: boolean;
  /** Move focus to the first field. Turn it off when something else already does it. */
  focus?: boolean;
  /** Draw the highlight inside the element instead of around it. */
  inset?: boolean;
  /** Vertical alignment used when scrolling. */
  block?: ScrollLogicalPosition;
  /** Re-run the cue while staying active when this value changes (e.g. another row selected). */
  trigger?: unknown;
}

/**
 * Shared "you are editing here" cue. While `active` turns true the returned ref's element is
 * scrolled into view (instantly when the user prefers reduced motion), its first field gets the
 * focus and a green highlight shows for about two seconds.
 *
 * The class is toggled on the element directly so the cue never re-renders the form.
 */
export function useFocusOnEdit<T extends HTMLElement = HTMLElement>(active: boolean, { scroll = true, focus = true, inset = false, block = "start", trigger }: Options = {}) {
  const ref = useRef<T>(null);

  useEffect(() => {
    const el = ref.current;
    if (!active || !el) return;
    const classes = inset ? [EDIT_HIGHLIGHT_CLASS, EDIT_HIGHLIGHT_INSET_CLASS] : [EDIT_HIGHLIGHT_CLASS];
    el.classList.add(...classes);
    if (scroll) {
      const reduce = typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
      el.scrollIntoView?.({ behavior: reduce ? "auto" : "smooth", block });
    }
    if (focus) el.querySelector<HTMLElement>(FIRST_FIELD)?.focus({ preventScroll: true });
    const timer = window.setTimeout(() => el.classList.remove(...classes), EDIT_HIGHLIGHT_MS);
    return () => {
      window.clearTimeout(timer);
      el.classList.remove(...classes);
    };
  }, [active, scroll, focus, inset, block, trigger]);

  return ref;
}
