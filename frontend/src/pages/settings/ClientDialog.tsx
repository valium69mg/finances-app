import { useState } from "react";
import { updateClients, type Client } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { EditDialog } from "./EditDialog";
import { replaceAt } from "./summary";
import { fieldGrid, useSave } from "./ui";
import { validateClients, type Errors } from "./validation";

type EditableKey = Exclude<keyof Client, "id">;

interface FieldDef {
  key: EditableKey;
  label: string;
  decimal?: boolean;
  hint?: string;
}

/** The client fields, grouped the way they are used: who the client is for the SAT, what goes on the CFDI, and the contract. */
const GROUPS: { legend: string; fields: FieldDef[] }[] = [
  {
    legend: "Fiscal",
    fields: [
      { key: "name", label: "Nombre" },
      { key: "type", label: "Tipo" },
      { key: "rfc", label: "RFC" },
      { key: "regimen", label: "Régimen fiscal" },
      { key: "postal_code", label: "Código postal", hint: "5 dígitos. Para público en general y clientes del extranjero se usa el del emisor." },
      { key: "tax_residence", label: "Residencia fiscal" },
      { key: "address", label: "Domicilio" },
    ],
  },
  {
    legend: "Factura (CFDI)",
    fields: [
      { key: "currency", label: "Moneda" },
      { key: "uso_cfdi", label: "Uso de CFDI" },
      { key: "concepto", label: "Concepto" },
      { key: "clave_prod_serv", label: "Clave de producto o servicio" },
      { key: "clave_unidad", label: "Clave de unidad" },
      { key: "iva_rate", label: "Tasa de IVA", decimal: true, hint: "Como fracción: 0.16 equivale a 16 %." },
      { key: "ret_isr_rate", label: "Retención de ISR", decimal: true },
      { key: "ret_iva_rate", label: "Retención de IVA", decimal: true },
    ],
  },
  {
    legend: "Contrato",
    fields: [
      { key: "contract", label: "Contrato" },
      { key: "real_payer", label: "Pagador real" },
    ],
  },
];

export function ClientDialog({ clients, index, onClose }: { clients: Client[]; index: number; onClose: () => void }) {
  const [draft, setDraft] = useState<Client>(clients[index]);
  const [errors, setErrors] = useState<Errors>({});
  const save = useSave(updateClients);

  function onSubmit() {
    const list = replaceAt(clients, index, draft);
    // Errors are keyed by position in the list; keep this client's and key them by field.
    const own = Object.fromEntries(
      Object.entries(validateClients(list))
        .filter(([k]) => k.startsWith(`${index}.`))
        .map(([k, v]) => [k.slice(`${index}.`.length), v]),
    );
    setErrors(own);
    if (Object.keys(own).length > 0) return false;
    save.mutate(list, { onSuccess: onClose });
    return true;
  }

  return (
    <EditDialog title={`Editar ${clients[index].name || clients[index].id}`} onClose={onClose} save={save} onSubmit={onSubmit}>
      {GROUPS.map((g) => (
        <fieldset key={g.legend} className="space-y-3">
          <legend className="mb-3 text-sm font-semibold">{g.legend}</legend>
          <div className={fieldGrid}>
            {g.fields.map((f) => (
              <TextField
                key={f.key}
                label={f.label}
                hint={f.hint}
                inputMode={f.decimal ? "decimal" : undefined}
                value={draft[f.key]}
                onChange={(e) => setDraft({ ...draft, [f.key]: e.target.value })}
                error={errors[f.key]}
                autoComplete="off"
              />
            ))}
          </div>
        </fieldset>
      ))}
    </EditDialog>
  );
}
