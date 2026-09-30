import { useEffect, useId, useRef, type KeyboardEvent, type ReactNode } from "react";
import { X } from "lucide-react";
import { useFocusOnEdit } from "../hooks/useFocusOnEdit";

interface Props {
  title: string;
  onClose: () => void;
  children: ReactNode;
  /** Flash the green "edit here" cue on the body (for data-entry dialogs, not confirmations). */
  highlight?: boolean;
}

const FOCUSABLE = 'button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled])';

/**
 * Accessible modal dialog: labelled by its title, closes with Escape or the
 * backdrop, keeps Tab inside, locks the page scroll and gives the focus back to
 * the element that opened it.
 */
export function Modal({ title, onClose, children, highlight = false }: Props) {
  // The dialog is already in view and Modal focuses its first field, so only the cue is applied.
  const bodyRef = useFocusOnEdit<HTMLDivElement>(highlight, { scroll: false, focus: false, inset: true });
  const titleId = useId();
  const ref = useRef<HTMLDivElement>(null);
  const opener = useRef<HTMLElement | null>(null);

  useEffect(() => {
    opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const first = ref.current?.querySelector<HTMLElement>('input:not([disabled]), select:not([disabled])') ?? ref.current?.querySelector<HTMLElement>(FOCUSABLE);
    first?.focus();
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
      opener.current?.focus();
    };
  }, []);

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (e.key === "Escape") {
      e.stopPropagation();
      onClose();
      return;
    }
    if (e.key !== "Tab") return;
    const focusable = ref.current?.querySelectorAll<HTMLElement>(FOCUSABLE);
    if (!focusable || focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }

  return (
    <div ref={ref} role="dialog" aria-modal="true" aria-labelledby={titleId} onKeyDown={onKeyDown} className="fixed inset-0 z-40 flex items-end justify-center sm:items-center sm:p-4">
      <div className="absolute inset-0 animate-fade-in bg-foreground/50" onClick={onClose} aria-hidden="true" />
      <div className="relative flex max-h-[92vh] w-full max-w-lg flex-col overflow-hidden rounded-t-xl border border-border bg-surface shadow-xl sm:rounded-xl">
        <div className="flex shrink-0 items-center justify-between gap-3 border-b border-border py-2 pl-5 pr-2">
          <h2 id={titleId} className="min-w-0 break-words text-lg font-semibold tracking-tight">
            {title}
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Cerrar"
            className="focus-ring flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-muted transition-colors duration-200 hover:bg-primary/10 hover:text-foreground"
          >
            <X className="h-6 w-6" aria-hidden="true" />
          </button>
        </div>
        <div ref={bodyRef} className="min-h-0 flex-1 overflow-y-auto p-5 pb-[calc(1.25rem+env(safe-area-inset-bottom))]">{children}</div>
      </div>
    </div>
  );
}
