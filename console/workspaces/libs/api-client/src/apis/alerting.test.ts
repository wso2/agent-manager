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

import { afterEach, describe, expect, it, vi } from "vitest";
import { getAlertEndpoint } from "./alerting";

function mockFetch(status: number, body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    ),
  );
}

describe("getAlertEndpoint", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("resolves to null when the org has no endpoint (404)", async () => {
    // After a delete the refetch returns 404; treating it as an error would
    // leave the stale endpoint on screen.
    mockFetch(404, { message: "No alert endpoint is configured" });
    await expect(getAlertEndpoint({ orgName: "acme" })).resolves.toBeNull();
  });

  it("returns the endpoint when one exists", async () => {
    mockFetch(200, { url: "https://a.example.com", enabled: true, headerNames: [] });
    await expect(getAlertEndpoint({ orgName: "acme" })).resolves.toMatchObject({
      url: "https://a.example.com",
    });
  });

  it("still fails on other errors", async () => {
    mockFetch(500, { message: "boom" });
    await expect(getAlertEndpoint({ orgName: "acme" })).rejects.toMatchObject({ status: 500 });
  });
});
