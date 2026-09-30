import { useState } from "react";
import { useFocusOnEdit } from "../../hooks/useFocusOnEdit";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Trash2, X } from "lucide-react";
import { deleteMonthClose, monthCloseKeys, type MonthClose } from "../../api/monthClose";
import { dangerButton, secondaryButton } from "../settings/ui";
import { periodLabel } from "../taxfiling/labels";
import { CloseReport } from "./CloseReport";
import { ConfirmDialog } from "./ConfirmDialog";
import { describeMonthCloseError } from "./errors";
import { closedAtLabel } from "./labels";

interface Props {
  close: MonthClose;
  onClose: () => void;
  onDeleted: (period: string) => void;
}

/** One stored close: its frozen figures and the action to discard it so the month can be closed again. */
export function CloseDetail({ close, onClose, onDeleted }: Props) {
  const sectionRef = useFocusOnEdit<HTMLElement>(true, { focus: false });
  const qc = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  const remove = useMutation({
    mutationFn: () => deleteMonthClose(close.period),
    onSuccess: async () => {
      setConfirming(false);
      onDeleted(close.period);
      await qc.invalidateQueries({ queryKey: monthCloseKeys.all });
    },
  });
  const label = periodLabel(close.period);

  return (
    <section ref={sectionRef} aria-label={`Detalle del cierre de ${label}`} className="space-y-5 border-t border-border pt-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-lg font-semibold capitalize tracking-tight">Cierre de {label}</h3>
          <p className="mt-0.5 text-sm text-muted">
            Guardado el {close.closed_at ? closedAtLabel(close.closed_at) : "—"}. Estas cifras no cambian aunque edites tus movimientos después.
          </p>
        </div>
        <button type="button" onClick={onClose} className={secondaryButton}>
          <X className="h-4 w-4" aria-hidden="true" />
          Cerrar detalle
        </button>
      </div>

      <CloseReport close={close} />

      <div className="border-t border-border pt-5">
        <button
          type="button"
          onClick={() => {
            remove.reset();
            setConfirming(true);
          }}
          className={dangerButton}
        >
          <Trash2 className="h-4 w-4" aria-hidden="true" />
          Eliminar cierre
        </button>
      </div>

      {confirming && (
        <ConfirmDialog
          title={`Eliminar el cierre de ${label}`}
          destructive
          confirmLabel="Sí, eliminar cierre"
          cancelLabel="No, conservarlo"
          pending={remove.isPending}
          error={remove.isError ? describeMonthCloseError(remove.error) : null}
          onConfirm={() => remove.mutate()}
          onCancel={() => setConfirming(false)}
        >
          <p>
            Se descarta el resumen guardado de {label}: sus cifras se pierden y no se pueden recuperar tal como estaban. Tus ingresos, gastos y ahorros no se
            tocan.
          </p>
          <p>Después podrás cerrar el mes de nuevo con los movimientos de hoy.</p>
        </ConfirmDialog>
      )}
    </section>
  );
}
