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

import { Stack, Typography, IconButton, Box, Collapse } from "@wso2/oxygen-ui";
import { ChevronRight, ChevronDown } from "@wso2/oxygen-ui-icons-react";
import { useState } from "react";

interface JSONViewProps {
  json: string;
}

interface TreeNodeProps {
  data: unknown;
  keyName?: string;
  level?: number;
}

function TreeNode({ data, keyName, level = 0 }: TreeNodeProps) {
  const [isExpanded, setIsExpanded] = useState(level < 2); // Auto-expand first 2 levels

  const isObject =
    data !== null && typeof data === "object" && !Array.isArray(data);
  const isArray = Array.isArray(data);
  const isPrimitive = !isObject && !isArray;

  const renderPrimitiveValue = (value: unknown) => {
    if (value === null) return "null";
    if (typeof value === "string") return `"${value}"`;
    if (typeof value === "boolean") return value.toString();
    if (typeof value === "number") return value.toString();
    if (value === undefined) return "undefined";
    return String(value);
  };

  const getValueColor = (value: unknown) => {
    if (value === null) return "text.disabled";
    if (typeof value === "string") return "text.primary";
    if (typeof value === "number") return "text.primary";
    if (typeof value === "boolean") return "error.main";
    return "text.primary";
  };

  if (isPrimitive) {
    return (
      <Stack direction="row" spacing={1} alignItems="flex-start">
        {keyName && (
          <Typography
            variant="caption"
            fontFamily="monospace"
            color="text.secondary"
            fontWeight={600}
          >
            {keyName}:
          </Typography>
        )}
        <Typography
          variant="caption"
          fontFamily="monospace"
          color={getValueColor(data)}
        >
          {renderPrimitiveValue(data)}
        </Typography>
      </Stack>
    );
  }

  const entries = isObject
    ? Object.entries(data as Record<string, unknown>)
    : (data as unknown[]).map((item, index) => [index.toString(), item]);

  const isEmpty = entries.length === 0;
  const openBracket = isArray ? "[" : "{";
  const closeBracket = isArray ? "]" : "}";

  return (
    <Stack direction="column" spacing={0.5}>
      <Stack direction="row" spacing={0.5} alignItems="center">
        {!isEmpty && (
          <IconButton
            size="small"
            onClick={() => setIsExpanded(!isExpanded)}
            sx={{ padding: "2px", minWidth: "20px", height: "20px" }}
          >
            {isExpanded ? (
              <ChevronDown size={16} />
            ) : (
              <ChevronRight size={16} />
            )}
          </IconButton>
        )}
        {isEmpty && <Box sx={{ width: "20px", minWidth: "20px" }} />}
        {keyName && (
          <Typography
            variant="caption"
            fontFamily="monospace"
            color="text.secondary"
            fontWeight={600}
          >
            {keyName}:
          </Typography>
        )}
        <Typography
          variant="caption"
          fontFamily="monospace"
          color="text.primary"
        >
          {openBracket}
        </Typography>
        {!isExpanded && !isEmpty && (
          <Typography
            variant="caption"
            fontFamily="monospace"
            sx={{ fontStyle: "italic" }}
            color="text.secondary"
          >
            {entries.length} {isArray ? "items" : "properties"}
          </Typography>
        )}
        {(isEmpty || !isExpanded) && (
          <Typography
            variant="caption"
            fontFamily="monospace"
            color="text.primary"
          >
            {closeBracket}
          </Typography>
        )}
      </Stack>

      <Collapse in={isExpanded && !isEmpty} timeout="auto" unmountOnExit>
        <Box sx={{ pl: 4 }}>
          <Stack direction="column" spacing={0.5} pl={1}>
            {entries.map(([key, value]) => (
              <TreeNode
                key={String(key)}
                keyName={isArray ? undefined : String(key)}
                data={value}
                level={level + 1}
              />
            ))}
          </Stack>

          <Typography
            variant="caption"
            fontFamily="monospace"
            color="text.primary"
          >
            {closeBracket}
          </Typography>
        </Box>
      </Collapse>
    </Stack>
  );
}

export function JSONView({ json }: JSONViewProps) {
  let parsedJson: unknown;

  try {
    parsedJson = JSON.parse(json);
  } catch {
    return (
      <Typography
        variant="caption"
        sx={{
          fontFamily: "monospace",
          whiteSpace: "pre-wrap",
          wordBreak: "break-word",
        }}
      >
        {json}
      </Typography>
    );
  }

  return (
    <Stack>
      <TreeNode data={parsedJson} />
    </Stack>
  );
}
