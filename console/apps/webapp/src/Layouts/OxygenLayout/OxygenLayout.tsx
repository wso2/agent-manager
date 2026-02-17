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

import { useMemo, useState } from "react";
import {
  AppShell,
  Header,
  Footer,
  ColorSchemeToggle,
  UserMenu,
} from "@wso2/oxygen-ui";
import { generatePath, Outlet, useNavigate } from "react-router-dom";
import { useAuthHooks } from "@agent-management-platform/auth";
import { Logo } from "@agent-management-platform/views";
// import { UserMenu } from "./UserMenu";
import { LogOutIcon } from "@wso2/oxygen-ui-icons-react";
import { LeftNavigation, type NavigationItem, type NavigationSection } from "./LeftNavigation";
import { useNavigationItems } from "./navigationItems";
import { TopNavigation } from "./TopNavigation";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { useListOrganizations } from "@agent-management-platform/api-client";

const getFlattenedItems = (
  mainItems: NavigationItem[],
  groupedItems: NavigationSection[],
) => {
  return [...mainItems, ...groupedItems.flatMap((item) => item.items)];
};
const getNavItemByKey = (
  mainItems: NavigationItem[],
  groupedItems: NavigationSection[],
  key: string,
) => {
  const flattenedItems = getFlattenedItems(mainItems, groupedItems);
  return flattenedItems.find((item) => item.label === key);
};

export function OxygenLayout() {
  const [collapsed, setCollapsed] = useState(false);
  const { userInfo, logout } = useAuthHooks();
  const navigate = useNavigate();

  const { data: organizations } = useListOrganizations();
  const homePath = useMemo(() => {
    return generatePath(absoluteRouteMap.children.org.path, {
      orgId: organizations?.organizations?.[0]?.name ?? "",
    });
  }, [organizations]);

  const user = {
    primaryLine: userInfo?.givenName ?? userInfo?.username ?? "User",
    secondaryLine: userInfo?.orgName ?? userInfo?.familyName ?? userInfo?.username ?? "Organization",
  };

  const navigationItems = useNavigationItems();
  const mainItems = navigationItems.filter((item) => item.type === "item");
  const groupedItems = navigationItems.filter(
    (item) => item.type === "section",
  );

  const activeItem = useMemo(() => {
    const flattenedItems = getFlattenedItems(mainItems, groupedItems);
    return flattenedItems.find((item) => item.isActive)?.label ?? "";
  }, [mainItems, groupedItems]);

  const handleLogout = async () => {
    await logout();
  };

  const handleNavigationClick = (itemKey: string) => {
    const item = getNavItemByKey(mainItems, groupedItems, itemKey);
    if (item?.href) {
      navigate(item.href);
    }
  };

  return (
    <AppShell>
      <AppShell.Navbar>
        <Header>
          <Header.Toggle
            collapsed={collapsed}
            onToggle={() => setCollapsed(!collapsed)}
          />
          <Header.Brand onClick={() => navigate(homePath)}>
            <Header.BrandLogo>
              <Logo width={180} />
            </Header.BrandLogo>
          </Header.Brand>
          <TopNavigation />
          <Header.Spacer />
          <Header.Actions>
            <ColorSchemeToggle />
            <UserMenu>
              <UserMenu.Trigger name={user.name} />
              <UserMenu.Header name={user.name} email={user.email} />
              <UserMenu.Divider />
              <UserMenu.Logout  onClick={handleLogout} />
            </UserMenu>
          </Header.Actions>
        </Header>
      </AppShell.Navbar>

      <AppShell.Sidebar>
        <LeftNavigation
          collapsed={collapsed}
          activeItem={activeItem}
          mainItems={mainItems}
          groupedItems={groupedItems}
          onNavigationClick={handleNavigationClick}
        />
      </AppShell.Sidebar>

      <AppShell.Main>
        <Outlet />
      </AppShell.Main>

      <AppShell.Footer>
        <Footer>
          <Footer.Copyright>
            © {new Date().getFullYear()} WSO2 LLC. All rights reserved.
          </Footer.Copyright>
          <Footer.Link href="#terms">Terms & Conditions</Footer.Link>
          <Footer.Link href="#privacy">Privacy Policy</Footer.Link>
        </Footer>

      </AppShell.Footer>
    </AppShell>
  );
}
