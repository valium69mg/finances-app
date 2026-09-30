import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarCheck, Info, Loader2 } from "lucide-react";
import { Link } from "react-router-dom";
import { ApiError } from "../../api/client";
import { createMonthClose, monthCloseKeys, previewMonthClose, type MonthClose } from "../../api/monthClose";
import { ErrorBanner, primaryButton, secondaryButton } from "../settings/ui";
import { periodLabel } from "../taxfiling/labels";
import { CloseReport } from "./CloseReport";
import { ConfirmDialog } from "./ConfirmDialog";
import { describeMonthCloseError } from "./errors";
import { closedAtLabel } from "./labels";

interface Props {
  period: string;
  /** YYYY-MM of the current month: a later period cannot be closed. */
  currentMonth: string;
  onClosed: (close: MonthClose) => void;
  onOpenStored: (period: string) => void;
}

/** The close computed now for a period (never stored) and the action that stores it as a snapshot. */
export function PreviewPanel({ period, currentMonth, onClosed, onOpenStored }: Props) {
  const qc = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  const preview = useQuery({ queryKey: monthCloseKeys.preview(period), queryFn: () => previewMonthClose(period), retry: false });
  const create = useMutation({
    mutationFn: () => createMonthClose(period),
    onSuccess: async (close) => {
      setConfirming(false);
      onClosed(close);
      await qc.invalidateQueries({ queryKey: monthCloseKeys.all });
    },
    onError: (err) => {
      // Someone closed it meanwhile: refresh so the page shows the stored close instead.
      if (err instanceof ApiError && err.code === "already_closed") void qc.invalidateQueries({ queryKey: monthCloseKeys.all });
    },
  });
  const label = periodLabel(period);
  const future = period > currentMonth;

  if (preview.isPending) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted">
        <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
        Calculando cierre…
      </p>
    );
  }
  if (preview.isError) {
    return (
      <div className="space-y-3">
        <ErrorBanner>{describeMonthCloseError(preview.error)}</ErrorBanner>
        <div className="flex flex-wrap items-center gap-3">
          <button type="button" onClick={() => void preview.refetch()} className={secondaryButton}>
            Reintentar
          </button>
          {preview.error instanceof ApiError && preview.error.code === "settings_incomplete" && (
            <Link to="/configuracion" className="text-sm font-medium underline">
              Ir a Configuración
            </Link>
          )}
        </div>
      </div>
    );
  }

  const { preview: close, existing } = preview.data;
  return (
    <div className="space-y-6">
      <h2 className="text-lg font-semibold capitalize tracking-tight">Vista previa del cierre de {label}</h2>

      {existing ? (
        <div role="status" className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border border-accent/60 bg-accent/15 px-4 py-3 text-sm">
          <Info className="h-4 w-4 shrink-0" aria-hidden="true" />
          <span className="min-w-0 flex-1">
            Este mes ya está cerrado desde el {existing.closed_at ? closedAtLabel(existing.closed_at) : "—"}. Lo que ves abajo es un cálculo con tus movimientos de hoy; el
            cierre guardado no cambia. Para generarlo de nuevo, elimínalo primero.
          </span>
          <button type="button" onClick={() => onOpenStored(period)} className={secondaryButton}>
            Ver cierre guardado
          </button>
        </div>
      ) : (
        <p className="text-sm text-muted">Es una vista previa: no se guarda nada hasta que cierres el mes.</p>
      )}

      <CloseReport close={close} />

      {!existing && (
        <div className="space-y-2 border-t border-border pt-5">
          {future && <p className="text-sm text-muted">Este mes aún no ha llegado, así que no se puede cerrar.</p>}
          {!future && period === currentMonth && (
            <p className="text-sm text-muted">Este mes aún no termina: el cierre guardado tomará solo lo registrado hasta hoy.</p>
          )}
          <button
            type="button"
            disabled={future}
            onClick={() => {
              create.reset();
              setConfirming(true);
            }}
            className={primaryButton}
          >
            <CalendarCheck className="h-4 w-4" aria-hidden="true" />
            Cerrar mes
          </button>
        </div>
      )}

      {confirming && (
        <ConfirmDialog
          title={`Cerrar ${label}`}
          confirmLabel="Sí, cerrar mes"
          cancelLabel="Cancelar"
          pending={create.isPending}
          error={create.isError ? describeMonthCloseError(create.error) : null}
          onConfirm={() => create.mutate()}
          onCancel={() => setConfirming(false)}
        >
          <p>
            Se guarda un resumen de {label} con las cifras de hoy. Es una foto fija: si después editas o agregas movimientos de ese mes, el cierre no cambia.
          </p>
          <p>Si te equivocas, podrás eliminar el cierre y generarlo de nuevo.</p>
        </ConfirmDialog>
      )}
    </div>
  );
}
