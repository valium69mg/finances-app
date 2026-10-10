import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarClock, Gift, HandCoins, Loader2, ShoppingBag } from "lucide-react";
import { expenseRequestsKeys, getRequestCategories } from "../../api/expenseRequests";
import { listPersonalCategories, personalKeys } from "../../api/personal";
import { convertWishToExpense, convertWishToFuture, convertWishToRequest, wishlistKeys, type WishItem } from "../../api/wishlist";
import { TextField } from "../../components/AuthCard";
import { Modal } from "../../components/Modal";
import { SelectField } from "../expenses/SelectField";
import { formatMoney, todayISO } from "../expenses/money";
import { ErrorBanner, primaryButton, secondaryButton } from "../settings/ui";
import { describeWishError } from "./errors";
import {
  defaultConvertDraft,
  toExpenseInput,
  toFutureInput,
  toRequestInput,
  validateConversion,
  type ConvertChoice,
  type ConvertDraft,
  type ConvertErrors,
} from "./form";

interface Props {
  item: WishItem;
  onClose: () => void;
  /** Called with the converted item and the way it was converted. */
  onConverted: (item: WishItem, choice: ConvertChoice) => void;
}

const CHOICES: { value: ConvertChoice; label: string; help: string; icon: typeof Gift }[] = [
  { value: "gasto_futuro", label: "Gasto futuro", help: "Para ir juntando el dinero con calma.", icon: CalendarClock },
  { value: "gasto", label: "Ya lo compré", help: "Se registra como un gasto tuyo.", icon: ShoppingBag },
  { value: "peticion", label: "Pedir al titular", help: "Se envía una petición de gasto.", icon: HandCoins },
];

/**
 * "Convertir": turns an active item into her future expense, her Gasto or an expense request. The item stays on
 * the list, marked as converted and linked to what it became.
 */
export function ConvertDialog({ item, onClose, onConverted }: Props) {
  const qc = useQueryClient();
  const [draft, setDraft] = useState<ConvertDraft>(() => defaultConvertDraft(item, todayISO()));
  const [errors, setErrors] = useState<ConvertErrors>({});
  const set = (patch: Partial<ConvertDraft>) => setDraft((d) => ({ ...d, ...patch }));

  const hers = useQuery({ queryKey: personalKeys.categories("Gasto"), queryFn: () => listPersonalCategories("Gasto"), retry: false });
  const owners = useQuery({
    queryKey: expenseRequestsKeys.categories,
    queryFn: getRequestCategories,
    retry: false,
    enabled: draft.choice === "peticion",
  });

  const convert = useMutation({
    mutationFn: () => {
      if (draft.choice === "gasto_futuro") return convertWishToFuture(item.id, toFutureInput(draft));
      if (draft.choice === "gasto") return convertWishToExpense(item.id, toExpenseInput(draft));
      return convertWishToRequest(item.id, toRequestInput(draft));
    },
    onSuccess: async (converted) => {
      // The conversion writes to her ledger or to the requests: refresh both, the wishlist included.
      await Promise.all([qc.invalidateQueries({ queryKey: personalKeys.all }), qc.invalidateQueries({ queryKey: expenseRequestsKeys.all })]);
      onConverted(converted, draft.choice);
    },
    // The list is the truth: an item converted meanwhile shows its real state behind the dialog's message.
    onError: () => void qc.invalidateQueries({ queryKey: wishlistKeys.all }),
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    convert.reset();
    const found = validateConversion(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) return;
    convert.mutate();
  }

  const herOptions = hers.data ?? [];
  const ownerOptions = owners.data ?? [];

  return (
    <Modal title={`Convertir ${item.name}`} onClose={onClose} highlight>
      <form noValidate onSubmit={onSubmit} className="space-y-5">
        <p className="text-sm text-muted">
          Precio estimado: {formatMoney(item.price)}. El deseo se queda en tu lista, marcado como convertido.
        </p>

        <fieldset className="space-y-2">
          <legend className="mb-1.5 text-sm font-medium">¿En qué lo quieres convertir?</legend>
          {CHOICES.map(({ value, label, help, icon: Icon }) => (
            <label key={value} className="flex min-h-11 cursor-pointer items-start gap-3 rounded-lg border border-border p-3 has-[:checked]:border-primary has-[:checked]:bg-primary/5">
              <input
                type="radio"
                name="convert-choice"
                value={value}
                checked={draft.choice === value}
                onChange={() => {
                  setErrors({});
                  convert.reset();
                  set({ choice: value });
                }}
                className="mt-1"
              />
              <span className="min-w-0">
                <span className="flex items-center gap-2 font-medium">
                  <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
                  {label}
                </span>
                <span className="block text-sm text-muted">{help}</span>
              </span>
            </label>
          ))}
        </fieldset>

        {draft.choice === "gasto_futuro" && (
          <div className="grid gap-4 sm:grid-cols-2">
            <TextField label="Monto a juntar (MXN)" inputMode="decimal" value={draft.target} onChange={(e) => set({ target: e.target.value })} error={errors.target} autoComplete="off" />
            <TextField label="Fecha de vencimiento" type="date" value={draft.dueDate} onChange={(e) => set({ dueDate: e.target.value })} error={errors.dueDate} />
          </div>
        )}

        {draft.choice === "gasto" && (
          <div className="grid gap-4 sm:grid-cols-2">
            <TextField label="Monto (MXN)" inputMode="decimal" value={draft.amount} onChange={(e) => set({ amount: e.target.value })} error={errors.amount} autoComplete="off" />
            <TextField label="Fecha" type="date" value={draft.date} onChange={(e) => set({ date: e.target.value })} error={errors.date} />
            <SelectField label="Categoría" value={draft.categoryId} onChange={(e) => set({ categoryId: e.target.value })} error={errors.category}>
              {herOptions.map((c) => (
                <option key={c.id} value={String(c.id)}>
                  {c.name}
                </option>
              ))}
            </SelectField>
          </div>
        )}

        {draft.choice === "peticion" && (
          <div className="grid gap-4 sm:grid-cols-2">
            <TextField label="Monto (MXN)" inputMode="decimal" value={draft.amount} onChange={(e) => set({ amount: e.target.value })} error={errors.amount} autoComplete="off" />
            <TextField label="Fecha del gasto" type="date" value={draft.date} onChange={(e) => set({ date: e.target.value })} error={errors.date} />
            <SelectField
              label="Categoría de la petición"
              value={draft.ownerCategory}
              onChange={(e) => set({ ownerCategory: e.target.value })}
              error={errors.ownerCategory}
              hint={owners.isPending ? "Cargando categorías…" : owners.isError ? "No se pudieron cargar las categorías." : "Son las categorías del titular."}
            >
              <option value="">Elige una categoría</option>
              {ownerOptions.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </SelectField>
          </div>
        )}

        {convert.isError && <ErrorBanner>{describeWishError(convert.error)}</ErrorBanner>}

        <div className="flex flex-wrap items-center gap-3">
          <button type="submit" disabled={convert.isPending} aria-busy={convert.isPending} className={primaryButton}>
            {convert.isPending && <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />}
            {convert.isPending ? "Convirtiendo…" : draft.choice === "peticion" ? "Enviar petición" : "Convertir"}
          </button>
          <button type="button" onClick={onClose} disabled={convert.isPending} className={secondaryButton}>
            Cancelar
          </button>
        </div>
      </form>
    </Modal>
  );
}
