import { useRef, type KeyboardEvent } from "react";

export interface TabItem<T extends string = string> {
  id: T;
  label: string;
}

interface Props<T extends string> {
  items: readonly TabItem<T>[];
  active: T;
  onSelect: (id: T) => void;
  /** Accessible name of the tab list. */
  label: string;
  /** Prefix of the tab and panel ids, unique on the page: `tab-<prefix>-<id>` and `panel-<prefix>-<id>`. */
  idPrefix: string;
}

export const tabId = (prefix: string, id: string) => `tab-${prefix}-${id}`;
export const panelId = (prefix: string, id: string) => `panel-${prefix}-${id}`;

/**
 * Horizontal tab list. Left and Right (wrapping), Home and End move to a tab and select it (roving tabindex); each tab
 * is at least 44px tall. The caller renders the panels with `role="tabpanel"`, `id={panelId(...)}` and
 * `aria-labelledby={tabId(...)}`.
 */
export function Tabs<T extends string>({ items, active, onSelect, label, idPrefix }: Props<T>) {
  const refs = useRef<Record<string, HTMLButtonElement | null>>({});

  function onKeyDown(e: KeyboardEvent<HTMLButtonElement>, index: number) {
    const last = items.length - 1;
    const target =
      e.key === "ArrowLeft" ? (index === 0 ? last : index - 1) : e.key === "ArrowRight" ? (index === last ? 0 : index + 1) : e.key === "Home" ? 0 : e.key === "End" ? last : null;
    if (target === null) return;
    e.preventDefault();
    onSelect(items[target].id);
    refs.current[items[target].id]?.focus();
  }

  return (
    <div role="tablist" aria-label={label} className="grid auto-cols-fr grid-flow-col gap-1 rounded-xl bg-border/40 p-1">
      {items.map((item, i) => {
        const selected = item.id === active;
        return (
          <button
            key={item.id}
            ref={(el) => {
              refs.current[item.id] = el;
            }}
            type="button"
            role="tab"
            id={tabId(idPrefix, item.id)}
            aria-selected={selected}
            aria-controls={panelId(idPrefix, item.id)}
            tabIndex={selected ? 0 : -1}
            onClick={() => onSelect(item.id)}
            onKeyDown={(e) => onKeyDown(e, i)}
            className={`focus-ring min-h-11 rounded-lg px-3 text-sm font-medium transition-colors duration-200 ${
              selected ? "bg-surface text-foreground shadow-sm" : "text-muted hover:text-foreground"
            }`}
          >
            {item.label}
          </button>
        );
      })}
    </div>
  );
}
