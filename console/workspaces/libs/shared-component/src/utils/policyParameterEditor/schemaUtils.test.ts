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

import { beforeEach, describe, expect, it } from "vitest";
import {
  coerceValuesToSchemaTypes,
  createArrayItemNodes,
  createDefaultArrayItem,
  deleteValueByPath,
  generateNodeId,
  getValueByPath,
  hasMeaningfulValue,
  initializeDefaultValues,
  normalizeRootSchema,
  resetNodeIdCounter,
  schemaToTreeNodes,
  setValueByPath,
} from "./schemaUtils";
import type { ParameterSchema } from "./types";

describe("normalizeRootSchema", () => {
  it.each([
    ["undefined", undefined],
    ["null", null],
    ["a string", "nope"],
    ["an array", [{ type: "object" }]],
    ["an object without a type", { properties: {} }],
    ["an unknown type", { type: "date" }],
  ])("falls back to an empty object schema for %s", (_label, input) => {
    expect(normalizeRootSchema(input)).toEqual({ type: "object", properties: {} });
  });

  it("keeps valid properties and drops malformed ones recursively", () => {
    const result = normalizeRootSchema({
      type: "object",
      required: ["a"],
      properties: {
        a: { type: "string", description: "A" },
        bad: { type: "weird" },
        notObject: 42,
        nested: {
          type: "object",
          properties: { inner: { type: "integer" }, junk: "x" },
        },
        list: { type: "array", items: { type: "boolean" } },
        brokenList: { type: "array", items: { type: "nope" } },
      },
    });

    expect(result).toEqual({
      type: "object",
      required: ["a"],
      properties: {
        a: { type: "string", description: "A" },
        nested: { type: "object", properties: { inner: { type: "integer" } } },
        list: { type: "array", items: { type: "boolean" } },
        brokenList: { type: "array" },
      },
    });
  });

  it("gives an object schema empty properties when properties is not a record", () => {
    expect(normalizeRootSchema({ type: "object", properties: ["x"] })).toEqual({
      type: "object",
      properties: {},
    });
  });

  it("strips properties from a non-object schema that declares none", () => {
    expect(normalizeRootSchema({ type: "string" })).toEqual({ type: "string" });
  });
});

describe("node ids", () => {
  it("generates sequential ids and resets", () => {
    resetNodeIdCounter();
    expect(generateNodeId()).toBe("node-1");
    expect(generateNodeId()).toBe("node-2");
    resetNodeIdCounter();
    expect(generateNodeId()).toBe("node-1");
  });
});

describe("schemaToTreeNodes", () => {
  it("returns no nodes for a non-object schema", () => {
    expect(schemaToTreeNodes({ type: "string" })).toEqual([]);
  });

  it("builds nested nodes with paths, depth and required flags", () => {
    const schema: ParameterSchema = {
      type: "object",
      required: ["name"],
      properties: {
        name: { type: "string" },
        headers: { type: "array", items: { type: "string" } },
        auth: {
          type: "object",
          required: ["token"],
          properties: { token: { type: "string" }, scheme: { type: "string" } },
        },
      },
    };

    const nodes = schemaToTreeNodes(schema);
    expect(nodes.map((n) => [n.path, n.depth, n.isRequired])).toEqual([
      ["name", 0, true],
      ["headers", 0, false],
      ["auth", 0, false],
    ]);
    expect(nodes[1].isArrayContainer).toBe(true);
    expect(nodes[2].children?.map((c) => [c.path, c.depth, c.isRequired])).toEqual([
      ["auth.token", 1, true],
      ["auth.scheme", 1, false],
    ]);
  });

  it("honours requiredFields passed by the caller", () => {
    const nodes = schemaToTreeNodes(
      { type: "object", properties: { x: { type: "string" } } },
      "root",
      2,
      ["x"],
    );
    expect(nodes[0]).toMatchObject({ path: "root.x", depth: 2, isRequired: true });
  });
});

describe("createArrayItemNodes", () => {
  beforeEach(() => resetNodeIdCounter());

  it("creates one node per item with object children", () => {
    const itemSchema: ParameterSchema = {
      type: "object",
      required: ["k"],
      properties: { k: { type: "string" } },
    };
    const nodes = createArrayItemNodes("headers", itemSchema, [{}, {}], 1);
    expect(nodes).toHaveLength(2);
    expect(nodes[1]).toMatchObject({
      id: "node-2",
      path: "headers.1",
      name: "Item 2",
      isArrayItem: true,
      arrayIndex: 1,
      parentArrayPath: "headers",
      depth: 1,
    });
    expect(nodes[1].children?.[0]).toMatchObject({ path: "headers.1.k", isRequired: true, depth: 2 });
  });

  it("creates leaf nodes for primitive items", () => {
    const nodes = createArrayItemNodes("tags", { type: "string" }, ["a"], 0);
    expect(nodes[0].children).toBeUndefined();
  });
});

describe("getValueByPath", () => {
  const obj = { a: { b: [{ c: 1 }] }, s: "str", n: null };

  it.each([
    ["", obj],
    ["a.b.0.c", 1],
    ["a.missing.c", undefined],
    ["s.length", undefined],
    ["n.x", undefined],
  ])("resolves %j", (path, expected) => {
    expect(getValueByPath(obj, path)).toEqual(expected);
  });
});

describe("setValueByPath", () => {
  it("replaces the whole object for an empty path", () => {
    expect(setValueByPath({ a: 1 }, "", { b: 2 })).toEqual({ b: 2 });
  });

  it("creates intermediate objects and arrays as the path requires", () => {
    expect(setValueByPath({}, "a.b", 1)).toEqual({ a: { b: 1 } });
    expect(setValueByPath({}, "list.0.name", "x")).toEqual({ list: [{ name: "x" }] });
    expect(setValueByPath({ a: "scalar" }, "a.b", 1)).toEqual({ a: { b: 1 } });
  });

  it("does not mutate the original object or its nested containers", () => {
    const original = { a: { b: 1 }, list: [1, 2] };
    const updated = setValueByPath(original, "a.c", 2);
    const updatedList = setValueByPath(original, "list.1", 9);
    expect(original).toEqual({ a: { b: 1 }, list: [1, 2] });
    expect(updated).toEqual({ a: { b: 1, c: 2 }, list: [1, 2] });
    expect(updatedList.list).toEqual([1, 9]);
    expect(updatedList.list).not.toBe(original.list);
  });
});

describe("deleteValueByPath", () => {
  it("deletes a top-level key", () => {
    expect(deleteValueByPath({ a: 1, b: 2 }, "a")).toEqual({ b: 2 });
  });

  it("splices an array item out", () => {
    expect(deleteValueByPath({ list: ["a", "b", "c"] }, "list.1")).toEqual({ list: ["a", "c"] });
  });

  it("deletes a nested object key", () => {
    expect(deleteValueByPath({ a: { b: 1, c: 2 } }, "a.b")).toEqual({ a: { c: 2 } });
  });

  it("returns an unchanged copy when the parent is not a container", () => {
    expect(deleteValueByPath({ a: "x" }, "a.b")).toEqual({ a: "x" });
  });
});

describe("hasMeaningfulValue", () => {
  it.each([
    [undefined, false],
    [null, false],
    ["", false],
    ["   ", false],
    ["x", true],
    [0, true],
    [false, true],
    [[], false],
    [["", null], false],
    [["", "a"], true],
    [{}, false],
    [{ a: "", b: { c: null } }, false],
    [{ a: { b: 1 } }, true],
  ])("%j -> %s", (value, expected) => {
    expect(hasMeaningfulValue(value)).toBe(expected);
  });
});

describe("initializeDefaultValues", () => {
  it("fills defaults per property type", () => {
    const schema: ParameterSchema = {
      type: "object",
      properties: {
        jsonPath: { type: "string", default: "$.x" },
        withDefault: { type: "string", default: "hello" },
        str: { type: "string" },
        flag: { type: "boolean" },
        list: { type: "array", items: { type: "string" } },
        min: { type: "integer", minimum: 3 },
        max: { type: "number", maximum: 9 },
        plainNumber: { type: "number" },
        nested: { type: "object", properties: { inner: { type: "boolean" } } },
      },
    };

    expect(initializeDefaultValues(schema)).toEqual({
      jsonPath: "",
      withDefault: "hello",
      str: "",
      flag: false,
      list: [],
      min: 3,
      max: 9,
      plainNumber: "",
      nested: { inner: false },
    });
  });

  it("keeps existing values and fills only missing nested defaults", () => {
    const schema: ParameterSchema = {
      type: "object",
      properties: {
        name: { type: "string", default: "d" },
        nested: {
          type: "object",
          properties: { a: { type: "string" }, b: { type: "string", default: "B" } },
        },
      },
    };
    expect(initializeDefaultValues(schema, { name: "kept", nested: { a: "A" } })).toEqual({
      name: "kept",
      nested: { a: "A", b: "B" },
    });
  });

  it("defaults a discriminator boolean to false even when its default is true", () => {
    const schema: ParameterSchema = {
      type: "object",
      properties: {
        enabled: { type: "boolean", default: true },
        config: { type: "object", properties: { url: { type: "string" } } },
      },
      anyOf: [
        { required: ["enabled"], properties: { enabled: { const: false } } },
        { required: ["enabled", "config"], properties: { enabled: { const: true } } },
      ],
    };
    // `config` is only required by a branch, so it stays unset until opted in.
    expect(initializeDefaultValues(schema)).toEqual({ enabled: false });
  });

  it("returns an empty object for a non-object schema", () => {
    expect(initializeDefaultValues({ type: "string" })).toEqual({});
  });
});

describe("coerceValuesToSchemaTypes", () => {
  const schema: ParameterSchema = {
    type: "object",
    required: ["reqStr"],
    properties: {
      reqStr: { type: "string" },
      bool: { type: "boolean" },
      boolFalse: { type: "boolean" },
      boolOther: { type: "boolean" },
      num: { type: "number" },
      badNum: { type: "number" },
      int: { type: "integer" },
      badInt: { type: "integer" },
      templated: { type: "integer" },
      obj: { type: "object" },
      optionalEmpty: { type: "string" },
      emptyList: { type: "array", items: { type: "string" } },
    },
  };

  it("coerces string inputs to their schema types", () => {
    const result = coerceValuesToSchemaTypes(schema, {
      reqStr: "",
      bool: "TRUE",
      boolFalse: "false",
      boolOther: "maybe",
      num: "1.5",
      badNum: "abc",
      int: "42.9",
      badInt: "zz",
      templated: "${env.PORT}",
      obj: "",
      optionalEmpty: "",
      emptyList: [],
    });
    expect(result).toEqual({
      reqStr: "",
      bool: true,
      boolFalse: false,
      boolOther: "maybe",
      num: 1.5,
      badNum: "abc",
      int: 42,
      badInt: "zz",
      templated: "${env.PORT}",
      emptyList: [],
    });
  });

  it("returns values untouched for a non-object schema", () => {
    const values = { a: "1" };
    expect(coerceValuesToSchemaTypes({ type: "string" }, values)).toBe(values);
  });

  it("recurses into nested objects and array items, keeping empty item keys", () => {
    const nestedSchema: ParameterSchema = {
      type: "object",
      properties: {
        limits: { type: "object", properties: { max: { type: "integer" } } },
        headers: {
          type: "array",
          items: {
            type: "object",
            properties: { name: { type: "string" }, value: { type: "string" } },
          },
        },
        ports: { type: "array", items: { type: "integer" } },
      },
    };
    expect(
      coerceValuesToSchemaTypes(nestedSchema, {
        limits: { max: "10" },
        headers: [{ name: "", value: "" }],
        ports: ["80", 443],
      }),
    ).toEqual({
      limits: { max: 10 },
      headers: [{ name: "", value: "" }],
      ports: [80, 443],
    });
  });

  it("strips empty keys of the non-selected oneOf branch", () => {
    const branchSchema: ParameterSchema = {
      type: "object",
      required: ["text", "messages"],
      properties: {
        text: { type: "string" },
        messages: { type: "array", items: { type: "string" } },
      },
      oneOf: [{ required: ["text"] }, { required: ["messages"] }],
    };
    expect(coerceValuesToSchemaTypes(branchSchema, { text: "", messages: ["hi"] })).toEqual({
      messages: ["hi"],
    });
    expect(coerceValuesToSchemaTypes(branchSchema, { text: "hello", messages: [] })).toEqual({
      text: "hello",
    });
  });
});

describe("createDefaultArrayItem", () => {
  it.each<[string, ParameterSchema, unknown]>([
    ["object", { type: "object", properties: { a: { type: "boolean" } } }, { a: false }],
    ["string", { type: "string" }, ""],
    ["string with default", { type: "string", default: "x" }, "x"],
    ["boolean", { type: "boolean" }, false],
    ["number with default", { type: "number", default: 0 }, 0],
    ["integer with minimum", { type: "integer", minimum: 5 }, 5],
    ["integer with maximum", { type: "integer", maximum: 7 }, 7],
    ["number without bounds", { type: "number" }, ""],
    ["array", { type: "array" }, []],
    ["object without properties", { type: "object" }, null],
  ])("%s", (_label, schema, expected) => {
    expect(createDefaultArrayItem(schema)).toEqual(expected);
  });
});
