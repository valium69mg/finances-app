import { useId, type ReactNode, type Ref, type SelectHTMLAttributes } from "react";
import { FieldError } from "../../components/AuthCard";

interface Props extends Omit<SelectHTMLAttributes<HTMLSelectElement>, "id"> {
  label: string;
  hint?: string;
  /** Validation message, shown under the field and announced through aria-invalid. */
  error?: string | null;
  selectRef?: Ref<HTMLSelectElement>;
  children: ReactNode;
}

const selectClass =
  "focus-ring block min-h-11 w-full rounded-lg border border-border bg-surface px-3 py-2 text-foreground transition-colors duration-200";

/** Labelled native select that matches TextField's look. */
export function SelectField({ label, hint, error, selectRef, children, ...select }: Props) {
  const id = useId();
  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;
  const describedBy = [error ? errorId : null, hint ? hintId : null].filter(Boolean).join(" ") || undefined;
  return (
    <div>
      <label htmlFor={id} className="mb-1.5 block text-sm font-medium">
        {label}
      </label>
      <select {...select} ref={selectRef} id={id} aria-invalid={error ? true : select["aria-invalid"]} aria-describedby={describedBy} className={selectClass}>
        {children}
      </select>
      {hint && (
        <p id={hintId} className="mt-1.5 text-sm text-muted">
          {hint}
        </p>
      )}
      {error && <FieldError id={errorId}>{error}</FieldError>}
    </div>
  );
}
