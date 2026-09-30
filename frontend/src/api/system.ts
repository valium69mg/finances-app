import { api } from "./client";

/**
 * System status API (read-only). Percentages and bytes are plain numbers; the
 * figures describe the whole server, sampled on each request.
 */

export interface MemoryStatus {
  total_bytes: number;
  used_bytes: number;
  available_bytes: number;
  used_percent: number;
}

export interface SwapStatus {
  total_bytes: number;
  used_bytes: number;
}

export interface DiskStatus {
  total_bytes: number;
  used_bytes: number;
  used_percent: number;
  path_monitored: boolean;
}

export interface SystemStatus {
  /** 0..100, one decimal. */
  cpu_percent: number;
  memory: MemoryStatus;
  /** Null when the machine has no swap. */
  swap: SwapStatus | null;
  /** Null when the monitored path cannot be measured (for example in local development). */
  disk: DiskStatus | null;
  uptime_seconds: number;
  load_average: { one: number; five: number; fifteen: number };
  /** Used-space percentage of the disk that triggers the e-mail alert. */
  disk_alert_percent: number;
  reminders_enabled: boolean;
  /** RFC 3339 timestamp taken by the server. */
  sampled_at: string;
}

export const systemKeys = {
  status: ["system", "status"] as const,
};

type Client = Pick<typeof api, "request">;

export function createSystemApi(client: Client = api) {
  return {
    status: () => client.request<SystemStatus>("/system/status"),
  };
}

const defaultApi = createSystemApi();

export const getSystemStatus = defaultApi.status;
