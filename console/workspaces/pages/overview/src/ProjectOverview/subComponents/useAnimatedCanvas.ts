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
import { useTheme } from "@wso2/oxygen-ui";
import { prefersReducedMotion } from "../../OrganizationOverview/motion";

/** The line and dot blue shared by the landing illustrations; the accent comes from the theme. */
export const LINE_RGB = "84, 104, 240";
export const LINE_COLOR = `rgb(${LINE_RGB})`;

/** Theme colours a canvas scene draws with. */
export interface ScenePalette {
  accent: string;
  text: string;
  surface: string;
  fontFamily: string;
}

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

/** 0 before `start`, easing out to 1 over `duration` seconds. */
export function entrance(sinceShown: number, start: number, duration: number) {
  const t = Math.max(0, Math.min(1, (sinceShown - start) / duration));
  return 1 - (1 - t) ** 3;
}

interface Options {
  /** Animates only while true; otherwise the last frame stays. */
  active?: boolean;
  /** The single frame drawn for users who prefer reduced motion. */
  stillTime?: number;
  /** Keeps animating this long after `active` turns false, so a fade-out stays alive. */
  lingerMs?: number;
}

/**
 * Runs `draw` on a canvas every animation frame: sized to the element and the
 * screen's pixel density, paused while off screen or inactive, and still for users
 * who prefer reduced motion. Returns the ref for the canvas.
 */
export function useAnimatedCanvas(
  draw: DrawFrame,
  { active = true, stillTime = 0, lingerMs = 400 }: Options = {},
) {
  const theme = useTheme();
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const palette: ScenePalette = {
    accent: theme.palette.primary.main,
    text: theme.palette.text.secondary,
    surface: theme.palette.background.paper,
    fontFamily: String(theme.typography.fontFamily ?? "sans-serif"),
  };
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

    const reduced = prefersReducedMotion();
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
          }, lingerMs);
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
    };
  }, [stillTime, lingerMs]);

  useEffect(() => {
    control.current?.setActive(active);
  }, [active]);

  return canvasRef;
}
