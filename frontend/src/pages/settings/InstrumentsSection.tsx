import { useState } from "react";
import { ChevronRight } from "lucide-react";
import type { Instruments } from "../../api/settings";
import { InstrumentDialog } from "./InstrumentDialog";
import { SectionHeader, rowButton } from "./ui";

/** Compact list of instruments (name, type, platform); a click edits one in a dialog. Adding instruments is not supported today. */
export function InstrumentsSection({ data }: { data: Instruments }) {
  const [editing, setEditing] = useState<number | null>(null);
  return (
    <div className="space-y-6">
      <SectionHeader title="Instrumentos" description="Instrumentos de inversión disponibles para tus ahorros. Toca uno para editarlo." />
      {data.instruments.length === 0 && <p className="text-sm text-muted">Aún no hay instrumentos configurados.</p>}
      <ul className="space-y-2">
        {data.instruments.map((ins, i) => (
          <li key={ins.id}>
            <button type="button" onClick={() => setEditing(i)} className={`${rowButton} flex-wrap sm:flex-nowrap`}>
              <span className="min-w-0 flex-1 break-words font-medium">{ins.name || ins.id}</span>
              <span className="text-sm text-muted sm:w-40">{ins.type || "—"}</span>
              <span className="text-sm text-muted sm:w-40">{ins.platform || "—"}</span>
              <ChevronRight className="hidden h-5 w-5 shrink-0 text-muted sm:block" aria-hidden="true" />
            </button>
          </li>
        ))}
      </ul>
      {editing !== null && <InstrumentDialog data={data} index={editing} onClose={() => setEditing(null)} />}
    </div>
  );
}
