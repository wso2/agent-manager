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

import { useEffect, useRef } from "react";
import { alpha, useTheme } from "@wso2/oxygen-ui";

/** The line and dot blue shared by the landing illustrations; the accent comes from the theme. */
export const LINE_RGB = "84, 104, 240";
export const LINE_COLOR = `rgb(${LINE_RGB})`;

/** Theme colours a canvas scene draws with. */
export interface ScenePalette {
  accent: string;
}

/** How long a canvas keeps animating after it turns inactive, so its fade-out stays alive. */
const LINGER_MS = 400;

/**
 * Draws one frame on a square canvas of `size` CSS pixels. `time` is seconds since
 * mount; `sinceShown` is seconds since the canvas last became active, for entrance
 * animations (Infinity when there is nothing to animate in).
 */
export type DrawFrame = (
  ctx: CanvasRenderingContext2D,
  size: number,
  time: number,
  palette: ScenePalette,
  sinceShown: number,
) => void;

export const clamp01 = (v: number) => Math.max(0, Math.min(1, v));

/** 0 before `start`, easing out to 1 over `duration` seconds. */
export function entrance(sinceShown: number, start: number, duration: number) {
  return 1 - (1 - clamp01((sinceShown - start) / duration)) ** 3;
}

// Gradients only change with the canvas size or the theme, so each is built once per
// key and reused every frame.
const gradients = new WeakMap<CanvasRenderingContext2D, Map<string, CanvasGradient>>();
function radial(ctx: CanvasRenderingContext2D, key: string, make: () => CanvasGradient) {
  let byKey = gradients.get(ctx);
  if (!byKey) gradients.set(ctx, (byKey = new Map()));
  let gradient = byKey.get(key);
  if (!gradient) {
    if (byKey.size > 16) byKey.clear(); // old sizes and themes
    byKey.set(key, (gradient = make()));
  }
  return gradient;
}

/** A gradient from `inner` at the centre to `outer` at radius `r`, cached. */
export function radialGradient(
  ctx: CanvasRenderingContext2D,
  cx: number,
  cy: number,
  r: number,
  inner: string,
  outer: string,
) {
  return radial(ctx, `${cx},${cy},${r},${inner},${outer}`, () => {
    const gradient = ctx.createRadialGradient(cx, cy, 0, cx, cy, r);
    gradient.addColorStop(0, inner);
    gradient.addColorStop(1, outer);
    return gradient;
  });
}

/** The warm accent glow both illustrations have at their centre. */
export function drawGlow(
  ctx: CanvasRenderingContext2D,
  cx: number,
  cy: number,
  r: number,
  accent: string,
  strength: number,
) {
  ctx.globalAlpha = 0.3 * strength;
  ctx.fillStyle = radialGradient(ctx, cx, cy, r, accent, alpha(accent, 0));
  ctx.fillRect(cx - r, cy - r, r * 2, r * 2);
  ctx.globalAlpha = 1;
}

interface Options {
  /** Animates only while true; otherwise the last frame stays. */
  active?: boolean;
  /** The single frame drawn for users who prefer reduced motion. */
  stillTime?: number;
}

/**
 * Runs `draw` on a canvas every animation frame: sized to the element and the
 * screen's pixel density, paused while off screen or inactive, and still for users
 * who prefer reduced motion. Returns the ref for the canvas.
 */
export function useAnimatedCanvas(
  draw: DrawFrame,
  { active = true, stillTime = 0 }: Options = {},
) {
  const theme = useTheme();
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const palette: ScenePalette = { accent: theme.palette.primary.main };
  // Read by the running loop, so a theme or draw change needs no restart.
  const latest = useRef({ draw, palette });
  useEffect(() => {
    latest.current = { draw, palette };
  });

  // Starting and stopping the loop goes through this, so `active` can change freely.
  const control = useRef<{ setActive: (on: boolean) => void } | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    const ctx = canvas?.getContext("2d");
    if (!canvas || !ctx) return undefined;

    // Followed live: turning reduced motion on mid-animation stops the loop on a still frame.
    const motionQuery =
      typeof window !== "undefined" ? window.matchMedia?.("(prefers-reduced-motion: reduce)") : undefined;
    let reduced = motionQuery?.matches ?? false;
    const start = performance.now();
    let cssSize = 0;
    let raf = 0;
    let visible = true;
    let running = false;
    let shownAt: number | null = null;
    let stopTimer: ReturnType<typeof setTimeout> | undefined;

    const render = (now: number) => {
      if (cssSize <= 0) return;
      const time = reduced ? stillTime : (now - start) / 1000;
      const sinceShown = reduced || shownAt === null ? Infinity : (now - shownAt) / 1000;
      latest.current.draw(ctx, cssSize, time, latest.current.palette, sinceShown);
    };
    const loop = (now: number) => {
      render(now);
      raf = running && visible && !reduced ? requestAnimationFrame(loop) : 0;
    };
    const kick = () => {
      if (!raf && running && visible && !reduced) raf = requestAnimationFrame(loop);
    };

    const resize = () => {
      cssSize = canvas.clientWidth;
      const dpr = window.devicePixelRatio || 1;
      canvas.width = Math.round(cssSize * dpr);
      canvas.height = Math.round(cssSize * dpr);
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      render(performance.now());
    };
    const resizeObserver = new ResizeObserver(resize);
    resizeObserver.observe(canvas);
    const visibility = new IntersectionObserver(([entry]) => {
      visible = entry.isIntersecting;
      kick();
    });
    visibility.observe(canvas);

    const onMotionChange = (event: MediaQueryListEvent) => {
      reduced = event.matches;
      if (reduced) {
        cancelAnimationFrame(raf);
        raf = 0;
      }
      render(performance.now());
      kick();
    };
    motionQuery?.addEventListener?.("change", onMotionChange);

    control.current = {
      setActive: (on) => {
        clearTimeout(stopTimer);
        if (on) {
          // Replay the entrance unless it is still on screen from a moment ago.
          if (!running) shownAt = performance.now();
          running = true;
          kick();
        } else {
          stopTimer = setTimeout(() => {
            running = false;
          }, LINGER_MS);
        }
      },
    };
    resize();
    return () => {
      control.current = null;
      clearTimeout(stopTimer);
      cancelAnimationFrame(raf);
      resizeObserver.disconnect();
      visibility.disconnect();
      motionQuery?.removeEventListener?.("change", onMotionChange);
    };
  }, [stillTime]);

  useEffect(() => {
    control.current?.setActive(active);
  }, [active]);

  return canvasRef;
}
