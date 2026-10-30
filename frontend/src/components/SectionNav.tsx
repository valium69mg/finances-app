import { useRef, type KeyboardEvent, type ReactNode } from "react";
import { SelectField } from "../pages/expenses/SelectField";

export interface SectionItem<T extends string = string> {
  id: T;
  label: string;
}

interface Props<T extends string> {
  items: readonly SectionItem<T>[];
  active: T;
  onSelect: (id: T) => void;
  /** Accessible name of the vertical tab list (lg and up). */
  label: string;
  /** Label of the native select that replaces the tab list on small screens. */
  selectLabel?: string;
  children: ReactNode;
}

/**
 * Section navigation shared by Configuración and Hogar: a sticky vertical tab list from lg up, a native select
 * ("Sección") below it, and the active section's panel beside/under it. The caller owns the active id (usually the
 * `?seccion=` param). Arrow keys, Home and End move between tabs (roving tabindex).
 */
export function SectionNav<T extends string>({ items, active, onSelect, label, selectLabel = "Sección", children }: Props<T>) {
  const tabRefs = useRef<Record<string, HTMLButtonElement | null>>({});
  const current = items.find((s) => s.id === active) ?? items[0];

  function onTabKeyDown(e: KeyboardEvent<HTMLButtonElement>, index: number) {
    const last = items.length - 1;
    const prev = e.key === "ArrowUp" || e.key === "ArrowLeft";
    const next = e.key === "ArrowDown" || e.key === "ArrowRight";
    const target = prev ? (index === 0 ? last : index - 1) : next ? (index === last ? 0 : index + 1) : e.key === "Home" ? 0 : e.key === "End" ? last : null;
    if (target === null) return;
    e.preventDefault();
    onSelect(items[target].id);
    tabRefs.current[items[target].id]?.focus();
  }

  return (
    <div className="lg:grid lg:grid-cols-[15rem_minmax(0,1fr)] lg:gap-8">
      <div className="lg:hidden">
        <SelectField
          label={selectLabel}
          value={current.id}
          onChange={(e) => {
            const picked = items.find((s) => s.id === e.target.value);
            if (picked) onSelect(picked.id);
          }}
        >
          {items.map((s) => (
            <option key={s.id} value={s.id}>
              {s.label}
            </option>
          ))}
        </SelectField>
      </div>

      <div role="tablist" aria-orientation="vertical" aria-label={label} className="sticky top-4 hidden self-start lg:flex lg:flex-col lg:gap-1">
        {items.map((s, i) => {
          const selected = s.id === current.id;
          return (
            <button
              key={s.id}
              ref={(el) => {
                tabRefs.current[s.id] = el;
              }}
              type="button"
              role="tab"
              id={`tab-${s.id}`}
              aria-selected={selected}
              aria-controls={`panel-${s.id}`}
              tabIndex={selected ? 0 : -1}
              onClick={() => onSelect(s.id)}
              onKeyDown={(e) => onTabKeyDown(e, i)}
              className={`focus-ring min-h-11 rounded-lg border-l-4 px-4 py-2 text-left text-sm transition-colors duration-200 ${
                selected ? "border-primary bg-primary/10 font-semibold text-primary" : "border-transparent text-muted hover:bg-primary/5 hover:text-foreground"
              }`}
            >
              {s.label}
            </button>
          );
        })}
      </div>

      <div role="tabpanel" id={`panel-${current.id}`} aria-labelledby={`tab-${current.id}`} tabIndex={-1} className="mt-6 min-w-0 lg:col-start-2 lg:row-start-1 lg:mt-0">
        {children}
      </div>
    </div>
  );
}
