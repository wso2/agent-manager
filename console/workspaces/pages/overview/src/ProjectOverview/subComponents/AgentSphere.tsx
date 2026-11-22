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

import { alpha, Box } from "@wso2/oxygen-ui";
import {
  entrance,
  LINE_RGB,
  useAnimatedCanvas,
  type DrawFrame,
  type ScenePalette,
} from "./useAnimatedCanvas";

type Vec = [number, number, number];
type Basis = readonly [Vec, Vec];

const CIRCLES = 24;
const SEGMENTS = 72;
const DOTS = 36;
const TILT = 0.38; // radians the globe leans toward the viewer
const TURN_MS = 60_000; // one full revolution of the globe
const COMET_LENGTH = 1.1; // radians of the highlighted trail
const COMET_SPEED = 0.35; // radians per second along its circle
const COMET_STEPS = 24;
const COMET_START = 3.8; // phase that starts the comet on the near side, in view
const COMET_FAR_ALPHA = 0.18; // behind the globe it dims like the far halves of the circles

/** Deterministic, so the globe looks the same on every render. */
function seeded(seed: number) {
  let s = seed;
  return () => {
    s = (s * 1664525 + 1013904223) % 4294967296;
    return s / 4294967296;
  };
}

function randomUnit(rand: () => number): Vec {
  const z = rand() * 2 - 1;
  const t = rand() * Math.PI * 2;
  const r = Math.sqrt(1 - z * z);
  return [r * Math.cos(t), r * Math.sin(t), z];
}

const cross = (a: Vec, b: Vec): Vec => [
  a[1] * b[2] - a[2] * b[1],
  a[2] * b[0] - a[0] * b[2],
  a[0] * b[1] - a[1] * b[0],
];
const normalize = (a: Vec): Vec => {
  const l = Math.hypot(a[0], a[1], a[2]);
  return [a[0] / l, a[1] / l, a[2] / l];
};

const rand = seeded(7);
/** Each great circle as an orthonormal pair (u, v): p(t) = u cos t + v sin t. */
const CIRCLE_BASES: Basis[] = Array.from({ length: CIRCLES }, () => {
  const n = randomUnit(rand);
  const helper: Vec = Math.abs(n[0]) < 0.9 ? [1, 0, 0] : [0, 1, 0];
  const u = normalize(cross(n, helper));
  return [u, cross(n, u)] as const;
});
/** Dots travel along the circles, each at its own phase, pace and direction. */
const DOT_ORBITS = Array.from({ length: DOTS }, (_, i) => ({
  circle: CIRCLE_BASES[(i * 7) % CIRCLES],
  phase: rand() * Math.PI * 2,
  speed: (0.08 + rand() * 0.14) * (rand() < 0.5 ? -1 : 1), // radians per second
}));
const COMET_CIRCLE = CIRCLE_BASES[0];

const pointOn = ([u, v]: Basis, t: number): Vec => {
  const c = Math.cos(t);
  const s = Math.sin(t);
  return [u[0] * c + v[0] * s, u[1] * c + v[1] * s, u[2] * c + v[2] * s];
};

/** Spin about the vertical axis, then lean toward the viewer. Returns x, y and depth (-1..1). */
function project([x, y, z]: Vec, angle: number): Vec {
  const cos = Math.cos(angle);
  const sin = Math.sin(angle);
  const x1 = x * cos + z * sin;
  const z1 = -x * sin + z * cos;
  return [
    x1,
    y * Math.cos(TILT) - z1 * Math.sin(TILT),
    y * Math.sin(TILT) + z1 * Math.cos(TILT),
  ];
}

/** Draws one frame. `time` is in seconds; the canvas is in CSS pixels after scaling. */
function draw(ctx: CanvasRenderingContext2D, size: number, time: number, palette: ScenePalette) {
  const radius = size * 0.44;
  const cx = size / 2;
  const cy = size / 2;
  const angle = ((time * 1000) / TURN_MS) * Math.PI * 2;
  const toScreen = (p: Vec): Vec => {
    const [x, y, z] = project(p, angle);
    return [cx + x * radius, cy + y * radius, z];
  };

  ctx.clearRect(0, 0, size, size);

  // Faint body, and the warm glow at the core.
  const body = ctx.createRadialGradient(cx, cy, 0, cx, cy, radius);
  body.addColorStop(0, `rgba(${LINE_RGB}, 0.02)`);
  body.addColorStop(1, `rgba(${LINE_RGB}, 0.08)`);
  ctx.fillStyle = body;
  ctx.beginPath();
  ctx.arc(cx, cy, radius, 0, Math.PI * 2);
  ctx.fill();

  const glow = ctx.createRadialGradient(cx, cy, 0, cx, cy, radius * 0.34);
  glow.addColorStop(0, palette.accent);
  glow.addColorStop(1, alpha(palette.accent, 0));
  ctx.globalAlpha = 0.3;
  ctx.fillStyle = glow;
  ctx.fillRect(cx - radius, cy - radius, radius * 2, radius * 2);
  ctx.globalAlpha = 1;

  // Circles: the far half faint, the near half brighter, each batched into one path.
  ctx.lineCap = "round";
  for (const front of [false, true]) {
    ctx.beginPath();
    for (const basis of CIRCLE_BASES) {
      let prev: Vec | null = null;
      for (let i = 0; i <= SEGMENTS; i++) {
        const p = toScreen(pointOn(basis, (i / SEGMENTS) * Math.PI * 2));
        if (prev && (prev[2] >= 0) === front && (p[2] >= 0) === front) {
          ctx.moveTo(prev[0], prev[1]);
          ctx.lineTo(p[0], p[1]);
        }
        prev = p;
      }
    }
    ctx.strokeStyle = `rgba(${LINE_RGB}, ${front ? 0.5 : 0.16})`;
    ctx.lineWidth = front ? 1 : 0.8;
    ctx.stroke();
  }

  // Dots on their orbits; size and brightness follow depth, so they fade around the rim.
  ctx.fillStyle = `rgb(${LINE_RGB})`;
  for (const dot of DOT_ORBITS) {
    const [x, y, z] = toScreen(pointOn(dot.circle, dot.phase + dot.speed * time));
    const near = (z + 1) / 2;
    ctx.globalAlpha = 0.15 + 0.85 * near * near;
    ctx.beginPath();
    ctx.arc(x, y, 1.2 + 2 * near, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.globalAlpha = 1;

  // The comet: a short accent trail moving along one circle, fading toward its tail.
  ctx.strokeStyle = palette.accent;
  ctx.lineWidth = 1.6;
  const head = COMET_START + time * COMET_SPEED;
  // 0 behind the globe, 1 in front, blended across the rim.
  const nearness = (z: number) => Math.max(0, Math.min(1, (z + 0.2) / 0.4));
  const depthAlpha = (z: number) => COMET_FAR_ALPHA + (1 - COMET_FAR_ALPHA) * nearness(z);
  let prev = toScreen(pointOn(COMET_CIRCLE, head - COMET_LENGTH));
  for (let i = 1; i <= COMET_STEPS; i++) {
    const t = head - COMET_LENGTH + (COMET_LENGTH * i) / COMET_STEPS;
    const p = toScreen(pointOn(COMET_CIRCLE, t));
    ctx.globalAlpha = 0.75 * (i / COMET_STEPS) * depthAlpha(p[2]);
    ctx.beginPath();
    ctx.moveTo(prev[0], prev[1]);
    ctx.lineTo(p[0], p[1]);
    ctx.stroke();
    prev = p;
  }
  ctx.globalAlpha = depthAlpha(prev[2]);
  ctx.fillStyle = palette.accent;
  ctx.beginPath();
  ctx.arc(prev[0], prev[1], 2.4, 0, Math.PI * 2);
  ctx.fill();
  ctx.globalAlpha = 1;
}

/** Each time the globe comes back, it grows into place from slightly smaller. */
const drawWithEntrance: DrawFrame = (ctx, size, time, palette, sinceShown) => {
  const scale = 0.92 + 0.08 * entrance(sinceShown, 0, 0.6);
  ctx.clearRect(0, 0, size, size);
  ctx.save();
  ctx.translate(size / 2, size / 2);
  ctx.scale(scale, scale);
  ctx.translate(-size / 2, -size / 2);
  draw(ctx, size, time, palette);
  ctx.restore();
};

/**
 * A slowly turning globe of great circles with dots travelling along them and one
 * accent comet: agents connected across models and tools. Drawn on a canvas;
 * decorative, paused while off screen or inactive, and still for users who prefer
 * reduced motion.
 */
export function AgentSphere({ size = 320, active = true }: { size?: number; active?: boolean }) {
  const canvasRef = useAnimatedCanvas(drawWithEntrance, { active });
  return (
    <Box
      component="canvas"
      ref={canvasRef}
      aria-hidden
      sx={{ width: size, maxWidth: "100%", aspectRatio: "1 / 1", display: "block" }}
    />
  );
}
