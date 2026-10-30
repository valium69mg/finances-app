// Applies the stored theme before the first paint (no flash). Kept external and tiny: the production CSP has no
// 'unsafe-inline'. Mirrors applyTheme in src/theme.ts; the stored value is "light" or "dark", anything else is "auto".
(function () {
  try {
    var pref = window.localStorage.getItem("theme");
    if (pref !== "light" && pref !== "dark") return;
    document.documentElement.setAttribute("data-theme", pref);
    var color = pref === "dark" ? "#0B1120" : "#F8FAFC";
    var metas = document.querySelectorAll('meta[name="theme-color"]');
    for (var i = 0; i < metas.length; i++) {
      metas[i].setAttribute("data-auto-color", metas[i].getAttribute("content"));
      metas[i].setAttribute("content", color);
    }
  } catch (e) {
    // Storage unavailable: stay on the OS scheme.
  }
})();
