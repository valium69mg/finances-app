import { describe, expect, it } from "vitest";
import { ApiError } from "../../api/client";
import { describeKeywordInUse, describeSaveError, keywordInUse } from "./ui";

const conflict = new ApiError(409, "keyword_in_use", "keyword", { error: "keyword_in_use", keyword: "heb", category: "Súper" });

describe("keyword_in_use", () => {
  it("reads the keyword and its owner from the error body", () => {
    expect(keywordInUse(conflict)).toEqual({ keyword: "heb", category: "Súper" });
    expect(keywordInUse(new ApiError(409, "other"))).toBeNull();
    expect(keywordInUse(new Error("x"))).toBeNull();
  });

  it("says which category owns the keyword", () => {
    expect(describeKeywordInUse(conflict)).toBe("La palabra clave «heb» ya pertenece a la categoría Súper.");
    expect(describeSaveError(conflict)).toBe("La palabra clave «heb» ya pertenece a la categoría Súper.");
  });

  it("falls back to a generic sentence when the body has no details", () => {
    expect(describeSaveError(new ApiError(409, "keyword_in_use"))).toMatch(/ya pertenece a otra categoría/);
  });
});
