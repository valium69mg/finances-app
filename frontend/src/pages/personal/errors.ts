import { ApiError } from "../../api/client";
import { describeMovementError } from "../movementErrors";

/** Maps a personal finances API failure to Spanish copy. */
export function describePersonalError(err: unknown, noun = "el movimiento"): string {
  if (err instanceof ApiError) {
    if (err.code === "category_exists") return "Ya tienes una categoría con ese nombre. Elige otro nombre.";
    if (err.code === "category_fixed") return "Esta categoría es fija y no se puede renombrar.";
    if (err.code === "settings_incomplete") return "Falta configurar el periodo de corte. Pide al titular que complete la configuración.";
  }
  return describeMovementError(err, { invalidCode: "invalid_personal", noun });
}
