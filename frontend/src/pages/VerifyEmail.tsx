import { useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { ApiError } from "../api/client";
import { verifyEmail } from "../api/auth";
import { AuthCard, buttonClass, inputClass } from "../components/AuthCard";

const MIN_LENGTH = 12;
const MAX_BYTES = 72;

function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === "invalid_token") return "El enlace no es válido o ya expiró. Solicita uno nuevo desde el inicio de sesión.";
    if (err.code === "weak_password") return `La contraseña debe tener al menos ${MIN_LENGTH} caracteres.`;
  }
  return "No se pudo completar la verificación. Inténtalo de nuevo.";
}

export function VerifyEmail() {
  const [params] = useSearchParams();
  const token = params.get("token");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);

  if (!token) {
    return (
      <AuthCard title="Verificar cuenta">
        <p role="alert" className="text-sm text-red-700">El enlace de verificación no es válido.</p>
        <Link to="/login" className="mt-4 block text-sm text-indigo-700 underline">Ir a iniciar sesión</Link>
      </AuthCard>
    );
  }

  if (done) {
    return (
      <AuthCard title="Cuenta verificada">
        <p role="status" className="text-sm text-slate-700">Tu contraseña fue actualizada. Ya puedes iniciar sesión.</p>
        <Link to="/login" className="mt-4 block text-sm text-indigo-700 underline">Ir a iniciar sesión</Link>
      </AuthCard>
    );
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    if (password.length < MIN_LENGTH) {
      setError(`La contraseña debe tener al menos ${MIN_LENGTH} caracteres.`);
      return;
    }
    if (new TextEncoder().encode(password).length > MAX_BYTES) {
      setError(`La contraseña no puede superar ${MAX_BYTES} bytes.`);
      return;
    }
    if (password !== confirm) {
      setError("Las contraseñas no coinciden.");
      return;
    }
    setBusy(true);
    try {
      await verifyEmail(token as string, password);
      setDone(true);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthCard title="Verificar cuenta">
      <form onSubmit={onSubmit} className="space-y-4">
        <label className="block text-sm font-medium text-slate-700">
          Nueva contraseña
          <input type="password" required minLength={MIN_LENGTH} autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} className={inputClass} />
        </label>
        <label className="block text-sm font-medium text-slate-700">
          Confirmar contraseña
          <input type="password" required autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} className={inputClass} />
        </label>
        {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
        <button type="submit" disabled={busy} className={buttonClass}>
          {busy ? "Guardando…" : "Guardar contraseña"}
        </button>
      </form>
    </AuthCard>
  );
}
