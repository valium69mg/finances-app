import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { THEME_KEY, applyTheme, initTheme, readTheme, resetThemeCache, setTheme } from "./theme";

function addMetas() {
  document.head.innerHTML =
    '<meta name="theme-color" content="#F8FAFC" media="(prefers-color-scheme: light)"><meta name="theme-color" content="#0B1120" media="(prefers-color-scheme: dark)">';
}
const metaContents = () => [...document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')].map((m) => m.content);

beforeEach(() => {
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
  addMetas();
  resetThemeCache();
});
afterEach(() => vi.restoreAllMocks());

describe("readTheme", () => {
  it("defaults to auto", () => {
    expect(readTheme()).toBe("auto");
  });
  it("reads a stored light or dark", () => {
    window.localStorage.setItem(THEME_KEY, "dark");
    expect(readTheme()).toBe("dark");
    window.localStorage.setItem(THEME_KEY, "light");
    expect(readTheme()).toBe("light");
  });
  it("treats an unknown value as auto", () => {
    window.localStorage.setItem(THEME_KEY, "sepia");
    expect(readTheme()).toBe("auto");
  });
  it("survives unreadable storage", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    expect(readTheme()).toBe("auto");
  });
});

describe("applyTheme", () => {
  it("sets data-theme and forces the theme-color of both metas", () => {
    applyTheme("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(metaContents()).toEqual(["#0B1120", "#0B1120"]);
    applyTheme("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
    expect(metaContents()).toEqual(["#F8FAFC", "#F8FAFC"]);
  });
  it("auto removes the attribute and restores the original colors", () => {
    applyTheme("dark");
    applyTheme("auto");
    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
    expect(metaContents()).toEqual(["#F8FAFC", "#0B1120"]);
  });
});

describe("setTheme / initTheme", () => {
  it("persists a forced theme and applies it", () => {
    setTheme("dark");
    expect(window.localStorage.getItem(THEME_KEY)).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });
  it("auto clears the stored value", () => {
    setTheme("dark");
    setTheme("auto");
    expect(window.localStorage.getItem(THEME_KEY)).toBeNull();
    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
  });
  it("still applies the theme when storage throws", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("full");
    });
    setTheme("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });
  it("initTheme applies the stored preference", () => {
    window.localStorage.setItem(THEME_KEY, "dark");
    initTheme();
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });
});
