import { LogOut } from "lucide-react";
import { useAuth } from "../auth/AuthContext";
import { dangerButton, secondaryButton } from "../pages/settings/ui";
import { Modal } from "./Modal";

/** "¿Cerrar sesión?" confirmation: Cancelar has the focus first, so a stray tap or Enter never signs the owner out. */
export function LogoutDialog({ onClose }: { onClose: () => void }) {
  const { logout } = useAuth();
  return (
    <Modal title="¿Cerrar sesión?" onClose={onClose}>
      <p className="text-sm text-muted">Tendrás que volver a iniciar sesión para entrar a tus finanzas.</p>
      <div className="mt-5 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
        <button type="button" data-autofocus onClick={onClose} className={secondaryButton}>
          Cancelar
        </button>
        <button type="button" onClick={() => void logout()} className={dangerButton}>
          <LogOut className="h-4 w-4" aria-hidden="true" />
          Cerrar sesión
        </button>
      </div>
    </Modal>
  );
}
