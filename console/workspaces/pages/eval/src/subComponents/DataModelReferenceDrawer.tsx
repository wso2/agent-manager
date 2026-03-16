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

import { useRef, useState } from "react";
import {
  Box,
  Chip,
  Collapse,
  IconButton,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  BookOpen,
  Check,
  ChevronDown,
  ChevronRight,
  Copy,
} from "@wso2/oxygen-ui-icons-react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
} from "@agent-management-platform/views";
import { MODEL_TREES, type FieldNode, type ModelTreeKey } from "../generated/evaluator-models.generated";

export type ReferenceTypeKey = ModelTreeKey;

// ---------------------------------------------------------------------------
// Type badge color mapping
// ---------------------------------------------------------------------------

type TypeColor = { bg: string; fg: string };

const TYPE_COLORS: Record<string, TypeColor> = {
  str:   { bg: "rgba(56, 142, 60, 0.12)",  fg: "#388e3c" },   // green
  int:   { bg: "rgba(30, 136, 229, 0.12)", fg: "#1e88e5" },   // blue
  float: { bg: "rgba(30, 136, 229, 0.12)", fg: "#1e88e5" },   // blue
  bool:  { bg: "rgba(156, 39, 176, 0.12)", fg: "#9c27b0" },   // purple
  Any:   { bg: "rgba(120, 120, 120, 0.12)", fg: "#757575" },   // grey
};

const LIST_COLOR:  TypeColor = { bg: "rgba(255, 152, 0, 0.12)", fg: "#ef6c00" };  // orange
const DICT_COLOR:  TypeColor = { bg: "rgba(211, 47, 47, 0.12)", fg: "#d32f2f" };  // red
const CLASS_COLOR: TypeColor = { bg: "rgba(2, 136, 209, 0.12)", fg: "#0288d1" };  // teal

function getTypeColor(type: string): TypeColor {
  if (TYPE_COLORS[type]) return TYPE_COLORS[type];
  if (type.startsWith("List[")) return LIST_COLOR;
  if (type.startsWith("Dict[")) return DICT_COLOR;
  // Compound types like "str | None", "float | None"
  const base = type.split("|")[0].trim();
  if (TYPE_COLORS[base]) return TYPE_COLORS[base];
  // Class/model types
  return CLASS_COLOR;
}

// ---------------------------------------------------------------------------
// Copy path button
// ---------------------------------------------------------------------------

function CopyPathButton({ path }: { path: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation();
    navigator.clipboard.writeText(path).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    }).catch(() => { /* clipboard unavailable in insecure contexts */ });
  };

  return (
    <Tooltip title={copied ? "Copied!" : `Copy: ${path}`} placement="top">
      <IconButton
        size="small"
        onClick={handleCopy}
        sx={{
          opacity: 0,
          transition: "opacity 0.15s",
          p: 0.25,
          ".tree-row:hover &": { opacity: 0.7 },
          "&:hover": { opacity: "1 !important" },
        }}
      >
        {copied ? <Check size={12} /> : <Copy size={12} />}
      </IconButton>
    </Tooltip>
  );
}

// ---------------------------------------------------------------------------
// Tree node renderer
// ---------------------------------------------------------------------------

const INDENT = 20;

function TreeNode({
  node,
  depth = 0,
  defaultExpanded = false,
  parentPath = "",
}: {
  node: FieldNode;
  depth?: number;
  defaultExpanded?: boolean;
  parentPath?: string;
}) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const hasChildren = node.children && node.children.length > 0;
  const isTypeNode = !node.type; // e.g. "LLMStep", "UserStep" — union variant names

  // Build access path for copy
  const cleanName = node.name.replace(/\(\)$/, "");
  const accessPath = parentPath
    ? node.isMethod
      ? `${parentPath}.${cleanName}()`
      : `${parentPath}.${cleanName}`
    : node.isMethod
      ? `${cleanName}()`
      : cleanName;

  return (
    <Box>
      <Box
        className="tree-row"
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 0.5,
          py: 0.5,
          pr: 1,
          cursor: hasChildren ? "pointer" : "default",
          borderRadius: 0.5,
          "&:hover": { bgcolor: "action.hover" },
          position: "relative",
        }}
        onClick={hasChildren ? () => setExpanded((v) => !v) : undefined}
        onKeyDown={hasChildren ? (e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            setExpanded((v) => !v);
          }
        } : undefined}
        tabIndex={hasChildren ? 0 : undefined}
        role={hasChildren ? "button" : undefined}
      >
        {/* Indent guides */}
        {Array.from({ length: depth }, (_, i) => (
          <Box
            key={i}
            sx={{
              position: "absolute",
              left: `${i * INDENT + 10}px`,
              top: 0,
              bottom: 0,
              width: "1px",
              bgcolor: "divider",
            }}
          />
        ))}

        {/* Indent spacer */}
        <Box sx={{ width: `${depth * INDENT}px`, flexShrink: 0 }} />

        {/* Expand icon or spacer */}
        <Box sx={{ width: 20, flexShrink: 0, display: "flex", alignItems: "center", justifyContent: "center" }}>
          {hasChildren && (
            expanded
              ? <ChevronDown size={14} />
              : <ChevronRight size={14} />
          )}
        </Box>

        {/* name: type — description */}
        <Typography
          variant="body2"
          component="span"
          sx={{
            fontFamily: "monospace",
            fontSize: "0.82rem",
            fontWeight: isTypeNode ? 600 : 400,
            color: node.isMethod ? "primary.main" : "text.primary",
            minWidth: 0,
            flexShrink: 0,
          }}
        >
          {node.name}
        </Typography>

        {node.type && (
          <>
            <Typography
              variant="body2"
              component="span"
              sx={{
                fontFamily: "monospace",
                fontSize: "0.82rem",
                color: "text.secondary",
                flexShrink: 0,
                mx: -0.25,
              }}
            >
              :
            </Typography>
            {(() => {
              const { bg, fg } = getTypeColor(node.type);
              return (
                <Chip
                  label={node.type}
                  size="small"
                  sx={{
                    height: 18,
                    fontSize: "0.7rem",
                    fontFamily: "monospace",
                    bgcolor: bg,
                    color: fg,
                    fontWeight: 500,
                    "& .MuiChip-label": { px: 0.75, py: 0 },
                    borderRadius: 0.5,
                    flexShrink: 0,
                  }}
                />
              );
            })()}
          </>
        )}

        <Typography
          variant="body2"
          component="span"
          sx={{
            color: "text.secondary",
            fontSize: "0.78rem",
            whiteSpace: "normal",
            wordBreak: "break-word",
            flex: 1,
            minWidth: 0,
          }}
        >
          &mdash; {node.description}
        </Typography>

        {/* Copy path button — visible on hover */}
        {depth > 0 && !isTypeNode && (
          <CopyPathButton path={accessPath} />
        )}
      </Box>

      {/* Children */}
      {hasChildren && (
        <Collapse in={expanded}>
          {node.children!.map((child) => (
            <TreeNode
              key={child.name}
              node={child}
              depth={depth + 1}
              parentPath={isTypeNode ? parentPath : accessPath}
            />
          ))}
        </Collapse>
      )}
    </Box>
  );
}

// ---------------------------------------------------------------------------
// Drawer component
// ---------------------------------------------------------------------------

interface DataModelReferenceDrawerProps {
  open: boolean;
  onClose: () => void;
  typeKey: ReferenceTypeKey;
}

export function DataModelReferenceDrawer({
  open,
  onClose,
  typeKey,
}: DataModelReferenceDrawerProps) {
  // Keep last typeKey so content doesn't change during close animation
  const lastTypeKey = useRef(typeKey);
  if (open) {
    lastTypeKey.current = typeKey;
  }
  const tree = MODEL_TREES[lastTypeKey.current];

  return (
    <DrawerWrapper open={open} onClose={onClose} minWidth={650} maxWidth={800}>
      <DrawerHeader
        icon={<BookOpen size={24} />}
        title={`${tree.className} Reference`}
        onClose={onClose}
      />
      <DrawerContent>
        <Chip
          label={`from ${tree.module} import ${tree.className}`}
          size="small"
          variant="outlined"
          sx={{ fontFamily: "monospace", fontSize: "0.8rem", alignSelf: "flex-start" }}
        />

        <Box
          sx={{
            border: 1,
            borderColor: "divider",
            borderRadius: 1,
            bgcolor: "background.paper",
            p: 1.5,
            overflow: "auto",
          }}
        >
          <TreeNode
            node={{
              name: tree.className,
              type: "",
              description: tree.description,
              children: tree.nodes,
            }}
            defaultExpanded
          />
        </Box>

        <Typography variant="caption" color="text.secondary" sx={{ mt: 1 }}>
          Click on items with arrows to expand. Hover to copy the access path.
        </Typography>
      </DrawerContent>
    </DrawerWrapper>
  );
}
