import { useState } from "react";
import { updateInstruments, type Instrument, type Instruments } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { EditDialog } from "./EditDialog";
import { replaceAt } from "./summary";
import { useSave } from "./ui";
import { validateInstruments, type Errors } from "./validation";

export function InstrumentDialog({ data, index, onClose }: { data: Instruments; index: number; onClose: () => void }) {
  const [draft, setDraft] = useState<Instrument>(data.instruments[index]);
  const [errors, setErrors] = useState<Errors>({});
  const save = useSave(updateInstruments);

  function onSubmit() {
    const list = replaceAt(data.instruments, index, draft);
    const found = validateInstruments(list);
    const own: Errors = found[`${index}.name`] ? { name: found[`${index}.name`] } : {};
    setErrors(own);
    if (Object.keys(own).length > 0) return false;
    save.mutate({ ...data, instruments: list }, { onSuccess: onClose });
    return true;
  }

  return (
    <EditDialog title={`Editar ${data.instruments[index].name || data.instruments[index].id}`} onClose={onClose} save={save} onSubmit={onSubmit}>
      <div className="grid gap-4">
        <TextField label="Nombre" value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} error={errors.name} autoComplete="off" />
        <TextField label="Tipo" value={draft.type} onChange={(e) => setDraft({ ...draft, type: e.target.value })} autoComplete="off" />
        <TextField label="Plataforma" value={draft.platform} onChange={(e) => setDraft({ ...draft, platform: e.target.value })} autoComplete="off" />
      </div>
    </EditDialog>
  );
}
