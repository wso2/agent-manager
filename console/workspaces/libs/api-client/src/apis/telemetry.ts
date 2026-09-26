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

import { globalConfig } from "@agent-management-platform/types";
import { httpPOST, SERVICE_BASE } from "../utils";

/** Dimension values must stay scalar — the service drops anything else. */
export type ConsoleActionDimensionValue = string | number | boolean;

/** One console interaction, as reported to the service. */
export interface ConsoleActionPayload {
  /**
   * Taxonomy name, e.g. "amp.console.navigation.page-view". Validated against
   * a server-side allowlist; an unrecognized name is accepted (202) and
   * dropped, so a console ahead of its service degrades quietly.
   */
  action: string;
  /** Browser clock, ISO 8601. Preserves ordering within a buffered flush. */
  occurredAt?: string;
  /** Route the action happened on. Path only — never include a query string. */
  page?: string;
  /** Per-tab session grouping id. Not a credential. */
  sessionId?: string;
  dimensions?: Record<string, ConsoleActionDimensionValue>;
}

export interface ConsoleActionBatchResponse {
  accepted: number;
  dropped: number;
}

const CONSOLE_ACTIONS_PATH = `${SERVICE_BASE}/telemetry/console-actions`;

/**
 * Report a batch of console actions.
 *
 * Telemetry is best-effort: the endpoint answers 202 for any well-formed
 * batch, and the caller drops the buffer either way. Never surface a failure
 * here to the user and never retry — a retry loop over a failing collector
 * would turn one bad deploy into sustained request volume from every open tab.
 */
export async function reportConsoleActions(
  actions: ConsoleActionPayload[],
  getToken?: () => Promise<string>,
): Promise<ConsoleActionBatchResponse | undefined> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(CONSOLE_ACTIONS_PATH, { actions }, { token });
  return res.json();
}

/**
 * Report a batch during page unload.
 *
 * A normal request does not survive teardown: the browser cancels it as the
 * document goes away, losing the session-end action and everything buffered
 * since the last flush — exactly the records that describe how a session
 * ended. `keepalive` lets the request outlive the document.
 *
 * `sendBeacon` is the other option and is deliberately not used: a beacon
 * cannot set headers, so the caller's token would have to travel in the URL,
 * and credentials do not belong in query strings (they land in access logs and
 * browser history). A keepalive fetch survives unload *and* carries the
 * Authorization header.
 *
 * Hand-rolled rather than routed through `httpPOST` because that helper awaits
 * and parses the response and sleeps before resolving; during unload there is
 * nothing to await with.
 *
 * Returns true when the flush was handed off to the browser.
 */
export function reportConsoleActionsOnUnload(
  actions: ConsoleActionPayload[],
  token: string | undefined,
): boolean {
  if (!token || actions.length === 0) {
    return false;
  }

  try {
    void window.fetch(`${globalConfig.apiBaseUrl}${CONSOLE_ACTIONS_PATH}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ actions }),
      keepalive: true,
    });
    return true;
  } catch {
    // Unload is not a place to handle errors; the actions are simply lost.
    return false;
  }
}
