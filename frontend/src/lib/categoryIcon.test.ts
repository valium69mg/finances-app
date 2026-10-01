import { Car, Gift, GraduationCap, HeartPulse, House, PawPrint, Plane, Shirt, ShoppingCart, Smartphone, Sparkles, Utensils, Wrench, Zap } from "lucide-react";
import { describe, expect, it } from "vitest";
import { categoryIcon, normalizeCategory } from "./categoryIcon";

describe("categoryIcon", () => {
  it.each([
    ["Comida fuera", Utensils],
    ["Restaurantes", Utensils],
    ["Supermercado", ShoppingCart],
    ["Despensa", ShoppingCart],
    ["Mandado", ShoppingCart],
    ["Transporte", Car],
    ["Auto", Car],
    ["Gasolina", Car],
    ["Casa", House],
    ["Renta", House],
    ["Hogar", House],
    ["Vivienda", House],
    ["Servicios", Zap],
    ["Luz", Zap],
    ["Salud", HeartPulse],
    ["Gastos médicos", HeartPulse],
    ["Ropa", Shirt],
    ["Viajes", Plane],
    ["Celular", Smartphone],
    ["Suscripciones", Smartphone],
    ["Teléfono", Smartphone],
    ["Regalos", Gift],
    ["Educación", GraduationCap],
    ["Cursos", GraduationCap],
    ["Mantenimiento", Wrench],
    ["Mascota", PawPrint],
  ])("maps %s", (name, icon) => {
    expect(categoryIcon(name)).toBe(icon);
  });

  it("ignores case and accents", () => {
    expect(categoryIcon("EDUCACIÓN")).toBe(GraduationCap);
    expect(categoryIcon("educacion")).toBe(GraduationCap);
    expect(categoryIcon("TELÉFONO")).toBe(Smartphone);
  });

  it("falls back to sparkles for unknown, empty and keyword-inside-a-word names", () => {
    expect(categoryIcon("Otros")).toBe(Sparkles);
    expect(categoryIcon("")).toBe(Sparkles);
    expect(categoryIcon("Automático")).toBe(Sparkles);
  });

  it("normalizes names", () => {
    expect(normalizeCategory("Educación Ñandú")).toBe("educacion nandu");
  });
});
