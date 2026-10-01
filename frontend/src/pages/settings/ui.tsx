import { useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, CheckCircle2, Loader2, Save } from "lucide-react";
import { ApiError } from "../../api/client";
import { settingsKeys } from "../../api/settings";

export const primaryButton =
  "focus-ring inline-flex min-h-11 items-center justify-center gap-2 rounded-lg bg-primary px-5 py-2 font-medium text-on-primary transition-colors duration-200 hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-60";

export const secondaryButton =
  "focus-ring inline-flex min-h-11 items-center justify-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm font-medium transition-colors duration-200 hover:bg-primary/10 disabled:cursor-not-allowed disabled:opacity-60";

export const dangerButton =
  "focus-ring inline-flex min-h-11 items-center justify-center gap-2 rounded-lg border border-destructive px-4 py-2 text-sm font-medium text-destructive transition-colors duration-200 hover:bg-destructive/10 disabled:cursor-not-allowed disabled:opacity-60";

/** Maps an API failure to Spanish copy; the backend `message` is shown for invalid_settings. */
export function describeSaveError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === "invalid_settings") return `Los datos no son válidos: ${err.message}`;
    if (err.code === "settings_incomplete") {
      return "Faltan parámetros fiscales. Completa los rangos de RESICO y los datos generales antes de continuar.";
    }
    if (err.status === 401) return "Tu sesión expiró. Inicia sesión de nuevo.";
    if (err.status === 403) return "Tu cuenta no tiene permiso para esta sección.";
    if (err.status >= 500) return "El servidor tuvo un problema. Intenta de nuevo en unos minutos.";
    return "No se pudo completar la operación. Revisa los datos e intenta de nuevo.";
  }
  return "No se pudo conectar con el servidor. Revisa tu conexión e intenta de nuevo.";
}

/** Mutation that refreshes every settings query (including month budgets) on success. */
export function useSave<T>(fn: (value: T) => Promise<void>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => qc.invalidateQueries({ queryKey: settingsKeys.all }),
  });
}

/**
 * Editable copy of server data. It resets whenever the server data changes
 * (TanStack Query's structural sharing keeps the reference stable when a
 * refetch returns identical data, so unsaved edits survive window refocus).
 */
export function useDraft<S, D>(source: S, toDraft: (s: S) => D) {
  const [draft, setDraft] = useState<D>(() => toDraft(source));
  const [prev, setPrev] = useState(source);
  if (prev !== source) {
    setPrev(source);
    setDraft(toDraft(source));
  }
  return [draft, setDraft] as const;
}

export function ErrorBanner({ children }: { children: ReactNode }) {
  return (
    <p role="alert" className="flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
      <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
      <span className="min-w-0 break-words">{children}</span>
    </p>
  );
}

/** The slice of a mutation the form frame needs (independent of the mutation's variables type). */
interface SaveState {
  isPending: boolean;
  isError: boolean;
  isSuccess: boolean;
  error: unknown;
  reset: () => void;
}

interface SectionFormProps {
  title: string;
  description?: string;
  save: SaveState;
  /** Validates and, when valid, starts the save. Returns false when the form has errors. */
  onSubmit: () => boolean;
  submitLabel?: string;
  extraActions?: ReactNode;
  /** Blocking validation message shown right above the save button (it must be visible where the user acts). */
  notice?: ReactNode;
  children: ReactNode;
}

/** Shared frame for a settings section: heading, fields, save button and feedback. */
export function SectionForm({ title, description, save, onSubmit, submitLabel = "Guardar cambios", extraActions, notice, children }: SectionFormProps) {
  const formRef = useRef<HTMLFormElement>(null);
  const [attempt, setAttempt] = useState(0);
  const headingId = useId();

  // Focus the first invalid field after a failed validation.
  useEffect(() => {
    if (attempt === 0) return;
    formRef.current?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
  }, [attempt]);

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!onSubmit()) setAttempt((n) => n + 1);
  }

  return (
    <form ref={formRef} noValidate onSubmit={handleSubmit} onChange={() => save.isSuccess && save.reset()} aria-labelledby={headingId} className="space-y-6">
      <div>
        <h2 id={headingId} className="text-lg font-semibold tracking-tight">
          {title}
        </h2>
        {description && <p className="mt-1 text-sm text-muted">{description}</p>}
      </div>
      {children}
      {notice}
      {save.isError && <ErrorBanner>{describeSaveError(save.error)}</ErrorBanner>}
      <div className="flex flex-wrap items-center gap-3">
        <button type="submit" disabled={save.isPending} aria-busy={save.isPending} className={primaryButton}>
          {save.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Save className="h-4 w-4" aria-hidden="true" />}
          {save.isPending ? "Guardando…" : submitLabel}
        </button>
        {extraActions}
        <span role="status" className="text-sm">
          {save.isSuccess && (
            <span className="flex items-center gap-1.5 text-foreground">
              <CheckCircle2 className="h-4 w-4 text-accent" aria-hidden="true" />
              Cambios guardados.
            </span>
          )}
        </span>
      </div>
    </form>
  );
}

export function Checkbox({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="flex min-h-11 cursor-pointer items-center gap-3 text-sm font-medium">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} className="focus-ring h-5 w-5 shrink-0 accent-primary" />
      {label}
    </label>
  );
}

export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <div className={`rounded-xl border border-border bg-surface p-4 sm:p-5 ${className}`}>{children}</div>;
}

export const fieldGrid = "grid gap-4 sm:grid-cols-2";
