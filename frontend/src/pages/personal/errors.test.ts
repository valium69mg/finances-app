import { describe, expect, it } from "vitest";
import { ApiError } from "../../api/client";
import { describePersonalError } from "./errors";

const apiError = (status: number, code: string, message = "") => new ApiError(status, code, message);

describe("describePersonalError", () => {
  it("explains a duplicate and a fixed category", () => {
    expect(describePersonalError(apiError(409, "category_exists"))).toMatch(/Ya tienes una categoría/);
    expect(describePersonalError(apiError(409, "category_fixed"))).toMatch(/fija/);
  });
  it("shows the backend message for invalid_personal", () => {
    expect(describePersonalError(apiError(400, "invalid_personal", "amount is required"))).toBe("Los datos no son válidos: amount is required");
  });
  it("falls back to the generic copy", () => {
    expect(describePersonalError(apiError(403, "forbidden"))).toMatch(/no tiene permiso/);
    expect(describePersonalError(new Error("x"))).toMatch(/No se pudo conectar/);
  });
});
