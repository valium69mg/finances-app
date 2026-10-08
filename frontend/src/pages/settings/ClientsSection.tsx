import { useState } from "react";
import { updateClients, type Client } from "../../api/settings";
import { TextField } from "../../components/AuthCard";
import { Card, SectionForm, fieldGrid, useDraft, useSave } from "./ui";
import { validateClients, type Errors } from "./validation";

type EditableKey = Exclude<keyof Client, "id">;

const FIELDS: { key: EditableKey; label: string; decimal?: boolean; hint?: string }[] = [
  { key: "name", label: "Nombre" },
  { key: "type", label: "Tipo" },
  { key: "currency", label: "Moneda" },
  { key: "iva_rate", label: "Tasa de IVA", decimal: true, hint: "Como fracción: 0.16 equivale a 16 %." },
  { key: "rfc", label: "RFC" },
  { key: "regimen", label: "Régimen fiscal" },
  { key: "postal_code", label: "Código postal", hint: "5 dígitos. Para público en general y clientes del extranjero se usa el del emisor." },
  { key: "uso_cfdi", label: "Uso de CFDI" },
  { key: "ret_isr_rate", label: "Retención de ISR", decimal: true },
  { key: "ret_iva_rate", label: "Retención de IVA", decimal: true },
  { key: "concepto", label: "Concepto" },
  { key: "clave_prod_serv", label: "Clave de producto o servicio" },
  { key: "clave_unidad", label: "Clave de unidad" },
  { key: "address", label: "Domicilio" },
  { key: "tax_residence", label: "Residencia fiscal" },
  { key: "contract", label: "Contrato" },
  { key: "real_payer", label: "Pagador real" },
];

export function ClientsSection({ data }: { data: Client[] }) {
  const [draft, setDraft] = useDraft(data, (d) => d);
  const [errors, setErrors] = useState<Errors>({});
  const save = useSave(updateClients);

  function onSubmit() {
    const found = validateClients(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) return false;
    save.mutate(draft);
    return true;
  }

  return (
    <SectionForm title="Clientes" description="Datos de facturación de cada cliente (CFDI)." save={save} onSubmit={onSubmit}>
      {draft.length === 0 && <p className="text-sm text-muted">Aún no hay clientes configurados.</p>}
      <ul className="space-y-3">
        {draft.map((c, i) => (
          <li key={c.id}>
            <Card>
              <h3 className="mb-3 min-w-0 break-words font-semibold">{data[i]?.name || c.id}</h3>
              <div className={fieldGrid}>
                {FIELDS.map((f) => (
                  <TextField
                    key={f.key}
                    label={`${f.label} (cliente ${i + 1})`}
                    hint={f.hint}
                    inputMode={f.decimal ? "decimal" : undefined}
                    value={c[f.key]}
                    onChange={(e) => setDraft(draft.map((x, j) => (j === i ? { ...x, [f.key]: e.target.value } : x)))}
                    error={errors[`${i}.${f.key}`]}
                  />
                ))}
              </div>
            </Card>
          </li>
        ))}
      </ul>
    </SectionForm>
  );
}
