import { useId, type Ref } from "react";
import { FieldError } from "../../components/AuthCard";

interface Props {
  label: string;
  /** Extension filter for the picker, e.g. ".xml". */
  accept: string;
  onChange: (file: File | null) => void;
  error?: string | null;
  hint?: string;
  disabled?: boolean;
  inputRef?: Ref<HTMLInputElement>;
}

const inputClass =
  "focus-ring block min-h-11 w-full min-w-0 rounded-lg border border-border bg-surface px-3 py-2 text-sm text-foreground file:mr-3 file:rounded-md file:border-0 file:bg-primary file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-on-primary aria-[invalid=true]:border-destructive disabled:cursor-not-allowed disabled:opacity-60";

/** Labelled file picker that matches TextField's look and error handling. */
export function FileField({ label, accept, onChange, error, hint, disabled, inputRef }: Props) {
  const id = useId();
  const errorId = `${id}-error`;
  const hintId = `${id}-hint`;
  const describedBy = [error ? errorId : null, hint ? hintId : null].filter(Boolean).join(" ") || undefined;
  return (
    <div>
      <label htmlFor={id} className="mb-1.5 block text-sm font-medium">
        {label}
      </label>
      <input
        id={id}
        ref={inputRef}
        type="file"
        accept={accept}
        disabled={disabled}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy}
        onChange={(e) => onChange(e.target.files?.[0] ?? null)}
        className={inputClass}
      />
      {hint && (
        <p id={hintId} className="mt-1.5 text-sm text-muted">
          {hint}
        </p>
      )}
      {error && <FieldError id={errorId}>{error}</FieldError>}
    </div>
  );
}
