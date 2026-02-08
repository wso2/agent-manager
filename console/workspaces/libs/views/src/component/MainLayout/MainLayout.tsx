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

import React, { useState, ReactNode } from 'react';
import { AppShell, Box, Footer } from '@wso2/oxygen-ui';
import {
  Sidebar,
  UserMenu,
  NavBarToolbar,
  User,
  NavigationSection as NavigationSectionType,
  UserMenuItem as UserMenuItemType,
  NavigationItem,
} from './subcomponents';
import { HeaderSelectProps } from './subcomponents/NavBarToolbar';

export interface MainLayoutProps {
  /** User information for the user menu */
  user?: User;
  /** Navigation items for mobile drawer */
  navigationItems?: Array<NavigationSectionType | NavigationItem>;
  /** User menu items */
  userMenuItems?: Array<UserMenuItemType>;
  /** Elements to display on the left side of the toolbar */
  leftElements?: ReactNode;
  /** Elements to display on the right side of the toolbar */
  rightElements?: ReactNode;
  /** Whether the sidebar is collapsed (icons only) */
  sidebarCollapsed?: boolean;
  /** Callback when sidebar collapse state changes */
  onSidebarToggle?: (collapsed: boolean) => void;
  /** Children to display inside the main content area */
  children?: ReactNode;
  /** Header select props */
  headerSelects?: HeaderSelectProps[];
  /** Home path */
  homePath?: string;
}

export function MainLayout({
  user,
  navigationItems = [],
  userMenuItems = [],
  leftElements,
  rightElements,
  sidebarCollapsed = true,
  onSidebarToggle,
  children,
  headerSelects,
  homePath,
}: MainLayoutProps) {
  const [userMenuAnchor, setUserMenuAnchor] = useState<null | HTMLElement>(
    null
  );
  const [sidebarOpen, setSidebarOpen] = useState(!sidebarCollapsed);

  const handleSidebarToggle = () => {
    const newState = !sidebarOpen;
    setSidebarOpen(newState);
    onSidebarToggle?.(!newState);
  };

  const handleUserMenuOpen = (event: React.MouseEvent<HTMLElement>) => {
    setUserMenuAnchor(event.currentTarget);
  };

  const handleUserMenuClose = () => {
    setUserMenuAnchor(null);
  };

  return (
    <>
      <Box sx={{ height: '100vh' }}>
        <AppShell>
        <AppShell.Navbar>
          <NavBarToolbar
            homePath={homePath}
            leftElements={leftElements}
            rightElements={rightElements}
            sidebarOpen={sidebarOpen}
            user={user}
            disableUserMenu={userMenuItems?.length === 0}
            onSidebarToggle={handleSidebarToggle}
            onUserMenuOpen={handleUserMenuOpen}
            headerSelects={headerSelects}
          />
        </AppShell.Navbar>
        <AppShell.Sidebar>
          <Sidebar
            onSidebarToggle={handleSidebarToggle}
            sidebarOpen={sidebarOpen}
            navigationSections={navigationItems}
            onNavigationClick={() => handleSidebarToggle()}
          />
        </AppShell.Sidebar>
        <AppShell.Main>
          <Box sx={{ height: 'calc(100vh - 72px)', overflowY: 'auto' }}>
            {children}
          </Box>
        </AppShell.Main>
        <AppShell.Footer>
          <Footer companyName="WSO2 LLC" />
        </AppShell.Footer>
        </AppShell>
      </Box>
      {user && (
        <UserMenu
          userMenuItems={userMenuItems}
          anchorEl={userMenuAnchor}
          open={Boolean(userMenuAnchor)}
          onClose={handleUserMenuClose}
        />
      )}
    </>
  );
}
