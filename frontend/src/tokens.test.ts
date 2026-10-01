import { describe, expect, it } from "vitest";
import css from "./index.css?raw";

type Rgb = [number, number, number];

/** Token values of one theme block, e.g. the `:root {` block or the explicit dark one. */
function tokensOf(blockStart: string): Record<string, Rgb> {
  const from = css.indexOf(blockStart);
  expect(from).toBeGreaterThanOrEqual(0);
  const body = css.slice(from, css.indexOf("}", from));
  const out: Record<string, Rgb> = {};
  for (const m of body.matchAll(/--color-([a-z-]+):\s*(\d+) (\d+) (\d+);/g)) out[m[1]] = [Number(m[2]), Number(m[3]), Number(m[4])];
  return out;
}

const channel = (c: number) => {
  const v = c / 255;
  return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
};
const luminance = ([r, g, b]: Rgb) => 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
function contrast(a: Rgb, b: Rgb): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}
const mix = (fg: Rgb, bg: Rgb, alpha: number): Rgb => fg.map((v, i) => Math.round(v * alpha + bg[i] * (1 - alpha))) as Rgb;

const themes = {
  light: tokensOf(":root {"),
  "dark (explicit)": tokensOf(':root[data-theme="dark"] {'),
  "dark (prefers-color-scheme)": tokensOf(':root:not([data-theme="light"]) {'),
};
const kinds = ["income", "expense", "saving"] as const;

describe("kind color tokens", () => {
  it("the two dark blocks hold the same values", () => {
    for (const k of kinds) for (const t of [k, `${k}-soft`]) expect(themes["dark (explicit)"][t]).toEqual(themes["dark (prefers-color-scheme)"][t]);
  });

  describe.each(Object.entries(themes))("%s", (_name, t) => {
    it.each(kinds)("%s: strong color reads on surface and background (text, 4.5:1)", (k) => {
      expect(contrast(t[k], t.surface)).toBeGreaterThanOrEqual(4.5);
      expect(contrast(t[k], t.background)).toBeGreaterThanOrEqual(4.5);
    });

    it.each(kinds)("%s: strong color reads on its soft tint, also over the page background (text, 4.5:1)", (k) => {
      expect(contrast(t[k], t[`${k}-soft`])).toBeGreaterThanOrEqual(4.5);
      expect(contrast(t[k], mix(t[k], t.background, 0.1))).toBeGreaterThanOrEqual(4.5);
    });

    it.each(kinds)("%s: foreground and muted text read on the soft tint (4.5:1)", (k) => {
      expect(contrast(t.foreground, t[`${k}-soft`])).toBeGreaterThanOrEqual(4.5);
      expect(contrast(t.muted, t[`${k}-soft`])).toBeGreaterThanOrEqual(4.5);
    });

    it.each(kinds)("%s: soft tint is about 10% of the strong color over the surface", (k) => {
      const expected = mix(t[k], t.surface, 0.1);
      t[`${k}-soft`].forEach((v, i) => expect(Math.abs(v - expected[i])).toBeLessThanOrEqual(2));
    });

    it("primary and the reserved red and amber stay readable as icons (3:1) on the surface", () => {
      for (const k of ["primary", "destructive", "warning"]) expect(contrast(t[k], t.surface)).toBeGreaterThanOrEqual(3);
    });

    it("kind colors stay distinguishable from the reserved destructive and warning tokens", () => {
      for (const k of kinds) for (const r of ["destructive", "warning"]) expect(t[k]).not.toEqual(t[r]);
    });
  });
});
