/*
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

import { describe, expect, it } from "vitest";

import {
  checkConditions,
  customPolicyOf,
  draftsFromConditions,
  fieldKeys,
  isBuiltInType,
  MAX_WHOLE_NUMBER,
  newCondition,
  newPair,
  newWindow,
  exactJsonNumbers,
  parseValue,
  type ConditionDraft,
} from "./planConditions";

const requestCount = (windows: Parameters<typeof newWindow>[0][]): ConditionDraft => ({
  ...newCondition("request-count"),
  windows: windows.map((window) => newWindow(window)),
});

const custom = (id: string, pairs: [string, string][] = []): ConditionDraft => ({
  ...newCondition("custom"),
  id,
  pairs: pairs.map(([name, value]) => ({ ...newPair(name), value })),
});

// Errors are matched by their text.
const messageIds = (drafts: ConditionDraft[]) =>
  [...checkConditions(drafts).errors.values()].map((error) => error.message);

describe("checkConditions", () => {
  it("builds the conditions the API takes, with the fixed id of a built-in type", () => {
    const tokens = { ...newCondition("token-count"), tokenType: "completion" as const };
    tokens.windows = [newWindow({ count: 10, timeUnit: "DAY" })];
    expect(checkConditions([tokens]).body).toEqual([
      {
        id: "tokens",
        params: {
          limits: [{ count: 10, timeAmount: 1, timeUnit: "DAY" }],
          onExceed: "reject",
          tokenType: "completion",
        },
        type: "token-count",
      },
    ]);
  });

  it("sends tokenType only for token-count", () => {
    const body = checkConditions([requestCount([{ count: 1 }])]).body!;
    expect(body[0].params).not.toHaveProperty("tokenType");
  });

  it("treats 60 minutes and 1 hour as the same window, but not 30 days and a month", () => {
    const same = requestCount([
      { count: 1, timeAmount: 60, timeUnit: "MINUTE" },
      { count: 2, timeAmount: 1, timeUnit: "HOUR" },
    ]);
    expect(checkConditions([same]).errors.has(fieldKeys.unit(same.key, same.windows[1].key))).toBe(
      true,
    );
    const apart = requestCount([
      { count: 1, timeAmount: 30, timeUnit: "DAY" },
      { count: 2, timeAmount: 1, timeUnit: "MONTH" },
    ]);
    expect(checkConditions([apart]).body).not.toBeNull();
  });

  it("refuses more than 20 conditions", () => {
    const drafts = Array.from({ length: 21 }, (_, index) => custom(`c-${index}`));
    expect(checkConditions(drafts).errors.has(fieldKeys.conditions())).toBe(true);
  });

  it("refuses two custom conditions with one id, and a custom id of a built-in condition", () => {
    expect(messageIds([custom("tier"), custom("tier")])).toHaveLength(1);
    expect(messageIds([custom("tool-calls")])[0]).toMatch(/is the name of a built-in condition/);
  });

  it("refuses custom values of more than 4096 bytes in all", () => {
    const big = custom("big", [["text", "x".repeat(4100)]]);
    expect(checkConditions([big]).errors.has(fieldKeys.params(big.key))).toBe(true);
  });

  it("skips an empty pair, and refuses a key used twice", () => {
    expect(checkConditions([custom("tier", [["", ""]])]).body).toEqual([
      { id: "tier", params: {}, type: "custom" },
    ]);
    expect(
      messageIds([
        custom("tier", [
          ["a", "1"],
          ["a", "2"],
        ]),
      ])[0],
    ).toMatch(/is used twice/);
  });
});

describe("checkConditions: whole numbers past what a number holds exactly", () => {
  it("refuses a count or window length that is not a safe integer, rather than rounding it", () => {
    const draft = requestCount([{ timeUnit: "MINUTE" }]);
    draft.windows[0].count = "9007199254740993";
    draft.windows[0].timeAmount = "9007199254740993";
    const check = checkConditions([draft]);
    expect(check.body).toBeNull();
    const count = check.errors.get(fieldKeys.count(draft.key, draft.windows[0].key));
    const amount = check.errors.get(fieldKeys.amount(draft.key, draft.windows[0].key));
    for (const error of [count, amount]) {
      expect(error?.message).toMatch(/Enter a whole number of at most/);
      expect(error?.message).toContain(MAX_WHOLE_NUMBER.toLocaleString());
    }
  });

  it("takes the largest safe integer as it is", () => {
    const draft = requestCount([{ timeUnit: "MONTH" }]);
    draft.windows[0].count = String(MAX_WHOLE_NUMBER);
    draft.windows[0].timeAmount = String(MAX_WHOLE_NUMBER);
    expect(checkConditions([draft]).body?.[0].params).toMatchObject({
      limits: [{ count: MAX_WHOLE_NUMBER, timeAmount: MAX_WHOLE_NUMBER, timeUnit: "MONTH" }],
    });
  });

  it("keeps apart two windows whose length in minutes is past the exact range", () => {
    const big = Math.floor(MAX_WHOLE_NUMBER / 60);
    const draft = requestCount([
      { count: 1, timeAmount: big, timeUnit: "HOUR" },
      { count: 2, timeAmount: big - 1, timeUnit: "HOUR" },
    ]);
    expect(checkConditions([draft]).body).not.toBeNull();
  });
});

describe("checkConditions: custom values", () => {
  it("sends an untouched saved value as it was, spaces and all, rather than typing it", () => {
    const [tier] = draftsFromConditions([
      { id: "tier", params: { flag: " true ", code: "007" }, type: "custom" },
    ]);
    expect(checkConditions([tier]).body?.[0].params).toEqual({ code: "007", flag: " true " });

    // Once edited, the value is typed from the text as usual.
    tier.pairs[0].value = "true";
    expect(checkConditions([tier]).body?.[0].params).toEqual({ code: "007", flag: true });
  });

  it("sends an untouched saved name as it was, and trims one the admin types", () => {
    const [tier] = draftsFromConditions([
      { id: "tier", params: { " region ": "eu" }, type: "custom" },
    ]);
    expect(checkConditions([tier]).body?.[0].params).toEqual({ " region ": "eu" });

    tier.pairs[0].name = " zone ";
    expect(checkConditions([tier]).body?.[0].params).toEqual({ zone: "eu" });
  });

  it("keeps a key named __proto__ as a key of its own, through to the JSON sent", () => {
    const draft = custom("tier", [
      ["__proto__", "gold"],
      ["constructor", "1"],
    ]);
    const check = checkConditions([draft]);
    expect(check.errors.size).toBe(0);
    const params = check.body![0].params as Record<string, unknown>;
    expect(Object.keys(params)).toEqual(["__proto__", "constructor"]);
    expect(JSON.stringify(check.body)).toBe(
      '[{"id":"tier","params":{"__proto__":"gold","constructor":1},"type":"custom"}]',
    );
  });

  it("refuses a __proto__ key used twice", () => {
    expect(
      messageIds([
        custom("tier", [
          ["__proto__", "1"],
          ["__proto__", "2"],
        ]),
      ])[0],
    ).toMatch(/is used twice/);
  });

  it("does not take a custom type or ID named after an Object property for a built-in one", () => {
    expect(isBuiltInType("constructor")).toBe(false);
    expect(isBuiltInType("__proto__")).toBe(false);
    expect(isBuiltInType("request-count")).toBe(true);
  });
});

describe("parseValue", () => {
  it("types numbers, booleans and JSON, and keeps other text", () => {
    expect(parseValue("42")).toBe(42);
    expect(parseValue("true")).toBe(true);
    expect(parseValue('{"a":1}')).toEqual({ a: 1 });
    expect(parseValue("{not json")).toBe("{not json");
    expect(parseValue("UTC")).toBe("UTC");
  });

  it("keeps a number it can’t hold exactly as text, on its own or inside JSON", () => {
    expect(parseValue("9".repeat(400))).toBe("9".repeat(400));
    expect(parseValue("9007199254740993")).toBe("9007199254740993");
    expect(parseValue("0.1234567890123456789")).toBe("0.1234567890123456789");
    expect(parseValue("1.25")).toBe(1.25);
    expect(parseValue('{"id":12345678901234567890}')).toBe('{"id":12345678901234567890}');
    expect(parseValue('{"limit":50}')).toEqual({ limit: 50 });
  });

  it("keeps a number that underflows to 0 or overflows as text", () => {
    const tiny = `0.${"0".repeat(399)}1`;
    expect(parseValue(tiny)).toBe(tiny);
    expect(parseValue('{"limit":1e-400}')).toBe('{"limit":1e-400}');
    expect(parseValue('{"limit":1e400}')).toBe('{"limit":1e400}');
    expect(parseValue("[1.2345678901234567]")).toBe("[1.2345678901234567]");
    // Zero and ordinary numbers still convert.
    expect(parseValue("0")).toBe(0);
    expect(parseValue("0.0")).toBe(0);
    expect(parseValue('{"a":0,"b":-2.5,"c":1e3,"d":[0.25]}')).toEqual({
      a: 0,
      b: -2.5,
      c: 1000,
      d: [0.25],
    });
  });

  it("checks every number in the JSON text itself, whatever the engine gives back", () => {
    expect(exactJsonNumbers('{"a":1,"b":[2.5,-3e2]}')).toBe(true);
    expect(exactJsonNumbers('{"a":1e-400}')).toBe(false);
    expect(exactJsonNumbers('{"a":12345678901234567890}')).toBe(false);
    // Digits inside strings, escaped quotes included, are not numbers.
    expect(exactJsonNumbers('{"id":"12345678901234567890","q":"a\\"1e-400"}')).toBe(true);
    expect(parseValue('{"id":"12345678901234567890"}')).toEqual({ id: "12345678901234567890" });
  });
});

describe("draftsFromConditions", () => {
  it("opens a saved condition with its windows, and a custom one with its values", () => {
    const [requests, tier] = draftsFromConditions([
      {
        id: "requests",
        params: { limits: [{ count: 5, timeAmount: 2, timeUnit: "HOUR" }], onExceed: "allow" },
        type: "request-count",
      },
      { id: "tier", params: { code: "007" }, type: "custom" },
    ]);
    expect(
      requests.windows.map(({ count, timeAmount, timeUnit }) => ({ count, timeAmount, timeUnit })),
    ).toEqual([{ count: "5", timeAmount: "2", timeUnit: "HOUR" }]);
    expect(requests.onExceed).toBe("allow");
    expect(checkConditions([tier]).body).toEqual([
      { id: "tier", params: { code: "007" }, type: "custom" },
    ]);
  });
});

describe("a custom condition’s policy (params.policy)", () => {
  it("is sent under the reserved key, version v1 when left empty, and left out when not given", () => {
    const withPolicy = {
      ...custom("region-limit", [["region", "eu"]]),
      policyName: "region-guard",
    };
    expect(checkConditions([withPolicy]).body).toEqual([
      {
        id: "region-limit",
        params: { policy: { name: "region-guard", version: "v1" }, region: "eu" },
        type: "custom",
      },
    ]);
    expect(checkConditions([custom("region-limit", [["region", "eu"]])]).body).toEqual([
      { id: "region-limit", params: { region: "eu" }, type: "custom" },
    ]);
  });

  it("refuses `policy` as a key of the key-value pairs, and a bad policy name or version", () => {
    expect(messageIds([custom("a", [["policy", "x"]])])).toEqual([
      expect.stringMatching(/reserved for the custom policy/),
    ]);
    expect(messageIds([{ ...custom("a"), policyName: "", policyVersion: "v1" }])).toEqual([
      expect.stringMatching(/Enter the policy name/),
    ]);
    expect(messageIds([{ ...custom("a"), policyName: "guard", policyVersion: "latest" }])).toEqual([
      expect.stringMatching(/Enter a version such as v1/),
    ]);
  });

  it("round-trips on edit: out of the pairs, into its own fields", () => {
    const [draft] = draftsFromConditions([
      {
        id: "region-limit",
        params: { policy: { name: "region-guard", version: "v2" }, region: "eu" },
        type: "custom",
      },
    ] as never);
    expect(draft.policyName).toBe("region-guard");
    expect(draft.policyVersion).toBe("v2");
    expect(draft.pairs.map((pair) => pair.name)).toEqual(["region"]);
    expect(checkConditions([draft]).body?.[0]).toEqual({
      id: "region-limit",
      params: { policy: { name: "region-guard", version: "v2" }, region: "eu" },
      type: "custom",
    });
  });

  it("is read as name and major version", () => {
    expect(
      customPolicyOf({ params: { policy: { name: "g", version: "v2.1.0" } }, type: "custom" }),
    ).toEqual({ name: "g", version: "v2" });
    expect(customPolicyOf({ params: { region: "eu" }, type: "custom" })).toBeNull();
    expect(customPolicyOf({ params: { policy: { name: "g" } }, type: "request-count" })).toBeNull();
  });
});
