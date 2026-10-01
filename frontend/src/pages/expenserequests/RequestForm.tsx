import { useRef, useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { Loader2, Send } from "lucide-react";
import { createExpenseRequest, type ExpenseRequest } from "../../api/expenseRequests";
import { TextField } from "../../components/AuthCard";
import { todayISO } from "../expenses/money";
import { SelectField } from "../expenses/SelectField";
import { ErrorBanner, fieldGrid, primaryButton } from "../settings/ui";
import { describeRequestError } from "./errors";
import { emptyDraft, MAX_DESCRIPTION_LENGTH, toNewRequestInput, validateRequest, type RequestDraft, type RequestErrors } from "./form";
import { useInvalidateRequests } from "./useInvalidate";

interface Props {
  /** Names of the Gasto categories the suggestion can be picked from. */
  categories: string[];
  onSaved: (request: ExpenseRequest) => void;
}

/** "Nueva petición": amount, description, an optional suggested category and the date of the expense. */
export function RequestForm({ categories, onSaved }: Props) {
  const invalidate = useInvalidateRequests();
  const [draft, setDraft] = useState<RequestDraft>(() => emptyDraft(todayISO()));
  const [errors, setErrors] = useState<RequestErrors>({});
  const amountRef = useRef<HTMLInputElement>(null);
  const descriptionRef = useRef<HTMLInputElement>(null);
  const dateRef = useRef<HTMLInputElement>(null);
  const set = (patch: Partial<RequestDraft>) => setDraft((d) => ({ ...d, ...patch }));

  const save = useMutation({
    mutationFn: () => createExpenseRequest(toNewRequestInput(draft)),
    onSuccess: async (request) => {
      await invalidate();
      onSaved(request);
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    save.reset();
    const found = validateRequest(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.amount) amountRef.current?.focus();
      else if (found.description) descriptionRef.current?.focus();
      else dateRef.current?.focus();
      return;
    }
    save.mutate();
  }

  return (
    <form noValidate onSubmit={onSubmit} aria-labelledby="request-form-title" className="space-y-5">
      <h3 id="request-form-title" className="text-base font-semibold tracking-tight">
        Nueva petición
      </h3>
      <div className={fieldGrid}>
        <TextField
          label="Monto (MXN)"
          inputMode="decimal"
          inputRef={amountRef}
          value={draft.amount}
          onChange={(e) => set({ amount: e.target.value })}
          error={errors.amount}
          autoComplete="off"
        />
        <TextField
          label="Descripción"
          inputRef={descriptionRef}
          value={draft.description}
          onChange={(e) => set({ description: e.target.value })}
          error={errors.description}
          maxLength={MAX_DESCRIPTION_LENGTH + 20}
          autoComplete="off"
        />
        <SelectField label="Categoría sugerida" value={draft.category} onChange={(e) => set({ category: e.target.value })} hint="Es solo una sugerencia: quien aprueba elige la categoría.">
          <option value="">Sin sugerencia</option>
          {categories.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </SelectField>
        <TextField label="Fecha del gasto" type="date" inputRef={dateRef} value={draft.date} onChange={(e) => set({ date: e.target.value })} error={errors.date} />
      </div>
      {save.isError && <ErrorBanner>{describeRequestError(save.error)}</ErrorBanner>}
      <div className="flex flex-wrap items-center gap-3">
        <button type="submit" disabled={save.isPending} aria-busy={save.isPending} className={primaryButton}>
          {save.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Send className="h-4 w-4" aria-hidden="true" />}
          {save.isPending ? "Enviando…" : "Enviar petición"}
        </button>
      </div>
    </form>
  );
}
