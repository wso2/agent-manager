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
import { alpha, Box } from "@wso2/oxygen-ui";
import {
  Activity,
  Bot,
  Brain,
  Cloud,
  Globe,
  IdCard,
  ShieldCheck,
  Users,
  Wrench,
} from "@wso2/oxygen-ui-icons-react";
import {
  LINE_COLOR,
  LINE_RGB,
  entrance,
  useAnimatedCanvas,
  type DrawFrame,
} from "./useAnimatedCanvas";

export type AgentHosting = "internal" | "external";

/** Positions are fractions of the square canvas. */
interface SceneNode {
  x: number;
  y: number;
  icon: React.ReactNode;
  /** The agent: larger, with the accent runtime ring. */
  hero?: boolean;
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
interface Scene {
  nodes: Record<string, SceneNode>;
  boundaries: Boundary[];
  /** Request journeys; each passes the guardrail at `gate`. */
  routes: string[][];
  gate: number;
  /** Trace data the agent sends to observability. */
  telemetry: [string, string];
  /** Where the agent's identity (AgentID) is issued from, when shown. */
  identity?: [string, string];
}

const ICON = 18;
const BADGE_ICON = 14;
const NODE_R = 0.068;
const HERO_R = 0.085;

const SCENES: Record<AgentHosting, Scene> = {
  internal: {
    boundaries: [{ x0: 0.06, y0: 0.24, x1: 0.94, y1: 0.75, icon: <Cloud size={BADGE_ICON} /> }],
    nodes: {
      users: { x: 0.4, y: 0.12, icon: <Users size={ICON} /> },
      agent: { x: 0.4, y: 0.4, icon: <Bot size={ICON} />, hero: true },
      traces: { x: 0.76, y: 0.4, icon: <Activity size={ICON} /> },
      gateway: { x: 0.4, y: 0.63, icon: <ShieldCheck size={ICON} /> },
      llm: { x: 0.22, y: 0.875, icon: <Brain size={ICON} /> },
      mcp: { x: 0.58, y: 0.875, icon: <Wrench size={ICON} /> },
    },
    routes: [
      ["users", "agent", "gateway", "llm"],
      ["users", "agent", "gateway", "mcp"],
    ],
    gate: 2,
    telemetry: ["agent", "traces"],
  },
  external: {
    boundaries: [
      { x0: 0.06, y0: 0.04, x1: 0.94, y1: 0.36, icon: <Globe size={BADGE_ICON} />, dashed: true },
      { x0: 0.06, y0: 0.43, x1: 0.94, y1: 0.71, icon: <Cloud size={BADGE_ICON} /> },
    ],
    nodes: {
      users: { x: 0.26, y: 0.2, icon: <Users size={ICON} /> },
      agent: { x: 0.6, y: 0.2, icon: <Bot size={ICON} />, hero: true },
      traces: { x: 0.26, y: 0.57, icon: <Activity size={ICON} /> },
      gateway: { x: 0.6, y: 0.57, icon: <ShieldCheck size={ICON} /> },
      llm: { x: 0.42, y: 0.88, icon: <Brain size={ICON} /> },
      mcp: { x: 0.78, y: 0.88, icon: <Wrench size={ICON} /> },
    },
    routes: [
      ["users", "agent", "gateway", "llm"],
      ["users", "agent", "gateway", "mcp"],
    ],
    gate: 2,
    telemetry: ["agent", "traces"],
  },
};

/** Where the identity issuer sits in each scene: inside the platform, beside the agent. */
const IDENTITY_AT: Record<AgentHosting, { x: number; y: number }> = {
  internal: { x: 0.18, y: 0.63 },
  external: { x: 0.82, y: 0.57 },
};

/** The scene with the platform issuing the agent its identity (AgentID). */
function withIdentity(hosting: AgentHosting): Scene {
  const scene = SCENES[hosting];
  return {
    ...scene,
    nodes: { ...scene.nodes, identity: { ...IDENTITY_AT[hosting], icon: <IdCard size={ICON} /> } },
    identity: ["identity", "agent"],
  };
}

const SPEED = 0.26; // canvas fractions per second
const SPAWN_EVERY = 0.85; // seconds between requests
const BLOCK_RATE = 0.25; // share of requests the guardrail stops
const BURST_S = 0.8; // how long a stopped request takes to fade
const FLASH_S = 0.45; // how long the guardrail reacts to a request
const TELEMETRY_EVERY = 0.6;
const IDENTITY_EVERY = 2.2; // seconds between identity tokens
const GLOW_R = 0.26; // radius of the warm glow, as a fraction of the canvas

// Entrance, in seconds after the scene is shown: frame, then links drawn along the
// flow, then nodes popping in the order traffic reaches them, then the traffic itself.
const ENTRY_ORDER = ["users", "agent", "identity", "gateway", "traces", "llm", "mcp"];
const NODE_ENTRY = (id: string) => 0.12 + 0.06 * Math.max(0, ENTRY_ORDER.indexOf(id));
const ENTRY_NODE_DELAY_MS = (id: string) => Math.round(NODE_ENTRY(id) * 1000);
const FLOW_ENTRY = 0.55;

const hash = (n: number) => {
  const x = Math.sin(n * 12.9898) * 43758.5453;
  return x - Math.floor(x);
};

/** A route as a polyline, with each corner's distance from the start. */
function polyline(scene: Scene, route: string[]) {
  const points = route.map((id) => scene.nodes[id]);
  const at = [0];
  for (let i = 1; i < points.length; i++) {
    at.push(at[i - 1] + Math.hypot(points[i].x - points[i - 1].x, points[i].y - points[i - 1].y));
  }
  return { points, at, length: at[at.length - 1] };
}

function pointAt(line: ReturnType<typeof polyline>, distance: number) {
  const d = Math.max(0, Math.min(line.length, distance));
  let i = 1;
  while (i < line.at.length - 1 && line.at[i] < d) i++;
  const a = line.points[i - 1];
  const b = line.points[i];
  const span = line.at[i] - line.at[i - 1] || 1;
  const k = (d - line.at[i - 1]) / span;
  return { x: a.x + (b.x - a.x) * k, y: a.y + (b.y - a.y) * k };
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

/** Clear space around a node: its disc, plus the agent's runtime ring. */
const clearance = (node: SceneNode) => (node.hero ? HERO_R + 0.02 : NODE_R);

/** A link from edge to edge of two nodes, so no line shows inside a disc. */
function trimmed(a: SceneNode, b: SceneNode) {
  const len = Math.hypot(b.x - a.x, b.y - a.y) || 1;
  const ux = (b.x - a.x) / len;
  const uy = (b.y - a.y) / len;
  return {
    x0: a.x + ux * clearance(a),
    y0: a.y + uy * clearance(a),
    x1: b.x - ux * clearance(b),
    y1: b.y - uy * clearance(b),
  };
}

function drawScene(scene: Scene): DrawFrame {
  const lines = scene.routes.map((route) => polyline(scene, route));
  const gateDistance = lines[0].at[scene.gate];
  const [telFrom, telTo] = scene.telemetry.map((id) => scene.nodes[id]);
  const telLength = Math.hypot(telTo.x - telFrom.x, telTo.y - telFrom.y);
  const gateway = scene.nodes.gateway;
  const nodeList = Object.values(scene.nodes);
  // Every distinct hop, trimmed to the discs' edges.
  const hops = new Map<string, ReturnType<typeof trimmed>>();
  for (const route of scene.routes) {
    for (let i = 1; i < route.length; i++) {
      hops.set(`${route[i - 1]}>${route[i]}`, trimmed(scene.nodes[route[i - 1]], scene.nodes[route[i]]));
    }
  }
  const telHop = trimmed(telFrom, telTo);
  const [idFrom, idTo] = scene.identity?.map((id) => scene.nodes[id]) ?? [];
  const idHop = idFrom && idTo ? trimmed(idFrom, idTo) : null;
  // Requests carry the identity from the agent onward, so only past this point.
  const agentDistance = lines[0].at[scene.routes[0].indexOf("agent")];
  /** 0 inside any disc, rising to 1 just outside it, so dots slip into nodes. */
  const outside = (x: number, y: number) =>
    nodeList.reduce((v, n) => {
      const gap = Math.hypot(x - n.x, y - n.y) - clearance(n);
      return Math.min(v, Math.max(0, Math.min(1, gap / 0.02)));
    }, 1);

  return (ctx, size, time, palette, sinceShown) => {
    const px = (v: number) => v * size;
    const enter = (start: number, duration: number) => entrance(sinceShown, start, duration);
    // Moving parts wait until the scene has built up.
    const flow = enter(FLOW_ENTRY, 0.3);
    ctx.clearRect(0, 0, size, size);
    ctx.lineCap = "round";

    // The same warm glow as the globe's core, behind everything, so the scenes and
    // the globe share a centre as they crossfade.
    const glowR = size * GLOW_R;
    const glow = ctx.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, glowR);
    glow.addColorStop(0, palette.accent);
    glow.addColorStop(1, alpha(palette.accent, 0));
    ctx.globalAlpha = 0.3 * enter(0, 0.5);
    ctx.fillStyle = glow;
    ctx.fillRect(size / 2 - glowR, size / 2 - glowR, glowR * 2, glowR * 2);
    ctx.globalAlpha = 1;

    // Boundaries: the platform solid, the user's own infrastructure dashed.
    ctx.globalAlpha = enter(0, 0.3);
    for (const b of scene.boundaries) {
      roundRect(ctx, px(b.x0), px(b.y0), px(b.x1 - b.x0), px(b.y1 - b.y0), 14);
      ctx.fillStyle = `rgba(${LINE_RGB}, ${b.dashed ? 0.02 : 0.06})`;
      ctx.fill();
      ctx.setLineDash(b.dashed ? [4, 4] : []);
      ctx.strokeStyle = `rgba(${LINE_RGB}, 0.32)`;
      ctx.lineWidth = 1;
      ctx.stroke();
      ctx.setLineDash([]);
    }
    ctx.globalAlpha = 1;

    // Links: every hop of the request routes, and the dotted telemetry link.
    ctx.strokeStyle = `rgba(${LINE_RGB}, 0.4)`;
    ctx.lineWidth = 1;
    // Each link draws along its length as the scene enters, one after another.
    const link = (hop: ReturnType<typeof trimmed>, grown: number) => {
      ctx.moveTo(px(hop.x0), px(hop.y0));
      ctx.lineTo(px(hop.x0 + (hop.x1 - hop.x0) * grown), px(hop.y0 + (hop.y1 - hop.y0) * grown));
    };
    ctx.beginPath();
    [...hops.values()].forEach((hop, i) => link(hop, enter(0.08 + 0.05 * i, 0.35)));
    ctx.stroke();
    ctx.setLineDash([2, 4]);
    ctx.beginPath();
    link(telHop, enter(0.35, 0.35));
    if (idHop) link(idHop, enter(0.3, 0.35));
    ctx.stroke();
    ctx.setLineDash([]);

    // Telemetry: small, faint dots streaming from the agent to traces.
    ctx.fillStyle = LINE_COLOR;
    const telSpan = telLength / (SPEED * 0.8);
    for (let k = Math.floor((time - telSpan) / TELEMETRY_EVERY); k * TELEMETRY_EVERY <= time; k++) {
      const age = time - k * TELEMETRY_EVERY;
      if (age < 0 || age > telSpan) continue;
      const f = age / telSpan;
      const tx = telFrom.x + (telTo.x - telFrom.x) * f;
      const ty = telFrom.y + (telTo.y - telFrom.y) * f;
      ctx.globalAlpha = 0.6 * Math.sin(Math.PI * f) * outside(tx, ty) * flow;
      ctx.beginPath();
      ctx.arc(px(tx), px(ty), 1.6, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.globalAlpha = 1;

    // Identity: tokens (hollow rings) travelling from the issuer to the agent.
    if (idHop) {
      ctx.strokeStyle = LINE_COLOR;
      ctx.lineWidth = 1.2;
      const span = Math.hypot(idHop.x1 - idHop.x0, idHop.y1 - idHop.y0) / (SPEED * 0.6);
      for (let k = Math.floor((time - span) / IDENTITY_EVERY); k * IDENTITY_EVERY <= time; k++) {
        const age = time - k * IDENTITY_EVERY;
        if (age < 0 || age > span) continue;
        const f = age / span;
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
      }
      ctx.globalAlpha = 1;
    }

    // Requests. Each is decided by its index, so the scene needs no state.
    let blockFlash = 0;
    let passFlash = 0;
    const bursts: { x: number; y: number; f: number }[] = [];
    const stopAt = gateDistance - NODE_R;
    const maxLife = lines[0].length / SPEED + BURST_S;
    for (let i = Math.floor((time - maxLife) / SPAWN_EVERY); i * SPAWN_EVERY <= time; i++) {
      const age = time - i * SPAWN_EVERY;
      if (age < 0) continue;
      const line = lines[Math.floor(hash(i + 0.5) * lines.length)];
      const blocked = hash(i) < BLOCK_RATE;
      const travelled = age * SPEED;
      const sinceGate = (travelled - stopAt) / SPEED;
      if (sinceGate >= 0 && sinceGate < FLASH_S) {
        const strength = 1 - sinceGate / FLASH_S;
        if (blocked) blockFlash = Math.max(blockFlash, strength);
        else passFlash = Math.max(passFlash, strength);
      }
      if (blocked && travelled >= stopAt) {
        const f = sinceGate / BURST_S;
        if (f <= 1) bursts.push({ ...pointAt(line, stopAt), f });
        continue;
      }
      if (travelled > line.length) continue;
      const p = pointAt(line, travelled);
      ctx.globalAlpha = outside(p.x, p.y) * flow;
      ctx.fillStyle = LINE_COLOR;
      ctx.beginPath();
      ctx.arc(px(p.x), px(p.y), 3, 0, Math.PI * 2);
      ctx.fill();
      if (idHop && travelled > agentDistance) {
        // Signed with the agent's identity on the way to the guardrails.
        ctx.strokeStyle = LINE_COLOR;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.arc(px(p.x), px(p.y), 6, 0, Math.PI * 2);
        ctx.stroke();
      }
    }
    ctx.globalAlpha = 1;

    // Nodes: see-through discs with a faint tint; links stop at their edges. Icons sit on top.
    for (const [id, node] of Object.entries(scene.nodes)) {
      // Pops in: grows from 80% while fading up.
      const shown = enter(NODE_ENTRY(id), 0.32);
      if (shown <= 0) continue;
      const r = px(node.hero ? HERO_R : NODE_R) * (0.8 + 0.2 * shown);
      ctx.globalAlpha = shown;
      ctx.beginPath();
      ctx.arc(px(node.x), px(node.y), r, 0, Math.PI * 2);
      ctx.fillStyle = `rgba(${LINE_RGB}, 0.08)`;
      ctx.fill();
      ctx.strokeStyle = `rgba(${LINE_RGB}, 0.6)`;
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
    ctx.globalAlpha = 1;
    passFlash *= flow;
    blockFlash *= flow;

    // The guardrail reacts: a blue pulse for a request it lets through, an accent ring
    // for one it stops.
    const gx = px(gateway.x);
    const gy = px(gateway.y);
    const gr = px(NODE_R);
    if (passFlash > 0) {
      ctx.beginPath();
      ctx.arc(gx, gy, gr, 0, Math.PI * 2);
      ctx.fillStyle = `rgba(${LINE_RGB}, ${0.18 * passFlash})`;
      ctx.fill();
    }
    if (blockFlash > 0) {
      ctx.beginPath();
      ctx.arc(gx, gy, gr + 3 * (1 - blockFlash), 0, Math.PI * 2);
      ctx.strokeStyle = alpha(palette.accent, blockFlash);
      ctx.lineWidth = 1.6;
      ctx.stroke();
    }

    // Stopped requests: an accent burst where they met the guardrail, fading out.
    for (const b of bursts) {
      const x = px(b.x);
      const y = px(b.y);
      ctx.globalAlpha = (1 - b.f) * flow;
      ctx.fillStyle = palette.accent;
      ctx.beginPath();
      ctx.arc(x, y, 3, 0, Math.PI * 2);
      ctx.fill();
      ctx.strokeStyle = palette.accent;
      ctx.lineWidth = 1;
      ctx.globalAlpha = (1 - b.f) * 0.7 * flow;
      ctx.beginPath();
      ctx.arc(x, y, 3 + 10 * b.f, 0, Math.PI * 2);
      ctx.stroke();
    }
    ctx.globalAlpha = 1;
  };
}

const SCENE_VARIANTS = {
  internal: { plain: SCENES.internal, identity: withIdentity("internal") },
  external: { plain: SCENES.external, identity: withIdentity("external") },
};
const DRAW = {
  internal: {
    plain: drawScene(SCENE_VARIANTS.internal.plain),
    identity: drawScene(SCENE_VARIANTS.internal.identity),
  },
  external: {
    plain: drawScene(SCENE_VARIANTS.external.plain),
    identity: drawScene(SCENE_VARIANTS.external.identity),
  },
};

const at = (x: number, y: number) => ({
  position: "absolute",
  left: `${x * 100}%`,
  top: `${y * 100}%`,
  transform: "translate(-50%, -50%)",
  display: "flex",
  lineHeight: 0,
} as const);

/**
 * How an agent runs and how its traffic flows, drawn with icons only: requests reach
 * the agent, its LLM and tool calls pass the guardrails (some are stopped), and traces
 * stream to the platform. `internal` runs on the platform; `external` runs in the user's
 * own infrastructure. With `identity`, the platform issues the agent its AgentID and the
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
  const variant = identity ? "identity" : "plain";
  // The still frame shows a request in flight and one being stopped.
  const canvasRef = useAnimatedCanvas(DRAW[hosting][variant], { active, stillTime: 3.1 });
  const scene = SCENE_VARIANTS[hosting][variant];
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
      {scene.boundaries.map((b, i) => (
        <Box
          key={i}
          sx={{
            ...at(b.x0 + 0.045, b.y0 + 0.045),
            color: LINE_COLOR,
            // With the boundary it marks.
            opacity: active ? 0.8 : 0,
            transition: active ? "opacity 300ms ease-out" : "opacity 200ms ease-in",
            "@media (prefers-reduced-motion: reduce)": { transition: "none" },
          }}
        >
          {b.icon}
        </Box>
      ))}
      {Object.entries(scene.nodes).map(([id, node]) => (
        <Box
          key={id}
          sx={{
            ...at(node.x, node.y),
            color: node.hero ? "primary.main" : LINE_COLOR,
            // In step with the node's disc: in after it on entry, straight out on exit.
            opacity: active ? 1 : 0,
            scale: active ? "1" : "0.8",
            transition: active
              ? `opacity 320ms ease-out ${ENTRY_NODE_DELAY_MS(id) + 60}ms, ` +
                `scale 320ms cubic-bezier(0.2, 0.7, 0.2, 1) ${ENTRY_NODE_DELAY_MS(id) + 60}ms`
              : "opacity 200ms ease-in, scale 200ms ease-in",
            "@media (prefers-reduced-motion: reduce)": { transition: "none" },
          }}
        >
          {node.icon}
        </Box>
      ))}
    </Box>
  );
}
