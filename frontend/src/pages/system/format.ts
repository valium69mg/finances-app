/** Formatting helpers of the Sistema page (pure, unit tested). */

import type { BackupStatus } from "../../api/system";

export type UsageLevel = "normal" | "warning" | "danger";

/** Usage at or above this percentage is a warning. */
export const WARNING_PERCENT = 60;
/** Usage at or above this percentage is critical. */
export const DANGER_PERCENT = 80;

/** Normal below 60, warning from 60 to below 80, danger from 80. */
export function usageLevel(percent: number): UsageLevel {
  if (percent >= DANGER_PERCENT) return "danger";
  if (percent >= WARNING_PERCENT) return "warning";
  return "normal";
}

export const LEVEL_LABEL: Record<UsageLevel, string> = {
  normal: "Normal",
  warning: "Atención",
  danger: "Crítico",
};

const oneDecimal = new Intl.NumberFormat("es-MX", { minimumFractionDigits: 1, maximumFractionDigits: 1 });
const twoDecimals = new Intl.NumberFormat("es-MX", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

const BYTES_PER_GB = 1024 ** 3;

/** Bytes as "1.9 GB" (binary gigabytes, what the VM and the volume are sized in), es-MX. */
export function formatGB(bytes: number): string {
  return `${oneDecimal.format(bytes / BYTES_PER_GB)} GB`;
}

/** A percentage with one decimal, e.g. "23.5%". */
export function formatPercent(percent: number): string {
  return `${oneDecimal.format(percent)}%`;
}

/** A load average with two decimals, e.g. "0.52". */
export function formatLoad(value: number): string {
  return twoDecimals.format(value);
}

/** Clamps a percentage to the 0..100 range of a bar. */
export function clampPercent(percent: number): number {
  if (!Number.isFinite(percent)) return 0;
  return Math.min(100, Math.max(0, percent));
}

/** Uptime as "3 días 4 h", "5 h 12 min" or "8 min". */
export function formatUptime(totalSeconds: number): string {
  const seconds = Math.max(0, Math.floor(totalSeconds));
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days} ${days === 1 ? "día" : "días"} ${hours} h`;
  if (hours > 0) return `${hours} h ${minutes} min`;
  return `${minutes} min`;
}

/** "Actualizado hace 4 s" (or minutes once a minute has passed). */
export function formatAge(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  if (s < 60) return `Actualizado hace ${s} s`;
  return `Actualizado hace ${Math.floor(s / 60)} min`;
}

const backupDate = new Intl.DateTimeFormat("es-MX", {
  timeZone: "America/Mexico_City",
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

export interface BackupLine {
  /** Date and time of the last backup in Mexico City, or a short explanation when there is none. */
  when: string;
  /** Spanish state label. */
  label: string;
  ok: boolean;
}

/** The "Último respaldo" line: when it ran and whether that is acceptable. */
export function describeBackup(backup: BackupStatus): BackupLine {
  const when = backup.last_success_at ? backupDate.format(new Date(backup.last_success_at)) : "Sin registro";
  switch (backup.status) {
    case "ok":
      return { when, label: "Al día", ok: true };
    case "stale":
      return { when, label: `Atrasado (más de ${backup.stale_after_hours} h)`, ok: false };
    case "missing":
      return { when: "Sin registro", label: "Sin respaldo registrado", ok: false };
    default:
      return { when: "Sin registro", label: "No se pudo leer el estado", ok: false };
  }
}
