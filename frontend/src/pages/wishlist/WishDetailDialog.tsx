import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Pencil, Repeat } from "lucide-react";
import { deleteWishItem, wishlistKeys, type WishItem } from "../../api/wishlist";
import { DeleteAction } from "../../components/DeleteAction";
import { Modal } from "../../components/Modal";
import { formatMoney } from "../expenses/money";
import { ErrorBanner, secondaryButton } from "../settings/ui";
import { describeWishError } from "./errors";
import { PriorityBadge } from "./labels";
import { WishDetails } from "./WishCard";

interface Props {
  item: WishItem;
  onClose: () => void;
  onConvert: (item: WishItem) => void;
  onEdit: (item: WishItem) => void;
  /** Called after the item was deleted so the page can show a notice. */
  onDeleted: (item: WishItem) => void;
}

/** The detail of one wish and what can be done with it: convert (while active), edit, or delete after a confirmation. */
export function WishDetailDialog({ item, onClose, onConvert, onEdit, onDeleted }: Props) {
  const qc = useQueryClient();
  const converted = item.status === "convertida";
  const remove = useMutation({
    mutationFn: () => deleteWishItem(item.id),
    onSuccess: async () => {
      onDeleted(item);
      await qc.invalidateQueries({ queryKey: wishlistKeys.all });
    },
    onError: () => void qc.invalidateQueries({ queryKey: wishlistKeys.all }),
  });

  return (
    <Modal title={item.name} onClose={onClose}>
      <div className="space-y-5">
        <div className="flex flex-wrap items-start justify-between gap-x-3 gap-y-2">
          <p className="text-sm tabular-nums text-muted">
            {formatMoney(item.price)} MXN · {item.category}
          </p>
          <PriorityBadge priority={item.priority} />
        </div>
        <WishDetails item={item} />

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
        </div>

        {remove.isError && <ErrorBanner>{describeWishError(remove.error)}</ErrorBanner>}
        <div className="border-t border-border pt-5">
          <DeleteAction
            name={item.name}
            question={converted ? "¿Eliminar este deseo? Lo que se creó con él no cambia." : "¿Eliminar este deseo?"}
            pending={remove.isPending}
            onStart={() => remove.reset()}
            onConfirm={() => remove.mutate()}
          />
        </div>
      </div>
    </Modal>
  );
}
