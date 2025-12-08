/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import React, { useState } from 'react';
import { IconButton, Menu, MenuItem } from '@wso2/oxygen-ui';
import { MoreVertical as MoreVert } from '@wso2/oxygen-ui-icons-react';

export interface ActionItem {
  label: string;
  value: string;
  onClick?: (row: any) => void;
}

export interface ActionMenuProps<T = any> {
  row: T;
  actions: ActionItem[];
  onActionClick: (action: string, row: T) => void;
}

export const ActionMenu = <T extends Record<string, any>>({
  row,
  actions,
  onActionClick,
}: ActionMenuProps<T>) => {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);

  const handleMenuOpen = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleActionClick = (action: ActionItem) => {
    handleMenuClose();
    onActionClick(action.value, row);
    action.onClick?.(row);
  };

  if (actions.length === 0) return null;

  return (
    <>
      <IconButton
        onClick={handleMenuOpen}
        size="small"
        aria-label="actions"
      >
        <MoreVert />
      </IconButton>
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'right',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
      >
        {actions.map((action) => (
          <MenuItem
            key={action.value}
            onClick={() => handleActionClick(action)}
          >
            {action.label}
          </MenuItem>
        ))}
      </Menu>
    </>
  );
};
