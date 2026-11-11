/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import { Avatar, Box, Stack, Typography } from "@wso2/oxygen-ui";
import { SelectionIndicator } from "@agent-management-platform/views";
import type { MCPProxyListItem } from "@agent-management-platform/types";
import { MCPLogo } from "@agent-management-platform/mcp-proxies";

interface MCPServerDisplayProps {
  server: MCPProxyListItem | null;
  isSelected: boolean;
  /** Show the MCP logo instead of the selection indicator (e.g. the selected card). */
  hideCheckbox?: boolean;
}

/**
 * Renders an MCP server's name, description, context and version. Shared by the
 * MCP-config create and view flows so the selection drawer items and the
 * selected-server card look identical.
 */
export function MCPServerDisplay({
  server,
  isSelected,
  hideCheckbox,
}: MCPServerDisplayProps) {
  if (!server) return null;
  const avatarSize = hideCheckbox ? 36 : 32;
  return (
    <Stack direction="row" spacing={2} flexGrow={1} alignItems="flex-start">
      {!hideCheckbox && <SelectionIndicator selected={isSelected} />}
      {hideCheckbox && (
        <Avatar
          sx={{
            height: avatarSize,
            width: avatarSize,
            backgroundColor: "action.selected",
          }}
        >
          <Box sx={{ color: "text.secondary", display: "inline-flex" }}>
            <MCPLogo size={20} />
          </Box>
        </Avatar>
      )}
      <Stack spacing={0.25} flexGrow={1} sx={{ minWidth: 0 }}>
        <Stack direction="row" alignItems="center" sx={{ minHeight: avatarSize }}>
          <Typography variant="h6">{server.name}</Typography>
        </Stack>
        {server.description && (
          <Typography variant="caption" color="text.secondary">
            {server.description}
          </Typography>
        )}
        <Stack direction="row" spacing={2} flexWrap="wrap">
          {server.context && (
            <Typography variant="caption" color="text.secondary">
              Context: {server.context}
            </Typography>
          )}
          {server.version && (
            <Typography variant="caption" color="text.secondary">
              Version: {server.version}
            </Typography>
          )}
        </Stack>
      </Stack>
    </Stack>
  );
}

export default MCPServerDisplay;
