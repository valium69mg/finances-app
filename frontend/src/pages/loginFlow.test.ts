import { describe, expect, it } from "vitest";
import { ApiError } from "../api/client";
import {
  MESSAGES,
  describeError,
  initialLoginFlow,
  loginFlowReducer,
  stepLabel,
  type LoginFlowState,
} from "./loginFlow";

describe("loginFlowReducer", () => {
  it("starts on the email step", () => {
    expect(initialLoginFlow).toEqual({ step: "email", email: "" });
  });

  it("moves to the password step for a verified account", () => {
    expect(loginFlowReducer(initialLoginFlow, { type: "identified", email: "a@b.co", status: "password_required" })).toEqual({
      step: "password",
      email: "a@b.co",
    });
  });

  it("moves to the confirmation panel when verification was sent", () => {
    expect(loginFlowReducer(initialLoginFlow, { type: "identified", email: "a@b.co", status: "verification_sent" })).toEqual({
      step: "verification_sent",
      email: "a@b.co",
    });
  });

  it.each<LoginFlowState>([
    { step: "password", email: "a@b.co" },
    { step: "verification_sent", email: "a@b.co" },
  ])("restart returns to step one from %j keeping the email", (state) => {
    expect(loginFlowReducer(state, { type: "restart" })).toEqual({ step: "email", email: "a@b.co" });
  });

  it("does not mutate the previous state", () => {
    const before: LoginFlowState = { step: "password", email: "a@b.co" };
    loginFlowReducer(before, { type: "restart" });
    expect(before).toEqual({ step: "password", email: "a@b.co" });
  });
});

describe("stepLabel", () => {
  it.each([
    ["email", "Paso 1 de 2"],
    ["password", "Paso 2 de 2"],
    ["verification_sent", null],
  ] as const)("%s -> %s", (step, label) => {
    expect(stepLabel(step)).toBe(label);
  });
});

describe("describeError", () => {
  const cases: [string, "identify" | "login", unknown, string, "email" | "password"][] = [
    ["identify 429", "identify", new ApiError(429, "rate_limited"), MESSAGES.rateLimited, "email"],
    ["identify invalid email", "identify", new ApiError(400, "invalid_email"), MESSAGES.invalidEmail, "email"],
    ["identify 500", "identify", new ApiError(500, "internal_error"), MESSAGES.generic, "email"],
    ["identify 401 is not a credential error", "identify", new ApiError(401, "x"), MESSAGES.generic, "email"],
    ["identify network", "identify", new TypeError("Failed to fetch"), MESSAGES.network, "email"],
    ["login 401", "login", new ApiError(401, "invalid_credentials"), MESSAGES.wrongCredentials, "password"],
    ["login 429", "login", new ApiError(429, "rate_limited"), MESSAGES.rateLimited, "password"],
    ["login invalid email focuses the email", "login", new ApiError(400, "invalid_email"), MESSAGES.invalidEmail, "email"],
    ["login 400 other", "login", new ApiError(400, "invalid_request"), MESSAGES.generic, "password"],
    ["login 500", "login", new ApiError(500, "internal_error"), MESSAGES.generic, "password"],
    ["login network", "login", new TypeError("Failed to fetch"), MESSAGES.network, "password"],
    ["login unknown error", "login", new Error("boom"), MESSAGES.generic, "password"],
    ["non-error value", "login", "oops", MESSAGES.generic, "password"],
  ];
  it.each(cases)("%s", (_name, stage, err, message, field) => {
    expect(describeError(stage, err)).toEqual({ message, field });
  });

  it("uses Spanish copy", () => {
    expect(MESSAGES.wrongCredentials).toBe("Correo o contraseña incorrectos.");
    expect(MESSAGES.rateLimited).toBe("Demasiados intentos. Inténtalo de nuevo en unos minutos.");
  });
});
