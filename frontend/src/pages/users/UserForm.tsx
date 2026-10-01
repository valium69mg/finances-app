import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, UserPlus } from "lucide-react";
import { createUser, usersKeys, type UserAccount } from "../../api/users";
import { TextField } from "../../components/AuthCard";
import { SelectField } from "../expenses/SelectField";
import { ErrorBanner, primaryButton } from "../settings/ui";
import { describeUsersError } from "./errors";
import { validateNewUser, type UserErrors } from "./form";

interface Props {
  onCreated: (user: UserAccount) => void;
}

/** Create form: the email and the role. Only household accounts can be created for now. */
export function UserForm({ onCreated }: Props) {
  const qc = useQueryClient();
  const [email, setEmail] = useState("");
  const [errors, setErrors] = useState<UserErrors>({});
  const emailRef = useRef<HTMLInputElement>(null);

  const save = useMutation({
    mutationFn: () => createUser({ email: email.trim(), role: "household" }),
    onSuccess: async (user) => {
      await qc.invalidateQueries({ queryKey: usersKeys.all });
      setEmail("");
      onCreated(user);
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    save.reset();
    const found = validateNewUser(email);
    setErrors(found);
    if (found.email) {
      emailRef.current?.focus();
      return;
    }
    save.mutate();
  }

  return (
    <form noValidate onSubmit={onSubmit} aria-labelledby="user-form-title" className="space-y-5">
      <h3 id="user-form-title" className="text-base font-semibold tracking-tight">
        Nuevo usuario
      </h3>
      <div className="grid gap-4 sm:grid-cols-2">
        <TextField
          label="Correo electrónico"
          type="email"
          inputRef={emailRef}
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          error={errors.email}
          autoComplete="off"
          hint="La cuenta se crea sin contraseña. Después envía la invitación para que la persona elija la suya."
        />
        <SelectField label="Rol" value="household" onChange={() => {}} hint="Un familiar solo ve el presupuesto por categoría.">
          <option value="household">Familiar</option>
        </SelectField>
      </div>
      {save.isError && <ErrorBanner>{describeUsersError(save.error)}</ErrorBanner>}
      <button type="submit" disabled={save.isPending} aria-busy={save.isPending} className={primaryButton}>
        {save.isPending ? <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <UserPlus className="h-4 w-4" aria-hidden="true" />}
        {save.isPending ? "Creando…" : "Crear usuario"}
      </button>
    </form>
  );
}
