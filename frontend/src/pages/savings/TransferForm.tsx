import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowRightLeft, CheckCircle2, Loader2 } from "lucide-react";
import { savingsKeys, transferSavings, type TransferInput } from "../../api/savings";
import type { AllSettings } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { isPositiveDecimal } from "../expenses/money";
import { SelectField } from "../expenses/SelectField";
import { ErrorBanner, fieldGrid, primaryButton } from "../settings/ui";
import { describeSavingsError } from "./errors";
import { InstrumentSelect } from "./InstrumentSelect";
import { SAVINGS_KIND } from "./SavingForm";

/** Moves money from one instrument to another as a pair of movements. */
export function TransferForm({ settings }: { settings: AllSettings }) {
  const qc = useQueryClient();
  const instruments = settings.instruments.instruments;
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [amount, setAmount] = useState("");
  const [category, setCategory] = useState("");
  const [description, setDescription] = useState("");
  const [errors, setErrors] = useState<{ from?: string; to?: string; amount?: string }>({});
  const amountRef = useRef<HTMLInputElement>(null);

  const transfer = useMutation({
    mutationFn: (input: TransferInput) => transferSavings(input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: savingsKeys.all });
      setAmount("");
      setDescription("");
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    transfer.reset();
    const found: typeof errors = {};
    if (!from) found.from = "Elige el instrumento de origen.";
    if (!to) found.to = "Elige el instrumento de destino.";
    else if (from && to === from) found.to = "El destino debe ser distinto al origen.";
    if (!isPositiveDecimal(amount)) found.amount = "Escribe un monto mayor a cero, por ejemplo 1000.00.";
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.amount && !found.from && !found.to) amountRef.current?.focus();
      return;
    }
    const input: TransferInput = { from, to, amount: amount.trim() };
    if (category) input.category = category;
    if (description.trim()) input.description = description.trim();
    transfer.mutate(input);
  }

  const categories = settings.categories.filter((c) => c.kind === SAVINGS_KIND).map((c) => c.name);

  return (
    <form noValidate onSubmit={onSubmit} aria-labelledby="transfer-form-title" className="space-y-5">
      <div>
        <h2 id="transfer-form-title" className="text-lg font-semibold tracking-tight">
          Traspasar entre instrumentos
        </h2>
        <p className="mt-1 text-sm text-muted">Mueve dinero de un instrumento a otro sin cambiar el total ahorrado.</p>
      </div>
      <div className={`${fieldGrid} lg:grid-cols-3`}>
        <InstrumentSelect label="Desde" value={from} onChange={setFrom} instruments={instruments} emptyLabel="Elige un instrumento" />
        <InstrumentSelect label="Hacia" value={to} onChange={setTo} instruments={instruments} emptyLabel="Elige un instrumento" />
        <TextField
          label="Monto a traspasar"
          inputMode="decimal"
          inputRef={amountRef}
          value={amount}
          onChange={(e) => setAmount(e.target.value)}
          error={errors.amount}
          autoComplete="off"
        />
        <SelectField label="Categoría del traspaso (opcional)" value={category} onChange={(e) => setCategory(e.target.value)}>
          <option value="">Automática (según los instrumentos)</option>
          {categories.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </SelectField>
        <TextField label="Descripción del traspaso (opcional)" value={description} onChange={(e) => setDescription(e.target.value)} autoComplete="off" />
      </div>

      {(errors.from || errors.to) && <ErrorBanner>{errors.from ?? errors.to}</ErrorBanner>}
      {transfer.isError && <ErrorBanner>{describeSavingsError(transfer.error)}</ErrorBanner>}

      <div className="flex flex-wrap items-center gap-3">
        <button type="submit" disabled={transfer.isPending} aria-busy={transfer.isPending} className={primaryButton}>
          {transfer.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
          ) : (
            <ArrowRightLeft className="h-4 w-4" aria-hidden="true" />
          )}
          {transfer.isPending ? "Traspasando…" : "Traspasar"}
        </button>
        <span role="status" className="text-sm">
          {transfer.isSuccess && (
            <span className="flex items-center gap-1.5">
              <CheckCircle2 className="h-4 w-4 text-accent" aria-hidden="true" />
              Traspaso registrado.
            </span>
          )}
        </span>
      </div>
    </form>
  );
}
