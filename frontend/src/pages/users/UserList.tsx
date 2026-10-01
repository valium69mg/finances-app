import { CheckCircle2, CircleSlash, Mail, MailCheck, ShieldCheck, UserCheck, UserX, Users, type LucideIcon } from "lucide-react";
import type { UserAccount } from "../../api/users";
import { EmptyNote } from "../../components/EmptyNote";
import { IconChip } from "../../components/IconChip";
import { dangerButton, secondaryButton } from "../settings/ui";
import { roleLabel } from "./errors";

export type UserAction = "invite" | "deactivate" | "activate";

interface Props {
  users: UserAccount[];
  /** Id of the signed-in owner: that account has no actions (it cannot be deactivated). */
  selfId: string | undefined;
  /** Accounts that already got an invitation in this visit: their button reads "Reenviar". */
  invited: ReadonlySet<string>;
  onAction: (action: UserAction, user: UserAccount) => void;
}

function Badge({ icon: Icon, children, tone }: { icon: LucideIcon; children: string; tone: "ok" | "off" | "neutral" }) {
  const color = tone === "ok" ? "text-accent" : tone === "off" ? "text-destructive" : "text-muted";
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-border px-2.5 py-1 text-xs font-medium">
      <Icon className={`h-3.5 w-3.5 shrink-0 ${color}`} aria-hidden="true" />
      {children}
    </span>
  );
}

/** One card per account: email, role, state and verification, plus the actions that make sense for it. */
export function UserList({ users, selfId, invited, onAction }: Props) {
  if (users.length === 0) return <EmptyNote icon={Users}>Aún no hay usuarios.</EmptyNote>;
  return (
    <ul aria-label="Usuarios" className="space-y-3">
      {users.map((u) => {
        const isSelf = u.id === selfId;
        return (
          <li key={u.id} className={`rounded-xl border p-4 ${u.active ? "border-border" : "border-border bg-background opacity-90"}`}>
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="flex min-w-0 items-center gap-3">
                <IconChip icon={u.role === "owner" ? ShieldCheck : Users} tone={u.active ? "primary" : "expense"} size="sm" />
                <div className="min-w-0">
                  <p className="min-w-0 break-all font-medium">{u.email}</p>
                  {isSelf && <p className="text-xs text-muted">Tu cuenta</p>}
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <Badge icon={u.role === "owner" ? ShieldCheck : Users} tone="neutral">
                  {roleLabel(u.role)}
                </Badge>
                <Badge icon={u.active ? CheckCircle2 : CircleSlash} tone={u.active ? "ok" : "off"}>
                  {u.active ? "Activo" : "Inactivo"}
                </Badge>
                <Badge icon={u.verified ? MailCheck : Mail} tone={u.verified ? "ok" : "neutral"}>
                  {u.verified ? "Verificado" : "Sin verificar"}
                </Badge>
              </div>
            </div>
            {!isSelf && (
              <div className="mt-3 flex flex-wrap gap-2">
                {u.active && !u.verified && (
                  <button type="button" onClick={() => onAction("invite", u)} className={secondaryButton} aria-label={`${invited.has(u.id) ? "Reenviar invitación" : "Enviar invitación"} a ${u.email}`}>
                    <Mail className="h-4 w-4" aria-hidden="true" />
                    {invited.has(u.id) ? "Reenviar invitación" : "Enviar invitación"}
                  </button>
                )}
                {u.active ? (
                  <button type="button" onClick={() => onAction("deactivate", u)} className={dangerButton} aria-label={`Desactivar a ${u.email}`}>
                    <UserX className="h-4 w-4" aria-hidden="true" />
                    Desactivar
                  </button>
                ) : (
                  <button type="button" onClick={() => onAction("activate", u)} className={secondaryButton} aria-label={`Activar a ${u.email}`}>
                    <UserCheck className="h-4 w-4" aria-hidden="true" />
                    Activar
                  </button>
                )}
              </div>
            )}
          </li>
        );
      })}
    </ul>
  );
}
