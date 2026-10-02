/**
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

import React, { useState } from "react";
import {
  Checkbox,
  IconButton,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  Tooltip,
} from "@wso2/oxygen-ui";
import { Columns3 } from "@wso2/oxygen-ui-icons-react";
import { OPTIONAL_TRACE_COLUMNS, type TraceColumn } from "../traceColumns";

export interface TraceColumnsMenuProps {
  visibleColumns: TraceColumn[];
  // Shown and not hideable, with the reason as the item's hint.
  lockedColumns?: Partial<Record<TraceColumn, string>>;
  onChange: (columns: TraceColumn[]) => void;
}

// Column visibility control for the optional trace list columns.
export const TraceColumnsMenu: React.FC<TraceColumnsMenuProps> = ({
  visibleColumns,
  lockedColumns = {},
  onChange,
}) => {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const toggle = (key: TraceColumn) =>
    onChange(
      visibleColumns.includes(key)
        ? visibleColumns.filter((c) => c !== key)
        : [...visibleColumns, key],
    );

  return (
    <>
      <Tooltip title="Columns">
        <IconButton
          size="small"
          aria-label="Columns"
          aria-haspopup="menu"
          onClick={(e) => setAnchor(e.currentTarget)}
        >
          <Columns3 size={16} />
        </IconButton>
      </Tooltip>
      <Menu anchorEl={anchor} open={!!anchor} onClose={() => setAnchor(null)}>
        {OPTIONAL_TRACE_COLUMNS.map(({ key, label }) => {
          const lockedReason = lockedColumns[key];
          const checked = !!lockedReason || visibleColumns.includes(key);
          return (
            <MenuItem
              key={key}
              role="menuitemcheckbox"
              aria-checked={checked}
              disabled={!!lockedReason}
              onClick={() => toggle(key)}
            >
              <ListItemIcon>
                <Checkbox size="small" edge="start" checked={checked} tabIndex={-1} disableRipple />
              </ListItemIcon>
              <ListItemText primary={label} secondary={lockedReason} />
            </MenuItem>
          );
        })}
      </Menu>
    </>
  );
};
