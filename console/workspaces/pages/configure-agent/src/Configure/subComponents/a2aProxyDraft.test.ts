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

import { describe, expect, it } from "vitest";
import type { A2AProxyResponse } from "@agent-management-platform/types";
import {
  type A2AProxyDraft,
  draftChanged,
  draftFromProxy,
  dropsStoredSecret,
  proxyRequestOf,
  upstreamAuthError,
} from "./a2aProxyDraft";

const stored: A2AProxyResponse = { status: "published", lastError: "", upstreamAuthHeader: "X-Key" };
const fresh = (header: string, value: string): A2AProxyDraft => ({ enabled: true, header, value, secret: "replace" });

describe("a2aProxyDraft", () => {
  it("starts from the server state", () => {
    expect(draftFromProxy(undefined)).toEqual({ enabled: false, header: "", value: "", secret: "replace" });
    expect(draftFromProxy(stored)).toEqual({ enabled: true, header: "X-Key", value: "", secret: "keep" });
  });

  it("keeps, replaces or clears the stored secret", () => {
    expect(proxyRequestOf(draftFromProxy(stored), stored)).toEqual({ upstreamAuth: { header: "X-Key" } });
    expect(proxyRequestOf({ ...draftFromProxy(stored), secret: "replace", value: "v" }, stored))
      .toEqual({ upstreamAuth: { header: "X-Key", value: "v" } });
    expect(proxyRequestOf({ ...draftFromProxy(stored), secret: "clear" }, stored)).toEqual({});
    expect(proxyRequestOf(fresh("", ""))).toEqual({});
  });

  it("asks before a save drops a stored secret", () => {
    expect(dropsStoredSecret({ ...draftFromProxy(stored), secret: "clear" }, stored)).toBe(true);
    expect(dropsStoredSecret({ ...draftFromProxy(stored), enabled: false }, stored)).toBe(true);
    expect(dropsStoredSecret({ ...draftFromProxy(stored), header: " " }, stored)).toBe(true);
    expect(dropsStoredSecret(draftFromProxy(stored), stored)).toBe(false);
  });

  it("mirrors the server's validation", () => {
    for (const bad of [
      fresh("X Key", "v"), fresh("Host", "v"), fresh("X-Key", ""), fresh("X-Key", "it's"),
      fresh("X-Key", "a\nb"), fresh("X-Key", '{{ secret "x" }}'), fresh("X-Key", "é".repeat(4097)),
      fresh("X-Key", "abc%2Fdef"), fresh("X-Key", "a\u001bb"), fresh("X-Key", "a\tb"),
      fresh("X-Key", "a\u007fb"), fresh("X-Key", "a\u0085b"),
    ]) {
      expect(upstreamAuthError(bad), JSON.stringify(bad)).not.toBeNull();
    }
    expect(upstreamAuthError(fresh("X-Key", "a".repeat(8192)))).toBeNull();
    expect(upstreamAuthError(fresh("", ""))).toBeNull();
    expect(upstreamAuthError(draftFromProxy(stored), stored)).toBeNull();
  });

  it("names the character the gateway cannot carry", () => {
    expect(upstreamAuthError(fresh("X-Key", "abc%2F"))).toBe("The value cannot contain '%'");
    expect(upstreamAuthError(fresh("X-Key", "a\tb"))).toBe("The value cannot contain control characters");
  });

  it("refuses the headers the gateway owns, in any case", () => {
    for (const header of [
      "host", "Content-Length", "transfer-encoding", "CONNECTION", "Upgrade", "TE", "Keep-Alive", "Trailer",
      "Proxy-Authorization", "proxy-connection",
    ]) {
      expect(upstreamAuthError(fresh(header, "v")), header).toBe(`The ${header} header cannot be set`);
    }
    expect(upstreamAuthError(fresh("Authorization", "Bearer t"))).toBeNull();
    expect(upstreamAuthError(fresh("X-Proxy-Key", "v"))).toBeNull();
  });

  it("sees toggle-only and auth-only changes", () => {
    expect(draftChanged({ ...draftFromProxy(undefined), enabled: true })).toBe(true);
    expect(draftChanged(draftFromProxy(stored), stored)).toBe(false);
    expect(draftChanged({ ...draftFromProxy(stored), header: "X-Other" }, stored)).toBe(true);
    expect(draftChanged({ ...draftFromProxy(stored), secret: "replace", value: "v" }, stored)).toBe(true);
    expect(draftChanged({ ...draftFromProxy(stored), enabled: false }, stored)).toBe(true);
  });
});
