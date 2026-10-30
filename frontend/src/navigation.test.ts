import { describe, expect, it } from "vitest";
import { dockItems, modulesFor, moreModules } from "./navigation";

const labels = (items: { label: string }[]) => items.map((i) => i.label);

describe("dockItems", () => {
  it("owner: Panel, Gastos, Ingresos", () => {
    expect(labels(dockItems("owner"))).toEqual(["Panel", "Gastos", "Ingresos"]);
  });
  it("household: Panel, Mis gastos, Mis ingresos", () => {
    expect(labels(dockItems("household"))).toEqual(["Panel", "Mis gastos", "Mis ingresos"]);
  });
});

describe("moreModules", () => {
  it.each(["owner", "household"] as const)("%s: dock + Más cover every module the role can open, without repeats", (role) => {
    const all = modulesFor(role).map((m) => m.path);
    const shown = [...dockItems(role).map((i) => i.path), ...moreModules(role).map((m) => m.path)];
    expect([...shown].sort()).toEqual([...all].sort());
    expect(new Set(shown).size).toBe(shown.length);
  });
  it("household sees no owner-only module", () => {
    const paths = moreModules("household").map((m) => m.path);
    expect(paths).toEqual(["/peticiones", "/mis-ahorros", "/mis-gastos-futuros", "/lista-de-deseos"]);
  });
  it("owner sees no personal module and keeps Hogar and Configuración", () => {
    const l = labels(moreModules("owner"));
    expect(l).not.toContain("Mis gastos");
    expect(l).toContain("Hogar");
    expect(l).toContain("Configuración");
  });
});
