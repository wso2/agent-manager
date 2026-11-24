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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it } from "vitest";
import { extractOpenApiResources, parseOpenApiSpecContent } from "./openApiSpec";

describe("parseOpenApiSpecContent", () => {
  it.each([
    ["undefined", undefined],
    ["blank", "   "],
    ["a JSON scalar", "42"],
    ["a YAML scalar", "just words"],
    ["invalid YAML", "a: [unclosed"],
  ])("returns undefined for %s", (_label, content) => {
    expect(parseOpenApiSpecContent(content)).toBeUndefined();
  });

  it("parses JSON", () => {
    expect(parseOpenApiSpecContent('{"openapi":"3.0.0"}')).toEqual({ openapi: "3.0.0" });
  });

  it("parses YAML when JSON fails", () => {
    expect(parseOpenApiSpecContent("openapi: 3.0.0\npaths: {}\n")).toEqual({
      openapi: "3.0.0",
      paths: {},
    });
  });
});

describe("extractOpenApiResources", () => {
  it("returns an empty list without paths", () => {
    expect(extractOpenApiResources(undefined)).toEqual([]);
    expect(extractOpenApiResources({ openapi: "3" })).toEqual([]);
  });

  it("flattens methods, skips non-HTTP keys and sorts by path then method", () => {
    const spec = {
      paths: {
        "/pets/{id}": {
          parameters: [],
          get: { description: "Get pet" },
          delete: null,
        },
        "/pets": {
          post: { summary: "Create pet" },
          GET: { summary: "List pets" },
        },
        "/empty": null,
      },
    };
    expect(extractOpenApiResources(spec)).toEqual([
      { method: "GET", path: "/pets", summary: "List pets" },
      { method: "POST", path: "/pets", summary: "Create pet" },
      { method: "DELETE", path: "/pets/{id}", summary: undefined },
      { method: "GET", path: "/pets/{id}", summary: "Get pet" },
    ]);
  });
});
