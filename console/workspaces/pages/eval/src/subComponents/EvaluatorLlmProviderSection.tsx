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

import { absoluteRouteMap } from "@agent-management-platform/types";
import { useListLLMProviders } from "@agent-management-platform/api-client";
import {
  CircularProgress,
  Divider,
  FormControl,
  InputLabel,
  ListItemIcon,
  MenuItem,
  Select,
  Typography,
} from "@wso2/oxygen-ui";
import { ExternalLink, Plus } from "@wso2/oxygen-ui-icons-react";
import { useState } from "react";
import { generatePath, useParams } from "react-router-dom";

const NONE_VALUE = "__none__";

interface EvaluatorLlmProviderSectionProps {
  /** The provider handle (id) currently selected */
  selectedProviderName?: string;
  onProviderChange: (name: string | undefined) => void;
}

export function EvaluatorLlmProviderSection({
  selectedProviderName,
  onProviderChange,
}: EvaluatorLlmProviderSectionProps) {
  const { orgId } = useParams<{ orgId: string }>();

  const [open, setOpen] = useState(false);
  const { data, isFetching, refetch } = useListLLMProviders({ orgName: orgId });
  const availableProviders = data?.providers ?? [];

  const addProviderPath = orgId
    ? generatePath(
        absoluteRouteMap.children.org.children.llmProviders.children.add.path,
        { orgId },
      )
    : null;

  return (
    <FormControl fullWidth size="small">
      <InputLabel id="llm-provider-label">LLM Provider</InputLabel>
      <Select
        labelId="llm-provider-label"
        label="LLM Provider"
        open={open}
        onOpen={() => { setOpen(true); refetch(); }}
        onClose={() => setOpen(false)}
        value={selectedProviderName ?? NONE_VALUE}
        onChange={(e) => {
          const value = e.target.value as string;
          onProviderChange(value === NONE_VALUE ? undefined : value);
        }}
        endAdornment={
          isFetching ? <CircularProgress size={14} sx={{ mr: 3 }} /> : undefined
        }
        renderValue={(val) => {
          if (val === NONE_VALUE) {
            return (
              <Typography variant="body2" color="text.secondary">
                None
              </Typography>
            );
          }
          const displayName =
            availableProviders.find((p) => p.id === val)?.name ?? (val as string);
          return <Typography variant="body2">{displayName}</Typography>;
        }}
      >
        <MenuItem value={NONE_VALUE}>
          <Typography variant="body2" color="text.secondary">
            — None —
          </Typography>
        </MenuItem>

        {availableProviders.map((provider) => (
          <MenuItem key={provider.id} value={provider.id}>
            {provider.name}
          </MenuItem>
        ))}

        {addProviderPath && (
          <>
            <Divider />
            <MenuItem
              onClick={() => {
                setOpen(false);
                window.open(addProviderPath, "_blank", "noopener,noreferrer");
              }}
            >
              <ListItemIcon>
                <Plus size={16} />
              </ListItemIcon>
              <Typography variant="body2" color="primary">
                Add LLM Provider
              </Typography>
              <ListItemIcon sx={{ ml: "auto", minWidth: "unset" }}>
                <ExternalLink size={14} />
              </ListItemIcon>
            </MenuItem>
          </>
        )}
      </Select>

      {!isFetching && availableProviders.length === 0 && (
        <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>
          No providers configured yet. Use the option above to add one.
        </Typography>
      )}
    </FormControl>
  );
}
