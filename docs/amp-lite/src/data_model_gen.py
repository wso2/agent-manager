#!/usr/bin/env python3
"""Generates the Agent Manager data-model diagram (HTML with inline SVG) for all three product tiers."""
import html
import os
import sys

OUT = sys.argv[1]
CROPS = sys.argv[2] if len(sys.argv) > 2 else None

W, H = 2880, 1720

AREAS = {
    "tenancy": dict(name="Tenancy", color="#2563eb", fill="#eff6ff"),
    "agents": dict(name="Agents and identity", color="#059669", fill="#ecfdf5"),
    "gateway": dict(name="AI gateway: gateways, artifacts, LLM and MCP", color="#7c3aed", fill="#f5f3ff"),
    "secrets": dict(name="Secrets", color="#dc2626", fill="#fef2f2"),
    "events": dict(name="Events and webhooks", color="#0891b2", fill="#ecfeff"),
    "audit": dict(name="Audit log and console usage", color="#b45309", fill="#fffbeb"),
    "internal": dict(name="Internals (workers)", color="#475569", fill="#f8fafc"),
    "eval": dict(name="Evaluation and alerts (Agent Manager tier; empty on AI Gateway)", color="#be185d", fill="#fdf2f8"),
    "runtime": dict(name="Agent runtime (Agent Runtime tier; empty below it)", color="#4d7c0f", fill="#f7fee7"),
    "deferred": dict(name="Kept outside this database", color="#64748b", fill="#ffffff"),
}

CHIPS = {
    "env": ("per environment", "#e2e8f0", "#334155"),
    "secret": ("uses a secret", "#fee2e2", "#991b1b"),
    "queue": ("worker queue", "#e0e7ff", "#3730a3"),
    "artifact": ("gateway artifact", "#ede9fe", "#5b21b6"),
}

PURPOSE_CW = 6.25
FIELD_KCW = 6.75
FIELD_NCW = 5.9


def esc(s):
    return html.escape(s, quote=False)


def wrap(text, width_px, cw):
    maxc = max(8, int(width_px / cw))
    words, lines, cur = text.split(), [], ""
    for w in words:
        if cur and len(cur) + 1 + len(w) > maxc:
            lines.append(cur)
            cur = w
        else:
            cur = (cur + " " + w) if cur else w
    if cur:
        lines.append(cur)
    return lines


class Card:
    def __init__(self, key, area, x, y, w, title, table, purpose, fields=(), chips=(), rel=None,
                 hidden=False, deferred=False):
        self.key, self.area, self.x, self.y, self.w = key, area, x, y, w
        self.title, self.table, self.purpose = title, table, purpose
        self.fields, self.chips, self.rel = list(fields), list(chips), rel
        self.hidden, self.deferred = hidden, deferred
        self.layout()

    def layout(self):
        inner = self.w - 24
        self.table_lines = []
        if self.table:
            maxc = int(inner / 6.8)
            cur = ""
            for part in self.table.split(", "):
                piece = part if not cur else cur + ", " + part
                if cur and len(piece) + 1 > maxc:
                    self.table_lines.append(cur + ",")
                    cur = part
                else:
                    cur = piece
            if cur:
                self.table_lines.append(cur)
            self.table_lines = [ln if len(ln) <= maxc else ln for ln in self.table_lines]
        # chip rows
        self.chip_rows, row, used = [], [], 0
        for ch in self.chips:
            cw = len(CHIPS[ch][0]) * 6.1 + 16
            if row and used + cw > inner:
                self.chip_rows.append(row)
                row, used = [], 0
            row.append(ch)
            used += cw + 6
        if row:
            self.chip_rows.append(row)
        self.purpose_lines = wrap(self.purpose, inner, PURPOSE_CW)
        self.rel_lines = wrap(self.rel, inner, 5.9) if self.rel else []
        self.field_rows = []
        for f in self.fields:
            k, n = (f if isinstance(f, tuple) else (f, ""))
            width = len(k) * FIELD_KCW + (len(n) * FIELD_NCW + 8 if n else 0)
            if width <= inner - 10:
                self.field_rows.append((k, n, False))
            else:
                self.field_rows.append((k, "", False))
                if n:
                    for ln in wrap(n, inner - 22, FIELD_NCW):
                        self.field_rows.append(("", ln, True))
        h = 12 + 18 + 14 * len(self.table_lines) + 6
        h += 16 * len(self.purpose_lines) + 4
        if self.rel_lines:
            h += 14 * len(self.rel_lines) + 2
        if self.field_rows:
            h += 8 + 16 * len(self.field_rows)
        if self.chips:
            h += 6 + 22 * len(self.chip_rows)
        h += 10
        self.h = h

    def top(self, dx=None):
        return (self.x + (self.w / 2 if dx is None else dx), self.y)

    def bottom(self, dx=None):
        return (self.x + (self.w / 2 if dx is None else dx), self.y + self.h)

    def left(self, dy=None):
        return (self.x, self.y + (self.h / 2 if dy is None else dy))

    def right(self, dy=None):
        return (self.x + self.w, self.y + (self.h / 2 if dy is None else dy))

    def svg(self):
        a = AREAS[self.area]
        col = a["color"]
        out = []
        dash = ' stroke-dasharray="7 5"' if (self.hidden or self.deferred) else ""
        fill = "#f8fafc" if self.hidden else ("#ffffff" if not self.deferred else "#fafafa")
        stroke = "#64748b" if self.hidden else ("#94a3b8" if self.deferred else col)
        out.append(f'<g class="card" id="card-{self.key}">')
        out.append(f'<rect x="{self.x}" y="{self.y}" width="{self.w}" height="{self.h}" rx="9" '
                   f'fill="{fill}" stroke="{stroke}" stroke-width="1.6"{dash} filter="url(#shadow)"/>')
        if not (self.hidden or self.deferred):
            out.append(f'<rect x="{self.x}" y="{self.y}" width="6" height="{self.h}" rx="3" fill="{col}"/>')
        tcol = "#64748b" if self.deferred else ("#334155" if self.hidden else "#0f172a")
        y = self.y + 24
        out.append(f'<text x="{self.x + 14}" y="{y}" class="ctitle" fill="{tcol}">{esc(self.title)}</text>')
        if self.hidden:
            bx = self.x + self.w - 82
            out.append(f'<rect x="{bx}" y="{self.y + 9}" width="70" height="20" rx="10" fill="#334155"/>')
            out.append(f'<text x="{bx + 35}" y="{self.y + 23}" class="badge" text-anchor="middle">HIDDEN</text>')
        for ln in self.table_lines:
            y += 14
            out.append(f'<text x="{self.x + 14}" y="{y}" class="ctable">{esc(ln)}</text>')
        y += 6
        for ln in self.purpose_lines:
            y += 16
            out.append(f'<text x="{self.x + 14}" y="{y}" class="cpurpose" fill="{tcol}">{esc(ln)}</text>')
        y += 4
        if self.rel_lines:
            for ln in self.rel_lines:
                y += 14
                out.append(f'<text x="{self.x + 14}" y="{y}" class="crel" fill="{col if not self.deferred else "#64748b"}">{esc(ln)}</text>')
            y += 2
        if self.field_rows:
            y += 6
            out.append(f'<line x1="{self.x + 14}" y1="{y - 1}" x2="{self.x + self.w - 12}" y2="{y - 1}" stroke="#e2e8f0"/>')
            for k, n, cont in self.field_rows:
                y += 16
                if cont:
                    out.append(f'<text x="{self.x + 30}" y="{y}" class="fnote">{esc(n)}</text>')
                else:
                    out.append(f'<text x="{self.x + 14}" y="{y}"><tspan class="fkey">{esc(k)}</tspan>'
                               + (f'<tspan class="fnote" dx="7">{esc(n)}</tspan>' if n else "") + '</text>')
        if self.chips:
            y += 10
            for row in self.chip_rows:
                cx = self.x + 14
                for c in row:
                    label, bg, fg = CHIPS[c]
                    cw = len(label) * 6.1 + 16
                    out.append(f'<rect x="{cx}" y="{y}" width="{cw:.0f}" height="18" rx="9" fill="{bg}"/>')
                    out.append(f'<text x="{cx + cw / 2:.0f}" y="{y + 13}" class="chip" fill="{fg}" text-anchor="middle">{esc(label)}</text>')
                    cx += cw + 6
                y += 22
        out.append('</g>')
        return "\n".join(out)


cards = {}


def card(*a, **kw):
    c = Card(*a, **kw)
    cards[c.key] = c
    return c


# ---------------------------------------------------------------- Tenancy
TOP = 190
card("project", "tenancy", 50, TOP, 230, "Project", "projects",
     "Groups an org's agents and the LLM and MCP proxies they use.",
     [("name", "URL name, immutable"), ("display_name", ""), ("description", "")],
     rel="Can't be deleted while it has agents, LLM proxies or MCP servers.")
card("org", "tenancy", 350, TOP, 240, "Organization", "organizations",
     "The tenant and root of all data. Never renamed or deleted.",
     [("handle", "URL name, unique"), ("display_name", ""), ("thunder_ou_id", "Thunder org unit")])
card("env", "tenancy", 660, TOP, 230, "Environment", "environments",
     "One per org, named default, created with the org and hidden below the Agent Runtime tier.",
     [("name", "'default' + runtime envs"), ("is_production", ""), ("runtime_id", "runtime tier"),
      ("isolation_tier", "gvisor | kata")],
     rel="Agent Runtime adds rows; per-environment tables don't change.", hidden=True)

# ---------------------------------------------------------------- Agents
AY = 640
card("agent", "agents", 60, AY, 250, "Agent", "agents",
     "An external agent registered with AMP. Lite connects agents; it doesn't build or run them.",
     [("name", "unique per org"), ("display_name", ""), ("provisioning_type", "external")])
card("labels", "agents", 360, AY, 240, "Agent label", "agent_labels",
     "Key/value tags for filtering agents.",
     [("label_key", ""), ("label_value", "")], rel="Up to 10 per agent.")
card("identity", "agents", 360, 0, 240, "Agent identity", "agent_identities",
     "The agent's identity in Thunder, plus the queue that provisions it.",
     [("thunder_agent_id", ""), ("thunder_client_id", ""), ("status", "pending ... completed | failed"),
      ("next_retry_at", "")],
     rel="Client secrets are never stored.", chips=["env", "queue"])
card("gwset", "agents", 360, 0, 240, "Agent gateway settings", "agent_gateway_settings",
     "How the gateway protects calls to this agent.",
     [("api_key_security", ""), ("oauth_*", "issuers, audiences"), ("cors_*", "origins, methods"),
      ("timeout_seconds", "")], chips=["env"])
card("aiapp", "agents", 640, AY, 250, "AI application", "ai_applications",
     "The agent's application record on the gateway.",
     [("handle", ""), ("name", "")], chips=["env"])
card("exposure", "agents", 640, 0, 250, "Agent exposure", "agent_exposures",
     "Publishes the agent through the gateway.",
     [("protocol", "a2a | rest"), ("artifact_id", "its gateway artifact")], chips=["env", "artifact"])
card("acard", "agents", 640, 0, 250, "Agent card", "agent_cards",
     "Cached A2A agent card, fetched and refreshed by a worker.",
     [("source_url", ""), ("card", "JSON"), ("card_hash", ""), ("status", "")], chips=["env", "queue"])
card("a2a", "agents", 640, 0, 250, "A2A proxy", "a2a_proxies",
     "Gateway route to an A2A agent's upstream.",
     [("upstream_auth_header", ""), ("upstream_auth_secret_id", "")], chips=["env", "secret"])
card("mconf", "agents", 60, 0, 250, "Agent model config", "agent_model_configs",
     "A named LLM or MCP connection the agent uses.",
     [("name", "unique per agent"), ("type", "llm | mcp")])
card("mlinks", "agents", 60, 0, 250, "Config bindings",
     "agent_model_config_llm, _mcp, _mcp_envs, _vars",
     "Which proxy each config points at, and the env-var names given to the agent.",
     [("llm_proxy_id", "+ policy settings"), ("mcp_proxy_id", "+ excluded / mapping"),
      ("variable_key", "-> variable_name")], chips=["env"])


def stack(keys, y0, gap=24):
    y = y0
    for k in keys:
        cards[k].y = y
        y += cards[k].h + gap


stack(["labels", "identity", "gwset"], AY)
stack(["aiapp", "exposure", "acard", "a2a"], AY)
stack(["agent", "mconf", "mlinks"], AY, gap=56)

# ---------------------------------------------------------------- Gateway area
GY = TOP
card("gwenv", "gateway", 975, GY, 245, "Gateway placement",
     "gateway_environments, environment_default_gateways",
     "Attaches each gateway to the hidden default environment and names the default ingress and egress gateway.",
     [("gateway_id", "attached gateway"), ("role", "ingress | egress default")],
     rel="New gateways are attached automatically. A default can't be detached or deleted.", chips=["env"])
card("gateway", "gateway", 1280, GY, 250, "Gateway", "gateways",
     "An AI gateway that serves agent, LLM and MCP traffic. Any number per org.",
     [("name", "unique per org"), ("functionality_type", "ingress | egress | both"),
      ("hosting", "managed | self_hosted"), ("vhost", ""), ("runtime_url", "")])
card("gwkids", "gateway", 1590, GY, 270, "Gateway tokens, IdPs and sync",
     "gateway_tokens, gateway_identity_providers, gateway_events, gateway_event_states",
     "How the gateway authenticates, which token issuers it trusts, and the change feed it syncs from.",
     [("token_prefix", "unique"), ("status", "active | revoked"), ("issuer, jwks_uri", "trusted IdP"),
      ("seq, action", "change-feed entry")])
RY = 620
card("apikeys", "gateway", 975, RY, 245, "API key", "api_keys",
     "Keys that let clients call a deployed artifact.",
     [("name", "unique per artifact"), ("key_hash", ""), ("masked_key", ""), ("status", ""), ("expires_at", "")])
card("artifact", "gateway", 1280, RY, 250, "Artifact", "artifacts",
     "Base row for anything deployable to a gateway.",
     [("kind", "LlmProvider | LlmProxy |"), ("", "McpProxy | McpProxyMapping |"), ("", "Agent | RestApi | WebSubAPI |"), ("", "OtelIngest"),
      ("handle", "unique per org"), ("name, version", "unique per org")])
card("deploy", "gateway", 1590, RY, 270, "Gateway deployment", "artifact_deployments",
     "Each rendered version of an artifact sent to a gateway.",
     [("gateway_id", ""), ("content", "rendered config"), ("base_deployment_id", "")])
card("status", "gateway", 1590, 0, 270, "Deployment status", "artifact_gateway_status",
     "Wanted vs actual state of an artifact on each gateway. The render queue.",
     [("status", "PENDING ... DEPLOYED | FAILED"), ("status_desired", ""), ("next_attempt_at", "")],
     chips=["queue"])
cards["status"].y = cards["deploy"].y + cards["deploy"].h + 22

TY = cards["status"].y + cards["status"].h + 80
TW = 210
card("prov", "gateway", 975, TY, TW, "LLM provider", "llm_providers",
     "An org-wide connection to an LLM service.",
     [("template_handle", ""), ("model_list", ""), ("configuration", "JSON"), ("upstream_secret_id", "")],
     chips=["secret"])
card("lproxy", "gateway", 1235, TY, TW, "LLM proxy", "llm_proxies",
     "A project's governed route to a provider. What agents call.",
     [("project_id", "owning project"), ("provider_id", "LLM provider"), ("configuration", "policies"),
      ("upstream_secret_id", "")], chips=["secret"])
card("mproxy", "gateway", 1495, TY, TW, "MCP proxy", "mcp_proxies",
     "Gateway front for an MCP server.",
     [("project_id", "empty = org-level"), ("configuration", "JSON"), ("status", "")])
card("mmap", "gateway", 1755, TY, TW, "MCP proxy mapping", "mcp_proxy_mappings",
     "An agent's own deployable mapping of an MCP proxy.",
     [("source_mcp_proxy_id", "the MCP proxy"), ("configuration", "JSON")])
BY = max(cards[k].y + cards[k].h for k in ("prov", "lproxy", "mproxy", "mmap")) + 70
card("tmpl", "gateway", 975, BY, TW, "LLM provider template", "llm_provider_templates",
     "An org's own provider template. Built-ins ship in code.",
     [("handle", "unique per org"), ("configuration", "JSON")])
card("scopes", "gateway", 1495, BY, TW, "MCP scope", "mcp_proxy_scopes, mcp_proxy_scope_tools",
     "Named permission on an MCP proxy, covering a set of tools.",
     [("action", "unique per proxy"), ("tool_name", "one row per tool")])
card("endpoint", "gateway", 1755, BY, 232, "MCP endpoint", "mcp_proxy_endpoints, mcp_proxy_endpoint_environments",
     "The upstream MCP server. One per proxy in Lite, placed in the default environment.",
     [("configuration", "upstream, policies"), ("upstream_secret_id", ""), ("artifact_id", "per env")],
     chips=["env", "secret"])

# ---------------------------------------------------------------- Right column
RX = 2050
card("secret", "secrets", RX, TOP, 300, "Secret", "secrets",
     "Every credential, encrypted in this database or held in an external vault.",
     [("owner_kind, owner_id", "who uses it"), ("provider", "db | vault name"),
      ("ciphertext", "AES-256-GCM"), ("key_id", "for key rotation"), ("external_ref", "vault path")])

EY = cards["secret"].y + cards["secret"].h + 36 + 22 + 58 + 40
ECW, ESP = 240, 265
card("sub", "events", RX, EY, ECW, "Event subscription", "event_subscriptions",
     "Which events to send where, for the org, a project or an agent.",
     [("scope", "org | project | agent"), ("project_id, agent_id", ""), ("enabled", "")],
     rel="Deleted with its agent.")
card("dest", "events", RX + ESP, EY, ECW, "Event destination", "event_destinations",
     "A webhook or other target for events.",
     [("name", "unique per org"), ("kind", ""), ("config", "JSON"), ("availability", "all | org | projects")],
     chips=["secret"])
card("delivery", "events", RX + 2 * ESP, EY, ECW, "Event delivery", "event_deliveries, event_delivery_subscriptions",
     "One event sent to one destination, with retries. Outlives deleted agents.",
     [("event_type", ""), ("status, attempts", ""), ("response_code", "")], chips=["queue"])
card("subfilt", "events", RX, 0, ECW, "Subscription filters",
     "event_subscription_types, event_subscription_environments",
     "Event types to match, and environments (always empty in Lite = all).",
     [("event_type", "one row per type"), ("environment_id", "")])
card("destproj", "events", RX + ESP, 0, ECW, "Destination projects", "event_destination_projects",
     "The projects a destination is limited to.",
     [("project_id", "")])
card("outbox", "events", RX + 2 * ESP, 0, ECW, "Event outbox", "event_outbox",
     "Events waiting to be turned into deliveries.",
     [("body", "JSON"), ("available_at", "")], chips=["queue"])
y2 = max(cards[k].y + cards[k].h for k in ("sub", "dest", "delivery")) + 60
for k in ("subfilt", "destproj", "outbox"):
    cards[k].y = y2

card("audit", "audit", RX, 0, 360, "Audit event", "audit_events",
     "One row per console or API action. Insert only; old rows aged out.",
     [("occurred_at", ""), ("action", "e.g. agent.delete"), ("actor_id", "username"),
      ("resource_type, resource_id", ""), ("outcome", "success | failure | deny")])
card("usage", "audit", RX + 390, 0, 380, "Usage event", "usage_events",
     "Console and API feature usage. Store and export only.",
     [("occurred_at", ""), ("source", "api | console"), ("feature_code", ""), ("username", ""),
      ("dimensions", "JSON")])
card("locks", "internal", RX, 0, 360, "Lock", "locks",
     "A lease that lets only one instance run a singleton worker.",
     [("name", "e.g. gateway.drift_scan"), ("owner", "empty = free"), ("expires_at", "")])
card("queues", "internal", RX + 390, 0, 380, "Queue columns", "claimed_by, claimed_until",
     "Rows a worker claims carry a lease instead of a lock: agent identities, agent cards, deployment status, event outbox, event deliveries, monitors, monitor runs, builds, agent deployments.",
     [], chips=["queue"])

# ---------------------------------------------------------------- Evaluation and runtime rows
RW, RSP = 320, 344
evaluation = [
    ("evaluator", "Custom evaluator", "evaluators",
     "An org's own code or LLM-judge evaluator. Built-ins ship in code.",
     [("identifier", "unique per org"), ("type", "code | llm_judge"), ("level", "trace | agent | llm")], []),
    ("monitor", "Monitor", "monitors, monitor_evaluators",
     "Scores an agent's traces on a schedule (future) or over a past window, with chosen evaluators.",
     [("type", "future | past"), ("next_run_at", ""), ("sampling_percent", "1-100"),
      ("judge_llm_proxy_id", "LLM judge route")], ["env", "queue", "secret"]),
    ("mrun", "Monitor run", "monitor_runs",
     "One evaluation job over a trace window. Deleted with its monitor.",
     [("trace_start, trace_end", ""), ("status", "pending ... success | failed"), ("job_ref", "runner id")],
     ["queue"]),
    ("score", "Scores", "monitor_run_evaluators, scores",
     "Per-evaluator aggregates of a run, and one score per trace or span.",
     [("aggregations", "JSON"), ("item_key", "trace or trace/span"), ("score_bp", "0-10000")], []),
    ("malert", "Monitor alert", "monitor_alerts, monitor_alert_destinations",
     "Score thresholds and run outcomes that notify event destinations.",
     [("thresholds", "JSON"), ("destination_id", "one row per destination")], []),
    ("osc", "Org service client", "org_service_clients",
     "The org's eval publisher and scheduler clients.",
     [("purpose", "eval_publisher | eval_scheduler"), ("secret_id", "")], ["secret"]),
]
runtime = [
    ("rt", "Runtime and pipeline", "runtimes, deployment_pipelines, _environments, _paths",
     "Where environments run agents, and the order agents are promoted in. Every org has a default pipeline.",
     [("kind", "kubernetes | docker"), ("position", "stage order"), ("source -> target", "promotion path")],
     ["secret"]),
    ("workload", "Agent workload", "agent_workloads, git_credentials",
     "What a hosted agent or MCP server runs and how it is built.",
     [("workload_type", "agent | mcp_server"), ("source_type", "repository | image | agent_kind"),
      ("repo_url, build_type", "")], ["secret"]),
    ("build", "Build", "builds",
     "One build of a hosted agent into an image, claimed by the build dispatcher.",
     [("status", "pending ... succeeded"), ("image_ref", ""), ("commit_id", "")], ["queue"]),
    ("kind", "Agent kind", "agent_kinds, agent_kind_versions, agent_kind_labels",
     "A published, reusable agent image others can deploy.",
     [("name", "unique per org"), ("version", "unique per kind"), ("image_ref", "")], []),
    ("release", "Release and deployment", "agent_releases, agent_deployments",
     "An immutable image + config snapshot, and which release runs in each environment.",
     [("desired_state", "active | suspended | undeployed"), ("status", ""), ("promoted_from_environment_id", "")],
     ["env", "queue"]),
    ("rconf", "Runtime config", "agent_runtime_configs, agent_env_vars, agent_file_mounts",
     "Resources, scaling, sandbox tier, instrumentation, env vars and files per environment.",
     [("isolation_tier", "sandbox"), ("replicas", ""), ("secret_id", "sensitive values")], ["env", "secret"]),
    ("memory", "Agent memory store", "agent_memory_stores",
     "Memory a hosted agent keeps, and where. The memory itself lives in the backend.",
     [("memory_type", "short_term | long_term"), ("backend", ""), ("retention_days", "")], ["env", "secret"]),
]
for rows, area in ((evaluation, "eval"), (runtime, "runtime")):
    x = 60
    for key, t, tb, p, f, ch in rows:
        card(key, area, x, 0, RW, t, tb, p, f, chips=ch)
        x += RSP
DW = 360
dx = 60
outside = [
    ("o_thunder", "Thunder", "own database",
     "Issues agent identities. Ours stores only the IDs it returns. One Thunder per install, set in config."),
    ("o_obs", "Traces, logs, metrics", "observability store",
     "Agent traces, runtime and build logs, and metrics live outside this database."),
    ("o_users", "Users and roles", "config file",
     "Console sign-in and role mapping come from a file. Tables refer to users by username."),
    ("o_mem", "Memory contents", "memory backend",
     "Agent memory records live in the configured backend; this database holds only the store's settings."),
]
for key, t, tb, p in outside:
    card(key, "deferred", dx, 0, DW, t, tb, p, deferred=True)
    dx += DW + 24

# ---------------------------------------------------------------- Areas


def bbox(keys, pad=30, top=58):
    xs = [cards[k].x for k in keys]
    ys = [cards[k].y for k in keys]
    xe = [cards[k].x + cards[k].w for k in keys]
    ye = [cards[k].y + cards[k].h for k in keys]
    return [min(xs) - pad, min(ys) - top, max(xe) + pad, max(ye) + pad]


area_boxes = {}
LEFT_X0, LEFT_X1 = 30, 915
GW_X0, GW_X1 = 945, 2002
R_X0, R_X1 = 2022, 2850
t_b = bbox(["project", "org", "env"])
area_boxes["tenancy"] = [LEFT_X0, t_b[1], LEFT_X1, t_b[3] + 104]
ag_b = bbox(["agent", "labels", "identity", "gwset", "aiapp", "exposure", "acard", "a2a", "mconf", "mlinks"])
gw_b = bbox(["gwenv", "gateway", "gwkids", "apikeys", "artifact", "deploy", "status", "prov", "lproxy",
             "mproxy", "mmap", "tmpl", "scopes", "endpoint"])
MID_BOTTOM = max(ag_b[3], gw_b[3]) + 70
area_boxes["agents"] = [LEFT_X0, area_boxes["tenancy"][3] + 22, LEFT_X1, MID_BOTTOM]
area_boxes["gateway"] = [GW_X0, area_boxes["tenancy"][1], GW_X1, MID_BOTTOM]
s_b = bbox(["secret"])
area_boxes["secrets"] = [R_X0, s_b[1], R_X1, s_b[3] + 6]
e_b = bbox(["sub", "dest", "delivery", "subfilt", "destproj", "outbox"])
area_boxes["events"] = [R_X0, area_boxes["secrets"][3] + 22, R_X1, e_b[3]]
for k in ("audit", "usage"):
    cards[k].y = area_boxes["events"][3] + 22 + 58
a_b = bbox(["audit", "usage"])
area_boxes["audit"] = [R_X0, a_b[1], R_X1, a_b[3]]
for k in ("locks", "queues"):
    cards[k].y = area_boxes["audit"][3] + 22 + 58
i_b = bbox(["locks", "queues"])
bottom = max(i_b[3], MID_BOTTOM)
area_boxes["internal"] = [R_X0, i_b[1], R_X1, bottom]
area_boxes["agents"][3] = area_boxes["gateway"][3] = bottom
MID_BOTTOM = bottom
y_top = bottom + 22
for area in ("eval", "runtime", "deferred"):
    row_y = y_top + 58
    rh = max(c.h for c in cards.values() if c.area == area)
    for c in cards.values():
        if c.area == area:
            c.y = row_y
            c.h = rh
    area_boxes[area] = [LEFT_X0, y_top, R_X1, row_y + rh + 30]
    y_top = area_boxes[area][3] + 22
H = int(area_boxes["deferred"][3] + 30)

# ---------------------------------------------------------------- Arrows
arrows = []


def arrow(points, label=None, lpos=None, style="own", head=True):
    arrows.append((points, label, lpos, style, head))


c = cards
# tenancy
oy = c["org"].y + c["org"].h / 2
arrow([(c["org"].x, oy), (c["project"].x + c["project"].w, oy)], "has many",
      ((c["org"].x + c["project"].x + c["project"].w) / 2, oy - 8, "middle"), "ref")
ey0 = c["org"].y + 50
arrow([(c["org"].x + c["org"].w, ey0), (c["env"].x, ey0)], "has one",
      ((c["org"].x + c["org"].w + c["env"].x) / 2, ey0 - 8, "middle"), "ref")
ox = c["org"].x + c["org"].w / 2
gy = c["org"].y - 16
arrow([(ox, c["org"].y), (ox, gy), (c["gateway"].x + 60, gy), (c["gateway"].x + 60, c["gateway"].y)],
      "an organization has many gateways", (775, gy - 7, "middle"), "ref")
ey = c["env"].y + 40
arrow([(c["gwenv"].x, ey), (c["env"].x + c["env"].w, ey)], "attached to",
      ((c["gwenv"].x + c["env"].x + c["env"].w) / 2, ey - 8, "middle"), "ref")
gwy = c["gwenv"].y + 40
arrow([(c["gateway"].x, gwy), (c["gwenv"].x + c["gwenv"].w, gwy)], "has",
      ((c["gwenv"].x + c["gwenv"].w + c["gateway"].x) / 2, gwy - 8, "middle"), "own")
# project -> agent
px = c["project"].x + 210
arrow([(px, c["project"].y + c["project"].h), (px, c["agent"].y)], "has many agents",
      (px - 8, c["project"].y + c["project"].h + 30, "end"), "ref")
# agent -> model config -> bindings
arrow([c["agent"].bottom(), c["mconf"].top()], "has many",
      (c["agent"].x + c["agent"].w / 2 + 8, c["agent"].y + c["agent"].h + 32, "start"), "own")
arrow([c["mconf"].bottom(), c["mlinks"].top()], "binds",
      (c["mconf"].x + c["mconf"].w / 2 + 8, c["mconf"].y + c["mconf"].h + 32, "start"), "own")
# agent bus
T1, T2 = 335, 620
BUS_TOP = c["agent"].y - 18
bus_end1 = c["gwset"].y + c["gwset"].h / 2
bus_end2 = c["a2a"].y + c["a2a"].h / 2
arrow([(c["agent"].x + c["agent"].w, c["agent"].y + 40), (T1, c["agent"].y + 40)], head=False)
arrow([(T1, BUS_TOP), (T1, bus_end1)], head=False)
arrow([(T1, BUS_TOP), (T2, BUS_TOP), (T2, bus_end2)], head=False)
for k in ("labels", "identity", "gwset"):
    yy = c[k].y + c[k].h / 2
    arrow([(T1, yy), (c[k].x, yy)])
for k in ("aiapp", "exposure", "acard", "a2a"):
    yy = c[k].y + c[k].h / 2
    arrow([(T2, yy), (c[k].x, yy)])
# gateway -> kids
kids_y = c["gateway"].y + 50
arrow([(c["gateway"].x + c["gateway"].w, kids_y), (c["gwkids"].x, kids_y)], "has",
      ((c["gateway"].x + c["gateway"].w + c["gwkids"].x) / 2, kids_y - 8, "middle"), "own")
# gateway -> deployments
gx = c["gateway"].x + c["gateway"].w - 40
mid_y = max((c["gateway"].y + c["gateway"].h + c["deploy"].y) / 2, c["gwkids"].y + c["gwkids"].h + 16)
dxp = c["deploy"].x + c["deploy"].w / 2
arrow([(gx, c["gateway"].y + c["gateway"].h), (gx, mid_y), (dxp, mid_y), (dxp, c["deploy"].y)],
      "receives deployments", (dxp + 8, mid_y + (c["deploy"].y - mid_y) / 2 + 4, "start"), "ref")
# artifact -> deploy, status, api keys
ay = c["deploy"].y + 40
arrow([(c["artifact"].x + c["artifact"].w, ay), (c["deploy"].x, ay)], "has",
      ((c["artifact"].x + c["artifact"].w + c["deploy"].x) / 2, ay - 8, "middle"), "own")
sy = c["status"].y + 40
sx = c["artifact"].x + c["artifact"].w + 30
ay2 = c["artifact"].y + c["artifact"].h - 40
arrow([(c["artifact"].x + c["artifact"].w, ay2), (sx, ay2), (sx, sy), (c["status"].x, sy)])
ky = c["apikeys"].y + 40
arrow([(c["artifact"].x, ky), (c["apikeys"].x + c["apikeys"].w, ky)], "has",
      ((c["artifact"].x + c["apikeys"].x + c["apikeys"].w) / 2, ky - 8, "middle"), "own")
# typed bus -> artifact
TB = c["prov"].y - 34
acx = c["artifact"].x + c["artifact"].w / 2
art_bottom = c["artifact"].y + c["artifact"].h
for k in ("prov", "lproxy", "mproxy", "mmap"):
    kx = c[k].x + c[k].w / 2
    arrow([(kx, c[k].y), (kx, TB)], style="isa", head=False)
arrow([(c["prov"].x + c["prov"].w / 2, TB), (c["mmap"].x + c["mmap"].w / 2, TB)], style="isa", head=False)
arrow([(acx, TB), (acx, art_bottom)], "each is an artifact (same id), deleted with it",
      (acx - 10, TB - 10, "end"), "isa")
# lproxy -> prov
ly = c["lproxy"].y + c["lproxy"].h - 40
arrow([(c["lproxy"].x, ly), (c["prov"].x + c["prov"].w, ly)], style="ref")
# prov -> template
tx = c["prov"].x + c["prov"].w / 2
arrow([(tx, c["prov"].y + c["prov"].h), (tx, c["tmpl"].y)], "built from (by handle)",
      (tx + 8, (c["prov"].y + c["prov"].h + c["tmpl"].y) / 2 + 4, "start"), "ref")
# mmap -> mproxy
my = c["mmap"].y + c["mmap"].h - 30
arrow([(c["mmap"].x, my), (c["mproxy"].x + c["mproxy"].w, my)], style="ref")
# mproxy -> scopes / endpoint
sxx = c["mproxy"].x + 70
arrow([(sxx, c["mproxy"].y + c["mproxy"].h), (sxx, c["scopes"].y)], "has",
      (sxx + 8, (c["mproxy"].y + c["mproxy"].h + c["scopes"].y) / 2 + 4, "start"), "own")
exx = c["mproxy"].x + c["mproxy"].w - 30
emid = c["scopes"].y - 22
enx = c["endpoint"].x + 60
arrow([(exx, c["mproxy"].y + c["mproxy"].h), (exx, emid), (enx, emid), (enx, c["endpoint"].y)])
# config bindings -> proxies (bottom route)
R1 = MID_BOTTOM - 46
R2 = MID_BOTTOM - 24
m = c["mlinks"]
lpx = c["lproxy"].x + 40
arrow([(m.x + 90, m.y + m.h), (m.x + 90, R1), (lpx, R1), (lpx, c["lproxy"].y + c["lproxy"].h)],
      "an LLM config uses an LLM proxy", (650, R1 - 7, "middle"), "ref")
gapx = (c["lproxy"].x + c["lproxy"].w + c["mproxy"].x) / 2
mpy = c["mproxy"].y + c["mproxy"].h - 26
arrow([(m.x + 160, m.y + m.h), (m.x + 160, R2), (gapx, R2), (gapx, mpy), (c["mproxy"].x, mpy)],
      "an MCP config uses an MCP proxy", (650, R2 + 15, "middle"), "ref")
# events (routed above the cards so labels have room)
for a_, b_, lab, st, off in (("sub", "dest", "sends to; in-use can't be deleted", "ref", 30),
                             ("dest", "delivery", "keeps delivery history", "own", 30)):
    x1 = c[a_].x + c[a_].w - 60
    x2 = c[b_].x + 60
    yt = c[a_].y - off
    arrow([(x1, c[a_].y), (x1, yt), (x2, yt), (x2, c[b_].y)], lab, ((x1 + x2) / 2, yt - 8, "middle"), st)
for a_, b_, lab in (("sub", "subfilt", "filtered by"), ("dest", "destproj", "limited to")):
    xx = c[a_].x + 50
    arrow([(xx, c[a_].y + c[a_].h), (xx, c[b_].y)], lab,
          (xx + 8, (c[a_].y + c[a_].h + c[b_].y) / 2 + 4, "start"), "own")
ox_ = c["outbox"].x + 50
arrow([(ox_, c["outbox"].y), (ox_, c["delivery"].y + c["delivery"].h)], "becomes",
      (ox_ + 8, (c["delivery"].y + c["delivery"].h + c["outbox"].y) / 2 + 4, "start"), "flow")

# ---------------------------------------------------------------- Render
STYLE = {
    "own": dict(dash="", color="#334155", marker="arrow"),
    "ref": dict(dash="6 4", color="#64748b", marker="arrowg"),
    "isa": dict(dash="", color="#7c3aed", marker="arrowp"),
    "flow": dict(dash="2 4", color="#0891b2", marker="arrowc"),
}


def path_d(points):
    return "M " + " L ".join(f"{x:.1f} {y:.1f}" for x, y in points)


def render_svg(viewbox=None, width=None):
    vb = viewbox or (0, 0, W, H)
    out = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{vb[0]} {vb[1]} {vb[2]} {vb[3]}" '
           f'width="{width or "100%"}" role="img" aria-labelledby="dtitle ddesc">']
    out.append('<title id="dtitle">Agent Manager data model</title>')
    out.append('<desc id="ddesc">Main tables of AMP Lite grouped into tenancy, agents, AI gateway, secrets, '
               'events, audit, internals, evaluation and agent runtime, with labelled relationships.</desc>')
    out.append("""<defs>
<filter id="shadow" x="-5%" y="-5%" width="110%" height="115%"><feDropShadow dx="0" dy="1" stdDeviation="1.4" flood-color="#0f172a" flood-opacity="0.10"/></filter>
<marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse"><path d="M0 0 L10 5 L0 10 z" fill="#334155"/></marker>
<marker id="arrowg" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse"><path d="M0 0 L10 5 L0 10 z" fill="#64748b"/></marker>
<marker id="arrowp" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="9" markerHeight="9" orient="auto-start-reverse"><path d="M0 0 L10 5 L0 10 z" fill="#ffffff" stroke="#7c3aed" stroke-width="1.5"/></marker>
<marker id="arrowc" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse"><path d="M0 0 L10 5 L0 10 z" fill="#0891b2"/></marker>
</defs>""")
    out.append(f'<rect x="0" y="0" width="{W}" height="{H}" fill="#ffffff"/>')
    out.append('<text x="30" y="52" class="title">Agent Manager data model</text>')
    out.append('<text x="32" y="80" class="subtitle">One schema for all three tiers (AI Gateway, Agent Manager, Agent Runtime), created on first '
               'start. Tables a tier does not use stay empty; environments stay hidden below Agent Runtime.</text>')
    # legend
    lx, ly0 = 1560, 14
    out.append(f'<rect x="{lx}" y="{ly0}" width="1290" height="104" rx="8" fill="#f8fafc" stroke="#cbd5e1"/>')
    out.append(f'<text x="{lx + 14}" y="{ly0 + 22}" class="legtitle">Legend</text>')
    items = [
        ("own", "owns: child is deleted with its parent (cascade)"),
        ("ref", "refers to: no cascade; delete is blocked or done by the service"),
        ("isa", "is a: typed row shares its artifact's id"),
        ("flow", "a worker turns one into the other"),
    ]
    for i, (st, txt) in enumerate(items):
        col_x = lx + 14 + (i % 2) * 400
        yy = ly0 + 50 + (i // 2) * 28
        s = STYLE[st]
        dash = f' stroke-dasharray="{s["dash"]}"' if s["dash"] else ""
        out.append(f'<path d="M {col_x} {yy - 4} L {col_x + 44} {yy - 4}" stroke="{s["color"]}" stroke-width="1.8" '
                   f'fill="none"{dash} marker-end="url(#{s["marker"]})"/>')
        out.append(f'<text x="{col_x + 54}" y="{yy}" class="legtext">{esc(txt)}</text>')
    chip_x = lx + 830
    for i, (key, desc) in enumerate([("env", "one row per environment; in Lite always the hidden default"),
                                     ("secret", "credential lives in Secrets, not in this row"),
                                     ("queue", "workers claim rows with a lease")]):
        label, bg, fg = CHIPS[key]
        yy = ly0 + 10 + i * 22
        cw = len(label) * 6.1 + 16
        out.append(f'<rect x="{chip_x}" y="{yy}" width="{cw:.0f}" height="18" rx="9" fill="{bg}"/>')
        out.append(f'<text x="{chip_x + cw / 2:.0f}" y="{yy + 13}" class="chip" fill="{fg}" text-anchor="middle">{esc(label)}</text>')
        out.append(f'<text x="{chip_x + 118}" y="{yy + 13}" class="legtext">{esc(desc)}</text>')
    yy = ly0 + 10 + 3 * 22 + 1
    out.append(f'<rect x="{chip_x}" y="{yy}" width="100" height="16" rx="4" fill="#ffffff" stroke="#94a3b8" stroke-dasharray="5 4"/>')
    out.append(f'<text x="{chip_x + 118}" y="{yy + 12}" class="legtext">dashed card: hidden, or outside this database</text>')
    # areas
    for key, box in area_boxes.items():
        a = AREAS[key]
        x0, y0, x1, y1 = box
        dash = ' stroke-dasharray="8 6"' if key == "deferred" else ""
        out.append(f'<rect x="{x0}" y="{y0}" width="{x1 - x0}" height="{y1 - y0}" rx="14" fill="{a["fill"]}" '
                   f'stroke="{a["color"]}" stroke-opacity="0.45" stroke-width="1.5"{dash}/>')
        out.append(f'<text x="{x0 + 18}" y="{y0 + 30}" class="aname" fill="{a["color"]}">{esc(a["name"].upper())}</text>')
    tb = area_boxes["tenancy"]
    tnotes = ["Every table carries org_id, and children point at parents through (org_id, parent id),",
              "so no row can reach another org's data.",
              "First start seeds the default org, its default environment and deployment pipeline,",
              "a default project, and the bundled gateway with its token."]
    for i, ln in enumerate(tnotes):
        out.append(f'<text x="{tb[0] + 290}" y="{tb[3] - 84 + i * 18 + (8 if i >= 2 else 0)}" class="anote">{esc(ln)}</text>')
    ab = area_boxes["audit"]
    out.append(f'<text x="{ab[0] + 380}" y="{ab[1] + 30}" class="anote">No foreign keys: rows outlive what they describe.</text>')
    sc = cards["secret"]
    notes = ["Referenced by secret_id from LLM providers,",
             "LLM proxies, MCP endpoints, A2A proxies and",
             "event destinations. The service deletes a",
             "secret after its owner.",
             "",
             "Production can keep values in an external",
             "vault instead (provider + external_ref)."]
    for i, ln in enumerate(notes):
        out.append(f'<text x="{sc.x + sc.w + 30}" y="{sc.y + 24 + i * 19}" class="anote2">{esc(ln)}</text>')
    out.append(f'<text x="{T1 + 12}" y="{BUS_TOP - 8}" class="alabel">each agent owns these; they are deleted with it</text>')
    for pts, label, lpos, style, head in arrows:
        s = STYLE[style]
        dash = f' stroke-dasharray="{s["dash"]}"' if s["dash"] else ""
        mk = f' marker-end="url(#{s["marker"]})"' if head else ""
        out.append(f'<path d="{path_d(pts)}" fill="none" stroke="{s["color"]}" stroke-width="1.7"{dash}{mk}/>')
    for pts, label, lpos, style, head in arrows:
        if label and lpos:
            x, y, anc = lpos
            out.append(f'<text x="{x:.1f}" y="{y:.1f}" text-anchor="{anc}" class="alabel">{esc(label)}</text>')
    for cd in cards.values():
        out.append(cd.svg())
    out.append("</svg>")
    return "\n".join(out)


CSS = """
:root { color-scheme: light; }
html, body { margin: 0; background: #ffffff; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; }
svg { display: block; height: auto; }
svg text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; }
.title { font-size: 30px; font-weight: 700; fill: #0f172a; }
.subtitle { font-size: 15px; fill: #475569; }
.aname { font-size: 13px; font-weight: 700; letter-spacing: 0.08em; }
.anote { font-size: 12.5px; fill: #475569; font-style: italic; }
.anote2 { font-size: 13px; fill: #7f1d1d; }
.ctitle { font-size: 15px; font-weight: 700; }
svg .ctable { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 11px; fill: #64748b; }
.cpurpose { font-size: 12.5px; }
.crel { font-size: 11.5px; font-style: italic; }
.fkey { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 11.5px; fill: #1e293b; }
.fnote { font-size: 11.5px; fill: #64748b; }
.chip { font-size: 10.5px; font-weight: 600; }
.badge { font-size: 10.5px; font-weight: 700; fill: #ffffff; letter-spacing: 0.06em; }
.alabel { font-size: 11.5px; fill: #334155; font-weight: 600; paint-order: stroke; stroke: #ffffff; stroke-width: 4px; stroke-linejoin: round; }
.legtitle { font-size: 13px; font-weight: 700; fill: #0f172a; }
.legtext { font-size: 12px; fill: #334155; }
"""


def page(svg, title="Agent Manager data model"):
    return f"""<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{title}</title>
<style>{CSS}</style>
</head>
<body>
{svg}
</body>
</html>
"""


with open(OUT, "w") as f:
    f.write(page(render_svg()))
print(f"{W}x{H}")

if CROPS:
    os.makedirs(CROPS, exist_ok=True)
    hw = W // 2
    hh = (H + 1) // 2
    for i, (x, y) in enumerate([(0, 0), (hw, 0), (0, hh), (hw, hh)]):
        with open(os.path.join(CROPS, f"crop{i}.html"), "w") as f:
            f.write(page(render_svg((x, y, hw, hh), width=hw)))
    print(f"crop {hw}x{hh}")
