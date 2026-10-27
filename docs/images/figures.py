#!/usr/bin/env python3
"""Draws the README figures. Run it from anywhere: python3 docs/images/figures.py"""

import os

HERE = os.path.dirname(os.path.abspath(__file__))

SANS = "ui-sans-serif, -apple-system, 'Segoe UI', Helvetica, Arial, sans-serif"
MONO = "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace"

THEMES = {
    "light": {
        "text": "#1F2328", "muted": "#656D76", "faint": "#AEB6BF", "line": "#D0D7DE",
        "panel": "#F6F8FA", "bg": "#FFFFFF",
        "amber": "#B7791F", "amber_bg": "#FFF6E0",
        "teal": "#1A7F72", "teal_bg": "#E7F5F2",
        "violet": "#7C4DDB", "violet_bg": "#F3EEFF",
        "slate": "#57606A", "slate_bg": "#F6F8FA",
        "pass": "#1A7F37", "pend": "#C69026", "fail": "#CF222E",
    },
    "dark": {
        "text": "#E6EDF3", "muted": "#8D96A0", "faint": "#484F58", "line": "#30363D",
        "panel": "#161B22", "bg": "#0D1117",
        "amber": "#E3A63A", "amber_bg": "#2A2112",
        "teal": "#3FB9A8", "teal_bg": "#0E2724",
        "violet": "#A98BF5", "violet_bg": "#211A36",
        "slate": "#8D96A0", "slate_bg": "#161B22",
        "pass": "#3FB950", "pend": "#D9A23A", "fail": "#F85149",
    },
}

ARROW_COLORS = ["muted", "amber", "teal", "violet", "faint"]


class SVG:
    def __init__(self, w, h, t):
        self.w, self.h, self.t, self.parts = w, h, t, []

    def add(self, s):
        self.parts.append(s)

    def rect(self, x, y, w, h, fill, stroke=None, r=10, sw=1.5, dash=None):
        st = f' stroke="{self.t[stroke]}" stroke-width="{sw}"' if stroke else ""
        if dash:
            st += f' stroke-dasharray="{dash}"'
        self.add(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}" fill="{self.t[fill] if fill else "none"}"{st}/>')

    def text(self, x, y, s, size=14, color="text", weight=400, anchor="start", mono=False):
        fam = MONO if mono else SANS
        s = s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
        self.add(f'<text x="{x}" y="{y}" font-family="{fam}" font-size="{size}" font-weight="{weight}" '
                 f'fill="{self.t[color]}" text-anchor="{anchor}">{s}</text>')

    def path(self, d, color="muted", sw=1.5, dash=None, arrow=False, fill="none"):
        st = f' stroke-dasharray="{dash}"' if dash else ""
        mk = f' marker-end="url(#arrow-{color})"' if arrow else ""
        f = self.t[fill] if fill != "none" else "none"
        self.add(f'<path d="{d}" fill="{f}" stroke="{self.t[color]}" stroke-width="{sw}" '
                 f'stroke-linecap="round" stroke-linejoin="round"{st}{mk}/>')

    def dot(self, x, y, r, color):
        self.add(f'<circle cx="{x}" cy="{y}" r="{r}" fill="{self.t[color]}"/>')

    def ring(self, x, y, r, color, fill="bg", sw=1.5):
        self.add(f'<circle cx="{x}" cy="{y}" r="{r}" fill="{self.t[fill]}" stroke="{self.t[color]}" stroke-width="{sw}"/>')

    # Icons, each drawn in a 16 by 16 box at (x, y).

    def cube(self, x, y, color):
        c = self.t[color]
        self.add(f'<g transform="translate({x},{y})" fill="none" stroke="{c}" stroke-width="1.4" stroke-linejoin="round">'
                 f'<path d="M8 1 L15 4.5 L15 11.5 L8 15 L1 11.5 L1 4.5 Z"/><path d="M1 4.5 L8 8 L15 4.5 M8 8 L8 15"/></g>')

    def doc(self, x, y, color):
        c = self.t[color]
        self.add(f'<g transform="translate({x},{y})" fill="none" stroke="{c}" stroke-width="1.4" stroke-linejoin="round">'
                 f'<path d="M3 1 H10 L13 4 V15 H3 Z"/><path d="M10 1 V4 H13 M5.5 8 H10.5 M5.5 11 H10.5"/></g>')

    def lock(self, x, y, color):
        c = self.t[color]
        self.add(f'<g transform="translate({x},{y})" fill="none" stroke="{c}" stroke-width="1.4" stroke-linejoin="round">'
                 f'<rect x="2.5" y="7" width="11" height="8" rx="1.5"/><path d="M5 7 V5 a3 3 0 0 1 6 0 V7"/></g>')

    def folder(self, x, y, color):
        c = self.t[color]
        self.add(f'<g transform="translate({x},{y})" fill="none" stroke="{c}" stroke-width="1.4" stroke-linejoin="round">'
                 f'<path d="M1 3.5 H6 L7.5 5 H15 V13.5 H1 Z"/></g>')

    def agent(self, x, y, color):
        c = self.t[color]
        self.add(f'<g transform="translate({x},{y})" fill="{c}">'
                 f'<path d="M8 0.5 L9.6 6.4 L15.5 8 L9.6 9.6 L8 15.5 L6.4 9.6 L0.5 8 L6.4 6.4 Z"/></g>')

    def person(self, x, y, color):
        c = self.t[color]
        self.add(f'<g transform="translate({x},{y})" fill="none" stroke="{c}" stroke-width="1.4">'
                 f'<circle cx="8" cy="5" r="3.2"/><path d="M2 15 a6 5.5 0 0 1 12 0"/></g>')

    def render(self):
        defs = "".join(
            f'<marker id="arrow-{c}" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" '
            f'orient="auto-start-reverse"><path d="M1 1 L9 5 L1 9 Z" fill="{self.t[c]}"/></marker>'
            for c in ARROW_COLORS)
        return (f'<svg xmlns="http://www.w3.org/2000/svg" width="{self.w}" height="{self.h}" '
                f'viewBox="0 0 {self.w} {self.h}"><defs>{defs}</defs>' + "".join(self.parts) + "</svg>\n")


def chip(s, x, y, w, h, icon, color, label, tag=None, mono=False):
    s.rect(x, y, w, h, "bg", color, r=7, sw=1)
    getattr(s, icon)(x + 10, y + (h - 16) / 2, color)
    s.text(x + 34, y + h / 2 + 5, label, 13, "text", mono=mono)
    if tag:
        s.text(x + w - 10, y + h / 2 + 4, tag, 11, "muted", anchor="end")


def overview(t):
    s = SVG(960, 486, t)

    s.rect(40, 40, 170, 60, "slate_bg", "slate")
    s.person(58, 52, "slate")
    s.text(84, 65, "You", 15, weight=600)
    s.text(58, 88, "post, message, merge", 12, "muted")

    s.rect(320, 20, 320, 100, "amber_bg", "amber", sw=1.8)
    s.cube(338, 34, "amber")
    s.text(362, 47, "container", 11, "muted")
    s.agent(338, 62, "amber")
    s.text(362, 75, "Orchestration agent", 16, weight=600)
    s.text(338, 103, "plans from every feature map, sees no code", 12.5, "muted")

    s.rect(750, 40, 170, 60, "slate_bg", "slate")
    s.text(768, 65, "Hikari", 15, weight=600)
    s.text(768, 88, "routes, checks, commits", 12, "muted")

    bus = 162
    for x, top in [(125, 100), (480, 120), (835, 100)]:
        s.path(f"M{x} {top} V{bus}", "muted")
    xs = [40, 350, 660]
    for x in xs:
        s.path(f"M{x + 130} {bus} V206", "muted")
    s.path(f"M40 {bus} H920", "muted", sw=2.5)
    s.rect(420, bus - 13, 120, 26, "bg", "muted", r=13, sw=1.2)
    s.text(480, bus + 5, "the room", 13, "muted", 600, "middle")

    parts = [("api", "backend/services/api"), ("web", "frontend/app"), ("billing", "backend/services/billing")]
    for x, (name, folder) in zip(xs, parts):
        y = 206
        s.rect(x, y, 260, 178, "teal_bg", "teal", sw=1.8)
        s.cube(x + 16, y + 14, "teal")
        s.text(x + 40, y + 27, name, 15, weight=700, mono=True)
        s.agent(x + 244 - 16, y + 14, "teal")
        s.text(x + 222, y + 27, "partition agent", 11.5, "muted", anchor="end")
        chip(s, x + 14, y + 46, 232, 34, "doc", "teal", "feature map", "public")
        chip(s, x + 14, y + 88, 232, 34, "lock", "teal", "code map, test map", "private")
        chip(s, x + 14, y + 130, 232, 34, "folder", "teal", folder, mono=True)
        s.path(f"M{x + 130} {y + 178} V418", "violet", dash="3 4")

    s.rect(40, 418, 880, 56, "violet_bg", "violet", sw=1.8)
    s.doc(60, 438, "violet")
    s.text(86, 451, "Shared knowledge", 15, weight=600)
    s.text(236, 451, "public wiki, vocabulary, policies", 13.5, "text")
    s.text(900, 451, "every agent reads it", 12, "muted", anchor="end")
    return s


def context(t):
    s = SVG(960, 468, t)
    s.text(30, 24, "YOUR MACHINE", 11.5, "muted", 700)
    s.text(600, 24, "THE API AGENT'S CONTAINER", 11.5, "muted", 700)

    rows = {}

    def card(y, title, icon, color, items):
        h = 40 + 30 * len(items) + 6
        s.rect(30, y, 360, h, "panel", "line")
        getattr(s, icon)(46, y + 12, "muted")
        s.text(70, y + 25, title, 14, weight=700, mono=True)
        for i, (key, label, c) in enumerate(items):
            ry = y + 40 + 30 * i
            if c:
                s.rect(40, ry, 340, 26, f"{c}_bg", c, r=6, sw=1)
            s.folder(52, ry + 5, c or "faint")
            s.text(76, ry + 18, label, 13, "text" if c else "faint", mono=True)
            if key:
                rows[key] = (380, ry + 13, c)
        return y + h

    y = card(38, "backend", "folder", "muted", [
        ("api", "services/api", "teal"),
        (None, "services/billing", None),
        ("money", "libs/money", "teal"),
    ])
    y = card(y + 14, "frontend", "folder", "muted", [
        (None, "app", None),
        (None, "e2e", None),
    ])
    card(y + 14, "vault", "doc", "muted", [
        ("self", "partitions/api/read-only/", "violet"),
        ("public", "public/", "violet"),
        (None, "partitions/web/", None),
    ])

    cx, cy, cw = 600, 96, 330
    mounts = [
        ("api", "/workspace/backend/services/api", "read-write", "teal"),
        ("api", "/hikari/private", "code map, test map", "teal"),
        ("money", "/workspace/backend/libs/money", "read-only", "teal"),
        ("self", "/hikari/self", "feature map, releases", "violet"),
        ("public", "/hikari/public", "wiki, vocabulary", "violet"),
    ]
    ch = 56 + 46 * len(mounts)
    s.rect(cx, cy, cw, ch, "teal_bg", "teal", sw=1.8)
    s.cube(cx + 16, cy + 16, "teal")
    s.text(cx + 40, cy + 29, "api", 15, weight=700, mono=True)
    s.agent(cx + cw - 32, cy + 16, "teal")
    s.text(cx + cw - 40, cy + 29, "partition agent", 11.5, "muted", anchor="end")
    for i, (key, path, mode, c) in enumerate(mounts):
        my = cy + 50 + 46 * i
        s.rect(cx + 14, my, cw - 28, 38, "bg", c, r=7, sw=1)
        s.text(cx + 26, my + 16, path, 12.5, "text", mono=True)
        s.text(cx + 26, my + 31, mode, 11.5, "muted")
        sx, sy, _ = rows[key]
        ex, ey = cx + 14, my + 19
        s.path(f"M{sx} {sy} C{sx + 110} {sy}, {ex - 110} {ey}, {ex - 4} {ey}", c, sw=1.4, arrow=True)

    s.text(cx + cw / 2, cy + ch + 28, "Nothing else on your machine exists for this agent.", 12.5, "muted", anchor="middle")
    return s


def network(t):
    s = SVG(960, 486, t)
    regions = [("web", 30), ("api", 340), ("billing", 650)]
    for name, x in regions:
        s.rect(x, 178, 280, 262, "teal_bg", "teal", r=14, sw=1.2, dash="5 5")
        s.cube(x + 16, 410, "teal")
        s.text(x + 40, 423, name, 14, weight=700, mono=True)
        s.text(x + 264, 423, "partition", 11.5, "muted", anchor="end")

    nodes = {
        "checkout": (480, 74, "amber", "orchestration feature", "pend"),
        "prices-in-cents": (800, 74, "violet", "rule, public wiki", None),
        "cart-page": (170, 244, "teal", "feature", "pass"),
        "checkout-form": (170, 344, "teal", "feature", "pend"),
        "create-order": (480, 244, "teal", "feature", "pass"),
        "order-api": (480, 344, "teal", "feature", "pass"),
        "charge-card": (790, 244, "teal", "feature", "pend"),
        "refunds": (790, 344, "teal", "feature", None),
    }
    links = [
        ("checkout", "cart-page"), ("checkout", "checkout-form"), ("checkout", "create-order"),
        ("checkout", "charge-card"), ("cart-page", "checkout-form"), ("create-order", "order-api"),
        ("checkout-form", "order-api"), ("prices-in-cents", "create-order"), ("prices-in-cents", "charge-card"),
        ("charge-card", "refunds"),
    ]
    for a, b in links:
        ax, ay = nodes[a][:2]
        bx, by = nodes[b][:2]
        hot = "checkout" in (a, b)
        s.path(f"M{ax} {ay} L{bx} {by}", "amber" if hot else "faint", sw=1.8 if hot else 1.4)

    for name, (x, y, c, kind, health) in nodes.items():
        w, h = (220, 58) if c != "teal" else (200, 50)
        fill = f"{c}_bg" if c != "teal" else "bg"
        s.rect(x - w / 2, y - h / 2, w, h, fill, c, r=9, sw=1.8 if c == "amber" else 1.2)
        s.doc(x - w / 2 + 12, y - 15, c)
        s.text(x - w / 2 + 36, y - 3, f"[[{name}]]", 13, "text", 600, mono=True)
        s.text(x - w / 2 + 36, y + 14, kind, 11.5, "muted")
        if health:
            s.dot(x + w / 2 - 16, y, 5, health)
        elif c == "teal":
            s.ring(x + w / 2 - 16, y, 4.5, "faint")

    s.text(30, 30, "A change to checkout touches", 13, "muted")
    s.text(30, 50, "4 features in 3 partitions.", 13, "text", 600)

    lx = 30
    for label, c in [("passing", "pass"), ("pending", "pend"), ("not linked to checkout", None)]:
        if c:
            s.dot(lx + 5, 468, 5, c)
        else:
            s.ring(lx + 5, 468, 4.5, "faint")
        s.text(lx + 16, 472, label, 12, "muted")
        lx += 16 + len(label) * 7 + 26
    return s


def loops(t):
    s = SVG(960, 410, t)

    def stage(x, title, lines, health=None):
        s.rect(x, 128, 180, 112, "amber_bg", "amber", sw=1.8)
        s.agent(x + 16, 144, "amber")
        s.text(x + 40, 157, title, 15, weight=600)
        for i, ln in enumerate(lines):
            s.text(x + 16, 184 + 18 * i, ln, 12.5, "muted")
        if health:
            s.dot(x + 21, 220, 5, health[0])
            s.text(x + 32, 224, health[1], 12.5, "text", mono=True)

    stage(30, "Plan", ["split the change", "by partition"])
    stage(750, "End to end", ["checkout's health is", "its features' health"], ("pass", "checkout"))

    lanes = [("web", 20), ("api", 132), ("billing", 244)]
    lx, lw = 280, 400
    for name, y in lanes:
        s.rect(lx, y, lw, 96, "teal_bg", "teal", sw=1.6)
        s.cube(lx + 16, y + 16, "teal")
        s.text(lx + 40, y + 29, name, 14, weight=700, mono=True)
        s.text(lx + 16, y + 56, "one context,", 11.5, "muted")
        s.text(lx + 16, y + 72, "until it passes", 11.5, "muted")
        steps = ["edit", "test", "map"]
        sx0, cy = lx + 140, y + 22
        for i, st in enumerate(steps):
            x = sx0 + 84 * i
            s.rect(x, cy, 64, 28, "bg", "teal", r=14, sw=1.2)
            s.text(x + 32, cy + 18, st, 12.5, "text", 600, "middle")
            if i:
                s.path(f"M{x - 20} {cy + 14} H{x - 4}", "teal", arrow=True)
        a, b = sx0 + 32, sx0 + 168 + 32
        s.path(f"M{b} {cy + 28} V{cy + 46} Q{b} {cy + 54} {b - 8} {cy + 54} H{a + 8} "
               f"Q{a} {cy + 54} {a} {cy + 46} V{cy + 32}", "teal", arrow=True)
        s.text(sx0 + 116, cy + 68, "health: failing, pending, passing", 10.5, "muted", anchor="middle")

        s.path(f"M210 184 C245 184, 245 {y + 48}, {lx - 4} {y + 48}", "amber", arrow=True)
        s.path(f"M{lx + lw} {y + 48} C{lx + lw + 35} {y + 48}, {lx + lw + 35} 184, 746 184", "amber", arrow=True)

    s.path("M840 240 V372 Q840 384 828 384 H132 Q120 384 120 372 V244", "amber", dash="4 5", arrow=True)
    s.rect(362, 372, 236, 24, "bg", None)
    s.text(480, 389, "a failing part goes back to its partition", 12.5, "muted", anchor="middle")
    return s


FIGURES = {"overview": overview, "context": context, "network": network, "loops": loops}

if __name__ == "__main__":
    for name, draw in FIGURES.items():
        for theme, tokens in THEMES.items():
            with open(os.path.join(HERE, f"{name}-{theme}.svg"), "w") as f:
                f.write(draw(tokens).render())
