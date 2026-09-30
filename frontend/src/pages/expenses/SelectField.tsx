import { useId, type ReactNode, type SelectHTMLAttributes } from "react";

interface Props extends Omit<SelectHTMLAttributes<HTMLSelectElement>, "id"> {
  label: string;
  hint?: string;
  children: ReactNode;
}

const selectClass =
  "focus-ring block min-h-11 w-full rounded-lg border border-border bg-surface px-3 py-2 text-foreground transition-colors duration-200";

/** Labelled native select that matches TextField's look. */
export function SelectField({ label, hint, children, ...select }: Props) {
  const id = useId();
  const hintId = `${id}-hint`;
  return (
    <div>
      <label htmlFor={id} className="mb-1.5 block text-sm font-medium">
        {label}
      </label>
      <select {...select} id={id} aria-describedby={hint ? hintId : undefined} className={selectClass}>
        {children}
      </select>
      {hint && (
        <p id={hintId} className="mt-1.5 text-sm text-muted">
          {hint}
        </p>
      )}
    </div>
  );
}
