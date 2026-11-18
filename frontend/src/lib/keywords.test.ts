import { describe, expect, it } from "vitest";
import { addKeywords, descriptionAfterPick, keywordOwners, matchKeywords, normalizeKeyword, removeKeyword, splitKeywords } from "./keywords";

describe("normalizeKeyword", () => {
  it("lowercases, trims and strips accents", () => {
    expect(normalizeKeyword("  CAFÉ ")).toBe("cafe");
    expect(normalizeKeyword("Educación")).toBe("educacion");
  });
});

describe("splitKeywords", () => {
  it("splits on commas and newlines, trimming and dropping empties", () => {
    expect(splitKeywords("a, b,, c \n d")).toEqual(["a", "b", "c", "d"]);
    expect(splitKeywords("  ,  ")).toEqual([]);
  });
});

describe("addKeywords", () => {
  it("adds new keywords and ignores duplicates ignoring case and accents", () => {
    const out = addKeywords(["cafe"], "Café, tacos, TACOS, uber");
    expect(out.keywords).toEqual(["cafe", "tacos", "uber"]);
    expect(out.duplicates).toEqual(["Café", "TACOS"]);
  });
  it("keeps the first spelling and does not mutate the input", () => {
    const list = ["Heb"];
    expect(addKeywords(list, "heb").keywords).toEqual(["Heb"]);
    expect(list).toEqual(["Heb"]);
  });
});

describe("removeKeyword", () => {
  it("removes by normalized comparison", () => {
    expect(removeKeyword(["Café", "uber"], "cafe")).toEqual(["uber"]);
  });
});

const owners = [
  { keyword: "heb", category: "Súper" },
  { keyword: "walmart", category: "Súper" },
  { keyword: "uber", category: "Transporte" },
  { keyword: "uber eats", category: "Comida fuera" },
  { keyword: "café", category: "Comida fuera" },
  { keyword: "cafeteria", category: "Ocio" },
];

describe("matchKeywords", () => {
  it("suggests nothing for an empty text", () => {
    expect(matchKeywords("  ", owners)).toEqual([]);
  });
  it("matches by normalized prefix and labels the category", () => {
    const out = matchKeywords("HE", owners);
    expect(out).toEqual([{ keyword: "heb", category: "Súper", label: "heb · Súper" }]);
  });
  it("ignores accents in both directions", () => {
    expect(matchKeywords("cafe", owners).map((s) => s.keyword)).toEqual(["café", "cafeteria"]);
  });
  it("matches a keyword contained in a longer description, prefixes first", () => {
    const out = matchKeywords("compra en walmart", owners).map((s) => s.keyword);
    expect(out).toEqual(["walmart"]);
    expect(matchKeywords("ub", owners).map((s) => s.keyword)).toEqual(["uber", "uber eats"]);
  });
  it("ranks prefix matches above substring matches and caps the list", () => {
    const list = [{ keyword: "superman", category: "A" }, { keyword: "super", category: "B" }, { keyword: "mega super", category: "C" }];
    expect(matchKeywords("super", list).map((s) => s.keyword)).toEqual(["superman", "super", "mega super"]);
    const many = Array.from({ length: 10 }, (_, i) => ({ keyword: `kw${i}`, category: "X" }));
    expect(matchKeywords("kw", many)).toHaveLength(6);
  });
  it("shows a keyword once when legacy data repeats it", () => {
    const out = matchKeywords("pre", [{ keyword: "predial", category: "A" }, { keyword: "Predial", category: "B" }]);
    expect(out).toHaveLength(1);
    expect(out[0].category).toBe("A");
  });
});

describe("descriptionAfterPick", () => {
  it("completes an empty description or a prefix of the keyword", () => {
    expect(descriptionAfterPick("", "heb")).toBe("heb");
    expect(descriptionAfterPick("he", "heb")).toBe("heb");
  });
  it("leaves any other description untouched", () => {
    expect(descriptionAfterPick("compra en heb semanal", "heb")).toBe("compra en heb semanal");
  });
});

describe("keywordOwners", () => {
  it("flattens the categories of one kind", () => {
    const cats = [
      { name: "Súper", kind: "Gasto", keywords: ["heb", "walmart"] },
      { name: "Sueldo", kind: "Ingreso", keywords: ["nomina"] },
    ];
    expect(keywordOwners(cats, "Gasto")).toEqual([
      { keyword: "heb", category: "Súper" },
      { keyword: "walmart", category: "Súper" },
    ]);
  });
});
