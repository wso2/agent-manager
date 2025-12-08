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

import {
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Tooltip,
} from '@wso2/oxygen-ui';
import { Link as RouterLink } from 'react-router-dom';
import { NavigationItem } from './Sidebar';

export interface NavigationItemButtonProps {
  item: NavigationItem;
  sidebarOpen: boolean;
  isMobile?: boolean;
  onNavigationClick?: () => void;
  subButton?: boolean;
}

export function NavigationItemButton({
  item,
  sidebarOpen,
  isMobile = false,
  onNavigationClick,
  subButton = false,
}: NavigationItemButtonProps) {
  return (
    <ListItem disablePadding>
      <Tooltip
        title={item.label}
        placement="right"
        disableHoverListener={sidebarOpen}
      >
        <ListItemButton
          onClick={() => {
            if ('onClick' in item && item.onClick) {
              item.onClick();
            }
            if (isMobile) {
              onNavigationClick?.();
            }
          }}
          component={item.href ? RouterLink : 'div'}
          to={item.href ?? ''}
          selected={item.isActive}
          sx={{
            pl: subButton && sidebarOpen ? 5 : 1.375,
            height: 44,
          }}
        >
          {item.icon && <ListItemIcon sx={{ minWidth: 40 }}>{item.icon}</ListItemIcon>}
          {sidebarOpen && (
            <ListItemText sx={{ textWrap: 'nowrap' }} primary={item.label} />
          )}
        </ListItemButton>
      </Tooltip>
    </ListItem>
  );
}
