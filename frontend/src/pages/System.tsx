import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { getSystemStatus, systemKeys, type SystemStatus } from "../api/system";
import { describeSystemError } from "./system/errors";
import { formatAge, formatGB, formatLoad, formatUptime } from "./system/format";
import { UsageCard, UsageCardSkeleton } from "./system/UsageCard";
import { ErrorBanner, secondaryButton } from "./settings/ui";

/** Refresh period while the tab is visible. */
const REFRESH_MS = 10_000;

/** Current time in ms, ticking every second while the document is visible. Timers are cleaned up. */
function useNow(): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    let timer: ReturnType<typeof setInterval> | null = null;
    const stop = () => {
      if (timer !== null) clearInterval(timer);
      timer = null;
    };
    const start = () => {
      stop();
      setNow(Date.now());
      timer = setInterval(() => setNow(Date.now()), 1000);
    };
    const onVisibility = () => (document.hidden ? stop() : start());
    if (!document.hidden) start();
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      stop();
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, []);
  return now;
}

function StatusBody({ status }: { status: SystemStatus }) {
  const { memory, swap, disk, load_average: load } = status;
  return (
    <div className="space-y-6">
      <div className="grid gap-4 md:grid-cols-3">
        <UsageCard id="cpu" title="CPU" percent={status.cpu_percent} barLabel="Uso de CPU">
          <p>Carga promedio (1, 5 y 15 min): {[load.one, load.five, load.fifteen].map(formatLoad).join(" · ")}</p>
        </UsageCard>

        <UsageCard id="memory" title="Memoria" percent={memory.used_percent} barLabel="Uso de memoria">
          <p>
            {formatGB(memory.used_bytes)} usados de {formatGB(memory.total_bytes)}
          </p>
          <p>{formatGB(memory.available_bytes)} disponibles</p>
          {swap && (
            <p>
              Swap: {formatGB(swap.used_bytes)} de {formatGB(swap.total_bytes)}
            </p>
          )}
        </UsageCard>

        {disk ? (
          <UsageCard id="disk" title="Disco de datos" percent={disk.used_percent} barLabel="Uso del disco de datos">
            <p>
              {formatGB(disk.used_bytes)} usados de {formatGB(disk.total_bytes)}
            </p>
            <p>{formatGB(Math.max(0, disk.total_bytes - disk.used_bytes))} libres</p>
            {status.reminders_enabled && <p>Se envía un aviso por correo al llegar a {status.disk_alert_percent}%</p>}
          </UsageCard>
        ) : (
          <section aria-labelledby="disk-title" data-testid="disk-card" className="min-w-0 rounded-xl border border-border bg-surface p-4 sm:p-5">
            <h2 id="disk-title" className="text-lg font-semibold tracking-tight">
              Disco de datos
            </h2>
            <p className="mt-3 text-sm text-muted">Sin medición del disco en este entorno</p>
          </section>
        )}
      </div>

      <p className="text-sm text-muted">
        Tiempo encendido: <span className="font-medium text-foreground">{formatUptime(status.uptime_seconds)}</span>
      </p>
    </div>
  );
}

/** Sistema: current CPU, memory and data-disk usage of the server. View only. */
export function System() {
  const query = useQuery({
    queryKey: systemKeys.status,
    queryFn: getSystemStatus,
    // react-query pauses the interval while the document is hidden and refetches when it is shown again.
    refetchInterval: REFRESH_MS,
    refetchIntervalInBackground: false,
    retry: false,
  });
  const now = useNow();
  const ageSeconds = query.dataUpdatedAt ? (now - query.dataUpdatedAt) / 1000 : 0;

  return (
    <section aria-labelledby="page-title">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="min-w-0">
          <h1 id="page-title" className="text-2xl font-semibold tracking-tight">
            Sistema
          </h1>
          <p className="mt-1 text-sm text-muted">Uso actual del servidor: procesador, memoria y disco de datos. Solo consulta; los avisos funcionan por separado.</p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          {query.data && (
            <p data-testid="system-age" className="text-sm text-muted">
              {formatAge(ageSeconds)}
            </p>
          )}
          <button type="button" onClick={() => void query.refetch()} disabled={query.isFetching} className={secondaryButton}>
            <RefreshCw className={`h-4 w-4 ${query.isFetching ? "animate-spin motion-reduce:animate-none" : ""}`} aria-hidden="true" />
            Actualizar
          </button>
        </div>
      </div>

      <div className="mt-6 space-y-4">
        {query.isError && <ErrorBanner>{describeSystemError(query.error)}</ErrorBanner>}
        {query.isPending && (
          <div aria-busy="true">
            <p role="status" className="sr-only">
              Cargando estado del servidor…
            </p>
            <div className="grid gap-4 md:grid-cols-3">
              <UsageCardSkeleton />
              <UsageCardSkeleton />
              <UsageCardSkeleton />
            </div>
          </div>
        )}
        {query.data && <StatusBody status={query.data} />}
      </div>
    </section>
  );
}
