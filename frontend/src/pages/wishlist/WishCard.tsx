import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, Loader2, Pencil, Repeat, Trash2 } from "lucide-react";
import { Link } from "react-router-dom";
import { deleteWishItem, wishlistKeys, type WishItem } from "../../api/wishlist";
import { formatMoney } from "../expenses/money";
import { StateBadge } from "../expenserequests/labels";
import { ErrorBanner, dangerButton, secondaryButton } from "../settings/ui";
import { describeWishError } from "./errors";
import { hogarLink } from "../hogar/links";
import { usePersonalScope } from "../personal/scope";
import { CONVERSION_TARGET, CONVERSION_VIEW, PriorityBadge, returnedNote, returnedViewNote } from "./labels";

interface Props {
  item: WishItem;
  onConvert: (item: WishItem) => void;
  onEdit: (item: WishItem) => void;
  onDeleted: (item: WishItem) => void;
}

/** True for a link the browser may open from the page: http or https only (the backend already enforces it). */
const safeUrl = (u: string | null): u is string => u !== null && /^https?:\/\//i.test(u);

/** One wishlist item: what it is, its priority, the store link and note, and (when converted) what it became. */
export function WishCard({ item, onConvert, onEdit, onDeleted }: Props) {
  const qc = useQueryClient();
  const { userId, readOnly } = usePersonalScope();
  const [confirming, setConfirming] = useState(false);
  const remove = useMutation({
    mutationFn: () => deleteWishItem(item.id),
    onSuccess: async () => {
      setConfirming(false);
      onDeleted(item);
      await qc.invalidateQueries({ queryKey: wishlistKeys.all });
    },
    onError: () => void qc.invalidateQueries({ queryKey: wishlistKeys.all }),
  });
  const converted = item.status === "convertida";
  // Read-only (the owner looking at a household user): third-person wording and links inside the Hogar view.
  const view = item.conversion ? CONVERSION_VIEW[item.conversion.type] : null;
  const target = item.conversion
    ? readOnly && view
      ? { text: view.text, link: view.link, path: view.section && userId ? hogarLink(userId, view.section) : "" }
      : CONVERSION_TARGET[item.conversion.type]
    : null;
  const returned = readOnly ? returnedViewNote(item) : returnedNote(item);

  return (
    <li data-testid="wish-card" data-status={item.status} className="space-y-3 p-4">
      <div className="flex flex-wrap items-start justify-between gap-x-3 gap-y-2">
        <div className="min-w-0">
          <p className="break-words font-medium">{item.name}</p>
          <p className="text-sm tabular-nums text-muted">
            {formatMoney(item.price)} MXN · {item.category}
          </p>
        </div>
        <PriorityBadge priority={item.priority} />
      </div>

      {(safeUrl(item.store_url) || item.note) && (
        <div className="space-y-1 text-sm">
          {safeUrl(item.store_url) && (
            <a href={item.store_url} target="_blank" rel="noopener noreferrer" className="focus-ring inline-flex min-h-6 items-center gap-1.5 rounded text-primary underline underline-offset-2">
              <ExternalLink className="h-4 w-4 shrink-0" aria-hidden="true" />
              Ver en la tienda
              <span className="sr-only"> (se abre en una pestaña nueva)</span>
            </a>
          )}
          {item.note && <p className="break-words text-muted">{item.note}</p>}
        </div>
      )}

      {converted && target && item.conversion && (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-sm">
          <span>{target.text}</span>
          {item.conversion.type === "peticion" && item.conversion.request_state && <StateBadge status={item.conversion.request_state} />}
          {target.path && (
            <Link to={target.path} className="focus-ring rounded text-primary underline underline-offset-2">
              {target.link}
            </Link>
          )}
        </div>
      )}
      {returned && <p className="text-sm text-muted">{returned}</p>}

      {remove.isError && <ErrorBanner>{describeWishError(remove.error)}</ErrorBanner>}

      {readOnly ? null : confirming ? (
        <div role="group" aria-label={`Confirmar eliminación de ${item.name}`} className="flex flex-wrap items-center gap-2">
          <span className="text-sm">{converted ? "¿Eliminar este deseo? Lo que se creó con él no cambia." : "¿Eliminar este deseo?"}</span>
          <button type="button" disabled={remove.isPending} onClick={() => remove.mutate()} className={dangerButton}>
            {remove.isPending && <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />}
            Sí, eliminar
          </button>
          <button type="button" disabled={remove.isPending} onClick={() => setConfirming(false)} className={secondaryButton}>
            Cancelar
          </button>
        </div>
      ) : (
        <div className="flex flex-wrap gap-2">
          {!converted && (
            <button type="button" onClick={() => onConvert(item)} aria-label={`Convertir ${item.name}`} className={secondaryButton}>
              <Repeat className="h-4 w-4" aria-hidden="true" />
              Convertir
            </button>
          )}
          <button type="button" onClick={() => onEdit(item)} aria-label={`Editar ${item.name}`} className={secondaryButton}>
            <Pencil className="h-4 w-4" aria-hidden="true" />
            Editar
          </button>
          <button
            type="button"
            onClick={() => {
              remove.reset();
              setConfirming(true);
            }}
            aria-label={`Eliminar ${item.name}`}
            className={dangerButton}
          >
            <Trash2 className="h-4 w-4" aria-hidden="true" />
            Eliminar
          </button>
        </div>
      )}
    </li>
  );
}
