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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  Box,
  Chip,
  Divider,
  IconButton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Edit, Lock, Trash2 } from "@wso2/oxygen-ui-icons-react";
import type { EndpointDraft } from "./EndpointFormFields";

interface EndpointRowProps {
  endpoint: EndpointDraft;
  environmentLabels: Map<string, string>;
  onRemove: () => void;
  onEdit?: () => void;
}

export function EndpointRow({
  endpoint,
  environmentLabels,
  onRemove,
  onEdit,
}: EndpointRowProps) {
  const toolCount = endpoint.fetchedInfo.tools?.length ?? 0;
  const resourceCount = endpoint.fetchedInfo.resources?.length ?? 0;
  const promptCount = endpoint.fetchedInfo.prompts?.length ?? 0;
  const hasAuth = Boolean(endpoint.authHeader);

  return (
    <Box
      sx={{
        border: "1px solid",
        borderColor: "divider",
        borderRadius: 1,
        p: 2,
      }}
    >
      <Stack
        direction="row"
        spacing={2}
        alignItems="flex-start"
        justifyContent="space-between"
      >
        <Stack spacing={1} sx={{ minWidth: 0, flex: 1 }}>
          <Stack
            direction="row"
            spacing={1}
            alignItems="center"
            flexWrap="wrap"
          >
            <Typography
              variant="subtitle2"
              fontWeight={600}
              color={endpoint.name ? "text.primary" : "text.secondary"}
              sx={endpoint.name ? undefined : { fontStyle: "italic" }}
            >
              {endpoint.name || "Unnamed endpoint"}
            </Typography>
            {endpoint.environments.map((envId) => (
              <Chip
                key={envId}
                label={environmentLabels.get(envId) || envId}
                size="small"
                color="primary"
                variant="outlined"
              />
            ))}
            {hasAuth ? (
              <Tooltip title={`Authenticated via ${endpoint.authHeader}`}>
                <Chip
                  icon={<Lock size={12} />}
                  label="Auth"
                  size="small"
                  variant="outlined"
                />
              </Tooltip>
            ) : null}
          </Stack>

          <Typography
            variant="body2"
            sx={{ wordBreak: "break-all", fontFamily: "monospace" }}
          >
            {endpoint.url}
          </Typography>

          <Divider flexItem />

          <Stack direction="row" spacing={2} alignItems="center">
            {endpoint.serverName ? (
              <Typography variant="caption" color="text.secondary">
                {endpoint.serverName}
                {endpoint.serverVersion ? ` · ${endpoint.serverVersion}` : ""}
              </Typography>
            ) : null}
            <Typography variant="caption" color="text.secondary">
              {toolCount} tools · {resourceCount} resources · {promptCount}{" "}
              prompts
            </Typography>
          </Stack>
        </Stack>

        <Stack direction="row" spacing={0.5}>
          {onEdit ? (
            <Tooltip title="Edit endpoint">
              <IconButton
                size="small"
                onClick={onEdit}
                aria-label="Edit endpoint"
              >
                <Edit size={16} />
              </IconButton>
            </Tooltip>
          ) : null}
          <Tooltip title="Remove endpoint">
            <IconButton
              size="small"
              onClick={onRemove}
              aria-label="Remove endpoint"
            >
              <Trash2 size={16} />
            </IconButton>
          </Tooltip>
        </Stack>
      </Stack>
    </Box>
  );
}

export default EndpointRow;
