import { ApiError } from "../../api/client";
import { describeSaveError } from "../settings/ui";

/** Maps a users API failure to Spanish copy; anything unknown falls back to the generic save error. */
export function describeUsersError(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case "invalid_email":
        return "Escribe un correo electrónico válido.";
      case "invalid_role":
        return "Por ahora solo se pueden crear cuentas de familiar.";
      case "email_taken":
        return "Ya existe una cuenta con ese correo.";
      case "cannot_change_self":
        return "No puedes desactivar tu propia cuenta.";
      case "already_verified":
        return "Esta persona ya activó su cuenta: no necesita invitación.";
      case "user_inactive":
        return "La cuenta está desactivada. Actívala para poder invitarla.";
      case "rate_limited":
        return "Se enviaron demasiadas invitaciones. Espera un minuto e intenta de nuevo.";
      case "email_failed":
        return "No se pudo enviar el correo. Intenta de nuevo en unos minutos.";
    }
    if (err.status === 404) return "La cuenta ya no existe. Actualiza la lista e intenta de nuevo.";
  }
  return describeSaveError(err);
}

/** Spanish label of a role. */
export const roleLabel = (role: string): string => (role === "owner" ? "Titular" : role === "household" ? "Familiar" : role);
