/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it } from "vitest";
import {
  isSelfHostedGateway,
  showGatewayHosting,
} from "@agent-management-platform/types";

const managed = { hosting: "MANAGED" as const };
const selfHosted = { hosting: "SELF_HOSTED" as const };

describe("isSelfHostedGateway", () => {
  it.each([
    ["SELF_HOSTED", selfHosted, true],
    ["MANAGED", managed, false],
    ["no hosting", {}, false],
  ])("%s", (_, gateway, expected) => {
    expect(isSelfHostedGateway(gateway)).toBe(expected);
  });
});

describe("showGatewayHosting", () => {
  it("hides hosting with the flag off and only managed gateways", () => {
    expect(showGatewayHosting(false, [managed])).toBe(false);
  });

  it("shows hosting with the flag off once a self-hosted gateway exists", () => {
    expect(showGatewayHosting(false, [managed, selfHosted])).toBe(true);
  });

  it("shows hosting with the flag on and no gateways", () => {
    expect(showGatewayHosting(true, [])).toBe(true);
  });
});
