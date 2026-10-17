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
import { isNotFoundError, nullIfNotFound, pendingRefetchInterval, POLL_INTERVAL } from "./utils";

describe("pendingRefetchInterval", () => {
  it("polls while pending", () => {
    expect(pendingRefetchInterval({ status: "pending" })).toBe(POLL_INTERVAL);
  });

  it("stops on any other status, or with no data", () => {
    expect(pendingRefetchInterval({ status: "fetched" })).toBe(false);
    expect(pendingRefetchInterval({ status: "published" })).toBe(false);
    expect(pendingRefetchInterval({ status: "failed" })).toBe(false);
    expect(pendingRefetchInterval(undefined)).toBe(false);
    expect(pendingRefetchInterval(null)).toBe(false);
  });
});

describe("nullIfNotFound", () => {
  it("resolves a 404 to null and rethrows anything else", async () => {
    const withStatus = (status: number) => Object.assign(new Error("x"), { status });
    await expect(nullIfNotFound(Promise.reject(withStatus(404)))).resolves.toBeNull();
    await expect(nullIfNotFound(Promise.reject(withStatus(503))))
      .rejects.toMatchObject({ status: 503 });
    await expect(nullIfNotFound(Promise.resolve("ok"))).resolves.toBe("ok");
    expect(isNotFoundError(null)).toBe(false);
  });
});
