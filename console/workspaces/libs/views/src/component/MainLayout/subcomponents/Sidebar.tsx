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

import { ReactNode, useMemo, useState } from 'react';
import {
  Box,
  Link,
  Sidebar as OxygenSidebar,
  Skeleton,
  useTheme,
} from '@wso2/oxygen-ui';
import { Link as RouterLink, useNavigate } from 'react-router-dom';
import {
  ChevronLeft as ChevronLeftOutlined,
  ChevronRight as ChevronRightOutlined,
} from '@wso2/oxygen-ui-icons-react';
export interface NavigationItem {
  label: string;
  icon?: ReactNode;
  onClick?: () => void;
  href?: string;
  isActive?: boolean;
  type: 'item';
}
export interface NavigationSection {
  title: string;
  items: Array<NavigationItem>;
  icon?: ReactNode;
  type: 'section';
}

export interface SidebarProps {
  /** Whether the sidebar is collapsed (icons only) */
  sidebarOpen?: boolean;
  /** Callback when sidebar is toggled */
  onSidebarToggle?: () => void;
  /** Navigation sections with optional titles */
  navigationSections: Array<NavigationSection | NavigationItem>;
  /** Whether this is mobile view */
  isMobile?: boolean;
  /** Callback when navigation item is clicked */
  onNavigationClick?: () => void;
}

const SIDEBAR_WIDTH = 240;
const COLLAPSED_SIDEBAR_WIDTH = 64;
const COLLAPSE_TOGGLE_ID = '__sidebar_toggle__';

export function Sidebar({
  sidebarOpen = true,
  onSidebarToggle,
  navigationSections,
  isMobile = false,
  onNavigationClick,
}: SidebarProps) {
  const theme = useTheme();
  const navigate = useNavigate();
  const [expandedMenus, setExpandedMenus] = useState<Record<string, boolean>>(
    {}
  );

  const flatItems = useMemo(
    () =>
      navigationSections.flatMap((navItem) =>
        navItem.type === 'section' ? navItem.items : [navItem]
      ),
    [navigationSections]
  );

  const activeItemId = useMemo(
    () => flatItems.find((item) => item.isActive)?.label,
    [flatItems]
  );

  const handleToggleExpand = (id: string) => {
    setExpandedMenus((prev) => ({
      ...prev,
      [id]: !prev[id],
    }));
  };

  const handleSelect = (id: string) => {
    if (id === COLLAPSE_TOGGLE_ID) {
      onSidebarToggle?.();
      return;
    }
    const targetItem = flatItems.find((item) => item.label === id);
    if (!targetItem) {
      return;
    }
    targetItem.onClick?.();
    if (targetItem.href) {
      navigate(targetItem.href);
    }
    if (isMobile) {
      onNavigationClick?.();
    }
  };

  return (
    <OxygenSidebar
      collapsed={!sidebarOpen}
      activeItem={activeItemId}
      expandedMenus={expandedMenus}
      onSelect={handleSelect}
      onToggleExpand={handleToggleExpand}
      width={SIDEBAR_WIDTH}
      collapsedWidth={COLLAPSED_SIDEBAR_WIDTH}
      sx={{
        pt: 1,
        px: 1,
        borderRight: 1,
        borderColor: 'divider',
        backgroundColor: 'background.paper',
        transition: theme.transitions.create('all', {
          duration: theme.transitions.duration.short,
        }),
      }}
    >
      <OxygenSidebar.Nav>
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'column',
            width: '100%',
            gap: 1,
          }}
        >
          {navigationSections.length === 0 && (
            <Box display="flex" flexDirection="column" gap={1}>
              <Skeleton variant="rounded" height={44} width="100%" />
              <Skeleton variant="rounded" height={44} width="100%" />
              <Skeleton variant="rounded" height={44} width="100%" />
            </Box>
          )}
          {navigationSections.map((navItem) =>
            navItem.type === 'section' ? (
              <OxygenSidebar.Category key={navItem.title}>
                <OxygenSidebar.CategoryLabel>
                  {navItem.title}
                </OxygenSidebar.CategoryLabel>
                {navItem.items.map((item) => (
                  <Link
                    key={item.label}
                    component={RouterLink}
                    to={item.href ?? ''}
                    sx={{ textDecoration: 'none' }}
                  >
                    <OxygenSidebar.Item id={item.label}>
                      {item.icon && (
                        <OxygenSidebar.ItemIcon>
                          {item.icon}
                        </OxygenSidebar.ItemIcon>
                      )}
                      <OxygenSidebar.ItemLabel>
                        {item.label}
                      </OxygenSidebar.ItemLabel>
                    </OxygenSidebar.Item>
                  </Link>
                ))}
              </OxygenSidebar.Category>
            ) : (
              <Link
                key={navItem.label}
                component={RouterLink}
                to={navItem.href ?? ''}
                sx={{ textDecoration: 'none' }}
              >
                <OxygenSidebar.Item id={navItem.label}>
                  {navItem.icon && (
                    <OxygenSidebar.ItemIcon>
                      {navItem.icon}
                    </OxygenSidebar.ItemIcon>
                  )}
                  <OxygenSidebar.ItemLabel>
                    {navItem.label}
                  </OxygenSidebar.ItemLabel>
                </OxygenSidebar.Item>
              </Link>
            )
          )}
        </Box>
      </OxygenSidebar.Nav>
      <OxygenSidebar.Footer>
        <Box sx={{ width: '100%', display: 'flex', flexDirection: 'column' }}>
          <OxygenSidebar.Item id={COLLAPSE_TOGGLE_ID}>
            <OxygenSidebar.ItemIcon>
              {sidebarOpen ? (
                <ChevronLeftOutlined fontSize="medium" />
              ) : (
                <ChevronRightOutlined fontSize="small" />
              )}
            </OxygenSidebar.ItemIcon>
            <OxygenSidebar.ItemLabel>
              {sidebarOpen ? 'Collapse' : 'Expand'}
            </OxygenSidebar.ItemLabel>
          </OxygenSidebar.Item>
        </Box>
      </OxygenSidebar.Footer>
    </OxygenSidebar>
  );
}
