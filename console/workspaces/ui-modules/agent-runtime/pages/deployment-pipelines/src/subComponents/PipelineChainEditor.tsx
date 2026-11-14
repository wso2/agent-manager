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

import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Box,
  Chip,
  Form,
  IconButton,
  Menu,
  MenuItem,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { ArrowRight, PlusCircle } from "@wso2/oxygen-ui-icons-react";
import type { Environment } from "@agent-management-platform/types";
import { getIsolationTierMeta } from "@agent-management-platform/shared-component";
import { chainOptionsFor } from "../utils/chainUtils";

interface PipelineChainEditorProps {
  chain: string[];
  envOptions: Environment[];
  onChange: (chain: string[]) => void;
  disabled?: boolean;
  /** Shows the chain without add, change or remove controls. */
  readOnly?: boolean;
}

export function PipelineChainEditor(
  { chain, envOptions, onChange, disabled, readOnly = false }: PipelineChainEditorProps,
) {
  const [menuAnchor, setMenuAnchor] = useState<{ el: HTMLElement; index: number | "add" } | null>(null);

  const handleOpenMenu = useCallback((e: React.MouseEvent<HTMLElement>, index: number | "add") => {
    setMenuAnchor({ el: e.currentTarget, index });
  }, []);

  const handleMenuSelect = useCallback((envName: string) => {
    if (menuAnchor === null) return;
    if (menuAnchor.index === "add") {
      onChange([...chain, envName]);
    } else {
      const next = [...chain];
      next[menuAnchor.index] = envName;
      onChange(next);
    }
    setMenuAnchor(null);
  }, [menuAnchor, chain, onChange]);

  const handleRemove = useCallback((index: number) => {
    onChange(chain.filter((_, i) => i !== index));
  }, [chain, onChange]);

  const menuOptions = useMemo(
    () => (menuAnchor ? chainOptionsFor(envOptions, chain, menuAnchor.index) : []),
    [menuAnchor, envOptions, chain],
  );

  // Pre-select the first environment when the chain starts empty.
  useEffect(() => {
    if (chain.length === 0 && envOptions.length > 0) {
      onChange([envOptions[0].name]);
    }
  }, [envOptions, chain.length, onChange]);

  const canRemove = chain.length > 1;
  const canAddMore = chain.length < envOptions.length;

  return (
    <Form.Section>
      <Form.Header>Promotion Chain</Form.Header>

      <Box sx={{ overflowX: "auto", pb: 1, mt: 1.5 }}>
        <Stack direction="row" alignItems="center" sx={{ minWidth: "max-content" }} spacing={0.5}>
          {chain.map((envName, index) => {
            const env = envOptions.find((e) => e.name === envName);
            const tierMeta = getIsolationTierMeta(env?.isolationTier);
            const TierIcon = tierMeta.icon;
            return (
              <Stack key={index} direction="row" alignItems="center" spacing={0.5}>
                <Tooltip title={tierMeta.fullLabel}>
                  <Chip
                    icon={<TierIcon size={14} />}
                    label={<Typography>
                      <Typography variant="body2" component="span">{env?.displayName ?? envName}
                      </Typography>
                      <Typography component="span" variant="caption">{env?.isProduction ? " (Production)" : ""}
                      </Typography>
                    </Typography>}
                    variant="outlined"
                    onClick={readOnly
                      ? undefined
                      : (e) => handleOpenMenu(e as React.MouseEvent<HTMLElement>, index)}
                    onDelete={canRemove && !readOnly ? () => handleRemove(index) : undefined}
                    disabled={disabled}
                    sx={{ "& .MuiChip-icon": { color: tierMeta.iconColor } }}
                  />
                </Tooltip>
                {index < chain.length - 1 && <ArrowRight size={14} />}
              </Stack>
            );
          })}

          {canAddMore && !readOnly && (
            <Tooltip title="Add environment">
              <IconButton
                size="medium"
                onClick={(e) => handleOpenMenu(e, "add")}
                disabled={disabled}
              >
                <PlusCircle size={18} />
              </IconButton>
            </Tooltip>
          )}
        </Stack>
      </Box>

      <Menu
        anchorEl={menuAnchor?.el}
        open={Boolean(menuAnchor)}
        onClose={() => setMenuAnchor(null)}
      >
        {menuOptions.length === 0 ? (
          <MenuItem disabled>No environments available</MenuItem>
        ) : (
          menuOptions.map((env) => {
            const tierMeta = getIsolationTierMeta(env.isolationTier);
            const TierIcon = tierMeta.icon;
            return (
              <MenuItem key={env.name} onClick={() => handleMenuSelect(env.name)}>
                <Stack direction="row" spacing={1} alignItems="center">
                  <Box display="inline-flex" alignItems="center" sx={{ color: tierMeta.iconColor }}>
                    <TierIcon size={14} />
                  </Box>
                  <span>
                    {env.displayName ?? env.name} {env.isProduction ? "(Production)" : ""}
                  </span>
                </Stack>
              </MenuItem>
            );
          })
        )}
      </Menu>
    </Form.Section>
  );
}
