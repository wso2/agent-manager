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

const PARAMETER_SCHEMA_TYPES = new Set<string>([
  "object",
  "array",
  "string",
  "boolean",
  "number",
  "integer",
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function createEmptyParameterSchema(): ParameterSchema {
  return {
    type: "object",
    properties: {},
  };
}

function normalizeSchema(value: unknown): ParameterSchema | undefined {
  if (!isRecord(value)) {
    return undefined;
  }

  if (
    typeof value.type !== "string" ||
    !PARAMETER_SCHEMA_TYPES.has(value.type)
  ) {
    return undefined;
  }

  const schema = { ...value, type: value.type } as ParameterSchema;

  if (value.properties !== undefined || value.type === "object") {
    schema.properties = normalizeProperties(value.properties);
  } else {
    delete schema.properties;
  }

  if (value.items !== undefined) {
    const items = normalizeSchema(value.items);
    if (items) {
      schema.items = items;
    } else {
      delete schema.items;
    }
  }

  return schema;
}

function normalizeProperties(value: unknown): Record<string, ParameterSchema> {
  if (!isRecord(value)) {
    return {};
  }

  return Object.entries(value).reduce<Record<string, ParameterSchema>>(
    (properties, [key, schema]) => {
      const normalizedSchema = normalizeSchema(schema);
      if (normalizedSchema) {
        properties[key] = normalizedSchema;
      }
      return properties;
    },
    {},
  );
}

/**
 * Normalizes an arbitrary parsed value (from YAML or already-JSON, e.g. a gateway
 * manifest's inline `parameters`/`systemParameters`) into a well-formed ParameterSchema,
 * falling back to an empty object schema when the input isn't schema-shaped.
 */
export function normalizeRootSchema(value: unknown): ParameterSchema {
  return normalizeSchema(value) ?? createEmptyParameterSchema();
}

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
    const currentValue = current[key];
    const shouldCreateIntermediateValue =
      currentValue === undefined ||
      currentValue === null ||
      typeof currentValue !== "object";

    if (shouldCreateIntermediateValue) {
      current[key] = isNextKeyArrayIndex ? [] : {};
    } else if (Array.isArray(currentValue)) {
      current[key] = [...currentValue];
    } else {
      current[key] = { ...(currentValue as Record<string, unknown>) };
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

/**
 * Check whether a boolean field acts as a discriminator (has a `const: false`
 * branch) in an anyOf/oneOf where the other branch requires additional fields.
 * When true, we default the field to `false` so the section starts disabled
 * and the user must explicitly opt in.
 */
function shouldDefaultToDisabled(
  key: string,
  schema: ParameterSchema,
): boolean {
  const branches = schema.anyOf ?? schema.oneOf ?? [];
  if (branches.length === 0) return false;
  return branches.some(
    (b) => b.properties?.[key]?.const === false && b.required?.includes(key),
  );
}

function isBranchRequiredKey(key: string, schema: ParameterSchema): boolean {
  const branches = schema.anyOf ?? schema.oneOf ?? [];
  return branches.some((branch) => branch.required?.includes(key));
}

export function hasMeaningfulValue(value: unknown): boolean {
  if (value === undefined || value === null || value === "") return false;
  if (typeof value === "string" && value.trim().length === 0) return false;
  if (Array.isArray(value)) return value.some(hasMeaningfulValue);
  if (typeof value === "object") {
    return Object.values(value as Record<string, unknown>).some(
      hasMeaningfulValue,
    );
  }
  return true;
}

export function initializeDefaultValues(
  schema: ParameterSchema,
  existingValues?: ParameterValues,
): ParameterValues {
  const result: ParameterValues = existingValues ? { ...existingValues } : {};

  if (schema.type === "object" && schema.properties) {
    Object.entries(schema.properties).forEach(([key, propSchema]) => {
      if (result[key] === undefined) {
        if (key === "jsonPath") {
          result[key] = "";
        } else if (
          propSchema.type === "boolean" &&
          shouldDefaultToDisabled(key, schema)
        ) {
          result[key] = false;
        } else if (propSchema.default !== undefined) {
          result[key] = propSchema.default;
        } else if (propSchema.type === "object" && propSchema.properties) {
          if (isBranchRequiredKey(key, schema)) {
            return;
          }
          result[key] = initializeDefaultValues(propSchema);
        } else if (propSchema.type === "array") {
          result[key] = [];
        } else if (propSchema.type === "boolean") {
          result[key] = false;
        } else if (
          propSchema.type === "number" ||
          propSchema.type === "integer"
        ) {
          result[key] = propSchema.minimum ?? propSchema.maximum ?? "";
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

/**
 * Given oneOf/anyOf branches, determine which branch keys should be stripped
 * because they are empty and belong to a non-selected branch.
 *
 * A branch is "selected" if at least one of its required fields has a
 * non-empty value. Keys from unselected branches that are empty get removed
 * so the backend doesn't see conflicting fields (e.g. both `text: ""` and
 * `messages: [...]`).
 */
function stripExclusiveBranchKeys(
  branches: { required?: string[] }[],
  result: ParameterValues,
): void {
  if (branches.length === 0) return;

  // Collect the set of "branch-exclusive" keys per branch.
  const branchKeys = branches.map((b) => new Set(b.required ?? []));

  // Determine which branch is selected: the one whose required fields
  // are all non-empty.
  let selectedIdx = -1;
  branchKeys.forEach((keys, idx) => {
    if (selectedIdx >= 0) return;
    let satisfied = keys.size > 0;
    keys.forEach((key) => {
      const v = result[key];
      if (!hasMeaningfulValue(v)) satisfied = false;
    });
    if (satisfied) selectedIdx = idx;
  });

  // Remove empty keys from non-selected branches.
  branchKeys.forEach((keys, idx) => {
    if (idx === selectedIdx) return;
    keys.forEach((key) => {
      const v = result[key];
      if (!hasMeaningfulValue(v)) {
        delete result[key];
      }
    });
  });
}

export function coerceValuesToSchemaTypes(
  schema: ParameterSchema,
  values: ParameterValues,
  options?: { isArrayItem?: boolean },
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
          // Array items are coerced but not stripped of empty keys: a
          // half-filled row (e.g. {name: "", value: ""}) must keep every
          // item-schema key present, or the backend receives a bare `{}`
          // with none of the row's fields.
          return coerceValuesToSchemaTypes(
            propSchema.items!,
            item as ParameterValues,
            { isArrayItem: true },
          );
        }
        return coerceValue(item, propSchema.items!);
      });
    } else {
      result[key] = coerceValue(val, propSchema);
    }
  });

  // Strip empty values from non-selected oneOf/anyOf branches.
  const branches = schema.oneOf ?? schema.anyOf ?? [];
  if (branches.length > 0) {
    stripExclusiveBranchKeys(branches, result);
  }

  if (options?.isArrayItem) {
    return result;
  }

  // Never send unset optional fields to the gateway: an explicit empty
  // string/object for an optional param (e.g. an unconfigured endpoint or
  // API key) fails config.toml parsing downstream, whereas omitting the
  // key lets the policy fall back to its own default. Required keys are
  // left as-is so blank-but-required fields still surface as a validation
  // error instead of silently vanishing.
  //
  // Array-typed fields are exempt: an emptied allow-list (e.g.
  // `allowedTools: []`) is a deliberate "block everything" choice, not an
  // unset field — dropping the key would silently fall back to the
  // policy's default list, which can flip a fail-closed intent to fail-open.
  //
  // `jsonPath` is exempt too: the policies read an empty jsonPath as "check
  // the whole payload", while a missing one falls back to an OpenAI path
  // ($.messages[-1].content) that cannot read other request shapes, such
  // as AWS Bedrock Converse, where content is a list of blocks.
  const requiredKeys = new Set(schema.required ?? []);
  Object.entries(schema.properties).forEach(([key, propSchema]) => {
    if (requiredKeys.has(key)) return;
    if (propSchema.type === "array") return;
    if (key === "jsonPath") {
      if (typeof result[key] === "string") {
        result[key] = (result[key] as string).trim();
      }
      return;
    }
    if (!hasMeaningfulValue(result[key])) {
      delete result[key];
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
    return itemSchema.default !== undefined ? itemSchema.default : itemSchema.minimum ?? itemSchema.maximum ?? "";
  if (itemSchema.type === "array") return [];
  return null;
}
