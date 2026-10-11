import { useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { CheckCircle2 } from "lucide-react";
import { ApiError } from "../api/client";
import { resetPassword, verifyEmail } from "../api/auth";
import { AuthCard, FieldError, PasswordField, SubmitButton, buttonClass, linkClass } from "../components/AuthCard";

const MIN_LENGTH = 12;
const MAX_BYTES = 72;

interface Errors {
  password?: string;
  confirm?: string;
  form?: string;
}

/** The two flows share the form and the password rules; only the call and the copy differ. */
interface Flow {
  complete: (token: string, password: string) => Promise<void>;
  title: string;
  doneTitle: string;
  subtitle: string;
  doneMessage: string;
  badLink: string;
  expired: string;
  failed: string;
  submit: string;
}

const VERIFY: Flow = {
  complete: verifyEmail,
  title: "Verificar cuenta",
  doneTitle: "Cuenta verificada",
  subtitle: "Crea la contraseña con la que iniciarás sesión.",
  doneMessage: "Tu contraseña fue actualizada. Ya puedes iniciar sesión.",
  badLink: "El enlace de verificación no es válido.",
  expired: "El enlace no es válido o ya expiró. Solicita uno nuevo desde el inicio de sesión.",
  failed: "No se pudo completar la verificación. Inténtalo de nuevo.",
  submit: "Guardar contraseña",
};

const RESET: Flow = {
  complete: resetPassword,
  title: "Restablecer contraseña",
  doneTitle: "Contraseña restablecida",
  subtitle: "Elige la nueva contraseña con la que iniciarás sesión.",
  doneMessage: "Tu contraseña fue actualizada y se cerraron tus sesiones abiertas. Ya puedes iniciar sesión.",
  badLink: "El enlace para restablecer la contraseña no es válido.",
  expired: "El enlace no es válido o ya expiró. Solicita uno nuevo desde el inicio de sesión con «¿Olvidaste tu contraseña?».",
  failed: "No se pudo restablecer la contraseña. Inténtalo de nuevo.",
  submit: "Guardar contraseña",
};

function errorMessage(flow: Flow, err: unknown): Errors {
  if (err instanceof ApiError) {
    if (err.code === "invalid_token") return { form: flow.expired };
    if (err.code === "weak_password") return { password: `La contraseña debe tener al menos ${MIN_LENGTH} caracteres.` };
  }
  return { form: flow.failed };
}

/** Page behind the verification and invitation email link (`/verify?token=`). */
export function VerifyEmail() {
  return <SetPasswordPage flow={VERIFY} />;
}

/** Page behind the password reset email link (`/restablecer?token=`). */
export function ResetPassword() {
  return <SetPasswordPage flow={RESET} />;
}

function SetPasswordPage({ flow }: { flow: Flow }) {
  const [params] = useSearchParams();
  const token = params.get("token");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [errors, setErrors] = useState<Errors>({});
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);

  if (!token) {
    return (
      <AuthCard title={flow.title}>
        <FieldError id="verify-token-error">{flow.badLink}</FieldError>
        <Link to="/login" className={`${linkClass} mt-4 inline-flex min-h-11 items-center`}>
          Ir a iniciar sesión
        </Link>
      </AuthCard>
    );
  }

  if (done) {
    return (
      <AuthCard title={flow.doneTitle}>
        <p role="status" className="flex items-start gap-2 text-sm text-muted">
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
          <span>{flow.doneMessage}</span>
        </p>
        <Link to="/login" className={`${buttonClass} mt-6`}>
          Ir a iniciar sesión
        </Link>
      </AuthCard>
    );
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setErrors({});
    if (password.length < MIN_LENGTH) {
      setErrors({ password: `La contraseña debe tener al menos ${MIN_LENGTH} caracteres.` });
      return;
    }
    if (new TextEncoder().encode(password).length > MAX_BYTES) {
      setErrors({ password: `La contraseña no puede superar ${MAX_BYTES} bytes.` });
      return;
    }
    if (password !== confirm) {
      setErrors({ confirm: "Las contraseñas no coinciden." });
      return;
    }
    setBusy(true);
    try {
      await flow.complete(token as string, password);
      setDone(true);
    } catch (err) {
      setErrors(errorMessage(flow, err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthCard title={flow.title} subtitle={flow.subtitle}>
      <form onSubmit={onSubmit} noValidate className="space-y-4">
        <PasswordField
          label="Nueva contraseña"
          required
          autoComplete="new-password"
          hint={`Mínimo ${MIN_LENGTH} caracteres.`}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          error={errors.password}
        />
        <PasswordField
          label="Confirmar contraseña"
          required
          autoComplete="new-password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          error={errors.confirm}
        />
        {errors.form && <FieldError id="verify-form-error">{errors.form}</FieldError>}
        <SubmitButton busy={busy} busyLabel="Guardando…">
          {flow.submit}
        </SubmitButton>
      </form>
    </AuthCard>
  );
}
