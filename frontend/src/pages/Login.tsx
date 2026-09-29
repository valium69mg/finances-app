import { useState, type FormEvent } from "react";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { CheckCircle2 } from "lucide-react";
import { ApiError } from "../api/client";
import { login } from "../api/auth";
import { useAuth } from "../auth/AuthContext";
import { AuthCard, FieldError, PasswordField, SubmitButton, TextField } from "../components/AuthCard";

function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 401) return "Correo o contraseña incorrectos.";
    if (err.status === 429) return "Demasiados intentos. Inténtalo de nuevo en unos minutos.";
  }
  return "No se pudo iniciar sesión. Inténtalo de nuevo.";
}

export function Login() {
  const { isAuthenticated, markLoggedIn } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const from = (location.state as { from?: string } | null)?.from ?? "/";
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  if (isAuthenticated) return <Navigate to={from} replace />;

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setNotice(null);
    setBusy(true);
    try {
      const result = await login(email, password);
      if (result.kind === "session") {
        markLoggedIn();
        navigate(from, { replace: true });
      } else {
        setNotice("Si el correo es válido, te enviamos un enlace para verificar tu cuenta");
      }
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthCard title="Iniciar sesión" subtitle="Accede a tus finanzas con tu correo y contraseña.">
      <form onSubmit={onSubmit} className="space-y-4">
        <TextField
          label="Correo electrónico"
          type="email"
          inputMode="email"
          required
          autoComplete="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        <div>
          <PasswordField
            label="Contraseña"
            required
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          {error && <FieldError id="login-error">{error}</FieldError>}
        </div>
        {notice && (
          <p role="status" className="flex items-start gap-1.5 text-sm text-muted">
            <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
            <span>{notice}</span>
          </p>
        )}
        <SubmitButton busy={busy} busyLabel="Entrando…">
          Entrar
        </SubmitButton>
      </form>
    </AuthCard>
  );
}
