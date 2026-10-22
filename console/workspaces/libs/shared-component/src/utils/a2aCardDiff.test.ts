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
import {
  a2aCardsIdentical,
  canonicalJson,
  diffA2ACards,
  withoutInterfaceUrls,
} from "./a2aCardDiff";

const skill = (name: string, extra = {}) => ({
  id: name,
  name,
  description: "d",
  tags: [],
  ...extra,
});

const full = {
  name: "Orders",
  version: "1.0",
  provider: { organization: "Acme", url: "https://acme.example" },
  supportedInterfaces: [
    { url: "https://x/rpc", protocolBinding: "JSONRPC" },
    "junk",
  ],
  skills: [skill("order-status", { inputModes: ["text/plain"] }), "junk"],
  capabilities: { streaming: true },
  securitySchemes: { bearer: {} },
};

const cases: Array<[unknown, unknown]> = [
  [
    { skills: [skill("order-status")] },
    { skills: [skill("order-status"), skill("issue-refund")] },
  ],
  [
    { skills: [skill("order-status"), skill("issue-refund")] },
    { skills: [skill("order-status", { description: "other" })] },
  ],
  [
    { version: "1", capabilities: { a: 1, b: 2 } },
    { capabilities: { b: 2, a: 1 }, version: "2" },
  ],
  [
    { supportedInterfaces: [{ url: "a", protocolBinding: "JSONRPC" }] },
    { supportedInterfaces: [{ url: "b", protocolBinding: "JSONRPC" }] },
  ],
  [
    { supportedInterfaces: [{ url: "a", protocolBinding: "JSONRPC" }] },
    { supportedInterfaces: [{ url: "a", protocolBinding: "HTTP+JSON" }] },
  ],
  [
    { skills: [skill("a", { inputModes: ["text/plain"] })] },
    { skills: [skill("a", { inputModes: ["text/html"] })] },
  ],
  [{ skills: [skill("a"), skill("b")] }, { skills: [skill("b"), skill("a")] }],
  [{ skills: [skill("a"), skill("a")] }, { skills: [skill("a")] }],
  [
    { a: { x: 1, y: [{ p: 1, q: 2 }] }, skills: [skill("a")] },
    { skills: [skill("a")], a: { y: [{ q: 2, p: 1 }], x: 1 } },
  ],
  [{ version: "" }, {}],
  [
    { version: "1", skills: [skill("a"), skill("b")] },
    { version: "2", skills: [skill("b"), skill("a")] },
  ],
  [{ skills: [skill("a")] }, { skills: "x" }],
  [full, full],
  [{}, {}],
];

describe("diffA2ACards", () => {
  it("reports added, removed and changed skills with no fields", () => {
    expect(diffA2ACards(...cases[0])).toEqual({
      fields: [],
      skills: { added: ["issue-refund"], removed: [], changed: [] },
    });
    expect(diffA2ACards(...cases[1])).toEqual({
      fields: [],
      skills: {
        added: [],
        removed: ["issue-refund"],
        changed: ["order-status"],
      },
    });
  });

  it("ignores key order in fields", () =>
    expect(diffA2ACards(...cases[2]).fields).toEqual(["version"]));

  it("ignores interface URLs but not bindings", () => {
    expect(diffA2ACards(...cases[3]).fields).toEqual([]);
    expect(diffA2ACards(...cases[4]).fields).toEqual(["supportedInterfaces"]);
  });

  it("a skill's inputModes change marks it changed", () =>
    expect(diffA2ACards(...cases[5]).skills.changed).toEqual(["a"]));

  it("reordered skills report skills as a differing field", () =>
    expect(diffA2ACards(...cases[6])).toEqual({
      fields: ["skills"],
      skills: { added: [], removed: [], changed: [] },
    }));

  it("duplicate skill names still differ", () =>
    expect(diffA2ACards(...cases[7]).fields).toContain("skills"));

  it("nested key order is not a difference", () =>
    expect(a2aCardsIdentical(diffA2ACards(...cases[8]))).toBe(true));

  it("a key missing on one side differs from any value", () =>
    expect(diffA2ACards(...cases[9]).fields).toEqual(["version"]));

  it("keeps fields sorted when skills is appended", () =>
    expect(diffA2ACards(...cases[10]).fields).toEqual(["skills", "version"]));

  it("treats a non-array skills side as removed skills", () =>
    expect(diffA2ACards(...cases[11])).toEqual({
      fields: [],
      skills: { added: [], removed: ["a"], changed: [] },
    }));

  it("reads non-object cards as empty", () =>
    expect(a2aCardsIdentical(diffA2ACards("card", [1]))).toBe(true));

  it.each(cases)(
    "all-empty iff canonical forms without interface URLs match (%#)",
    (a, b) => {
      const asRecord = (v: unknown) => v as Record<string, unknown>;
      const same =
        canonicalJson(withoutInterfaceUrls(asRecord(a))) ===
        canonicalJson(withoutInterfaceUrls(asRecord(b)));
      expect(a2aCardsIdentical(diffA2ACards(a, b))).toBe(same);
    },
  );
});

describe("canonicalJson", () => {
  it("sorts keys at every depth", () =>
    expect(canonicalJson({ b: [{ d: 1, c: 2 }], a: null })).toBe(
      '{"a":null,"b":[{"c":2,"d":1}]}',
    ));

  it("returns undefined for undefined", () =>
    expect(canonicalJson(undefined)).toBeUndefined());
});

describe("withoutInterfaceUrls", () => {
  it("drops url from object interface items only and leaves the input untouched", () => {
    const card = {
      supportedInterfaces: [{ url: "a", protocolBinding: "JSONRPC" }, "junk"],
      name: "n",
    };
    expect(withoutInterfaceUrls(card)).toEqual({
      supportedInterfaces: [{ protocolBinding: "JSONRPC" }, "junk"],
      name: "n",
    });
    expect(card.supportedInterfaces[0]).toEqual({
      url: "a",
      protocolBinding: "JSONRPC",
    });
  });

  it("leaves a non-array supportedInterfaces as is", () =>
    expect(withoutInterfaceUrls({ supportedInterfaces: "x" })).toEqual({
      supportedInterfaces: "x",
    }));
});
