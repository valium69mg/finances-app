import { describe, expect, it } from "vitest";
import type { WishItem } from "../../api/wishlist";
import {
  defaultConvertDraft,
  draftOf,
  emptyDraft,
  isStoreUrl,
  toExpenseInput,
  toFutureInput,
  toRequestInput,
  toWishInput,
  validateConversion,
  validateWish,
} from "./form";

const ITEM: WishItem = {
  id: 1,
  name: "Bolsa",
  price: "499.90",
  category_id: 5,
  category: "Ropa",
  store_url: "https://tienda.example/b",
  priority: "alta",
  note: "roja",
  status: "activa",
  conversion: null,
  created_at: "2026-10-01T12:00:00Z",
  updated_at: "2026-10-01T12:00:00Z",
};

describe("isStoreUrl", () => {
  it("accepts http and https links with a host only", () => {
    expect(isStoreUrl("https://tienda.example/a?b=1")).toBe(true);
    expect(isStoreUrl(" http://tienda.example ")).toBe(true);
    for (const bad of ["ftp://x.example", "javascript:alert(1)", "tienda.example", "https://", "https://a b.example", "//x.example"]) {
      expect(isStoreUrl(bad), bad).toBe(false);
    }
  });
});

describe("validateWish", () => {
  const ok = { ...emptyDraft("5"), name: "Bolsa", price: "499.90" };

  it("accepts the minimum and defaults the priority to media", () => {
    expect(validateWish(ok)).toEqual({});
    expect(ok.priority).toBe("media");
  });

  it("reports each invalid field", () => {
    expect(validateWish({ ...ok, name: "  " }).name).toBeTruthy();
    expect(validateWish({ ...ok, name: "a".repeat(121) }).name).toContain("120");
    for (const price of ["", "0", "-5", "1.234", "abc"]) expect(validateWish({ ...ok, price }).price, price).toBeTruthy();
    expect(validateWish({ ...ok, categoryId: "" }).category).toBeTruthy();
    expect(validateWish({ ...ok, storeUrl: "javascript:alert(1)" }).storeUrl).toBeTruthy();
    expect(validateWish({ ...ok, storeUrl: "https://a.example/" + "a".repeat(500) }).storeUrl).toContain("500");
    expect(validateWish({ ...ok, note: "a".repeat(501) }).note).toContain("500");
  });
});

describe("wish input", () => {
  it("omits the empty optional fields and trims", () => {
    expect(toWishInput({ ...emptyDraft("5"), name: " Bolsa ", price: " 10 " })).toEqual({ name: "Bolsa", price: "10", category_id: 5, priority: "media" });
  });

  it("round-trips an item through the draft", () => {
    expect(toWishInput(draftOf(ITEM))).toEqual({
      name: "Bolsa",
      price: "499.90",
      category_id: 5,
      priority: "alta",
      store_url: "https://tienda.example/b",
      note: "roja",
    });
  });
});

describe("conversion drafts", () => {
  const base = defaultConvertDraft(ITEM, "2026-10-10");

  it("defaults amounts to the price, the date to today and the category to the item's", () => {
    expect(base).toMatchObject({ choice: "gasto_futuro", target: "499.90", amount: "499.90", date: "2026-10-10", categoryId: "5", dueDate: "", ownerCategory: "" });
  });

  it("requires a due date for a future expense and nothing of the other ways", () => {
    expect(validateConversion(base).dueDate).toBeTruthy();
    expect(validateConversion({ ...base, dueDate: "2026-12-20" })).toEqual({});
    expect(validateConversion({ ...base, dueDate: "2026-12-20", target: "0" }).target).toBeTruthy();
  });

  it("checks the amount, date and category of a Gasto", () => {
    const g = { ...base, choice: "gasto" as const };
    expect(validateConversion(g)).toEqual({});
    expect(validateConversion({ ...g, amount: "1.234" }).amount).toBeTruthy();
    expect(validateConversion({ ...g, date: "" }).date).toBeTruthy();
    expect(validateConversion({ ...g, categoryId: "" }).category).toBeTruthy();
  });

  it("requires the owner category of a request", () => {
    const r = { ...base, choice: "peticion" as const };
    expect(validateConversion(r).ownerCategory).toBeTruthy();
    expect(validateConversion({ ...r, ownerCategory: "Personal" })).toEqual({});
  });

  it("builds the body of each way", () => {
    expect(toFutureInput({ ...base, dueDate: "2026-12-20" })).toEqual({ target_amount: "499.90", due_date: "2026-12-20" });
    expect(toExpenseInput(base)).toEqual({ amount: "499.90", date: "2026-10-10", category_id: 5 });
    expect(toRequestInput({ ...base, ownerCategory: "Personal" })).toEqual({ amount: "499.90", date: "2026-10-10", category: "Personal" });
  });
});
