import type { Client } from "../../api/settings";
import { withValue } from "../expenses/formHelpers";
import { SelectField } from "../expenses/SelectField";

interface Props {
  label: string;
  /** Selected client id; empty selects the placeholder option. */
  value: string;
  onChange: (id: string) => void;
  clients: Client[];
  /** Text of the empty option, e.g. "Elige un cliente" or "Todos los clientes". */
  emptyLabel: string;
}

/** Client dropdown fed by the Settings clients (same shape as InstrumentSelect). */
export function ClientSelect({ label, value, onChange, clients, emptyLabel }: Props) {
  // A current value that is no longer configured stays selectable (an old invoice).
  const ids = withValue(
    clients.map((c) => c.id),
    value,
  );
  return (
    <SelectField label={label} value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">{emptyLabel}</option>
      {ids.map((id) => (
        <option key={id} value={id}>
          {clients.find((c) => c.id === id)?.name ?? id}
        </option>
      ))}
    </SelectField>
  );
}
