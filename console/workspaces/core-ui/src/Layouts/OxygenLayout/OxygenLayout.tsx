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
import { generatePath, Outlet, useNavigate, useParams } from "react-router-dom";
import { Settings } from "@wso2/oxygen-ui-icons-react";
import { ProfileDrawer } from "./ProfileDrawer";
import { useAuthHooks } from "@agent-management-platform/auth";
import { Logo, useExternalComponentModules } from "@agent-management-platform/views";
import { globalConfig, absoluteRouteMap } from "@agent-management-platform/types";
import { LeftNavigation, combineNavItems, flattenWithChildren } from "./LeftNavigation";
import { useNavigationItems } from "./navigationItems";
import { TopNavigation } from "./TopNavigation";
import { useListOrganizations } from "@agent-management-platform/api-client";
import { MountPoints } from "../../types";

export function OxygenLayout() {
  const [collapsed, setCollapsed] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);
  const { userInfo, logout } = useAuthHooks();
  const navigate = useNavigate();
  const { orgId } = useParams();

  const externalTopRightComponentModules =
    useExternalComponentModules(MountPoints.TopRightPanel);
  const externalLogoComponentModules = useExternalComponentModules(MountPoints.TopLogo);
  const externalTopLeftComponentModules =
    useExternalComponentModules(MountPoints.TopLeftPanel);
  const externalBottomLeftComponentModules =
    useExternalComponentModules(MountPoints.BottomLeftPanel);
  const externalBottomRightComponentModules =
    useExternalComponentModules(MountPoints.BottomRightPanel);
  const { data: organizations } = useListOrganizations();
  const homePath = useMemo(() => {
    return generatePath(absoluteRouteMap.children.org.path, {
      orgId: organizations?.organizations?.[0]?.name ?? "",
    });
  }, [organizations]);

  const user = {
    primaryLine: userInfo?.givenName ?? userInfo?.username ?? "User",
    secondaryLine: userInfo?.orgName ?? userInfo?.email ?? userInfo?.username ?? userInfo?.givenName ?? "",
  };

  const navigationItems = useNavigationItems();
  const mainItems = navigationItems.filter((item) => item.type === "item");
  const groupedItems = navigationItems.filter(
    (item) => item.type === "section",
  );

  const activeItem = useMemo(() => {
    const flattenedItems = flattenWithChildren(combineNavItems(mainItems, groupedItems));
    return flattenedItems.find((item) => item.isActive)?.label ?? "";
  }, [mainItems, groupedItems]);

  const handleLogout = async () => {
    await logout();
  };

  return (
    <>
      <AppShell>
        <AppShell.Navbar>
          <Header>
            <Header.Toggle
              collapsed={collapsed}
              onToggle={() => setCollapsed(!collapsed)}
            />
            <Header.Brand onClick={() => navigate(homePath)}>
              <Header.BrandLogo>
                {externalLogoComponentModules && externalLogoComponentModules.length > 0 ? (
                  <div style={{ display: "flex", flexDirection: "row", alignItems: "center", gap: 8 }}>
                    {externalLogoComponentModules.map((module) => (
                      <module.component key={module.moduleName} />
                    ))}
                  </div>
                ) : (
                  <Logo width={192} />
                )}
              </Header.BrandLogo>
            </Header.Brand>
            <TopNavigation />
            {
              externalTopLeftComponentModules?.map((module) => (
                <div key={module.moduleName}>
                  <module.component />
                </div>
              ))
            }
            <Header.Spacer />
            {externalTopRightComponentModules?.map((module) => (
              <div key={module.moduleName}>
                <module.component />
              </div>
            ))
            }
            <Header.Actions>
              <ColorSchemeToggle />
              <UserMenu>
                <UserMenu.Trigger name={user.primaryLine} />
                <UserMenu.Header name={user.primaryLine} email={user.secondaryLine} />
                <UserMenu.Divider />
                {orgId && (
                  <UserMenu.Item
                    icon={<Settings />}
                    label="Profile Settings"
                    onClick={() => setProfileOpen(true)}
                  />
                )}
                <UserMenu.Logout onClick={handleLogout} />
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
          />
        </AppShell.Sidebar>

        <AppShell.Main>
          <Outlet />
        </AppShell.Main>

        <AppShell.Footer>
          <Footer>

            <Footer.Copyright>
              © {new Date().getFullYear()} WSO2 LLC.
            </Footer.Copyright>
            <Footer.Divider />
            {
              globalConfig.ampVersion && (
                <Footer.Version>
                  {
                    (
                      `${globalConfig.ampVersion}`
                    )
                  }
                </Footer.Version>
              )
            }
            {
              externalBottomLeftComponentModules?.map((module) => (
                <div key={module.moduleName}>
                  <module.component />
                </div>
              ))
            }
            {
              externalBottomRightComponentModules?.map((module) => (
                <div key={module.moduleName}>
                  <module.component />
                </div>
              ))
            }

            {globalConfig.docsUrl && (
              <Footer.Link href={globalConfig.docsUrl + "/get-started/what-is-amp/"} target="_blank" rel="noopener noreferrer">Documentation</Footer.Link>
            )}
            {globalConfig.footerLinks?.termsOfUseUrl && (
              <Footer.Link href={globalConfig.footerLinks.termsOfUseUrl} target="_blank" rel="noopener noreferrer">Terms & Conditions</Footer.Link>
            )}
            {globalConfig.footerLinks?.privacyPolicyUrl && (
              <Footer.Link href={globalConfig.footerLinks.privacyPolicyUrl} target="_blank" rel="noopener noreferrer">Privacy Policy</Footer.Link>
            )}
          </Footer>
        </AppShell.Footer>
      </AppShell>

      <ProfileDrawer open={profileOpen} onClose={() => setProfileOpen(false)} />
    </>
  );
}
