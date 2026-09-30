import { ApiError } from "../../api/client";
import { describeSaveError } from "../settings/ui";

/** Spanish copy for a failed status read: 503 means the server could not read its own counters. */
export function describeSystemError(err: unknown): string {
  if (err instanceof ApiError && err.status === 503) return "No se pudo leer el estado del servidor";
  return describeSaveError(err);
}
