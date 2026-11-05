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

import type { ReactElement } from "react";
import type { InputInterfaceType } from "@agent-management-platform/types";

export interface AgentTypeIconProps {
  type: InputInterfaceType;
  size?: number;
}

const STROKE = {
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.75,
  strokeLinecap: "round",
  strokeLinejoin: "round",
} as const;

/** A speech bubble with three dots: an agent you talk to. */
function ChatGlyph() {
  return (
    <>
      <path {...STROKE} d="M21 12a8 8 0 0 1-11.6 7.1L4 20l1.1-4.7A8 8 0 1 1 21 12z" />
      <circle cx="8.5" cy="12" r="1" fill="currentColor" />
      <circle cx="12" cy="12" r="1" fill="currentColor" />
      <circle cx="15.5" cy="12" r="1" fill="currentColor" />
    </>
  );
}

/** A rounded badge labelled API: an agent behind your own HTTP API. */
function CustomApiGlyph() {
  return (
    <>
      <rect {...STROKE} x="2.75" y="6" width="18.5" height="12" rx="3" />
      <text
        x="12"
        y="14.6"
        textAnchor="middle"
        fontSize="7.5"
        fontWeight="700"
        fontFamily="inherit"
        fill="currentColor"
      >
        API
      </text>
    </>
  );
}

/** Two agents trading messages: arrows in both directions between two nodes. */
function A2AGlyph() {
  return (
    <>
      <circle {...STROKE} cx="4.75" cy="12" r="2.25" />
      <circle {...STROKE} cx="19.25" cy="12" r="2.25" />
      <path {...STROKE} d="M8.75 9.5h6.5m-2-2 2 2-2 2" />
      <path {...STROKE} d="M15.25 14.5h-6.5m2-2-2 2 2 2" />
    </>
  );
}

const GLYPHS: Record<InputInterfaceType, () => ReactElement> = {
  DEFAULT: ChatGlyph,
  CUSTOM: CustomApiGlyph,
  A2A: A2AGlyph,
};

/**
 * The icon for an agent type, composed from simple shapes so each type reads at a glance.
 * Draws in currentColor, so wrap it in an IconTile or set `color` to theme it.
 */
export function AgentTypeIcon({ type, size = 18 }: AgentTypeIconProps) {
  const Glyph = GLYPHS[type];
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <Glyph />
    </svg>
  );
}
