import { X } from "lucide-react";

/** An active list filter shown as a chip with a button that clears it (`Categoría: Salud`). */
export function FilterChip({ label, clearLabel, onClear }: { label: string; clearLabel: string; onClear: () => void }) {
  return (
    <p className="mb-4">
      <span className="inline-flex max-w-full items-center gap-1 rounded-full border border-border bg-background py-1 pl-3 pr-1 text-sm">
        <span className="min-w-0 break-words">{label}</span>
        <button
          type="button"
          onClick={onClear}
          aria-label={clearLabel}
          className="focus-ring inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-muted transition-colors duration-200 hover:bg-border hover:text-foreground"
        >
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      </span>
    </p>
  );
}
