/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it, vi } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { agentCardRefetchInterval, removeAgentCardFromCache } from "./agent-card";
import { POLL_INTERVAL } from "../utils";

// The real package drags in oxygen-ui, which cannot load under node.
vi.mock("@agent-management-platform/views", () => ({ useSnackBar: () => ({}) }));

describe("agentCardRefetchInterval", () => {
  it("polls while the card is pending", () => {
    expect(agentCardRefetchInterval({ status: "pending", source: "platform", sourceUrl: "", lastError: "" }))
      .toBe(POLL_INTERVAL);
  });

  it("stops once fetched or failed, or with no data", () => {
    expect(agentCardRefetchInterval({ status: "fetched", source: "platform", sourceUrl: "", lastError: "" })).toBe(false);
    expect(agentCardRefetchInterval({ status: "failed", source: "platform", sourceUrl: "", lastError: "x" })).toBe(false);
    expect(agentCardRefetchInterval(undefined)).toBe(false);
  });

  it("stops polling a stale pending card once the row is gone", () => {
    const stale = { status: "pending", source: "external", sourceUrl: "u", lastError: "" } as const;
    expect(agentCardRefetchInterval(stale, Object.assign(new Error("nf"), { status: 404 }))).toBe(false);
  });

  it("keeps polling a pending card through a transient error", () => {
    const pending = { status: "pending", source: "external", sourceUrl: "u", lastError: "" } as const;
    expect(agentCardRefetchInterval(pending, Object.assign(new Error("boom"), { status: 503 }))).toBe(POLL_INTERVAL);
  });
});

describe("removeAgentCardFromCache", () => {
  it("drops the cached card for that agent and environment only", () => {
    const qc = new QueryClient();
    const params = { orgName: "o", projName: "p", agentName: "a", envId: "dev" };
    qc.setQueryData(["agent-card", "o", "p", "a", "dev"], { status: "fetched" });
    qc.setQueryData(["agent-card", "o", "p", "a", "prod"], { status: "fetched" });
    removeAgentCardFromCache(qc, params);
    expect(qc.getQueryData(["agent-card", "o", "p", "a", "dev"])).toBeUndefined();
    expect(qc.getQueryData(["agent-card", "o", "p", "a", "prod"])).toBeDefined();
  });
});
