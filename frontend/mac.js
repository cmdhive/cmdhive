// Mac only (does nothing elsewhere): Cmdhive's icons are Segoe Fluent glyphs, a font
// that macOS does not have, so they are drawn as small SVGs instead; ⌘ stands in
// for Ctrl in the shortcuts.
(() => {
  if (!/Mac/i.test(navigator.platform || navigator.userAgent)) return;
  document.documentElement.classList.add("mac");

  const P = (d) => `<path d="${d}"/>`;
  const circle = (cx, cy, r) => `<circle cx="${cx}" cy="${cy}" r="${r}"/>`;
  const X = P("M3.5 3.5l9 9M12.5 3.5l-9 9");
  const PLUS = P("M8 3v10M3 8h10");
  const CHECKBOX = P("M3 3h10v10H3z") + P("M5.5 8.2l1.8 1.8 3.2-3.4");
  const ICONS = {
    E8BB: X, E711: X, E751: X,
    E921: P("M3 8h10"), E922: P("M3 3h10v10H3z"), E923: P("M5 5V3h8v8h-2") + P("M3 5h8v8H3z"),
    E70F: P("M3 13l.6-2.8 7-7a1.4 1.4 0 012 2l-7 7z"),
    E721: circle(7, 7, 4) + P("M10 10l3.5 3.5"),
    E70D: P("M4 6l4 4 4-4"), E70E: P("M4 10l4-4 4 4"), E76C: P("M6 4l4 4-4 4"),
    E8A5: P("M4 2h5l3 3v9H4z") + P("M9 2v3h3"),
    E774: circle(8, 8, 6) + `<ellipse cx="8" cy="8" rx="2.8" ry="6"/>` + P("M2 8h12"),
    E756: P("M2.5 3.5h11v9h-11z") + P("M5 6.5l2 1.5-2 1.5M8.5 10h2.5"),
    E723: P("M10.5 5.5l-4.2 4.2a1.5 1.5 0 002.1 2.1l4.6-4.6a3 3 0 00-4.2-4.2L4.4 7.6a4.5 4.5 0 006.4 6.4"),
    E90F: circle(8, 8, 5) + circle(8, 8, 1.5),
    E8F1: P("M8 2l1.5 4.5L14 8l-4.5 1.5L8 14 6.5 9.5 2 8l4.5-1.5z"),
    E783: circle(8, 8, 6) + P("M8 7.2v3.8M8 5v.1"),
    E72E: P("M3.5 7.5h9v6h-9z") + P("M5.5 7.5v-2a2.5 2.5 0 015 0v2"),
    E713: circle(8, 8, 2.3) + P("M8 1.8v2M8 12.2v2M1.8 8h2M12.2 8h2M3.6 3.6l1.4 1.4M11 11l1.4 1.4M12.4 3.6L11 5M5 11l-1.4 1.4"),
    E710: PLUS,
    E945: P("M9 2L4 9h4l-1 5 5-7H8z"),
    E930: circle(8, 8, 6) + P("M5.5 8.2l1.8 1.8 3.2-3.4"),
    E8BC: P("M3 4h10M3 8h10M3 12h6"),
    E8A7: P("M6.5 3H3v10h10V9.5M9 3h4v4M13 3L7.5 8.5"),
    E898: P("M8 13V3M4 7l4-4 4 4"),
    E896: P("M8 3v8M4.5 8l3.5 3.5L11.5 8M3 13h10"),
    E7E8: P("M8 2v6M4.8 4.5a5 5 0 106.4 0"),
    E7BA: P("M8 2.5l6 10.5H2z") + P("M8 7v3M8 11.6v.1"),
    E765: P("M2 4.5h12v7H2z") + P("M4.5 7h.1M7 7h.1M9.5 7h.1M12 7h.1M5 9.5h6"),
    E74B: P("M8 3v9M4 8.5l4 4 4-4"),
    E73A: CHECKBOX,
    E72C: P("M13 8a5 5 0 11-1.6-3.7M13 3v3h-3"),
    E72B: P("M10 3L5 8l5 5"), E72A: P("M6 3l5 5-5 5"),
    E724: P("M2 8l12-5-4 11-2.5-4.5z"),
    E71E: circle(7, 7, 4) + P("M10 10l3.5 3.5M5.2 7h3.6M7 5.2v3.6"),
    E71B: P("M6.5 9.5l3-3M7 5l1-1a2.5 2.5 0 013.5 3.5l-1 1M9 11l-1 1A2.5 2.5 0 014.5 8.5l1-1"),
    E718: P("M9 2l5 5-2 1-2 2 .5 3-1 1-2.5-2.5L3 14"),
    E8C8: P("M5.5 5.5h7v8h-7z") + P("M3.5 10.5v-8h7"),
    E81C: circle(8, 8, 6) + P("M8 4.5V8l2.5 1.5"),
    E770: P("M1.5 3h9.5v6.5H1.5z") + P("M4.5 12h5") + P("M11.5 6h3v6.5h-3z"),
    E753: P("M4.5 12h7a2.5 2.5 0 00.2-5A3.5 3.5 0 005 6.6 2.7 2.7 0 004.5 12z"),
    E8B7: P("M1.5 4.5h4l1.5 2h7.5v7h-13z"),
  };
  const svg = (inner) => `<svg class="msvg" viewBox="0 0 16 16" width="1em" height="1em" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round">${inner}</svg>`;
  const glyph = /[-]/;
  const keys = /\bCtrl\+/g;

  function fix(text) {
    const frag = document.createDocumentFragment();
    let rest = text;
    for (let m; (m = glyph.exec(rest)); ) {
      if (m.index) frag.append(rest.slice(0, m.index));
      const code = m[0].codePointAt(0).toString(16).toUpperCase();
      const span = document.createElement("span");
      span.className = "mglyph";
      span.innerHTML = svg(ICONS[code] || circle(8, 8, 1.5));
      frag.append(span);
      rest = rest.slice(m.index + 1);
    }
    if (rest) frag.append(rest);
    return frag;
  }

  function scan(root) {
    if (root.nodeType === Node.TEXT_NODE) return scanText(root);
    if (root.nodeType !== Node.ELEMENT_NODE) return;
    const w = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    const nodes = [];
    for (let n = w.nextNode(); n; n = w.nextNode()) nodes.push(n);
    nodes.forEach(scanText);
    root.querySelectorAll?.("[title],[placeholder]").forEach(scanAttrs);
    scanAttrs(root);
  }
  function scanText(n) {
    const t = n.nodeValue;
    if (!t || n.parentNode?.closest?.("script,style,textarea,.xterm")) return;
    if (glyph.test(t)) n.replaceWith(fix(t));
    else if (keys.test(t)) { keys.lastIndex = 0; n.nodeValue = t.replace(keys, "⌘"); }
    keys.lastIndex = 0;
  }
  function scanAttrs(el) {
    for (const a of ["title", "placeholder"]) {
      const v = el.getAttribute?.(a);
      if (v && v.includes("Ctrl+")) el.setAttribute(a, v.replace(keys, "⌘"));
    }
  }

  // Cmd does what Ctrl does in Cmdhive's shortcuts, except where the Mac has its
  // own meaning (copy, paste, cut, select all, undo), and never in the terminal,
  // where Ctrl stays Ctrl.
  window.addEventListener("keydown", (e) => {
    if (!e.metaKey || e.ctrlKey || e.altKey) return;
    if (/^[cvxaz]$/i.test(e.key) || e.target.closest?.(".xterm")) return;
    Object.defineProperty(e, "ctrlKey", { get: () => true });
    Object.defineProperty(e, "metaKey", { get: () => false });
  }, true);

  const start = () => {
    scan(document.body);
    new MutationObserver((muts) => {
      for (const m of muts) {
        if (m.type === "attributes") scanAttrs(m.target);
        else if (m.type === "characterData") scan(m.target);
        else m.addedNodes.forEach(scan);
      }
    }).observe(document.body, { childList: true, subtree: true, characterData: true, attributes: true, attributeFilter: ["title", "placeholder"] });
  };
  if (document.body) start(); else document.addEventListener("DOMContentLoaded", start);
})();
