/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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
import { EMPTY_A2A_CARD, parseA2ACard } from "./a2aCard";

const full = {
  name: "Orders",
  description: "d",
  version: "1.0",
  provider: { organization: "Acme", url: "https://acme.example" },
  supportedInterfaces: [
    { url: "https://x/rpc", protocolBinding: "JSONRPC" },
    { protocolBinding: "HTTP+JSON" },
    { url: "https://x/rest", protocolBinding: 7 },
  ],
  skills: [
    {
      id: "s1",
      name: "order-status",
      description: "sd",
      tags: ["orders"],
      examples: ["Where is order 7?"],
      inputModes: ["text/plain"],
      outputModes: ["application/json"],
    },
    "junk",
  ],
  capabilities: {
    streaming: true,
    pushNotifications: "true",
    extendedAgentCard: 1,
  },
  securitySchemes: { bearer: {}, apiKey: {} },
};

describe("parseA2ACard", () => {
  it("reads every field of a full card", () =>
    expect(parseA2ACard(full)).toEqual({
      name: "Orders",
      description: "d",
      version: "1.0",
      providerOrganization: "Acme",
      providerUrl: "https://acme.example",
      interfaces: [
        { url: "https://x/rpc", protocolBinding: "JSONRPC" },
        { url: "https://x/rest", protocolBinding: "" },
      ],
      skills: [
        {
          id: "s1",
          name: "order-status",
          description: "sd",
          tags: ["orders"],
          examples: ["Where is order 7?"],
          inputModes: ["text/plain"],
          outputModes: ["application/json"],
        },
      ],
      capabilities: {
        streaming: true,
        pushNotifications: false,
        extendedAgentCard: false,
      },
      securitySchemes: ["bearer", "apiKey"],
    }));

  it.each([undefined, null, "card", [1]])("reads %j as the empty card", (v) =>
    expect(parseA2ACard(v)).toEqual(EMPTY_A2A_CARD),
  );

  it("is defensive", () => {
    expect(parseA2ACard({ skills: "x", capabilities: null })).toMatchObject({
      skills: [],
      capabilities: {
        streaming: false,
        pushNotifications: false,
        extendedAgentCard: false,
      },
    });
    expect(
      parseA2ACard({
        provider: { organization: "Acme", url: "javascript:alert(1)" },
      }),
    ).toMatchObject({ providerOrganization: "Acme", providerUrl: "" });
    expect(
      parseA2ACard({ skills: [{ name: "a", tags: ["t", 3, null] }] }).skills[0],
    ).toMatchObject({
      id: "",
      tags: ["t"],
      examples: [],
    });
  });
});
