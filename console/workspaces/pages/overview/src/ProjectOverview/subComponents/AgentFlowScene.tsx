/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React from "react";
import { Box } from "@wso2/oxygen-ui";
import {
  Activity,
  Bot,
  Brain,
  Cloud,
  Globe,
  IdCard,
  Network,
  ShieldCheck,
  Users,
  Wrench,
} from "@wso2/oxygen-ui-icons-react";
import { REDUCED } from "../../OrganizationOverview/motion";
import {
  clamp01,
  drawGlow,
  entrance,
  LINE_COLOR,
  LINE_RGB,
  useAnimatedCanvas,
  type DrawFrame,
} from "./useAnimatedCanvas";

export type AgentHosting = "internal" | "external";

type NodeId = "users" | "ingress" | "agent" | "identity" | "gateway" | "traces" | "llm" | "mcp";
/** A position as fractions of the square canvas. */
interface Point {
  x: number;
  y: number;
}
interface Boundary {
  x0: number;
  y0: number;
  x1: number;
  y1: number;
  icon: React.ReactNode;
  /** The user's own infrastructure, as opposed to the platform. */
  dashed?: boolean;
}

const ICON = 18;
const BADGE_ICON = 14;
const NODE_R = 0.068;
const HERO_R = 0.085;

// Both scenes share the same icons and traffic; the layout and routes differ.
const NODE_ICONS: Record<NodeId, React.ReactNode> = {
  users: <Users size={ICON} />,
  ingress: <Network size={ICON} />,
  agent: <Bot size={ICON} />,
  identity: <IdCard size={ICON} />,
  gateway: <ShieldCheck size={ICON} />,
  traces: <Activity size={ICON} />,
  llm: <Brain size={ICON} />,
  mcp: <Wrench size={ICON} />,
};
/**
 * Request journeys; each passes the guardrails ("gateway") on its way out. A platform-hosted
 * agent is reached through the environment's ingress gateway, which checks CORS and auth
 * first; an external agent is reached in the user's own infrastructure.
 */
const ROUTES: Record<AgentHosting, NodeId[][]> = {
  internal: [
    ["users", "ingress", "agent", "gateway", "llm"],
    ["users", "ingress", "agent", "gateway", "mcp"],
  ],
  external: [
    ["users", "agent", "gateway", "llm"],
    ["users", "agent", "gateway", "mcp"],
  ],
};
/** The agent sends traces; with AgentID, the platform issues the agent its identity. */
const TELEMETRY: [NodeId, NodeId] = ["agent", "traces"];
const IDENTITY: [NodeId, NodeId] = ["identity", "agent"];

const LAYOUTS: Record<
  AgentHosting,
  { boundaries: Boundary[]; at: Partial<Record<NodeId, Point>> }
> = {
  internal: {
    // Users reach in from the left; the ingress gateway is the platform's front door.
    boundaries: [{ x0: 0.24, y0: 0.06, x1: 0.94, y1: 0.77, icon: <Cloud size={BADGE_ICON} /> }],
    at: {
      users: { x: 0.11, y: 0.17 },
      ingress: { x: 0.4, y: 0.17 },
      agent: { x: 0.4, y: 0.42 },
      traces: { x: 0.76, y: 0.42 },
      gateway: { x: 0.4, y: 0.66 },
      identity: { x: 0.76, y: 0.66 },
      llm: { x: 0.22, y: 0.89 },
      mcp: { x: 0.58, y: 0.89 },
    },
  },
  external: {
    boundaries: [
      { x0: 0.06, y0: 0.04, x1: 0.94, y1: 0.36, icon: <Globe size={BADGE_ICON} />, dashed: true },
      { x0: 0.06, y0: 0.43, x1: 0.94, y1: 0.71, icon: <Cloud size={BADGE_ICON} /> },
    ],
    at: {
      users: { x: 0.26, y: 0.2 },
      agent: { x: 0.6, y: 0.2 },
      traces: { x: 0.26, y: 0.57 },
      gateway: { x: 0.6, y: 0.57 },
      identity: { x: 0.82, y: 0.57 },
      llm: { x: 0.42, y: 0.88 },
      mcp: { x: 0.78, y: 0.88 },
    },
  },
};

// About the pace of the globe's dots and comet, so the two read as one illustration.
const SPEED = 0.11; // canvas fractions per second
const SPAWN_EVERY = 1.6; // seconds between requests, spaced out to match the slower pace
const BLOCK_RATE = 0.3; // share of requests the guardrail stops
const REJECT_RATE = 0.2; // share of requests the ingress gateway turns away (failed auth)
const BURST_S = 0.8; // how long a stopped request takes to fade
const FLASH_S = 0.45; // how long the guardrail reacts to a request
const TELEMETRY_EVERY = 1;
const IDENTITY_EVERY = 2.2; // seconds between identity tokens
const GLOW_R = 0.26; // radius of the warm glow, as a fraction of the canvas

// Entrance, in seconds after the scene is shown: frame, then links drawn along the
// flow, then nodes popping in the order traffic reaches them, then the traffic itself.
const ENTRY_ORDER: NodeId[] = [
  "users",
  "ingress",
  "agent",
  "identity",
  "gateway",
  "traces",
  "llm",
  "mcp",
];
const nodeEntry = (id: NodeId) => 0.12 + 0.06 * ENTRY_ORDER.indexOf(id);
const FLOW_ENTRY = 0.55;

const BOUNDARY_STROKE = `rgba(${LINE_RGB}, 0.32)`;
const PLATFORM_FILL = `rgba(${LINE_RGB}, 0.06)`;
const INFRA_FILL = `rgba(${LINE_RGB}, 0.02)`;
const LINK_STROKE = `rgba(${LINE_RGB}, 0.4)`;
const NODE_FILL = `rgba(${LINE_RGB}, 0.08)`;
const NODE_STROKE = `rgba(${LINE_RGB}, 0.6)`;

const hash = (n: number) => {
  const x = Math.sin(n * 12.9898) * 43758.5453;
  return x - Math.floor(x);
};

interface SceneNode extends Point {
  id: NodeId;
  /** The agent: larger, with the accent runtime ring. */
  hero: boolean;
  r: number;
  /** Clear space around it: its disc, plus the agent's runtime ring. */
  clearance: number;
}
interface Hop {
  x0: number;
  y0: number;
  x1: number;
  y1: number;
}

/** A link from edge to edge of two nodes, so no line shows inside a disc. */
function trimmed(a: SceneNode, b: SceneNode): Hop {
  const len = Math.hypot(b.x - a.x, b.y - a.y) || 1;
  const ux = (b.x - a.x) / len;
  const uy = (b.y - a.y) / len;
  return {
    x0: a.x + ux * a.clearance,
    y0: a.y + uy * a.clearance,
    x1: b.x - ux * b.clearance,
    y1: b.y - uy * b.clearance,
  };
}

/** A route as a polyline, with each corner's distance from the start. */
function polyline(points: SceneNode[]) {
  const at = [0];
  for (let i = 1; i < points.length; i++) {
    at.push(at[i - 1] + Math.hypot(points[i].x - points[i - 1].x, points[i].y - points[i - 1].y));
  }
  return { points, at, length: at[at.length - 1] };
}

/** The point `distance` along a polyline, written into `out` to avoid allocating per frame. */
function pointAt(line: ReturnType<typeof polyline>, distance: number, out: Point) {
  const d = Math.max(0, Math.min(line.length, distance));
  let i = 1;
  while (i < line.at.length - 1 && line.at[i] < d) i++;
  const a = line.points[i - 1];
  const b = line.points[i];
  const k = (d - line.at[i - 1]) / (line.at[i] - line.at[i - 1] || 1);
  out.x = a.x + (b.x - a.x) * k;
  out.y = a.y + (b.y - a.y) * k;
  return out;
}

/**
 * Calls `each` with the progress (0..1) of every item currently travelling a link,
 * for items sent every `every` seconds that take `span` seconds to cross it.
 */
function forEachInFlight(time: number, every: number, span: number, each: (f: number) => void) {
  for (let k = Math.floor((time - span) / every); k * every <= time; k++) {
    const age = time - k * every;
    if (age >= 0 && age <= span) each(age / span);
  }
}

function roundRect(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  r: number,
) {
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + w, y, x + w, y + h, r);
  ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r);
  ctx.arcTo(x, y, x + w, y, r);
  ctx.closePath();
}

function buildScene(hosting: AgentHosting, showIdentity: boolean) {
  const layout = LAYOUTS[hosting];
  const routes = ROUTES[hosting];
  const ids = (Object.keys(layout.at) as NodeId[]).filter((id) => showIdentity || id !== "identity");
  const nodes = Object.fromEntries(
    ids.map((id) => {
      const hero = id === "agent";
      const r = hero ? HERO_R : NODE_R;
      return [id, { ...(layout.at[id] as Point), id, hero, r, clearance: hero ? r + 0.02 : r }];
    }),
  ) as Record<NodeId, SceneNode>;
  const nodeList = ids.map((id) => nodes[id]);

  const lines = routes.map((route) => polyline(route.map((id) => nodes[id])));
  // Requests carry the identity from the agent onward, so only past this point.
  const agentDistance = lines[0].at[routes[0].indexOf("agent")];
  // Where requests can be stopped, in the order they reach them: the ingress gateway
  // (when the route has one) turns some away, then the guardrails stop some. Every
  // route shares its start up to the guardrails, so the first route's distances hold.
  const gates = (["ingress", "gateway"] as const)
    .filter((id) => routes[0].includes(id))
    .map((id) => ({
      node: nodes[id],
      stopAt: lines[0].at[routes[0].indexOf(id)] - NODE_R,
      rate: id === "ingress" ? REJECT_RATE : BLOCK_RATE,
    }));
  // Every distinct hop, trimmed to the discs' edges, in the order traffic takes them.
  const hops = [
    ...new Map(
      routes.flatMap((route) =>
        route
          .slice(1)
          .map((id, i) => [`${route[i]}>${id}`, trimmed(nodes[route[i]], nodes[id])] as const),
      ),
    ).values(),
  ];
  const telHop = trimmed(nodes[TELEMETRY[0]], nodes[TELEMETRY[1]]);
  const idHop = showIdentity ? trimmed(nodes[IDENTITY[0]], nodes[IDENTITY[1]]) : null;
  /** 0 inside any disc, rising to 1 just outside it, so dots slip into nodes. */
  const outside = (x: number, y: number) => {
    let v = 1;
    for (const n of nodeList) {
      v = Math.min(v, clamp01((Math.hypot(x - n.x, y - n.y) - n.clearance) / 0.02));
    }
    return v;
  };
  const spot: Point = { x: 0, y: 0 };

  const draw: DrawFrame = (ctx, size, time, palette, sinceShown) => {
    const px = (v: number) => v * size;
    const enter = (start: number, duration: number) => entrance(sinceShown, start, duration);
    // Moving parts wait until the scene has built up.
    const flow = enter(FLOW_ENTRY, 0.3);
    ctx.clearRect(0, 0, size, size);
    ctx.lineCap = "round";

    // The same warm glow as the globe's core, behind everything, so the scenes and
    // the globe share a centre as they crossfade.
    drawGlow(ctx, size / 2, size / 2, size * GLOW_R, palette.accent, enter(0, 0.5));

    // Boundaries: the platform solid, the user's own infrastructure dashed.
    ctx.globalAlpha = enter(0, 0.3);
    ctx.strokeStyle = BOUNDARY_STROKE;
    ctx.lineWidth = 1;
    for (const b of layout.boundaries) {
      roundRect(ctx, px(b.x0), px(b.y0), px(b.x1 - b.x0), px(b.y1 - b.y0), 14);
      ctx.fillStyle = b.dashed ? INFRA_FILL : PLATFORM_FILL;
      ctx.fill();
      ctx.setLineDash(b.dashed ? [4, 4] : []);
      ctx.stroke();
    }
    ctx.setLineDash([]);
    ctx.globalAlpha = 1;

    // Links, each drawn along its length as the scene enters, one after another; the
    // telemetry and identity links dotted.
    const link = (hop: Hop, grown: number) => {
      ctx.moveTo(px(hop.x0), px(hop.y0));
      ctx.lineTo(px(hop.x0 + (hop.x1 - hop.x0) * grown), px(hop.y0 + (hop.y1 - hop.y0) * grown));
    };
    ctx.strokeStyle = LINK_STROKE;
    ctx.beginPath();
    hops.forEach((hop, i) => link(hop, enter(0.08 + 0.05 * i, 0.35)));
    ctx.stroke();
    ctx.setLineDash([2, 4]);
    ctx.beginPath();
    link(telHop, enter(0.35, 0.35));
    if (idHop) link(idHop, enter(0.3, 0.35));
    ctx.stroke();
    ctx.setLineDash([]);

    // Telemetry: small, faint dots streaming from the agent to traces.
    ctx.fillStyle = LINE_COLOR;
    const telSpan = Math.hypot(telHop.x1 - telHop.x0, telHop.y1 - telHop.y0) / (SPEED * 0.8);
    forEachInFlight(time, TELEMETRY_EVERY, telSpan, (f) => {
      const x = telHop.x0 + (telHop.x1 - telHop.x0) * f;
      const y = telHop.y0 + (telHop.y1 - telHop.y0) * f;
      ctx.globalAlpha = 0.6 * Math.sin(Math.PI * f) * outside(x, y) * flow;
      ctx.beginPath();
      ctx.arc(px(x), px(y), 1.6, 0, Math.PI * 2);
      ctx.fill();
    });

    // Identity: tokens (hollow rings) travelling from the issuer to the agent.
    if (idHop) {
      ctx.strokeStyle = LINE_COLOR;
      ctx.lineWidth = 1.2;
      const idSpan = Math.hypot(idHop.x1 - idHop.x0, idHop.y1 - idHop.y0) / (SPEED * 0.6);
      forEachInFlight(time, IDENTITY_EVERY, idSpan, (f) => {
        ctx.globalAlpha = 0.8 * Math.sin(Math.PI * f) * flow;
        ctx.beginPath();
        ctx.arc(
          px(idHop.x0 + (idHop.x1 - idHop.x0) * f),
          px(idHop.y0 + (idHop.y1 - idHop.y0) * f),
          3.2,
          0,
          Math.PI * 2,
        );
        ctx.stroke();
      });
    }
    ctx.globalAlpha = 1;

    // Requests. Each is decided by its index, so the scene needs no state. Stopped
    // ones burst at the gate that stopped them; each gate flashes for those it handles.
    const blockFlash = gates.map(() => 0);
    const passFlash = gates.map(() => 0);
    const maxLife = lines[0].length / SPEED + BURST_S;
    ctx.fillStyle = LINE_COLOR;
    ctx.strokeStyle = LINE_COLOR;
    ctx.lineWidth = 1;
    const bursts: number[] = []; // x, y, progress triples
    for (let i = Math.floor((time - maxLife) / SPAWN_EVERY); i * SPAWN_EVERY <= time; i++) {
      const age = time - i * SPAWN_EVERY;
      if (age < 0) continue;
      const line = lines[Math.floor(hash(i + 0.5) * lines.length)];
      // Which gate stops it, if any: the rates are stacked over one roll.
      const roll = hash(i);
      let stoppedBy = -1;
      for (let g = 0, below = 0; g < gates.length; g++) {
        below += gates[g].rate;
        if (roll < below) {
          stoppedBy = g;
          break;
        }
      }
      const travelled = age * SPEED;
      const lastGate = stoppedBy < 0 ? gates.length - 1 : stoppedBy;
      for (let g = 0; g <= lastGate; g++) {
        const sinceGate = (travelled - gates[g].stopAt) / SPEED;
        if (sinceGate >= 0 && sinceGate < FLASH_S) {
          const strength = 1 - sinceGate / FLASH_S;
          if (g === stoppedBy) blockFlash[g] = Math.max(blockFlash[g], strength);
          else passFlash[g] = Math.max(passFlash[g], strength);
        }
      }
      if (stoppedBy >= 0 && travelled >= gates[stoppedBy].stopAt) {
        const stopAt = gates[stoppedBy].stopAt;
        const f = (travelled - stopAt) / SPEED / BURST_S;
        if (f <= 1) {
          pointAt(line, stopAt, spot);
          bursts.push(spot.x, spot.y, f);
        }
        continue;
      }
      if (travelled > line.length) continue;
      const p = pointAt(line, travelled, spot);
      ctx.globalAlpha = outside(p.x, p.y) * flow;
      ctx.beginPath();
      ctx.arc(px(p.x), px(p.y), 3, 0, Math.PI * 2);
      ctx.fill();
      if (idHop && travelled > agentDistance) {
        // Signed with the agent's identity on the way to the guardrails.
        ctx.beginPath();
        ctx.arc(px(p.x), px(p.y), 6, 0, Math.PI * 2);
        ctx.stroke();
      }
    }

    // Nodes: see-through discs with a faint tint, popping in (growing from 80% while
    // fading up). Links stop at their edges; icons sit on top.
    for (const node of nodeList) {
      const shown = enter(nodeEntry(node.id), 0.32);
      if (shown <= 0) continue;
      const r = px(node.r) * (0.8 + 0.2 * shown);
      ctx.globalAlpha = shown;
      ctx.beginPath();
      ctx.arc(px(node.x), px(node.y), r, 0, Math.PI * 2);
      ctx.fillStyle = NODE_FILL;
      ctx.fill();
      ctx.strokeStyle = NODE_STROKE;
      ctx.lineWidth = 1;
      ctx.stroke();
      if (node.hero) {
        // The runtime: an accent arc turning around the agent while it serves.
        ctx.beginPath();
        ctx.arc(px(node.x), px(node.y), r + 4, time * 2.2, time * 2.2 + Math.PI * 0.7);
        ctx.strokeStyle = palette.accent;
        ctx.lineWidth = 1.6;
        ctx.stroke();
      }
    }

    // Each gate reacts: a blue pulse for a request it lets through, an accent ring for
    // one it stops.
    const gr = px(NODE_R);
    gates.forEach((gate, g) => {
      const gx = px(gate.node.x);
      const gy = px(gate.node.y);
      if (passFlash[g] > 0) {
        ctx.globalAlpha = 0.18 * passFlash[g] * flow;
        ctx.fillStyle = LINE_COLOR;
        ctx.beginPath();
        ctx.arc(gx, gy, gr, 0, Math.PI * 2);
        ctx.fill();
      }
      if (blockFlash[g] > 0) {
        ctx.globalAlpha = blockFlash[g] * flow;
        ctx.strokeStyle = palette.accent;
        ctx.lineWidth = 1.6;
        ctx.beginPath();
        ctx.arc(gx, gy, gr + 3 * (1 - blockFlash[g]), 0, Math.PI * 2);
        ctx.stroke();
      }
    });

    // Stopped requests: an accent burst where they met the gate, fading out.
    ctx.fillStyle = palette.accent;
    ctx.strokeStyle = palette.accent;
    ctx.lineWidth = 1;
    for (let b = 0; b < bursts.length; b += 3) {
      const x = px(bursts[b]);
      const y = px(bursts[b + 1]);
      const f = bursts[b + 2];
      ctx.globalAlpha = (1 - f) * flow;
      ctx.beginPath();
      ctx.arc(x, y, 3, 0, Math.PI * 2);
      ctx.fill();
      ctx.globalAlpha = (1 - f) * 0.7 * flow;
      ctx.beginPath();
      ctx.arc(x, y, 3 + 10 * f, 0, Math.PI * 2);
      ctx.stroke();
    }
    ctx.globalAlpha = 1;
  };

  return { boundaries: layout.boundaries, nodeList, draw };
}

/**
 * The single frame shown for reduced motion, per scene: requests in flight and one being
 * stopped (internal: at the ingress gateway, with another fading at the guardrails).
 * Found by stepping through the timeline; recheck if the timing or layout changes.
 */
const STILL_TIME: Record<AgentHosting, number> = { internal: 13.4, external: 9.25 };

const SCENES = {
  internal: { plain: buildScene("internal", false), identity: buildScene("internal", true) },
  external: { plain: buildScene("external", false), identity: buildScene("external", true) },
};

const at = ({ x, y }: Point) =>
  ({
    position: "absolute",
    left: `${x * 100}%`,
    top: `${y * 100}%`,
    transform: "translate(-50%, -50%)",
    display: "flex",
    lineHeight: 0,
  }) as const;

/** An overlay icon fading (and optionally growing) in after `delayMs`, straight out on exit. */
const fadeSx = (shown: boolean, opacity: number, delayMs: number, grow: boolean) =>
  ({
    opacity: shown ? opacity : 0,
    scale: grow && !shown ? "0.8" : "1",
    transition: shown
      ? `opacity 320ms ease-out ${delayMs}ms, scale 320ms cubic-bezier(0.2, 0.7, 0.2, 1) ${delayMs}ms`
      : "opacity 200ms ease-in, scale 200ms ease-in",
    [REDUCED]: { transition: "none" },
  }) as const;

/**
 * How an agent runs and how its traffic flows, drawn with icons only: requests reach
 * the agent, its LLM and tool calls pass the guardrails (some are stopped), and traces
 * stream to the platform. `internal` runs on the platform, behind the environment's
 * ingress gateway (which turns away requests that fail auth); `external` runs in the
 * user's own infrastructure. With `identity`, the platform issues the agent its AgentID and the
 * agent's calls carry it. Decorative.
 */
export function AgentFlowScene({
  hosting,
  identity = false,
  size = 320,
  active = true,
}: {
  hosting: AgentHosting;
  /** Show AgentID: the issuer, tokens to the agent, and requests that carry it. */
  identity?: boolean;
  size?: number;
  active?: boolean;
}) {
  const scene = SCENES[hosting][identity ? "identity" : "plain"];
  const canvasRef = useAnimatedCanvas(scene.draw, { active, stillTime: STILL_TIME[hosting] });
  return (
    <Box
      aria-hidden
      sx={{ position: "relative", width: size, maxWidth: "100%", aspectRatio: "1 / 1" }}
    >
      <Box
        component="canvas"
        ref={canvasRef}
        sx={{ width: "100%", height: "100%", display: "block" }}
      />
      {/* Boundary badges come in with their boundary; node icons just after their disc. */}
      {scene.boundaries.map((b, i) => (
        <Box
          key={i}
          sx={{
            ...at({ x: b.x0 + 0.045, y: b.y0 + 0.045 }),
            color: LINE_COLOR,
            ...fadeSx(active, 0.8, 0, false),
          }}
        >
          {b.icon}
        </Box>
      ))}
      {scene.nodeList.map((node) => (
        <Box
          key={node.id}
          sx={{
            ...at(node),
            color: node.hero ? "primary.main" : LINE_COLOR,
            ...fadeSx(active, 1, Math.round(nodeEntry(node.id) * 1000) + 60, true),
          }}
        >
          {NODE_ICONS[node.id]}
        </Box>
      ))}
    </Box>
  );
}
