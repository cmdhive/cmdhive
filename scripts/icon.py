# Draws Cmdhive's mark: a honeycomb with the terminal at its heart.
# Writes site/logo.svg and site/logo-small.svg. Run: python scripts/icon.py
import math
import pathlib

ACCENT = "#d97757"
ACCENT_LIT = "#e88e6f"
TILE_TOP, TILE_BOT = "#26262d", "#101013"
INK = "#f4f4f5"
SITE = pathlib.Path(__file__).resolve().parent.parent / "site"


def hexagon(cx, cy, r):
    """A flat-top hexagon: the shape a honeycomb cell actually has."""
    pts = []
    for i in range(6):
        a = math.radians(60 * i)
        pts.append((cx + r * math.cos(a), cy + r * math.sin(a)))
    return "M" + " L".join("%.1f %.1f" % p for p in pts) + " Z"


def ring(cx, cy, r, gap):
    """The six cells packed around a centre one."""
    step = r + gap
    h = step * math.sqrt(3) / 2
    return [(cx + 1.5 * step, cy - h), (cx + 1.5 * step, cy + h), (cx, cy + 2 * h),
            (cx - 1.5 * step, cy + h), (cx - 1.5 * step, cy - h), (cx, cy - 2 * h)]


def prompt(cx, cy, r, weight=0.30):
    """The > and _ of a command line, sized from the cell around it."""
    w, arm = r * weight, r * 0.42
    bar_w, bar_h = r * 0.52, r * 0.24
    chevron = "M%.0f %.0f L%.0f %.0f L%.0f %.0f" % (
        cx - arm - r * 0.10, cy - arm, cx + r * 0.06, cy, cx - arm - r * 0.10, cy + arm)
    return ('  <path d="%s" fill="none" stroke="%s" stroke-width="%.0f" '
            'stroke-linecap="round" stroke-linejoin="round"/>\n'
            '  <rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" rx="%.0f" fill="%s"/>'
            ) % (chevron, INK, w, cx + r * 0.18, cy + arm * 0.54, bar_w, bar_h, bar_h / 2.6, INK)


def tile(edge=True):
    out = ['  <rect x="46" y="46" width="932" height="932" rx="208" fill="url(#tile)"/>']
    if edge:
        out.append('  <rect x="47.5" y="47.5" width="929" height="929" rx="206.5" fill="none" '
                   'stroke="#ffffff" stroke-opacity=".07" stroke-width="3"/>')
    return out


def defs():
    return ['  <defs>',
            '    <linearGradient id="tile" x1="0" y1="0" x2="0" y2="1">',
            '      <stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/>' % (TILE_TOP, TILE_BOT),
            '    </linearGradient>',
            '    <linearGradient id="cell" x1="0" y1="0" x2="0.3" y2="1">',
            '      <stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/>' % (ACCENT_LIT, ACCENT),
            '    </linearGradient>',
            '    <radialGradient id="glow" cx="0.5" cy="0.5" r="0.5">',
            '      <stop offset="0" stop-color="%s" stop-opacity=".40"/>' % ACCENT,
            '      <stop offset="1" stop-color="%s" stop-opacity="0"/>' % ACCENT,
            '    </radialGradient>',
            '  </defs>']


def build_full():
    """The hive: six cells around the one the command line lives in."""
    cx = cy = 512
    r, gap = 140, 14
    out = ['<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">']
    out += defs() + tile()
    out.append('  <circle cx="%d" cy="%d" r="430" fill="url(#glow)"/>' % (cx, cy))
    out.append('  <g fill="url(#cell)">')
    for x, y in ring(cx, cy, r, gap):
        out.append('    <path d="%s"/>' % hexagon(x, y, r))
    out.append('  </g>')
    out.append('  <path d="%s" fill="%s"/>' % (hexagon(cx, cy, r + gap / 2), TILE_BOT))
    out.append('  <path d="%s" fill="none" stroke="%s" stroke-opacity=".5" stroke-width="6"/>'
               % (hexagon(cx, cy, r - 5), ACCENT))
    out.append(prompt(cx, cy, r))
    out.append('</svg>')
    (SITE / "logo.svg").write_text("\n".join(out) + "\n", encoding="utf-8")
    print("wrote site/logo.svg")


def build_small():
    """At 16-32 px seven cells are a blur. One cell, one prompt: still a hive,
    still a terminal, and it survives the taskbar."""
    cx = cy = 512
    r = 420
    out = ['<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">']
    out += defs() + tile(edge=False)
    out.append('  <path d="%s" fill="url(#cell)"/>' % hexagon(cx, cy, r))
    out.append('  <path d="%s" fill="%s"/>' % (hexagon(cx, cy, r - 104), TILE_BOT))
    out.append(prompt(cx, cy, r - 104, weight=0.24))
    out.append('</svg>')
    (SITE / "logo-small.svg").write_text("\n".join(out) + "\n", encoding="utf-8")
    print("wrote site/logo-small.svg")


build_full()
build_small()
