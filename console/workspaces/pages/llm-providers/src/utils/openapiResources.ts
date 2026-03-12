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

import yaml from "js-yaml";

export const HTTP_METHODS = new Set([
  "get",
  "post",
  "put",
  "delete",
  "patch",
  "head",
  "options",
  "trace",
]);

export type ResourceItem = {
  method: string;
  path: string;
  summary?: string;
};

export function parseOpenApiSpec(
  text: string,
): Record<string, unknown> | null {
  if (!text?.trim()) return null;
  try {
    const spec = JSON.parse(text) as Record<string, unknown>;
    return spec && typeof spec === "object" ? spec : null;
  } catch {
    try {
      const spec = yaml.load(text) as Record<string, unknown>;
      return spec && typeof spec === "object" ? spec : null;
    } catch {
      return null;
    }
  }
}

export function extractResourcesFromSpec(
  spec: Record<string, unknown>,
): ResourceItem[] {
  const paths = spec?.paths as Record<string, unknown> | undefined;
  if (!paths || typeof paths !== "object") return [];

  const extracted: ResourceItem[] = [];

  for (const path of Object.keys(paths)) {
    const operations = paths[path] as Record<string, unknown> | undefined;
    if (!operations || typeof operations !== "object") continue;

    for (const methodKey of Object.keys(operations)) {
      if (!HTTP_METHODS.has(methodKey.toLowerCase())) continue;

      const op = (operations[methodKey] || {}) as Record<string, unknown>;
      extracted.push({
        method: methodKey.toUpperCase(),
        path,
        summary: (op?.summary || op?.description) as string | undefined,
      });
    }
  }

  extracted.sort((a, b) => {
    const p = a.path.localeCompare(b.path);
    if (p !== 0) return p;
    return a.method.localeCompare(b.method);
  });

  return extracted;
}

export function getResourceKey(
  r: ResourceItem,
  separator: string = "::",
): string {
  return `${r.method}${separator}${r.path}`;
}

export function getMethodChipColor(
  method: string,
): "info" | "success" | "error" | "default" {
  const m = method.toUpperCase();
  if (m === "GET") return "info";
  if (m === "POST") return "success";
  if (m === "DELETE") return "error";
  return "default";
}
