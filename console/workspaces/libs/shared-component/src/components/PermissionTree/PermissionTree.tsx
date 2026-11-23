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

import React, { useEffect, useMemo, useRef, useState } from "react";
import {
  Box,
  Checkbox,
  Chip,
  Collapse,
  IconButton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown, ChevronRight } from "@wso2/oxygen-ui-icons-react";

export interface PermissionTreeItem {
  /** Unique identifier for this permission — the exact value sent to/received from the API. */
  id: string;
  /** Hierarchy path segments this permission is grouped under, e.g. ["amp", "catalog", "read"]. */
  path: string[];
  /** Optional description shown under the row. */
  description?: string;
}

export interface PermissionTreeProps {
  /** Full catalog of assignable permissions. */
  items: PermissionTreeItem[];
  /** Ids of the currently-selected permissions. */
  selectedIds: string[];
  /** Called with the full next set of selected ids whenever the user toggles a row. */
  onChange: (selectedIds: string[]) => void;
  /** Disables all checkboxes; the tree remains expandable. */
  readOnly?: boolean;
  /** Formats a path segment into a display label. Defaults to title-casing on "-"/"_". */
  formatSegmentLabel?: (segment: string, path: string[]) => string;
  emptyMessage?: string;
}

interface TreeNode {
  key: string;
  segment: string;
  path: string[];
  item?: PermissionTreeItem;
  children: TreeNode[];
  /** All selectable ids under this node, including its own (if any). */
  descendantIds: string[];
}

const defaultFormatSegmentLabel = (segment: string) =>
  segment
    .split(/[-_]/g)
    .filter(Boolean)
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");

function buildTree(items: PermissionTreeItem[]): TreeNode[] {
  const roots: TreeNode[] = [];
  const nodeByKey = new Map<string, TreeNode>();

  const getOrCreateNode = (path: string[]): TreeNode => {
    const key = JSON.stringify(path);
    const existing = nodeByKey.get(key);
    if (existing) return existing;

    const node: TreeNode = {
      key,
      segment: path[path.length - 1],
      path,
      children: [],
      descendantIds: [],
    };
    nodeByKey.set(key, node);
    if (path.length === 1) {
      roots.push(node);
    } else {
      getOrCreateNode(path.slice(0, -1)).children.push(node);
    }
    return node;
  };

  for (const item of items) {
    getOrCreateNode(item.path).item = item;
  }

  const fillDescendantIds = (node: TreeNode): string[] => {
    const own = node.item ? [node.item.id] : [];
    const fromChildren = node.children.flatMap(fillDescendantIds);
    node.descendantIds = [...own, ...fromChildren];
    return node.descendantIds;
  };
  roots.forEach(fillDescendantIds);

  return roots;
}

interface PermissionTreeRowProps {
  node: TreeNode;
  depth: number;
  selectedIdSet: Set<string>;
  isExpanded: (key: string) => boolean;
  onToggleExpand: (key: string) => void;
  onToggleNode: (node: TreeNode) => void;
  readOnly: boolean;
  formatLabel: (segment: string, path: string[]) => string;
}

const PermissionTreeRow: React.FC<PermissionTreeRowProps> = ({
  node,
  depth,
  selectedIdSet,
  isExpanded,
  onToggleExpand,
  onToggleNode,
  readOnly,
  formatLabel,
}) => {
  const hasChildren = node.children.length > 0;
  const expanded = hasChildren && isExpanded(node.key);
  const total = node.descendantIds.length;
  const selectedCount = node.descendantIds.filter((id) =>
    selectedIdSet.has(id),
  ).length;
  const checked = total > 0 && selectedCount === total;
  const indeterminate = selectedCount > 0 && selectedCount < total;
  const rowLabel = formatLabel(node.segment, node.path);
  // Full path (not just the leaf segment) so rows sharing a leaf name in
  // different branches still get a unique, unambiguous accessible name.
  const fullPathLabel = node.path
    .map((segment) => formatLabel(segment, node.path))
    .join(" / ");
  const canToggle = !readOnly && total > 0;
  // MUI only renders the indeterminate icon; set the native property so
  // assistive technology announces a partially selected group as mixed.
  const checkboxRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (checkboxRef.current) {
      checkboxRef.current.indeterminate = indeterminate;
    }
  }, [indeterminate]);

  return (
    <Box>
      <Stack
        direction="row"
        alignItems="center"
        spacing={0.5}
        sx={{ pl: depth * 3, py: 0.25 }}
      >
        {hasChildren ? (
          <IconButton
            size="small"
            onClick={() => onToggleExpand(node.key)}
            aria-label={expanded ? "Collapse" : "Expand"}
          >
            {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          </IconButton>
        ) : (
          <Box sx={{ width: 32, flexShrink: 0 }} />
        )}
        <Checkbox
          size="small"
          checked={checked}
          indeterminate={indeterminate}
          disabled={!canToggle}
          onChange={() => onToggleNode(node)}
          inputRef={checkboxRef}
          inputProps={{ "aria-label": fullPathLabel }}
        />
        <Stack
          direction="row"
          alignItems="center"
          spacing={1}
          onClick={canToggle ? () => onToggleNode(node) : undefined}
          sx={canToggle ? { cursor: "pointer" } : undefined}
        >
          <Typography variant="body2" fontWeight={node.item ? 400 : 600}>
            {rowLabel}
          </Typography>
          {node.item && (
            <Chip
              label={node.item.id}
              size="small"
              variant="outlined"
              sx={{ fontFamily: "monospace", fontSize: "0.7rem" }}
            />
          )}
        </Stack>
      </Stack>
      {node.item?.description && (
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ display: "block", pl: depth * 3 + 8 }}
        >
          {node.item.description}
        </Typography>
      )}
      {hasChildren && (
        <Collapse in={expanded} timeout="auto" unmountOnExit>
          <Box>
            {node.children.map((child) => (
              <PermissionTreeRow
                key={child.key}
                node={child}
                depth={depth + 1}
                selectedIdSet={selectedIdSet}
                isExpanded={isExpanded}
                onToggleExpand={onToggleExpand}
                onToggleNode={onToggleNode}
                readOnly={readOnly}
                formatLabel={formatLabel}
              />
            ))}
          </Box>
        </Collapse>
      )}
    </Box>
  );
};

/**
 * Renders a flat permission catalog — each item carrying its own hierarchy
 * `path` (e.g. `["amp", "catalog", "read"]` for a scope like "amp:catalog:read",
 * or `[resource, action]` for a two-level catalog) — as an expandable checkbox
 * tree. Group nodes have no permission of their own; their checkbox is a pure aggregate
 * over descendant permissions and toggling it selects/deselects the whole
 * subtree at once. A node that is itself a real permission AND a parent of
 * finer-grained ones (e.g. "amp:catalog" alongside "amp:catalog:read") is
 * aggregated together with its descendants the same way.
 */
export const PermissionTree: React.FC<PermissionTreeProps> = ({
  items,
  selectedIds,
  onChange,
  readOnly = false,
  formatSegmentLabel = defaultFormatSegmentLabel,
  emptyMessage = "No permissions available.",
}) => {
  const tree = useMemo(() => buildTree(items), [items]);
  // Top-level/group nodes are expanded by default so the catalog isn't fully
  // collapsed on mount; the user can still collapse/expand any node from there.
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(
    () => new Set(tree.map((node) => node.key)),
  );
  const selectedIdSet = useMemo(() => new Set(selectedIds), [selectedIds]);

  const isExpanded = (key: string) => expandedKeys.has(key);

  const handleToggleExpand = (key: string) => {
    setExpandedKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  const handleToggleNode = (node: TreeNode) => {
    if (readOnly) return;
    const ids = node.descendantIds;
    if (!ids.length) return;
    const idsSet = new Set(ids);
    const allSelected = ids.every((id) => selectedIdSet.has(id));
    const kept = selectedIds.filter((id) => !idsSet.has(id));
    onChange(allSelected ? kept : [...kept, ...ids]);
  };

  if (items.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        {emptyMessage}
      </Typography>
    );
  }

  return (
    <Box>
      {tree.map((node) => (
        <PermissionTreeRow
          key={node.key}
          node={node}
          depth={0}
          selectedIdSet={selectedIdSet}
          isExpanded={isExpanded}
          onToggleExpand={handleToggleExpand}
          onToggleNode={handleToggleNode}
          readOnly={readOnly}
          formatLabel={formatSegmentLabel}
        />
      ))}
    </Box>
  );
};

export default PermissionTree;
