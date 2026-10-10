import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, Plus, Save } from "lucide-react";
import {
  createPersonalMovement,
  personalKeys,
  updatePersonalMovement,
  type PersonalCategory,
  type PersonalMovement,
  type PersonalMovementInput,
} from "../../api/personal";
import { TextField } from "../../components/AuthCard";
import { Modal } from "../../components/Modal";
import { SelectField } from "../expenses/SelectField";
import { isValuationAmount, todayISO } from "../expenses/money";
import { ErrorBanner, primaryButton, secondaryButton } from "../settings/ui";
import type { PersonalPageConfig } from "./config";
import { describePersonalError } from "./errors";
import { isNegative } from "./totals";

interface Props {
  config: PersonalPageConfig;
  categories: PersonalCategory[];
  /** When set, the dialog edits this movement instead of creating one. */
  editing: PersonalMovement | null;
  onClose: () => void;
}

type Direction = "aportacion" | "retiro";

/** Create/edit dialog shared by the three personal pages. An Ahorro asks "Aportación" or "Retiro"; a withdrawal is sent negative. */
export function MovementDialog({ config, categories, editing, onClose }: Props) {
  const qc = useQueryClient();
  const isSaving = config.kind === "Ahorro";
  const [description, setDescription] = useState(editing?.description ?? "");
  const [amount, setAmount] = useState(editing ? editing.amount.replace(/^-/, "") : "");
  const [direction, setDirection] = useState<Direction>(editing && isNegative(editing.amount) ? "retiro" : "aportacion");
  const [categoryId, setCategoryId] = useState<string>(String(editing?.category_id ?? categories[0]?.id ?? ""));
  const [date, setDate] = useState(editing?.date ?? todayISO());
  const [errors, setErrors] = useState<{ amount?: string; category?: string; date?: string }>({});
  const amountRef = useRef<HTMLInputElement>(null);

  const save = useMutation({
    mutationFn: (input: PersonalMovementInput) => (editing ? updatePersonalMovement(editing.id, input) : createPersonalMovement(input)),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: personalKeys.all });
      onClose();
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const found: typeof errors = {};
    if (!isValuationAmount(amount)) found.amount = "Escribe un monto mayor a cero con máximo dos decimales, por ejemplo 250.50.";
    if (categoryId === "") found.category = "Elige una categoría.";
    if (!date) found.date = "Elige una fecha.";
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.amount) amountRef.current?.focus();
      return;
    }
    const value = amount.trim();
    const input: PersonalMovementInput = {
      kind: config.kind,
      category_id: Number(categoryId),
      amount: isSaving && direction === "retiro" ? `-${value}` : value,
      description: description.trim(),
      date,
    };
    save.mutate(input);
  }

  // A category of the movement that is no longer listed stays selectable.
  const options = editing && !categories.some((c) => c.id === editing.category_id) ? [...categories, { id: editing.category_id, name: editing.category }] : categories;
  const title = editing ? `Editar ${config.noun.replace(/^el /, "")}` : config.newLabel;

  return (
    <Modal title={title} onClose={onClose} highlight={editing !== null}>
      <form noValidate onSubmit={onSubmit} className="space-y-5">
        <div className="grid gap-4">
          {isSaving && (
            <SelectField label="Tipo" value={direction} onChange={(e) => setDirection(e.target.value as Direction)}>
              <option value="aportacion">Aportación</option>
              <option value="retiro">Retiro</option>
            </SelectField>
          )}
          <TextField label="Descripción" value={description} onChange={(e) => setDescription(e.target.value)} autoComplete="off" />
          <TextField
            label="Monto (MXN)"
            inputMode="decimal"
            inputRef={amountRef}
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            error={errors.amount}
            hint={isSaving && direction === "retiro" ? "Escribe el monto sin signo; se registra como retiro." : undefined}
            autoComplete="off"
          />
          <SelectField label="Categoría" value={categoryId} onChange={(e) => setCategoryId(e.target.value)} hint={errors.category}>
            {options.map((c) => (
              <option key={c.id} value={String(c.id)}>
                {c.name}
              </option>
            ))}
          </SelectField>
          <TextField label="Fecha" type="date" value={date} onChange={(e) => setDate(e.target.value)} error={errors.date} />
        </div>

        {save.isError && <ErrorBanner>{describePersonalError(save.error, config.noun)}</ErrorBanner>}

        <div className="flex flex-wrap items-center gap-3">
          <button type="submit" disabled={save.isPending} aria-busy={save.isPending} className={primaryButton}>
            {save.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            ) : editing ? (
              <Save className="h-4 w-4" aria-hidden="true" />
            ) : (
              <Plus className="h-4 w-4" aria-hidden="true" />
            )}
            {save.isPending ? "Guardando…" : editing ? "Guardar cambios" : "Guardar"}
          </button>
          <button type="button" onClick={onClose} className={secondaryButton}>
            Cancelar
          </button>
        </div>
      </form>
    </Modal>
  );
}
