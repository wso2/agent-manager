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

import { ParameterSchema, SchemaTreeNode, ParameterValues } from "./types";

let nodeIdCounter = 0;
export const generateNodeId = (): string => {
  nodeIdCounter += 1;
  return `node-${nodeIdCounter}`;
};

export const resetNodeIdCounter = (): void => {
  nodeIdCounter = 0;
};

export function schemaToTreeNodes(
  schema: ParameterSchema,
  parentPath: string = "",
  depth: number = 0,
  requiredFields: string[] = [],
): SchemaTreeNode[] {
  const nodes: SchemaTreeNode[] = [];

  if (schema.type === "object" && schema.properties) {
    const required = schema.required || [];

    Object.entries(schema.properties).forEach(([key, propSchema]) => {
      const path = parentPath ? `${parentPath}.${key}` : key;
      const isRequired =
        required.includes(key) || requiredFields.includes(key);

      const node: SchemaTreeNode = {
        id: path,
        path,
        name: key,
        schema: propSchema,
        depth,
        isRequired,
        isExpanded: false,
      };

      if (propSchema.type === "object" && propSchema.properties) {
        node.children = schemaToTreeNodes(
          propSchema,
          path,
          depth + 1,
          propSchema.required || [],
        );
      } else if (propSchema.type === "array") {
        node.isArrayContainer = true;
      }

      nodes.push(node);
    });
  }

  return nodes;
}

export function createArrayItemNodes(
  arrayPath: string,
  itemSchema: ParameterSchema,
  items: unknown[],
  depth: number,
): SchemaTreeNode[] {
  return items.map((_, index) => {
    const itemPath = `${arrayPath}.${index}`;
    const node: SchemaTreeNode = {
      id: generateNodeId(),
      path: itemPath,
      name: `Item ${index + 1}`,
      schema: itemSchema,
      depth,
      isRequired: false,
      isExpanded: false,
      isArrayItem: true,
      arrayIndex: index,
      parentArrayPath: arrayPath,
    };

    if (itemSchema.type === "object" && itemSchema.properties) {
      node.children = schemaToTreeNodes(
        itemSchema,
        itemPath,
        depth + 1,
        itemSchema.required || [],
      );
    }

    return node;
  });
}

export function getValueByPath(obj: ParameterValues, path: string): unknown {
  if (!path) return obj;

  const keys = path.split(".");
  let current: unknown = obj;

  for (const key of keys) {
    if (current === null || current === undefined) return undefined;
    if (typeof current === "object") {
      current = (current as Record<string, unknown>)[key];
    } else {
      return undefined;
    }
  }

  return current;
}

export function setValueByPath(
  obj: ParameterValues,
  path: string,
  value: unknown,
): ParameterValues {
  if (!path) return value as ParameterValues;

  const keys = path.split(".");
  const result = { ...obj };
  let current: Record<string, unknown> = result;

  for (let i = 0; i < keys.length - 1; i++) {
    const key = keys[i];
    const nextKey = keys[i + 1];
    const isNextKeyArrayIndex = /^\d+$/.test(nextKey);

    if (current[key] === undefined || current[key] === null) {
      current[key] = isNextKeyArrayIndex ? [] : {};
    } else if (Array.isArray(current[key])) {
      current[key] = [...(current[key] as unknown[])];
    } else if (typeof current[key] === "object") {
      current[key] = { ...(current[key] as Record<string, unknown>) };
    }

    current = current[key] as Record<string, unknown>;
  }

  const lastKey = keys[keys.length - 1];
  current[lastKey] = value;

  return result;
}

export function deleteValueByPath(
  obj: ParameterValues,
  path: string,
): ParameterValues {
  const keys = path.split(".");
  const result = { ...obj };

  if (keys.length === 1) {
    delete result[keys[0]];
    return result;
  }

  const parentPath = keys.slice(0, -1).join(".");
  const lastKey = keys[keys.length - 1];
  const parent = getValueByPath(result, parentPath);

  if (Array.isArray(parent)) {
    const index = parseInt(lastKey, 10);
    const newArray = [...parent];
    newArray.splice(index, 1);
    return setValueByPath(result, parentPath, newArray);
  }

  if (typeof parent === "object" && parent !== null) {
    const newParent = { ...(parent as Record<string, unknown>) };
    delete newParent[lastKey];
    return setValueByPath(result, parentPath, newParent);
  }

  return result;
}

export function initializeDefaultValues(
  schema: ParameterSchema,
  existingValues?: ParameterValues,
): ParameterValues {
  const result: ParameterValues = existingValues ? { ...existingValues } : {};

  if (schema.type === "object" && schema.properties) {
    Object.entries(schema.properties).forEach(([key, propSchema]) => {
      if (result[key] === undefined) {
        if (propSchema.default !== undefined) {
          result[key] = propSchema.default;
        } else if (propSchema.type === "object" && propSchema.properties) {
          result[key] = initializeDefaultValues(propSchema);
        } else if (propSchema.type === "array") {
          result[key] = [];
        } else if (propSchema.type === "boolean") {
          result[key] = false;
        } else if (
          propSchema.type === "number" ||
          propSchema.type === "integer"
        ) {
          result[key] = propSchema.default ?? 0;
        } else {
          result[key] = "";
        }
      } else if (
        propSchema.type === "object" &&
        propSchema.properties &&
        typeof result[key] === "object"
      ) {
        result[key] = initializeDefaultValues(
          propSchema,
          result[key] as ParameterValues,
        );
      }
    });
  }

  return result;
}

function coerceValue(value: unknown, schema: ParameterSchema): unknown {
  if (value === null || value === undefined) return value;
  if (typeof value === "string" && /\$\{.+\}/.test(value)) return value;

  switch (schema.type) {
    case "boolean": {
      if (typeof value === "boolean") return value;
      if (typeof value === "string") {
        if (value.toLowerCase() === "true") return true;
        if (value.toLowerCase() === "false") return false;
      }
      return value;
    }
    case "number": {
      if (typeof value === "number") return value;
      if (typeof value === "string" && value !== "") {
        const parsed = parseFloat(value);
        if (!Number.isNaN(parsed)) return parsed;
      }
      return value;
    }
    case "integer": {
      if (typeof value === "number") return value;
      if (typeof value === "string" && value !== "") {
        const parsed = parseInt(value, 10);
        if (!Number.isNaN(parsed)) return parsed;
      }
      return value;
    }
    case "object": {
      if (typeof value === "object" && !Array.isArray(value)) return value;
      if (typeof value === "string" && value === "") return {};
      return value;
    }
    default:
      return value;
  }
}

export function coerceValuesToSchemaTypes(
  schema: ParameterSchema,
  values: ParameterValues,
): ParameterValues {
  if (schema.type !== "object" || !schema.properties) return values;

  const result: ParameterValues = { ...values };

  Object.entries(schema.properties).forEach(([key, propSchema]) => {
    const val = result[key];
    if (val === undefined) return;

    if (
      propSchema.type === "object" &&
      propSchema.properties &&
      typeof val === "object" &&
      val !== null &&
      !Array.isArray(val)
    ) {
      result[key] = coerceValuesToSchemaTypes(
        propSchema,
        val as ParameterValues,
      );
    } else if (
      propSchema.type === "array" &&
      Array.isArray(val) &&
      propSchema.items
    ) {
      result[key] = val.map((item) => {
        if (
          propSchema.items!.type === "object" &&
          propSchema.items!.properties &&
          typeof item === "object" &&
          item !== null
        ) {
          return coerceValuesToSchemaTypes(
            propSchema.items!,
            item as ParameterValues,
          );
        }
        return coerceValue(item, propSchema.items!);
      });
    } else {
      result[key] = coerceValue(val, propSchema);
    }
  });

  return result;
}

export function createDefaultArrayItem(itemSchema: ParameterSchema): unknown {
  if (itemSchema.type === "object" && itemSchema.properties) {
    return initializeDefaultValues(itemSchema);
  }
  if (itemSchema.type === "string") return itemSchema.default ?? "";
  if (itemSchema.type === "boolean") return itemSchema.default ?? false;
  if (
    itemSchema.type === "number" ||
    itemSchema.type === "integer"
  )
    return itemSchema.default ?? 0;
  if (itemSchema.type === "array") return [];
  return null;
}
