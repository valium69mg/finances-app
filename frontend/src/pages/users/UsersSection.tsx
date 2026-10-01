import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Loader2 } from "lucide-react";
import { fetchMe } from "../../api/auth";
import { activateUser, deactivateUser, inviteUser, listUsers, usersKeys, type UserAccount } from "../../api/users";
import { InlineFormPanel } from "../../components/InlineFormPanel";
import { ConfirmDialog } from "../monthclose/ConfirmDialog";
import { ErrorBanner, secondaryButton } from "../settings/ui";
import { describeUsersError } from "./errors";
import { UserForm } from "./UserForm";
import { UserList, type UserAction } from "./UserList";

interface Pending {
  action: UserAction;
  user: UserAccount;
}

const TITLES: Record<UserAction, string> = {
  invite: "Enviar invitación",
  deactivate: "Desactivar usuario",
  activate: "Activar usuario",
};

const CONFIRM: Record<UserAction, string> = {
  invite: "Enviar",
  deactivate: "Desactivar",
  activate: "Activar",
};

function ActionText({ pending, resend }: { pending: Pending; resend: boolean }) {
  const email = <strong className="break-all">{pending.user.email}</strong>;
  switch (pending.action) {
    case "invite":
      return (
        <p>
          Se enviará a {email} un correo con un enlace para verificar su correo y elegir su contraseña. El enlace dura 1 hora y solo se puede usar una vez.
          {resend && " Si ya había recibido otro enlace, este lo reemplaza."}
        </p>
      );
    case "deactivate":
      return (
        <p>
          {email} dejará de poder iniciar sesión y se cerrarán sus sesiones abiertas. Podrás activarla de nuevo cuando quieras.
        </p>
      );
    case "activate":
      return <p>{email} podrá iniciar sesión de nuevo. Tendrá que escribir su contraseña otra vez.</p>;
  }
}

/** Configuración > Usuarios (owner only): the accounts, their state and the invitation email. */
export function UsersSection() {
  const qc = useQueryClient();
  const [formOpen, setFormOpen] = useState(false);
  const [pending, setPending] = useState<Pending | null>(null);
  const [invited, setInvited] = useState<ReadonlySet<string>>(new Set());
  const [notice, setNotice] = useState<string | null>(null);
  const users = useQuery({ queryKey: usersKeys.all, queryFn: listUsers, retry: false });
  const me = useQuery({ queryKey: ["me"], queryFn: fetchMe, retry: false, staleTime: 5 * 60_000 });

  const run = useMutation({
    mutationFn: ({ action, user }: Pending) => (action === "invite" ? inviteUser(user.id) : action === "deactivate" ? deactivateUser(user.id) : activateUser(user.id)),
    onSuccess: async (_data, { action, user }) => {
      await qc.invalidateQueries({ queryKey: usersKeys.all });
      setPending(null);
      if (action === "invite") {
        setInvited((cur) => new Set(cur).add(user.id));
        setNotice(`Invitación enviada a ${user.email}. El enlace dura 1 hora.`);
      } else if (action === "deactivate") {
        setNotice(`${user.email} se desactivó y sus sesiones se cerraron.`);
      } else {
        setNotice(`${user.email} está activa de nuevo.`);
      }
    },
  });

  function ask(action: UserAction, user: UserAccount) {
    run.reset();
    setNotice(null);
    setPending({ action, user });
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold tracking-tight">Usuarios</h2>
        <p className="mt-1 text-sm text-muted">
          Las cuentas que pueden entrar a la app. Una cuenta nueva nace sin contraseña: envía la invitación y la persona verifica su correo y elige la suya.
        </p>
      </div>

      <InlineFormPanel id="user-form-panel" label="+ Nuevo usuario" open={formOpen} onToggle={() => setFormOpen((o) => !o)} editing={false}>
        <UserForm
          onCreated={(user) => {
            setFormOpen(false);
            setNotice(`${user.email} se creó sin contraseña. Envía la invitación para que pueda entrar.`);
          }}
        />
      </InlineFormPanel>

      {notice && (
        <p role="status" className="flex items-start gap-2 rounded-lg border border-accent/60 bg-accent/15 px-4 py-3 text-sm">
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
          <span className="min-w-0 break-words">{notice}</span>
        </p>
      )}

      {users.isPending && (
        <p role="status" className="flex items-center gap-2 text-sm text-muted">
          <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
          Cargando usuarios…
        </p>
      )}
      {users.isError && (
        <div className="space-y-3">
          <ErrorBanner>{describeUsersError(users.error)}</ErrorBanner>
          <button type="button" onClick={() => void users.refetch()} className={secondaryButton}>
            Reintentar
          </button>
        </div>
      )}
      {users.data && <UserList users={users.data} selfId={me.data?.id} invited={invited} onAction={ask} />}

      {pending && (
        <ConfirmDialog
          title={TITLES[pending.action]}
          confirmLabel={CONFIRM[pending.action]}
          cancelLabel="Cancelar"
          destructive={pending.action === "deactivate"}
          pending={run.isPending}
          error={run.isError ? describeUsersError(run.error) : null}
          onConfirm={() => run.mutate(pending)}
          onCancel={() => setPending(null)}
        >
          <ActionText pending={pending} resend={invited.has(pending.user.id)} />
        </ConfirmDialog>
      )}
    </div>
  );
}
