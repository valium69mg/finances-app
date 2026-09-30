import { useState } from "react";
import { updateInstruments, type Instruments } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { Card, SectionForm, fieldGrid, useDraft, useSave } from "./ui";
import { validateInstruments, type Errors } from "./validation";

export function InstrumentsSection({ data }: { data: Instruments }) {
  const [draft, setDraft] = useDraft(data, (d) => d);
  const [errors, setErrors] = useState<Errors>({});
  const save = useSave(updateInstruments);

  const patch = (i: number, key: "name" | "type" | "platform", value: string) =>
    setDraft({ ...draft, instruments: draft.instruments.map((x, j) => (j === i ? { ...x, [key]: value } : x)) });

  function onSubmit() {
    const found = validateInstruments(draft.instruments);
    setErrors(found);
    if (Object.keys(found).length > 0) return false;
    save.mutate(draft);
    return true;
  }

  return (
    <SectionForm title="Instrumentos" description="Instrumentos de inversión disponibles para tus ahorros." save={save} onSubmit={onSubmit}>
      {draft.instruments.length === 0 && <p className="text-sm text-muted">Aún no hay instrumentos configurados.</p>}
      <ul className="space-y-3">
        {draft.instruments.map((ins, i) => (
          <li key={ins.id}>
            <Card>
              <h3 className="mb-3 min-w-0 break-words font-semibold">{data.instruments[i]?.name || ins.id}</h3>
              <div className={fieldGrid}>
                <TextField label={`Nombre (instrumento ${i + 1})`} value={ins.name} onChange={(e) => patch(i, "name", e.target.value)} error={errors[`${i}.name`]} />
                <TextField label={`Tipo (instrumento ${i + 1})`} value={ins.type} onChange={(e) => patch(i, "type", e.target.value)} />
                <TextField label={`Plataforma (instrumento ${i + 1})`} value={ins.platform} onChange={(e) => patch(i, "platform", e.target.value)} />
              </div>
            </Card>
          </li>
        ))}
      </ul>
    </SectionForm>
  );
}
