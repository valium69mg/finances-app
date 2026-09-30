import type { Instrument } from "../../api/settings";
import { withValue } from "../expenses/formHelpers";
import { SelectField } from "../expenses/SelectField";

interface Props {
  label: string;
  /** Selected instrument id; empty selects the placeholder option. */
  value: string;
  onChange: (id: string) => void;
  instruments: Instrument[];
  /** Text of the empty option, e.g. "Elige un instrumento" or "Automático (según la categoría)". */
  emptyLabel: string;
}

/** Instrument dropdown shared by the saving, transfer and valuation forms. */
export function InstrumentSelect({ label, value, onChange, instruments, emptyLabel }: Props) {
  // A current value that is no longer configured stays selectable (editing an old saving).
  const ids = withValue(
    instruments.map((i) => i.id),
    value,
  );
  return (
    <SelectField label={label} value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">{emptyLabel}</option>
      {ids.map((id) => (
        <option key={id} value={id}>
          {instruments.find((i) => i.id === id)?.name ?? id}
        </option>
      ))}
    </SelectField>
  );
}
