import type { ReactNode } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertCircle } from "lucide-react";
import { ApiError } from "../../api/client";
import { settingsKeys } from "../../api/settings";

export const primaryButton =
  "focus-ring inline-flex min-h-11 items-center justify-center gap-2 rounded-lg bg-primary px-5 py-2 font-medium text-on-primary transition-colors duration-200 hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-60";

export const secondaryButton =
  "focus-ring inline-flex min-h-11 items-center justify-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm font-medium transition-colors duration-200 hover:bg-primary/10 disabled:cursor-not-allowed disabled:opacity-60";

/** A standalone text link ("Ver pagos recurrentes", "Ir a Configuración"): the text stays small, the hit area is 44px tall. */
export const tapLink = "focus-ring inline-flex min-h-11 items-center rounded font-medium underline underline-offset-2";

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

export function ErrorBanner({ children }: { children: ReactNode }) {
  return (
    <p role="alert" className="flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
      <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
      <span className="min-w-0 break-words">{children}</span>
    </p>
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

/** Full-width clickable row or card that opens an edit dialog. */
export const rowButton =
  "focus-ring flex min-h-11 w-full items-center gap-3 rounded-xl border border-border bg-surface px-4 py-3 text-left transition-colors duration-200 hover:bg-primary/5";

/** Heading of a settings section: title, what it is for and, optionally, its main action. */
export function SectionHeader({ title, description, action }: { title: string; description?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <h2 className="text-lg font-semibold tracking-tight">{title}</h2>
        {description && <p className="mt-1 text-sm text-muted">{description}</p>}
      </div>
      {action}
    </div>
  );
}
