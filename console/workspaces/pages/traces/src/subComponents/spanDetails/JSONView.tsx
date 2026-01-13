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

  const isObject = data !== null && typeof data === "object" && !Array.isArray(data);
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
    if (typeof value === "string") return "info.light";
    if (typeof value === "number") return "info.main";
    if (typeof value === "boolean") return "error.main";
    return "text.primary";
  };

  if (isPrimitive) {
    return (
      <Stack direction="row" spacing={1} alignItems="center">
        {keyName && (
          <Typography
            variant="caption"
            fontFamily="monospace"
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
        {isEmpty && (
          <Box sx={{ width: "20px", minWidth: "20px" }} />
        )}
        {keyName && (
          <Typography
            variant="caption"
            fontFamily="monospace"
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
        {!isExpanded && (
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
          <Stack direction="column" spacing={0.5}>
            {entries.map(([key, value]) => (
              <TreeNode
                key={String(key)}
                keyName={isArray ? undefined : String(key)}
                data={value}
                level={level + 1}
              />
            ))}
          </Stack>
        </Box>

        <Typography
          variant="caption"
          fontFamily="monospace"
          sx={{ marginLeft: "20px" }}
          color="text.primary"
        >
          {closeBracket}
        </Typography>
      </Collapse>
    </Stack>
  );
}

export function JSONView({ json }: JSONViewProps) {
  let parsedJson: unknown;
  
  try {
    parsedJson = JSON.parse(json);
  } catch (error) {
    return (
        <Typography
        variant="caption"
        sx={{
          fontFamily: "monospace",
          whiteSpace: "pre-wrap",
          wordBreak: "break-word",
        }}
      >
        {(json)}
      </Typography>
    );
  }

  return (
    <Box sx={{ padding: 2, fontFamily: "monospace", fontSize: "14px" }}>
      <TreeNode data={parsedJson} />
    </Box>
  );
}
