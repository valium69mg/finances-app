import { ApiError } from "../api/client";
import type { IdentifyStatus } from "../api/auth";

export type LoginStep = "email" | "password" | "verification_sent";

export interface LoginFlowState {
  step: LoginStep;
  /** Trimmed email the user submitted in step one; empty until identified. */
  email: string;
}

export type LoginFlowAction =
  | { type: "identified"; email: string; status: IdentifyStatus }
  | { type: "restart" };

export const initialLoginFlow: LoginFlowState = { step: "email", email: "" };

export function loginFlowReducer(state: LoginFlowState, action: LoginFlowAction): LoginFlowState {
  switch (action.type) {
    case "identified":
      return {
        step: action.status === "password_required" ? "password" : "verification_sent",
        email: action.email,
      };
    case "restart":
      // Keep the email so the user can correct a typo instead of retyping it.
      return { step: "email", email: state.email };
  }
}

/** "Paso N de 2" for the two form steps; the confirmation panel has no counter. */
export function stepLabel(step: LoginStep): string | null {
  if (step === "email") return "Paso 1 de 2";
  if (step === "password") return "Paso 2 de 2";
  return null;
}

/** Which request failed; decides the wording and which field receives focus. */
export type LoginStage = "identify" | "login";

export interface DescribedError {
  message: string;
  /** The first invalid field, which should receive focus. */
  field: "email" | "password";
}

export const MESSAGES = {
  emailRequired: "Ingresa tu correo electrónico.",
  passwordRequired: "Ingresa tu contraseña.",
  invalidEmail: "Ingresa un correo electrónico válido.",
  wrongCredentials: "Correo o contraseña incorrectos.",
  rateLimited: "Demasiados intentos. Inténtalo de nuevo en unos minutos.",
  network: "No se pudo conectar con el servidor. Revisa tu conexión e inténtalo de nuevo.",
  generic: "Algo salió mal. Inténtalo de nuevo.",
} as const;

export function describeError(stage: LoginStage, err: unknown): DescribedError {
  const field = stage === "identify" ? "email" : "password";
  if (err instanceof ApiError) {
    if (err.status === 429) return { message: MESSAGES.rateLimited, field };
    if (err.status === 400 && err.code === "invalid_email") return { message: MESSAGES.invalidEmail, field: "email" };
    if (err.status === 401 && stage === "login") return { message: MESSAGES.wrongCredentials, field };
    return { message: MESSAGES.generic, field };
  }
  // fetch rejects with a TypeError when the network is down or the request is blocked.
  if (err instanceof TypeError) return { message: MESSAGES.network, field };
  return { message: MESSAGES.generic, field };
}
