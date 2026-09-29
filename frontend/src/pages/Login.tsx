import { useEffect, useReducer, useRef, useState, type FormEvent } from "react";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { MailCheck } from "lucide-react";
import { identify, login } from "../api/auth";
import { useAuth } from "../auth/AuthContext";
import { AuthCard, PasswordField, SubmitButton, TextField, buttonClass, linkClass } from "../components/AuthCard";
import {
  MESSAGES,
  describeError,
  initialLoginFlow,
  loginFlowReducer,
  stepLabel,
  type DescribedError,
  type LoginStage,
} from "./loginFlow";

const SUBTITLES = {
  email: "Ingresa tu correo para continuar.",
  password: "Escribe tu contraseña para entrar.",
  verification_sent: undefined,
} as const;

export function Login() {
  const { isAuthenticated, markLoggedIn } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const from = (location.state as { from?: string } | null)?.from ?? "/";

  const [flow, dispatch] = useReducer(loginFlowReducer, initialLoginFlow);
  const [emailInput, setEmailInput] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<(DescribedError & { stage: LoginStage }) | null>(null);
  const [busy, setBusy] = useState(false);

  const emailRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  // Bumped on every failure so focus returns to the field even when the message is unchanged.
  const [errorTick, setErrorTick] = useState(0);

  // Move focus to where the user must act after each step change.
  useEffect(() => {
    if (flow.step === "email") emailRef.current?.focus();
    else if (flow.step === "password") passwordRef.current?.focus();
    else panelRef.current?.focus();
  }, [flow.step]);

  // Focus management: the first invalid field receives focus after an error.
  useEffect(() => {
    if (!error) return;
    (error.field === "email" ? emailRef : passwordRef).current?.focus();
  }, [error, errorTick]);

  if (isAuthenticated) return <Navigate to={from} replace />;

  function fail(stage: LoginStage, described: DescribedError) {
    setError({ ...described, stage });
    setErrorTick((n) => n + 1);
  }

  async function onIdentify(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const email = emailInput.trim();
    if (!email) return fail("identify", { message: MESSAGES.emailRequired, field: "email" });
    setBusy(true);
    try {
      const status = await identify(email);
      setPassword("");
      dispatch({ type: "identified", email, status });
    } catch (err) {
      fail("identify", describeError("identify", err));
    } finally {
      setBusy(false);
    }
  }

  async function onLogin(e: FormEvent) {
    e.preventDefault();
    setError(null);
    if (!password) return fail("login", { message: MESSAGES.passwordRequired, field: "password" });
    setBusy(true);
    try {
      await login(flow.email, password);
      markLoggedIn();
      navigate(from, { replace: true });
    } catch (err) {
      setBusy(false);
      fail("login", describeError("login", err));
    }
  }

  function restart() {
    setError(null);
    setPassword("");
    setEmailInput(flow.email);
    dispatch({ type: "restart" });
  }

  const counter = stepLabel(flow.step);
  const title = flow.step === "verification_sent" ? "Revisa tu correo" : "Iniciar sesión";

  return (
    <AuthCard title={title} subtitle={SUBTITLES[flow.step]}>
      {counter && (
        <p data-testid="login-step" className="-mt-2 mb-4 text-xs font-medium uppercase tracking-wide text-muted">
          {counter}
        </p>
      )}

      {flow.step === "email" && (
        <form onSubmit={onIdentify} noValidate className="space-y-4">
          <TextField
            label="Correo electrónico"
            type="email"
            name="email"
            inputMode="email"
            required
            autoComplete="username"
            value={emailInput}
            onChange={(e) => setEmailInput(e.target.value)}
            error={error?.stage === "identify" ? error.message : null}
            inputRef={emailRef}
          />
          <SubmitButton busy={busy} busyLabel="Comprobando…">
            Continuar
          </SubmitButton>
        </form>
      )}

      {flow.step === "password" && (
        <form onSubmit={onLogin} noValidate className="space-y-4">
          <div>
            <TextField
              label="Correo electrónico"
              type="email"
              name="email"
              readOnly
              autoComplete="username"
              value={flow.email}
            />
            <button type="button" onClick={restart} className={`${linkClass} mt-1.5 text-sm`}>
              Cambiar correo
            </button>
          </div>
          <PasswordField
            label="Contraseña"
            name="password"
            required
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            error={error?.stage === "login" ? error.message : null}
            inputRef={passwordRef}
          />
          <SubmitButton busy={busy} busyLabel="Entrando…">
            Iniciar sesión
          </SubmitButton>
        </form>
      )}

      {flow.step === "verification_sent" && (
        <div ref={panelRef} tabIndex={-1} role="status" className="focus-ring space-y-4 rounded-lg outline-none">
          <p className="flex items-start gap-2 text-sm">
            <MailCheck className="mt-0.5 h-5 w-5 shrink-0 text-accent" aria-hidden="true" />
            <span>Te enviamos un correo para verificar tu cuenta. Revisa tu bandeja y sigue el enlace.</span>
          </p>
          <button type="button" onClick={restart} className={`${buttonClass} !bg-transparent !text-primary border border-border hover:!bg-background`}>
            Usar otro correo
          </button>
        </div>
      )}
    </AuthCard>
  );
}
