/*
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

import { Box, Chip, Stack, Typography } from "@wso2/oxygen-ui";

export type ResourceViewItem = {
  method: string;
  path: string;
  summary?: string;
};

type ResourceRowProps = {
  resource: ResourceViewItem;
  selected?: boolean;
  showSummary?: boolean;
  onClick?: () => void;
  trailing?: React.ReactNode;
};

export default function ResourceRow({
  resource,
  selected = false,
  showSummary = true,
  onClick,
  trailing,
}: ResourceRowProps) {
  const method = String(resource.method ?? "GET").toUpperCase();
  const path = String(resource.path ?? "");
  const methodColor =
    method === "GET"
      ? "info"
      : method === "POST"
        ? "success"
        : method === "DELETE"
          ? "error"
          : "default";

  return (
    <Box
      onClick={onClick}
      sx={{
        display: "flex",
        alignItems: "center",
        gap: 1,
        p: 1.25,
        border: "1px solid",
        borderColor: selected ? "primary.main" : "divider",
        borderRadius: 1,
        bgcolor: selected ? "action.selected" : "background.paper",
        cursor: onClick ? "pointer" : "default",
        minWidth: 0,
        "&:hover": onClick ? { bgcolor: "action.hover" } : {},
      }}
    >
      <Chip
        label={method}
        size="small"
        color={methodColor}
        sx={{ minWidth: 56, justifyContent: "center", flexShrink: 0 }}
      />
      <Stack spacing={0.25} sx={{ flex: 1, minWidth: 0 }}>
        <Typography
          variant="body2"
          sx={{
            fontFamily: "monospace",
            fontWeight: 500,
            minWidth: 0,
            maxWidth: "100%",
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
        >
          {path}
        </Typography>
        {showSummary && (resource.summary || "") && (
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{
              minWidth: 0,
              maxWidth: "100%",
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
              display: "block",
            }}
          >
            {resource.summary}
          </Typography>
        )}
      </Stack>
      {trailing}
    </Box>
  );
}
