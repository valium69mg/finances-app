import { useState } from "react";
import { ChevronRight } from "lucide-react";
import type { Client } from "../../api/settings";
import { ClientDialog } from "./ClientDialog";
import { SectionHeader, rowButton } from "./ui";

const chip = "rounded-full border border-border px-2 py-0.5 text-xs text-muted";

/** One compact card per client; a click edits it in a dialog. There is no way to add or remove clients from the app today. */
export function ClientsSection({ data }: { data: Client[] }) {
  const [editing, setEditing] = useState<number | null>(null);
  return (
    <div className="space-y-6">
      <SectionHeader title="Clientes" description="Datos de facturación de cada cliente (CFDI). Toca uno para editarlo." />
      {data.length === 0 && <p className="text-sm text-muted">Aún no hay clientes configurados.</p>}
      <ul className="grid gap-3 md:grid-cols-2">
        {data.map((c, i) => (
          <li key={c.id}>
            <button type="button" onClick={() => setEditing(i)} className={`${rowButton} h-full`}>
              <span className="min-w-0 flex-1 space-y-1.5">
                <span className="block break-words font-semibold">{c.name || c.id}</span>
                <span className="block text-sm text-muted">
                  RFC {c.rfc || "—"} · Régimen {c.regimen || "—"}
                </span>
                <span className="flex flex-wrap gap-2">
                  {c.type && <span className={chip}>{c.type}</span>}
                  {c.currency && <span className={chip}>{c.currency}</span>}
                </span>
              </span>
              <ChevronRight className="h-5 w-5 shrink-0 text-muted" aria-hidden="true" />
            </button>
          </li>
        ))}
      </ul>
      {editing !== null && <ClientDialog clients={data} index={editing} onClose={() => setEditing(null)} />}
    </div>
  );
}
