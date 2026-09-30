import { describe, expect, it } from "vitest";
import { ApiError } from "../api/client";
import { describeExpenseError } from "./expenses/errors";
import { describeIncomeError } from "./income/errors";
import { describeMovementError } from "./movementErrors";
import { describeSavingsError, describeValuationError } from "./savings/errors";

describe("describeMovementError", () => {
  const opts = { invalidCode: "invalid_thing", noun: "la cosa" };

  it("shows the backend message for the module's validation code", () => {
    expect(describeMovementError(new ApiError(400, "invalid_thing", "monto inválido"), opts)).toBe("Los datos no son válidos: monto inválido");
  });

  it("names the noun on a 404", () => {
    expect(describeMovementError(new ApiError(404, "not_found"), opts)).toBe("La cosa ya no existe. Actualiza la lista e intenta de nuevo.");
  });

  it("falls back to the generic save errors", () => {
    expect(describeMovementError(new ApiError(500, "internal_error"), opts)).toContain("El servidor tuvo un problema");
    expect(describeMovementError(new ApiError(400, "other_code", "x"), opts)).toContain("No se pudo completar");
    expect(describeMovementError(new TypeError("offline"), opts)).toContain("No se pudo conectar");
  });
});

describe("module wrappers", () => {
  it("each module only reacts to its own validation code", () => {
    expect(describeExpenseError(new ApiError(400, "invalid_expense", "m"))).toContain("Los datos no son válidos");
    expect(describeIncomeError(new ApiError(400, "invalid_income", "m"))).toContain("Los datos no son válidos");
    expect(describeSavingsError(new ApiError(400, "invalid_saving", "m"))).toContain("Los datos no son válidos");
    expect(describeSavingsError(new ApiError(400, "invalid_valuation", "m"))).not.toContain("Los datos no son válidos");
    expect(describeValuationError(new ApiError(400, "invalid_valuation", "m"))).toContain("Los datos no son válidos");
  });

  it("never blames the saving for a valuation failure", () => {
    expect(describeValuationError(new ApiError(404, "not_found"))).toBe("La valuación ya no existe. Actualiza la lista e intenta de nuevo.");
    expect(describeValuationError(new ApiError(404, "not_found"))).not.toContain("ahorro");
    expect(describeSavingsError(new ApiError(404, "not_found"))).toContain("El ahorro ya no existe");
    expect(describeExpenseError(new ApiError(404, "not_found"))).toContain("El gasto ya no existe");
    expect(describeIncomeError(new ApiError(404, "not_found"))).toContain("El ingreso ya no existe");
  });
});
