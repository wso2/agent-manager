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

import React, { useCallback, useMemo, useRef, useEffect } from "react";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  Button,
  IconButton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown, Plus, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { SchemaTreeNode, ParameterSchema, ParameterValues } from "./types";
import { schemaToTreeNodes, getValueByPath } from "./schemaUtils";
import { getFieldRenderer } from "./FieldRenderers";

// ---------------------------------------------------------------------------
// Shared classifier: splits nodes into main vs advanced sections.
// Priority: x-wso2-policy-advanced-param flag > isRequired > has required children.
// ---------------------------------------------------------------------------

function classifyNodes(nodes: SchemaTreeNode[]): {
  mainNodes: SchemaTreeNode[];
  advancedNodes: SchemaTreeNode[];
} {
  // Only apply advanced/main split when at least one node explicitly declares
  // the flag. If no node has it, treat all as main to avoid spurious accordions.
  const hasExplicitFlags = nodes.some(
    (n) => n.schema["x-wso2-policy-advanced-param"] !== undefined,
  );

  if (!hasExplicitFlags) {
    return { mainNodes: nodes, advancedNodes: [] };
  }

  const mainNodes: SchemaTreeNode[] = [];
  const advancedNodes: SchemaTreeNode[] = [];

  nodes.forEach((n) => {
    const advancedFlag = n.schema["x-wso2-policy-advanced-param"];
    if (advancedFlag === false) {
      mainNodes.push(n);
      return;
    }
    if (advancedFlag === true) {
      advancedNodes.push(n);
      return;
    }
    // Flag absent but siblings have flags: fall back to isRequired / required children.
    const hasRequiredChildren =
      n.schema.type === "object" &&
      n.schema.properties !== undefined &&
      (n.schema.required?.length ?? 0) > 0;
    if (n.isRequired || hasRequiredChildren) {
      mainNodes.push(n);
    } else {
      advancedNodes.push(n);
    }
  });

  return { mainNodes, advancedNodes };
}

// ---------------------------------------------------------------------------
// Single tree node
// ---------------------------------------------------------------------------

interface SchemaTreeNodeProps {
  node: SchemaTreeNode;
  values: ParameterValues;
  onChange: (path: string, value: unknown) => void;
  onAddArrayItem: (arrayPath: string, itemSchema: ParameterSchema) => void;
  onDeleteArrayItem: (arrayPath: string, index: number) => void;
  errors: Record<string, string>;
  disabled?: boolean;
}

const SchemaTreeNodeComponent: React.FC<SchemaTreeNodeProps> = ({
  node,
  values,
  onChange,
  onAddArrayItem,
  onDeleteArrayItem,
  errors,
  disabled,
}) => {
  const value = getValueByPath(values, node.path);
  const isComplexArray =
    node.schema.type === "array" && node.schema.items?.type === "object";
  const arrayValue = useMemo(
    () => (Array.isArray(value) ? value : []),
    [value],
  );

  const FieldRenderer = getFieldRenderer(node);
  const isLeaf = FieldRenderer !== null;

  const handleAddArrayItem = useCallback(
    (arrayPath: string, itemSchema: ParameterSchema) => {
      onAddArrayItem(arrayPath, itemSchema);
    },
    [onAddArrayItem],
  );

  const prevArrayLengthRef = useRef(arrayValue.length);
  const newItemIndex =
    arrayValue.length > prevArrayLengthRef.current ? arrayValue.length - 1 : -1;

  const arrayItems = useMemo(() => {
    if (!isComplexArray) return [];
    return arrayValue.map((_, index) => {
      const itemPath = `${node.path}.${index}`;
      const itemNode: SchemaTreeNode = {
        id: itemPath,
        path: itemPath,
        name: `Item ${index + 1}`,
        schema: node.schema.items!,
        depth: node.depth + 1,
        isRequired: false,
        isExpanded: index === newItemIndex,
        isArrayItem: true,
        arrayIndex: index,
        parentArrayPath: node.path,
        children:
          node.schema.items?.type === "object" && node.schema.items?.properties
            ? schemaToTreeNodes(
                node.schema.items,
                itemPath,
                node.depth + 2,
                node.schema.items.required || [],
              )
            : undefined,
      };
      return itemNode;
    });
  }, [isComplexArray, arrayValue, node, newItemIndex]);

  useEffect(() => {
    prevArrayLengthRef.current = arrayValue.length;
  });

  // ---- Leaf node: delegate to the field renderer ----
  if (isLeaf) {
    return (
      <FieldRenderer
        node={node}
        value={value}
        onChange={onChange}
        error={errors[node.path]}
        disabled={disabled}
      />
    );
  }

  // ---- Expandable node (object / complex array): use Accordion ----
  const summaryLabel = (
    <Stack direction="row" alignItems="center" spacing={1} sx={{ flex: 1 }}>
      <Typography variant="h6">{node.name}</Typography>
      {!node.isRequired && (
        <Typography variant="caption" color="text.secondary">
          (Optional)
        </Typography>
      )}
      {node.isArrayItem && node.parentArrayPath !== undefined && (
        <IconButton
          size="small"
          color="error"
          onClick={(e) => {
            e.stopPropagation();
            onDeleteArrayItem(node.parentArrayPath!, node.arrayIndex!);
          }}
          disabled={disabled}
          aria-label={`Delete ${node.name}`}
          sx={{ ml: "auto" }}
        >
          <Trash2 size={14} />
        </IconButton>
      )}
    </Stack>
  );

  return (
    <Accordion defaultExpanded={node.isExpanded ?? false} disableGutters>
      <AccordionSummary expandIcon={<ChevronDown size={16} />}>
        {summaryLabel}
      </AccordionSummary>
      <AccordionDetails>
        {node.schema.description && (
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            {node.schema.description}
          </Typography>
        )}

        {/* Children of object nodes */}
        {node.children && node.children.length > 0 && !isComplexArray && (() => {
          const { mainNodes, advancedNodes } = classifyNodes(node.children!);
          return (
            <>
              {mainNodes.length > 0 && (
                <Stack>
                  {mainNodes.map((child) => (
                    <SchemaTreeNodeComponent
                      key={child.id}
                      node={child}
                      values={values}
                      onChange={onChange}
                      onAddArrayItem={handleAddArrayItem}
                      onDeleteArrayItem={onDeleteArrayItem}
                      errors={errors}
                      disabled={disabled}
                    />
                  ))}
                </Stack>
              )}
              {advancedNodes.length > 0 && (
                <Accordion disableGutters sx={{ mt: 1 }}>
                  <AccordionSummary expandIcon={<ChevronDown size={16} />}>
                    <Typography variant="body2">Advanced Settings</Typography>
                  </AccordionSummary>
                  <AccordionDetails>
                    <Stack>
                      {advancedNodes.map((child) => (
                        <SchemaTreeNodeComponent
                          key={child.id}
                          node={child}
                          values={values}
                          onChange={onChange}
                          onAddArrayItem={handleAddArrayItem}
                          onDeleteArrayItem={onDeleteArrayItem}
                          errors={errors}
                          disabled={disabled}
                        />
                      ))}
                    </Stack>
                  </AccordionDetails>
                </Accordion>
              )}
            </>
          );
        })()}

        {/* Complex array items */}
        {isComplexArray && (
          <Stack spacing={1.5}>
            <Box>
              <Button
                variant="outlined"
                size="small"
                startIcon={<Plus size={14} />}
                onClick={() =>
                  handleAddArrayItem(node.path, node.schema.items!)
                }
                disabled={disabled}
              >
                Add Item
              </Button>
            </Box>
            <Stack>
              {arrayItems.map((itemNode) => (
                <SchemaTreeNodeComponent
                  key={itemNode.id}
                  node={itemNode}
                  values={values}
                  onChange={onChange}
                  onAddArrayItem={handleAddArrayItem}
                  onDeleteArrayItem={onDeleteArrayItem}
                  errors={errors}
                  disabled={disabled}
                />
              ))}
            </Stack>
          </Stack>
        )}
      </AccordionDetails>
    </Accordion>
  );
};

// ---------------------------------------------------------------------------
// Main SchemaTree
// ---------------------------------------------------------------------------

interface SchemaTreeProps {
  schema: ParameterSchema;
  values: ParameterValues;
  onChange: (path: string, value: unknown) => void;
  onAddArrayItem: (arrayPath: string, itemSchema: ParameterSchema) => void;
  onDeleteArrayItem: (arrayPath: string, index: number) => void;
  errors: Record<string, string>;
  disabled?: boolean;
}

const SchemaTree: React.FC<SchemaTreeProps> = ({
  schema,
  values,
  onChange,
  onAddArrayItem,
  onDeleteArrayItem,
  errors,
  disabled,
}) => {
  const treeNodes = useMemo(
    () => schemaToTreeNodes(schema, "", 0, schema.required || []),
    [schema],
  );

  const { mainNodes, advancedNodes } = useMemo(
    () => classifyNodes(treeNodes),
    [treeNodes],
  );

  return (
    <Stack spacing={2}>
      {/* Main (non-advanced) fields */}
      {mainNodes.length > 0 && (
        <>
          <Typography variant="h6">Parameters</Typography>
          <Stack>
            {mainNodes.map((node) => (
              <SchemaTreeNodeComponent
                key={node.id}
                node={node}
                values={values}
                onChange={onChange}
                onAddArrayItem={onAddArrayItem}
                onDeleteArrayItem={onDeleteArrayItem}
                errors={errors}
                disabled={disabled}
              />
            ))}
          </Stack>
        </>
      )}

      {/* Advanced fields wrapped in an Accordion */}
      {advancedNodes.length > 0 && (
        <Stack spacing={4}>
          <Accordion>
            <AccordionSummary expandIcon={<ChevronDown size={16} />}>
              <Typography variant="h6">Advanced Settings</Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Stack>
                {advancedNodes.map((node) => (
                  <SchemaTreeNodeComponent
                    key={node.id}
                    node={node}
                    values={values}
                    onChange={onChange}
                    onAddArrayItem={onAddArrayItem}
                    onDeleteArrayItem={onDeleteArrayItem}
                    errors={errors}
                    disabled={disabled}
                  />
                ))}
              </Stack>
            </AccordionDetails>
          </Accordion>
        </Stack>
      )}
    </Stack>
  );
};

export default SchemaTree;
