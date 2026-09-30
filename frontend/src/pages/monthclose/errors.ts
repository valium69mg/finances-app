import { ApiError } from "../../api/client";
import { describeSaveError } from "../settings/ui";

/**
 * Maps a month close API failure to Spanish copy. The backend `message` is only
 * appended for the validation code where it says what is wrong; everything else
 * falls back to the generic save error (session, server, network).
 */
export function describeMonthCloseError(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case "invalid_close":
        return `El periodo no es válido: ${err.message}`;
      case "already_closed":
        return "Este mes ya está cerrado. Si necesitas generarlo de nuevo, elimina el cierre guardado y ciérralo otra vez.";
      case "settings_incomplete":
        return "Faltan datos en Configuración para calcular el cierre: los presupuestos, los meses del fondo de emergencia o los rangos de RESICO.";
    }
    if (err.status === 404) return "El cierre ya no existe. Actualiza el historial e intenta de nuevo.";
  }
  return describeSaveError(err);
}
