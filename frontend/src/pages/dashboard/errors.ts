import { describeMovementError } from "../movementErrors";

/**
 * Maps a dashboard API failure to Spanish copy: the backend `message` for
 * invalid_dashboard (a malformed month) and the shared copy for incomplete
 * settings (422 settings_incomplete) and every other failure.
 */
export function describeDashboardError(err: unknown): string {
  return describeMovementError(err, { invalidCode: "invalid_dashboard", noun: "el panel" });
}
