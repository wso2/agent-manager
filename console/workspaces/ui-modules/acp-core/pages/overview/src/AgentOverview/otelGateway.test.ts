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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it } from "vitest";
import type { GatewayResponse } from "@agent-management-platform/types";
import { otelGatewayVhost } from "./otelGateway";

const gateway = (
  vhost: string,
  gatewayType: GatewayResponse["gatewayType"],
  hosting?: GatewayResponse["hosting"],
): GatewayResponse => ({ vhost, gatewayType, hosting }) as GatewayResponse;

describe("otelGatewayVhost", () => {
  it("never picks a self-hosted gateway", () => {
    expect(
      otelGatewayVhost([gateway("https://customer.example.com", "BOTH", "SELF_HOSTED")]),
    ).toBeUndefined();
    expect(
      otelGatewayVhost([
        gateway("https://customer.example.com", "BOTH", "SELF_HOSTED"),
        gateway("https://managed.example.com", "BOTH", "MANAGED"),
      ]),
    ).toBe("https://managed.example.com");
  });

  it("prefers an ingress-capable managed gateway", () => {
    expect(
      otelGatewayVhost([
        gateway("https://egress.example.com", "EGRESS"),
        gateway("https://ingress.example.com", "INGRESS"),
      ]),
    ).toBe("https://ingress.example.com");
  });

  it("falls back to another managed gateway", () => {
    expect(otelGatewayVhost([gateway("https://egress.example.com", "EGRESS")])).toBe(
      "https://egress.example.com",
    );
  });

  it("has nothing for no gateways", () => {
    expect(otelGatewayVhost(undefined)).toBeUndefined();
    expect(otelGatewayVhost([])).toBeUndefined();
  });
});
