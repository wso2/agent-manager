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

import { Box } from "@wso2/oxygen-ui";
import {
  clamp01,
  drawGlow,
  entrance,
  LINE_COLOR,
  LINE_RGB,
  radialGradient,
  useAnimatedCanvas,
  type DrawFrame,
} from "./useAnimatedCanvas";

type Vec = [number, number, number];
type Basis = readonly [Vec, Vec];

const CIRCLES = 24;
const SEGMENTS = 72;
const DOTS = 36;
const TILT = 0.38; // radians the globe leans toward the viewer
const COS_TILT = Math.cos(TILT);
const SIN_TILT = Math.sin(TILT);
const TURN_MS = 60_000; // one full revolution of the globe
const COMET_LENGTH = 1.1; // radians of the highlighted trail
const COMET_SPEED = 0.35; // radians per second along its circle
const COMET_STEPS = 24;
const COMET_START = 3.8; // phase that starts the comet on the near side, in view
const COMET_FAR_ALPHA = 0.18; // behind the globe it dims like the far halves of the circles
const FRONT_STROKE = `rgba(${LINE_RGB}, 0.5)`;
const BACK_STROKE = `rgba(${LINE_RGB}, 0.16)`;
const BODY_INNER = `rgba(${LINE_RGB}, 0.02)`;
const BODY_OUTER = `rgba(${LINE_RGB}, 0.08)`;

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

const POINTS = SEGMENTS + 1;
/** Every circle's points in 3D, which never change: x, y, z per point. */
const CIRCLE_POINTS = new Float32Array(CIRCLES * POINTS * 3);
CIRCLE_BASES.forEach(([u, v], c) => {
  for (let i = 0; i < POINTS; i++) {
    const t = (i / SEGMENTS) * Math.PI * 2;
    const cos = Math.cos(t);
    const sin = Math.sin(t);
    const o = (c * POINTS + i) * 3;
    CIRCLE_POINTS[o] = u[0] * cos + v[0] * sin;
    CIRCLE_POINTS[o + 1] = u[1] * cos + v[1] * sin;
    CIRCLE_POINTS[o + 2] = u[2] * cos + v[2] * sin;
  }
});
/** The same points on screen, rewritten each frame instead of allocated. */
const SCREEN = new Float32Array(CIRCLE_POINTS.length);

/** Draws one frame. `time` is in seconds; the canvas is in CSS pixels. */
const draw: DrawFrame = (ctx, size, time, palette, sinceShown) => {
  ctx.clearRect(0, 0, size, size);
  // Each time the globe comes back, it grows into place from slightly smaller.
  const radius = size * 0.44 * (0.92 + 0.08 * entrance(sinceShown, 0, 0.6));
  const cx = size / 2;
  const cy = size / 2;
  const angle = ((time * 1000) / TURN_MS) * Math.PI * 2;
  const cosA = Math.cos(angle);
  const sinA = Math.sin(angle);
  // Spin about the vertical axis, lean toward the viewer, then scale to the screen.
  // Writes x, y and depth (-1..1) into `out` at `o`.
  const project = (x: number, y: number, z: number, out: Float32Array | number[], o: number) => {
    const z1 = -x * sinA + z * cosA;
    out[o] = cx + (x * cosA + z * sinA) * radius;
    out[o + 1] = cy + (y * COS_TILT - z1 * SIN_TILT) * radius;
    out[o + 2] = y * SIN_TILT + z1 * COS_TILT;
  };
  const scratch = [0, 0, 0];
  const onCircle = ([u, v]: Basis, t: number) => {
    const cos = Math.cos(t);
    const sin = Math.sin(t);
    project(
      u[0] * cos + v[0] * sin,
      u[1] * cos + v[1] * sin,
      u[2] * cos + v[2] * sin,
      scratch,
      0,
    );
    return scratch;
  };

  // Faint body, and the warm glow at the core.
  ctx.fillStyle = radialGradient(ctx, cx, cy, radius, BODY_INNER, BODY_OUTER);
  ctx.beginPath();
  ctx.arc(cx, cy, radius, 0, Math.PI * 2);
  ctx.fill();
  drawGlow(ctx, cx, cy, radius * 0.34, palette.accent, 1);

  // Circles: every point projected once, then the far half faint and the near half
  // brighter, each batched into one path.
  for (let o = 0; o < CIRCLE_POINTS.length; o += 3) {
    project(CIRCLE_POINTS[o], CIRCLE_POINTS[o + 1], CIRCLE_POINTS[o + 2], SCREEN, o);
  }
  ctx.lineCap = "round";
  for (const front of [false, true]) {
    ctx.beginPath();
    for (let c = 0; c < CIRCLES; c++) {
      let drawing = false;
      for (let i = 1; i < POINTS; i++) {
        const o = (c * POINTS + i) * 3;
        if ((SCREEN[o - 1] >= 0) === front && (SCREEN[o + 2] >= 0) === front) {
          if (!drawing) ctx.moveTo(SCREEN[o - 3], SCREEN[o - 2]);
          ctx.lineTo(SCREEN[o], SCREEN[o + 1]);
          drawing = true;
        } else {
          drawing = false;
        }
      }
    }
    ctx.strokeStyle = front ? FRONT_STROKE : BACK_STROKE;
    ctx.lineWidth = front ? 1 : 0.8;
    ctx.stroke();
  }

  // Dots on their orbits; size and brightness follow depth, so they fade around the rim.
  ctx.fillStyle = LINE_COLOR;
  for (const dot of DOT_ORBITS) {
    const [x, y, z] = onCircle(dot.circle, dot.phase + dot.speed * time);
    const near = (z + 1) / 2;
    ctx.globalAlpha = 0.15 + 0.85 * near * near;
    ctx.beginPath();
    ctx.arc(x, y, 1.2 + 2 * near, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.globalAlpha = 1;

  // The comet: a short accent trail moving along one circle, fading toward its tail.
  // Behind the globe it dims rather than vanishing, blended across the rim.
  const depthAlpha = (z: number) =>
    COMET_FAR_ALPHA + (1 - COMET_FAR_ALPHA) * clamp01((z + 0.2) / 0.4);
  ctx.strokeStyle = palette.accent;
  ctx.lineWidth = 1.6;
  const tail = COMET_START + time * COMET_SPEED - COMET_LENGTH;
  let [px, py] = onCircle(COMET_CIRCLE, tail);
  let pz = 0;
  for (let i = 1; i <= COMET_STEPS; i++) {
    const [x, y, z] = onCircle(COMET_CIRCLE, tail + (COMET_LENGTH * i) / COMET_STEPS);
    ctx.globalAlpha = 0.75 * (i / COMET_STEPS) * depthAlpha(z);
    ctx.beginPath();
    ctx.moveTo(px, py);
    ctx.lineTo(x, y);
    ctx.stroke();
    [px, py, pz] = [x, y, z];
  }
  ctx.globalAlpha = depthAlpha(pz);
  ctx.fillStyle = palette.accent;
  ctx.beginPath();
  ctx.arc(px, py, 2.4, 0, Math.PI * 2);
  ctx.fill();
  ctx.globalAlpha = 1;
};

/**
 * A slowly turning globe of great circles with dots travelling along them and one
 * accent comet: agents connected across models and tools. Drawn on a canvas;
 * decorative, paused while off screen or inactive, and still for users who prefer
 * reduced motion.
 */
export function AgentSphere({ size = 320, active = true }: { size?: number; active?: boolean }) {
  const canvasRef = useAnimatedCanvas(draw, { active });
  return (
    <Box
      component="canvas"
      ref={canvasRef}
      aria-hidden
      sx={{ width: size, maxWidth: "100%", aspectRatio: "1 / 1", display: "block" }}
    />
  );
}
