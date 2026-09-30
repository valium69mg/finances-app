import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Loader2, Plus } from "lucide-react";
import { addValuation, savingsKeys, type ValuationInput } from "../../api/savings";
import type { AllSettings } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { isValuationAmount, todayISO } from "../expenses/money";
import { ErrorBanner, fieldGrid, primaryButton } from "../settings/ui";
import { describeValuationError } from "./errors";
import { InstrumentSelect } from "./InstrumentSelect";

/** Appends a manual valuation. Valuations are append-only: a correction is a newer valuation. */
export function ValuationForm({ settings }: { settings: AllSettings }) {
  const qc = useQueryClient();
  const instruments = settings.instruments.instruments;
  const [instrument, setInstrument] = useState("");
  const [value, setValue] = useState("");
  // null = the user never touched the date, so it is "today" whenever it is read
  // (a page left open past midnight must not record yesterday).
  const [date, setDate] = useState<string | null>(null);
  const [note, setNote] = useState("");
  const [errors, setErrors] = useState<{ instrument?: string; value?: string; date?: string }>({});
  const valueRef = useRef<HTMLInputElement>(null);

  const add = useMutation({
    mutationFn: (input: ValuationInput) => addValuation(input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: savingsKeys.all });
      setValue("");
      setNote("");
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    add.reset();
    const effectiveDate = date ?? todayISO();
    const found: typeof errors = {};
    if (!instrument) found.instrument = "Elige un instrumento.";
    if (!isValuationAmount(value)) {
      found.value = "Escribe un valor mayor a cero, con máximo 2 decimales y menor a 1,000,000,000,000, por ejemplo 25000.00.";
    }
    if (!effectiveDate) found.date = "Elige una fecha.";
    setErrors(found);
    if (Object.keys(found).length > 0) {
      if (found.value) valueRef.current?.focus();
      return;
    }
    const input: ValuationInput = { instrument, value_mxn: value.trim(), date: effectiveDate };
    if (note.trim()) input.note = note.trim();
    add.mutate(input);
  }

  return (
    <form noValidate onSubmit={onSubmit} aria-labelledby="valuation-form-title" className="space-y-5">
      <div>
        <h2 id="valuation-form-title" className="text-lg font-semibold tracking-tight">
          Registrar valuación
        </h2>
        <p className="mt-1 text-sm text-muted">
          Las valuaciones no se editan ni se eliminan: para corregir una, registra una nueva con fecha posterior.
        </p>
      </div>
      <div className={`${fieldGrid} lg:grid-cols-3`}>
        <InstrumentSelect
          label="Instrumento a valuar"
          value={instrument}
          onChange={setInstrument}
          instruments={instruments}
          emptyLabel="Elige un instrumento"
        />
        <TextField
          label="Valor actual (MXN)"
          inputMode="decimal"
          inputRef={valueRef}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          error={errors.value}
          autoComplete="off"
        />
        <TextField label="Fecha de la valuación" type="date" value={date ?? todayISO()} onChange={(e) => setDate(e.target.value)} error={errors.date} />
        <TextField label="Nota (opcional)" value={note} onChange={(e) => setNote(e.target.value)} autoComplete="off" />
      </div>

      {errors.instrument && <ErrorBanner>{errors.instrument}</ErrorBanner>}
      {add.isError && <ErrorBanner>{describeValuationError(add.error)}</ErrorBanner>}

      <div className="flex flex-wrap items-center gap-3">
        <button type="submit" disabled={add.isPending} aria-busy={add.isPending} className={primaryButton}>
          {add.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <Plus className="h-4 w-4" aria-hidden="true" />}
          {add.isPending ? "Guardando…" : "Registrar valuación"}
        </button>
        <span role="status" className="text-sm">
          {add.isSuccess && (
            <span className="flex items-center gap-1.5">
              <CheckCircle2 className="h-4 w-4 text-accent" aria-hidden="true" />
              Valuación registrada.
            </span>
          )}
        </span>
      </div>
    </form>
  );
}
