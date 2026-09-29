import { useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { CheckCircle2 } from "lucide-react";
import { ApiError } from "../api/client";
import { verifyEmail } from "../api/auth";
import { AuthCard, FieldError, PasswordField, SubmitButton, buttonClass, linkClass } from "../components/AuthCard";

const MIN_LENGTH = 12;
const MAX_BYTES = 72;

interface Errors {
  password?: string;
  confirm?: string;
  form?: string;
}

function errorMessage(err: unknown): Errors {
  if (err instanceof ApiError) {
    if (err.code === "invalid_token") {
      return { form: "El enlace no es válido o ya expiró. Solicita uno nuevo desde el inicio de sesión." };
    }
    if (err.code === "weak_password") return { password: `La contraseña debe tener al menos ${MIN_LENGTH} caracteres.` };
  }
  return { form: "No se pudo completar la verificación. Inténtalo de nuevo." };
}

export function VerifyEmail() {
  const [params] = useSearchParams();
  const token = params.get("token");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [errors, setErrors] = useState<Errors>({});
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);

  if (!token) {
    return (
      <AuthCard title="Verificar cuenta">
        <FieldError id="verify-token-error">El enlace de verificación no es válido.</FieldError>
        <Link to="/login" className={`${linkClass} mt-4 inline-flex min-h-11 items-center`}>
          Ir a iniciar sesión
        </Link>
      </AuthCard>
    );
  }

  if (done) {
    return (
      <AuthCard title="Cuenta verificada">
        <p role="status" className="flex items-start gap-2 text-sm text-muted">
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
          <span>Tu contraseña fue actualizada. Ya puedes iniciar sesión.</span>
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
      await verifyEmail(token as string, password);
      setDone(true);
    } catch (err) {
      setErrors(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthCard title="Verificar cuenta" subtitle="Crea la contraseña con la que iniciarás sesión.">
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
          Guardar contraseña
        </SubmitButton>
      </form>
    </AuthCard>
  );
}
