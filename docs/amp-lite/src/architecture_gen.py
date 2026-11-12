#!/usr/bin/env python3
"""Generate the AMP Lite architecture diagram as draw.io XML and an HTML render.

Usage: python3 architecture_gen.py <out_dir>
Logos are base64 SVGs in logos.json next to this script.
"""
import base64
import json
import os
import html
import re
import sys

OUT_DIR = sys.argv[1]
IMG = json.load(open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "logos.json")))

STACK_ICON = base64.b64encode(b'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g fill="none" stroke="#1D63ED" stroke-width="1.8"><rect x="3" y="13" width="5" height="5"/><rect x="9.5" y="13" width="5" height="5"/><rect x="16" y="13" width="5" height="5"/><rect x="6.2" y="6.5" width="5" height="5"/><rect x="12.8" y="6.5" width="5" height="5"/></g></svg>''').decode()
DB_ICON = base64.b64encode(b'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g fill="none" stroke="#336791" stroke-width="2"><ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v14c0 1.7 3.6 3 8 3s8-1.3 8-3V5"/><path d="M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/></g></svg>''').decode()

W, H = 2330, 1090
FONT = "Helvetica"
cells = []   # (kind, dict)

def box(id, x, y, w, h, stroke, fill="#FFFFFF", arc=3, sw=1.5, dashed=False, dash="6 4"):
    cells.append(("box", dict(id=id, x=x, y=y, w=w, h=h, stroke=stroke, fill=fill, arc=arc, sw=sw, dashed=dashed, dash=dash)))

def text(id, x, y, w, h, value, size=11.5, color="#475569", bold=False, italic=False, align="left", valign="top", fill=None):
    cells.append(("text", dict(id=id, x=x, y=y, w=w, h=h, value=value, size=size, color=color, bold=bold, italic=italic, align=align, valign=valign, fill=fill)))

def image(id, x, y, w, h, b64):
    cells.append(("image", dict(id=id, x=x, y=y, w=w, h=h, b64=b64)))

def pill(id, x, y, w, label, fill="#334155", h=18):
    cells.append(("pill", dict(id=id, x=x, y=y, w=w, h=h, label=label, fill=fill)))

def sub(id, x, y, w, h, value, stroke, fill, color="#0f172a", dashed=False):
    cells.append(("sub", dict(id=id, x=x, y=y, w=w, h=h, value=value, stroke=stroke, fill=fill, color=color, dashed=dashed)))

GEOM = {}
def reg(id, x, y, w, h):
    GEOM[id] = (x, y, w, h)

def edge(id, pts, color, src=None, tgt=None, dashed=False, start=False):
    cells.append(("edge", dict(id=id, pts=pts, color=color, src=src, tgt=tgt, dashed=dashed, start=start)))

def label(id, x, y, w, value, color, align="left", fill=None, h=16):
    text(id, x, y, w, h, value, size=11, color=color, bold=True, align=align, valign="middle", fill=fill)

def comp(id, x, y, w, h, stroke, title, body, logos=(), prod=None, prod_color=None, title_size=14, body_h=None, logo_size=26):
    """A component card in the reference style: logo(s), bold title, optional product line, body."""
    box(id, x, y, w, h, stroke)
    reg(id, x, y, w, h)
    lx = x + 18
    for i, b in enumerate(logos):
        image(f"{id}_logo{i}", lx, y + 16, logo_size, logo_size, b)
        lx += logo_size + 8
    tx = lx + (4 if logos else 0)
    text(f"{id}_title", tx, y + 14, x + w - 10 - tx, 30, title, size=title_size, color="#0f172a", bold=True, valign="middle")
    by = y + 50
    if prod:
        text(f"{id}_prod", x + 18, by, w - 36, 16, prod, size=11, color=prod_color, bold=True)
        by += 20
    text(f"{id}_body", x + 18, by, w - 36, (y + h - 10) - by, body)

GREEN, RED, INDIGO, ORANGE, BLUE, REDIS, SLATE, GREY = "#059669", "#e11d48", "#4f46e5", "#d97706", "#336791", "#D82C20", "#334155", "#94a3b8"

# ---------------------------------------------------------------- header
image("hdr_logo", 40, 24, 260, 36, IMG["hdr_logo"])
text("hdr_title", 320, 24, 900, 36, "AMP Lite · Architecture (development preview)", size=22, color="#0f172a", bold=True, valign="middle")
image("hdr_wso2", 2150, 26, 140, 35, IMG["hdr_wso2"])

# ---------------------------------------------------------------- actors
def actor(id, x, w, title, subtitle, logos=()):
    box(id, x, 110, w, 80, "#94a3b8", arc=6)
    reg(id, x, 110, w, 80)
    lx = x + 14
    for i, b in enumerate(logos):
        image(f"{id}_logo{i}", lx, 124, 22, 22, b)
        lx += 30
    text(f"{id}_t", lx + (6 if logos else 0), 122, x + w - 4 - lx - (6 if logos else 0), 26, title, size=14, color="#0f172a", bold=True, valign="middle")
    text(f"{id}_s", x + 14, 160, w - 28, 18, subtitle)

actor("a_team", 300, 820, "Platform teams &amp; developers", "Console · CLI · AI coding assistants (MCP)")
actor("a_llm", 1180, 240, "LLMs &amp; MCP", "model providers · MCP servers", logos=[IMG[f"a_llm_logo{i}"] for i in range(4)])
actor("a_cons", 1440, 200, "Agent consumers", "apps, users, other agents")
actor("a_ext", 1660, 380, "Your agents (hosted elsewhere)", "any cloud, on-prem or SaaS · registered in AMP as external agents")

# ---------------------------------------------------------------- stack + ports
box("cluster", 260, 230, 1800, 630, "#1D63ED", fill="none", arc=1, sw=2, dashed=True, dash="10 6")
image("cl_logo", 840, 242, 24, 24, STACK_ICON)
text("cl_lbl", 872, 244, 420, 20, "Docker Compose stack (docker compose up)", size=14, color="#1D63ED", valign="middle")

pill("lb_cp", 350, 221, 440, "PORTS · console :3000 · API + MCP :9000 · ThunderID :8080")
pill("egress", 1260, 221, 100, "EGRESS", fill="#7f1d1d")
pill("lb_dp", 1400, 221, 610, "PORT :19080 · AI Gateway runtime · agent endpoints · trace ingest")
pill("lb_op", 1560, 851, 360, "PORT :9098 · Agent Manager Observer · traces")
for pid, x, y, w in (("lb_cp", 350, 221, 440), ("egress", 1260, 221, 100), ("lb_dp", 1400, 221, 610), ("lb_op", 1560, 851, 360)):
    reg(pid, x, y, w, 18)

# ---------------------------------------------------------------- control plane
box("cp", 280, 290, 860, 530, GREEN, fill="#F2FBF7", arc=2)
text("cp_lbl", 300, 298, 820, 20, "CONTROL PLANE", size=13, color=GREEN, bold=True, valign="middle")

comp("console", 300, 330, 240, 180, GREEN, "Console",
     "Web UI for agents, gateways, LLM and MCP providers, traces and users",
     logos=[IMG["console_logo0"]])
comp("amapi", 560, 330, 560, 180, GREEN, "Agent Manager API",
     "REST API and MCP server<br/>registers external agents and issues their agent tokens<br/>"
     "LLM, MCP, A2A and gateway config; RBAC and audit log<br/>at startup: migrates the database and registers the default gateway<br/>"
     "gateways connect over a WebSocket on :9243",
     logos=[IMG["amapi_logo0"]])
comp("thunder", 300, 560, 820, 170, RED, "ThunderID",
     "<b>For people and services:</b> single sign-on (console, CLI, MCP); users, roles and groups<br/><br/>"
     "<b>For agents:</b> an agent ID and OAuth client per external agent, created by Agent Manager<br/><br/>"
     "the only identity provider; thunder-setup applies the generated bootstrap and re-runs only when it changes",
     logos=[IMG["thunder_logo0"]], logo_size=22, prod="one per install", prod_color=RED)

sub("init", 300, 742, 820, 62,
    "<b>amp-init</b> (one-shot, before everything else): generates the ThunderID config and bootstrap, "
    "and the gateway's config.toml, encryption key, TLS certificate and registration token; all set from .env",
    SLATE, "#F1F5F9")

# ---------------------------------------------------------------- data plane
box("dp", 1180, 290, 860, 280, INDIGO, fill="#F5F5FF", arc=2)
text("dp_lbl", 1200, 298, 820, 20, "DATA PLANE", size=13, color=INDIGO, bold=True, valign="middle")
comp("gw", 1200, 330, 560, 220, INDIGO, "AI Gateway",
     "• inbound: the public entry point for agent endpoints; API keys and OAuth tokens checked here<br/>"
     "• outbound: LLM and MCP proxy, guardrails, token and cost limits; provider keys stay in the gateway<br/>"
     "• gateway-controller pulls its config from Agent Manager; gateway-runtime (Envoy) serves the traffic",
     logos=[IMG["gw_logo0"]], prod="default gateway registered at startup · more can be added per org", prod_color=INDIGO)
comp("tracegw", 1780, 330, 240, 220, ORANGE, "Trace ingest",
     "OTLP at /otel/v1/traces<br/>checks the agent token<br/>maps its organization, project and agent claims to "
     "x-user-*-name headers<br/>forwards to the OTel Collector<br/>Agent Manager pushes this API to every gateway",
     logos=[IMG["tracegw_logo0"]], prod="/otel API on the AI Gateway", prod_color=ORANGE)

# ---------------------------------------------------------------- observability plane
box("op", 1180, 610, 860, 210, ORANGE, fill="#FFF8EE", arc=2)
text("op_lbl", 1200, 618, 820, 20, "OBSERVABILITY PLANE", size=13, color=ORANGE, bold=True, valign="middle")
comp("observer", 1200, 646, 260, 140, ORANGE, "Agent Manager Observer",
     "traces per agent for the console, CLI and AI assistants<br/>applies the otel-traces-* index template at startup",
     logos=[IMG["observer_logo0"]], title_size=13)
sub("tags", 1480, 646, 260, 140,
    "<b>Tenant tags on every span</b><br/>agent token claims: organization · project · agent<br/>"
    "↓ gateway headers: x-user-*-name<br/>↓ resource attributes: agent-manager/*<br/>"
    "the observer filters queries on these",
    ORANGE, "#FFF1DC")
comp("collect", 1760, 646, 260, 140, ORANGE, "Collect",
     "adds agent-manager/* resource attributes from the headers; writes spans to OpenSearch",
     logos=[IMG["collect_logo0"]], prod="OpenTelemetry Collector", prod_color=ORANGE)

# ---------------------------------------------------------------- backing services
def backing(id, x, y, w, h, sub_text=""):
    box(id, x, y, w, h, "#94a3b8", fill="#F8FAFC", dashed=True)
    text(f"{id}_lbl", x + 14, y + 8, w - 28, 16, "BACKING SERVICES", size=12, color="#64748b", bold=True, valign="middle")
    if sub_text:
        text(f"{id}_sub", x + 14, y + 26, w - 28, 30, sub_text, size=10.5, italic=True)

backing("bk_left", 40, 290, 200, 530)
comp("sqldb", 54, 340, 172, 230, BLUE, "SQL database",
     "Agent Manager data, audit log, and secrets encrypted with ENCRYPTION_KEY (AES-256-GCM)<br/><i>SQLite by default, PostgreSQL 18 with the postgres profile; SQL Server 2025 and Oracle 19c planned</i>",
     logos=[DB_ICON], logo_size=22, title_size=13)
comp("thdb", 54, 610, 172, 190, BLUE, "ThunderID data",
     "users, agent identities and OAuth clients<br/>Thunder's own database, separate from Agent Manager's",
     logos=[DB_ICON], logo_size=22, title_size=13)

backing("bk_right", 2080, 590, 210, 230)
comp("os", 2094, 630, 182, 180, "#005EB8", "OpenSearch",
     "traces (otel-traces-* index)",
     logos=[IMG["os_logo0"]], logo_size=22, title_size=13)


# ---------------------------------------------------------------- flows
# external entry
edge("f_team_lb1", [(420, 190), (420, 221)], GREEN, "a_team", "lb_cp")
edge("f_lb_console", [(420, 239), (420, 330)], GREEN, "lb_cp", "console")
label("l_manage", 428, 270, 80, "manage", GREEN)
edge("f_team_lb2", [(700, 190), (700, 221)], GREEN, "a_team", "lb_cp")
edge("f_lb_api", [(700, 239), (700, 330)], GREEN, "lb_cp", "amapi")
label("l_api", 708, 270, 90, "API · MCP", GREEN)

edge("f_cons_lb", [(1480, 190), (1480, 221)], INDIGO, "a_cons", "lb_dp")
edge("f_lb_gw1", [(1480, 239), (1480, 330)], INDIGO, "lb_dp", "gw")
label("l_invoke", 1488, 270, 110, "invoke agents", INDIGO)
edge("f_ext_lb", [(1740, 190), (1740, 221)], INDIGO, "a_ext", "lb_dp")
edge("f_lb_gw2", [(1740, 239), (1740, 330)], INDIGO, "lb_dp", "gw")
label("l_ext", 1610, 270, 124, "LLM &amp; MCP calls", INDIGO, align="right")
edge("f_ext_lb_tr", [(1975, 190), (1975, 221)], ORANGE, "a_ext", "lb_dp")
edge("f_lb_tgw", [(1975, 239), (1975, 330)], ORANGE, "lb_dp", "tracegw")
label("l_ext_tr", 1870, 270, 98, "traces (OTLP)", ORANGE, align="right")

edge("f_gw_eg", [(1310, 330), (1310, 239)], INDIGO, "gw", "egress")
edge("f_eg_llm", [(1310, 221), (1310, 190)], INDIGO, "egress", "a_llm")
edge("f_eg_ag", [(1350, 221), (1350, 204), (1700, 204), (1700, 190)], INDIGO, "egress", "a_ext")
label("l_llm", 1318, 270, 150, "LLM · MCP · to agents", INDIGO)

# trace views from the browser
edge("f_team_lb3", [(1100, 110), (1100, 96), (2310, 96), (2310, 860), (1920, 860)], GREEN, "a_team", "lb_op")
label("l_traceui", 2150, 74, 150, "trace views (browser)", GREEN)
edge("f_lb_obs", [(1560, 860), (1160, 860), (1160, 740), (1200, 740)], GREEN, "lb_op", "observer")

# control plane
edge("f_api_gwcfg", [(1120, 400), (1200, 400)], GREEN, "amapi", "gw")
label("l_gwcfg", 1128, 366, 64, "config<br/>:9243", GREEN, align="center", fill="#FFFFFF", h=30)
edge("f_sso", [(420, 510), (420, 560)], RED, "console", "thunder", dashed=True)
label("l_sso", 428, 546, 100, "single sign-on", RED)
edge("f_api_th", [(620, 510), (620, 560)], RED, "amapi", "thunder", dashed=True)
label("l_ids", 628, 546, 160, "creates agent identities", RED)
edge("f_api_db", [(580, 510), (580, 528), (226, 528)], BLUE, "amapi", "sqldb")
label("l_data", 288, 510, 80, "read / write", BLUE)
edge("f_th_db", [(300, 680), (226, 680)], BLUE, "thunder", "thdb")

# telemetry
edge("f_tgw_collect", [(1960, 550), (1960, 646)], ORANGE, "tracegw", "collect")
label("l_fwd", 1730, 582, 222, "spans + x-user-*-name headers", ORANGE, align="right")
edge("f_collect_os", [(2020, 700), (2094, 700)], BLUE, "collect", "os")
label("l_write", 2026, 684, 50, "write", BLUE)
edge("f_obs_os", [(1330, 786), (1330, 800), (2094, 800)], BLUE, "observer", "os")
label("l_query", 1480, 803, 140, "query traces", BLUE)

# ---------------------------------------------------------------- legend
box("legend", 280, 900, 2010, 130, "#cbd5e1", arc=4, sw=1)
text("lg_title", 296, 908, 200, 18, "Flows", size=12, color="#0f172a", bold=True, valign="middle")
LG = [
    (GREEN, False, "manage, configure"),
    (RED, True, "identity · single sign-on · agent identities"),
    (INDIGO, False, "agent traffic"),
    (ORANGE, False, "telemetry (traces)"),
    (BLUE, False, "data reads / writes"),
]
cols = [296, 796, 1296, 1796]
for i, (c, d, t) in enumerate(LG):
    x = cols[i % 4]
    y = 948 if i < 4 else 978
    edge(f"lg_e{i}", [(x, y), (x + 34, y)], c, dashed=d)
    text(f"lg_t{i}", x + 42, y - 8, 226, 16, t, size=11, color="#0f172a", valign="middle")
pill("lg_lb", 296, 1002, 120, "PORT")
text("lg_lb_t", 424, 1002, 1100, 18,
     "a published host port: each service listens on its own port; no ingress, load balancer or reverse proxy",
     size=11, color="#0f172a", valign="middle")

text("footer", 40, 1050, 2250, 20,
     "AMP Lite (WSO2 Agent Manager without OpenChoreo) as it runs in development today; not production-ready. "
     "Agents run outside the stack and are registered as external agents; each organization has one internal default environment. "
     "Logos are trademarks of their respective owners.",
     size=10.5, valign="middle")

# ================================================================ draw.io
def esc(s):
    return html.escape(s, quote=True)

def rel(gid, px, py):
    x, y, w, h = GEOM[gid]
    return round((px - x) / w, 4), round((py - y) / h, 4)

out = []
for kind, c in cells:
    base = f"fontFamily={FONT};"
    if kind == "box":
        st = base + f"rounded=1;arcSize={c['arc']};whiteSpace=wrap;html=1;fillColor={c['fill']};strokeColor={c['stroke']};strokeWidth={c['sw']};"
        if c["dashed"]:
            st += f"dashed=1;dashPattern={c['dash']};"
        out.append(f'<mxCell id="{c["id"]}" value="" style="{st}" parent="1" vertex="1"><mxGeometry x="{c["x"]}" y="{c["y"]}" width="{c["w"]}" height="{c["h"]}" as="geometry" /></mxCell>')
    elif kind == "text":
        fs = (1 if c["bold"] else 0) | (2 if c["italic"] else 0)
        st = base + f"text;html=1;whiteSpace=wrap;align={c['align']};verticalAlign={c['valign']};fontSize={c['size']};fontColor={c['color']};spacing=0;fontStyle={fs};"
        if c["fill"]:
            st += f"fillColor={c['fill']};"
        out.append(f'<mxCell id="{c["id"]}" value="{esc(c["value"])}" style="{st}" parent="1" vertex="1"><mxGeometry x="{c["x"]}" y="{c["y"]}" width="{c["w"]}" height="{c["h"]}" as="geometry" /></mxCell>')
    elif kind == "image":
        st = base + f"shape=image;html=1;imageAspect=1;aspect=fixed;image=data:image/svg+xml,{c['b64']};"
        out.append(f'<mxCell id="{c["id"]}" value="" style="{st}" parent="1" vertex="1"><mxGeometry x="{c["x"]}" y="{c["y"]}" width="{c["w"]}" height="{c["h"]}" as="geometry" /></mxCell>')
    elif kind == "pill":
        st = base + f"rounded=1;arcSize=50;whiteSpace=wrap;html=1;fillColor={c['fill']};strokeColor=none;fontColor=#FFFFFF;fontSize=10;align=center;verticalAlign=middle;"
        out.append(f'<mxCell id="{c["id"]}" value="{esc("<b>" + c["label"] + "</b>")}" style="{st}" parent="1" vertex="1"><mxGeometry x="{c["x"]}" y="{c["y"]}" width="{c["w"]}" height="{c["h"]}" as="geometry" /></mxCell>')
    elif kind == "sub":
        st = base + f"rounded=1;arcSize=6;whiteSpace=wrap;html=1;fillColor={c['fill']};strokeColor={c['stroke']};strokeWidth=1;fontSize=11.5;fontColor={c['color']};align=left;verticalAlign=top;spacingLeft=12;spacingRight=8;spacingTop=10;"
        if c["dashed"]:
            st += "dashed=1;dashPattern=6 4;"
        out.append(f'<mxCell id="{c["id"]}" value="{esc(c["value"])}" style="{st}" parent="1" vertex="1"><mxGeometry x="{c["x"]}" y="{c["y"]}" width="{c["w"]}" height="{c["h"]}" as="geometry" /></mxCell>')
    elif kind == "edge":
        st = base + f"edgeStyle=orthogonalEdgeStyle;rounded=1;html=1;endArrow=block;endFill=1;endSize=7;strokeWidth=2;strokeColor={c['color']};jumpStyle=arc;jumpSize=10;"
        if c["dashed"]:
            st += "dashed=1;dashPattern=6 4;"
        if c["start"]:
            st += "startArrow=block;startFill=1;startSize=7;"
        pts = c["pts"]
        conn = ""
        if c["src"]:
            ex, ey = rel(c["src"], *pts[0])
            ix, iy = rel(c["tgt"], *pts[-1])
            st += f"exitX={ex};exitY={ey};exitDx=0;exitDy=0;entryX={ix};entryY={iy};entryDx=0;entryDy=0;"
            conn = f' source="{c["src"]}" target="{c["tgt"]}"'
            way = "".join(f'<mxPoint x="{x}" y="{y}" />' for x, y in pts[1:-1])
            geo = f'<mxGeometry relative="1" as="geometry">' + (f'<Array as="points">{way}</Array>' if way else "") + "</mxGeometry>"
        else:
            geo = (f'<mxGeometry relative="1" as="geometry"><mxPoint x="{pts[0][0]}" y="{pts[0][1]}" as="sourcePoint" />'
                   f'<mxPoint x="{pts[-1][0]}" y="{pts[-1][1]}" as="targetPoint" /></mxGeometry>')
        out.append(f'<mxCell id="{c["id"]}" value="" style="{st}" parent="1" edge="1"{conn}>{geo}</mxCell>')

drawio = f'''<mxfile host="drawio" version="22.1.2" type="device">
  <diagram name="AMP Lite — architecture (development preview)" id="amp-lite-architecture">
    <mxGraphModel dx="1600" dy="1000" grid="0" gridSize="10" guides="1" tooltips="1" connect="1" arrows="1" fold="1" page="1" pageScale="1" pageWidth="{W}" pageHeight="{H}" background="#ffffff" math="0" shadow="0">
      <root>
        <mxCell id="0" />
        <mxCell id="1" parent="0" />
''' + "\n".join(out) + '''
      </root>
    </mxGraphModel>
  </diagram>
</mxfile>
'''
open(f"{OUT_DIR}/architecture.drawio", "w").write(drawio)

# ================================================================ HTML render
def path_d(pts, r=8):
    d = f"M{pts[0][0]},{pts[0][1]}"
    for i in range(1, len(pts) - 1):
        (x0, y0), (x1, y1), (x2, y2) = pts[i - 1], pts[i], pts[i + 1]
        def toward(ax, ay, bx, by, dist):
            L = max(abs(bx - ax), abs(by - ay)) or 1
            dd = min(dist, L / 2)
            return ax + (bx - ax) / L * dd, ay + (by - ay) / L * dd
        p1 = toward(x1, y1, x0, y0, r)
        p2 = toward(x1, y1, x2, y2, r)
        d += f" L{p1[0]},{p1[1]} Q{x1},{y1} {p2[0]},{p2[1]}"
    d += f" L{pts[-1][0]},{pts[-1][1]}"
    return d

colors = sorted({c["color"] for k, c in cells if k == "edge"})
markers = "".join(
    f'<marker id="m{col[1:]}" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="4.2" markerHeight="4.2" orient="auto-start-reverse">'
    f'<path d="M0,0 L10,5 L0,10 z" fill="{col}"/></marker>' for col in colors)

parts = []
z = 0
for kind, c in cells:
    z += 1
    if kind == "edge":
        pts = list(c["pts"])
        # shorten the last segment so the arrow tip lands on the boundary
        (xa, ya), (xb, yb) = pts[-2], pts[-1]
        L = max(abs(xb - xa), abs(yb - ya)) or 1
        dash = ' stroke-dasharray="12 8"' if c["dashed"] else ""
        start = f' marker-start="url(#m{c["color"][1:]})"' if c["start"] else ""
        parts.append(f'<svg class="e" style="z-index:{z}" width="{W}" height="{H}"><path d="{path_d(pts)}" fill="none" stroke="{c["color"]}" stroke-width="2"{dash} marker-end="url(#m{c["color"][1:]})"{start}/></svg>')
        continue
    pos = f'left:{c["x"]}px;top:{c["y"]}px;width:{c["w"]}px;height:{c["h"]}px;z-index:{z};'
    if kind == "box":
        bstyle = "dashed" if c["dashed"] else "solid"
        radius = min(c["w"], c["h"]) * c["arc"] / 100 if c["arc"] else 0
        fill = "transparent" if c["fill"] == "none" else c["fill"]
        if c["dashed"]:
            # SVG rect so the dash pattern matches draw.io
            parts.append(f'<svg class="e" style="z-index:{z};left:{c["x"]-2}px;top:{c["y"]-2}px" width="{c["w"]+4}" height="{c["h"]+4}">'
                         f'<rect x="2" y="2" width="{c["w"]}" height="{c["h"]}" rx="{radius}" fill="{fill}" stroke="{c["stroke"]}" stroke-width="{c["sw"]}" stroke-dasharray="{c["dash"].replace(" ", " ")}" transform=""/></svg>')
        else:
            parts.append(f'<div class="a" style="{pos}box-sizing:border-box;border:{c["sw"]}px solid {c["stroke"]};background:{fill};border-radius:{radius}px"></div>')
    elif kind == "text":
        jc = {"top": "flex-start", "middle": "center", "bottom": "flex-end"}[c["valign"]]
        bg = f"background:{c['fill']};" if c["fill"] else ""
        parts.append(f'<div class="a t" style="{pos}{bg}justify-content:{jc};text-align:{c["align"]};font-size:{c["size"]}px;color:{c["color"]};'
                     f'font-weight:{700 if c["bold"] else 400};font-style:{"italic" if c["italic"] else "normal"}"><div>{c["value"]}</div></div>')
    elif kind == "image":
        parts.append(f'<img class="a" style="{pos}object-fit:contain" src="data:image/svg+xml;base64,{c["b64"]}"/>')
    elif kind == "pill":
        parts.append(f'<div class="a pill" style="{pos}background:{c["fill"]}"><b>{c["label"]}</b></div>')
    elif kind == "sub":
        bstyle = "dashed" if c["dashed"] else "solid"
        parts.append(f'<div class="a sub" style="{pos}box-sizing:border-box;border:1px {bstyle} {c["stroke"]};background:{c["fill"]};color:{c["color"]}">{c["value"]}</div>')

page = f'''<!doctype html>
<html><head><meta charset="utf-8"><title>AMP Lite Architecture</title>
<style>
:root {{ color-scheme: light; }}
html,body {{ margin:0; background:#ffffff; }}
#c {{ position:relative; width:{W}px; height:{H}px; font-family:Helvetica, Arial, sans-serif; background:#ffffff; }}
.a {{ position:absolute; }}
.e {{ position:absolute; left:0; top:0; pointer-events:none; overflow:visible; }}
.t {{ display:flex; flex-direction:column; line-height:1.2; }}
.pill {{ border-radius:9px; color:#fff; font-size:10px; display:flex; align-items:center; justify-content:center; white-space:nowrap; }}
.sub {{ border-radius:6px; font-size:11.5px; line-height:1.2; padding:10px 8px 0 12px; }}
</style></head>
<body><div id="c"><svg class="e" width="0" height="0"><defs>{markers}</defs></svg>
{chr(10).join(parts)}
</div></body></html>
'''
open(f"{OUT_DIR}/architecture.html", "w").write(page)
print("ok", len(cells), "cells")
