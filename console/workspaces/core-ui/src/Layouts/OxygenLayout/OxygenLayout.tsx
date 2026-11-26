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
  Box,
  Header,
  Footer,
  ColorSchemeToggle,
  UserMenu,
} from "@wso2/oxygen-ui";
import { generatePath, Outlet, useNavigate, useParams } from "react-router-dom";
import { Boxes, LogOut, Settings } from "@wso2/oxygen-ui-icons-react";
import { ProfileDrawer } from "./ProfileDrawer";
import { useAuthHooks } from "@agent-management-platform/auth";
import { Logo, PageFooterProvider, useExternalComponentModules } from "@agent-management-platform/views";
import { globalConfig, absoluteRouteMap } from "@agent-management-platform/types";
import { LeftNavigation, combineNavItems, flattenWithChildren } from "./LeftNavigation";
import { useNavigationItems } from "./navigationItems";
import { TopNavigation } from "./TopNavigation";
import { orgRouteId, useGetUserProfile, useListOrganizations } from "@agent-management-platform/api-client";
import { MountPoints } from "../../types";
import { isMarketplaceEnabled, MODULE_MANAGER_ROUTE } from "../../pages/ModuleManager";
import { useFeatureFlag } from "@agent-management-platform/feature-management";

// Oxygen UI's UserMenu treats a non-URL `avatar` string as literal initials
// text (see getUserMenuAvatarProps), so this doubles as the no-picture
// fallback: first letter of each of the first two words, e.g. "John Doe" -> "JD".
function getUserAvatarInitials(fullName: string): string {
  const words = fullName.trim().split(/\s+/).filter(Boolean);
  const initials = words.slice(0, 2).map((word) => word.charAt(0)).join("");
  return (initials || "U").toUpperCase();
}

export function OxygenLayout() {
  const [collapsed, setCollapsed] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);
  const { userInfo, logout } = useAuthHooks();
  const navigate = useNavigate();
  const { orgId } = useParams();
  const profileManagementEnabled = useFeatureFlag("enableProfileManagement");
  const { data: userProfile } = useGetUserProfile({
    orgName: orgId || "default",
    userId: userInfo?.sub || "",
  });

  const externalTopRightComponentModules =
    useExternalComponentModules(MountPoints.TopRightPanel);
  const externalLogoComponentModules = useExternalComponentModules(MountPoints.TopLogo);
  const externalTopStartComponentModules =
    useExternalComponentModules(MountPoints.TopStartPanel);
  const externalTopLeftComponentModules =
    useExternalComponentModules(MountPoints.TopLeftPanel);
  const externalBottomLeftComponentModules =
    useExternalComponentModules(MountPoints.BottomLeftPanel);
  const externalBottomRightComponentModules =
    useExternalComponentModules(MountPoints.BottomRightPanel);
  const { data: organizations } = useListOrganizations();
  const homePath = useMemo(() => {
    return generatePath(absoluteRouteMap.children.org.path, {
      orgId: orgRouteId(organizations?.organizations?.[0]) ?? "",
    });
  }, [organizations]);

  const fullName = [userInfo?.givenName, userInfo?.familyName]
    .filter(Boolean)
    .join(" ")
    .trim();

  const user = {
    primaryLine: fullName || userInfo?.username || "User",
    secondaryLine: userInfo?.orgName ?? userInfo?.email ?? userInfo?.username ?? userInfo?.givenName ?? "",
  };

  // UserMenu takes an image URL or initials as a string; profile attributes are untyped.
  const picture = userProfile?.attributes?.picture;
  const avatar =
    (typeof picture === "string" && picture) || getUserAvatarInitials(user.primaryLine);

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

  // Rendered inside each page's PageContent (see PageLayout) rather than as an
  // AppShell footer, so it sits at the bottom of short pages and scrolls with
  // long ones.
  // The row is as tall as the sidebar's footer (1px divider + 8px padding +
  // 44px item + 8px padding), so its top line lines up with the divider above
  // "Collapse". The line is inset by the page padding, as in the Choreo console.
  const footer = (
    <Box component="footer" sx={{ px: 3, width: "100%" }}>
    <Box
      sx={(theme) => ({
        borderTop: `1px solid ${(theme.vars || theme).palette.divider}`,
        boxSizing: "border-box",
        minHeight: 61,
        display: "flex",
        flexDirection: { xs: "column", sm: "row" },
        alignItems: { xs: "flex-start", sm: "center" },
        justifyContent: "space-between",
        gap: 2,
        py: { xs: 1.5, sm: 0 },
      })}
    >
      <Box display="flex" alignItems="center" gap={2} flexWrap="wrap">
      <Footer.Copyright>
        © {new Date().getFullYear()} WSO2 LLC.
      </Footer.Copyright>
      {
        globalConfig.ampVersion && [
          <Footer.Divider key="footer-divider" />,
          <Footer.Version key="footer-version">
            {`${globalConfig.ampVersion}`}
          </Footer.Version>,
        ]
      }
      {
        externalBottomLeftComponentModules?.map((module) => (
          <div key={module.moduleName}>
            <module.component />
          </div>
        ))
      }
      </Box>
      <Box display="flex" alignItems="center" gap={2} flexWrap="wrap">
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
      {globalConfig.footerLinks?.supportUrl && (
        <Footer.Link href={globalConfig.footerLinks.supportUrl} target="_blank" rel="noopener noreferrer">Support</Footer.Link>
      )}
      {globalConfig.footerLinks?.termsOfUseUrl && (
        <Footer.Link href={globalConfig.footerLinks.termsOfUseUrl} target="_blank" rel="noopener noreferrer">Terms of Use</Footer.Link>
      )}
      {globalConfig.footerLinks?.privacyPolicyUrl && (
        <Footer.Link href={globalConfig.footerLinks.privacyPolicyUrl} target="_blank" rel="noopener noreferrer">Privacy Policy</Footer.Link>
      )}
      </Box>
    </Box>
    </Box>
  );

  return (
    <>
      <AppShell>
        <AppShell.Navbar>
          <Header>
            {externalTopStartComponentModules?.map((module) => (
              <div key={module.moduleName}>
                <module.component />
              </div>
            ))}
            <Header.Brand onClick={() => navigate(homePath)}>
              <Header.BrandLogo>
                {externalLogoComponentModules && externalLogoComponentModules.length > 0 ? (
                  <div style={{ display: "flex", flexDirection: "row", alignItems: "center", gap: 8 }}>
                    {externalLogoComponentModules.map((module) => (
                      <module.component key={module.moduleName} />
                    ))}
                  </div>
                ) : (
                  <Logo />
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
                <UserMenu.Trigger
                  name={user.primaryLine}
                  avatar={avatar}
                />
                <UserMenu.Header
                  name={user.primaryLine}
                  email={user.secondaryLine}
                  avatar={avatar}
                />
                <UserMenu.Divider />
                {orgId && profileManagementEnabled && (
                  <UserMenu.Item
                    icon={<Settings />}
                    label="Profile settings"
                    onClick={() => setProfileOpen(true)}
                  />
                )}
                {orgId && isMarketplaceEnabled() && (
                  <UserMenu.Item
                    icon={<Boxes />}
                    label="AI module manager"
                    onClick={() => navigate(generatePath(MODULE_MANAGER_ROUTE, { orgId }))}
                  />
                )}
                <UserMenu.Logout icon={<LogOut />} label="Sign out" onClick={handleLogout} />
              </UserMenu>
            </Header.Actions>
          </Header>
        </AppShell.Navbar>

        <AppShell.Sidebar>
          <LeftNavigation
            collapsed={collapsed}
            onToggleCollapse={() => setCollapsed((prev) => !prev)}
            activeItem={activeItem}
            mainItems={mainItems}
            groupedItems={groupedItems}
          />
        </AppShell.Sidebar>

        <AppShell.Main>
          <PageFooterProvider value={footer}>
            <Outlet />
          </PageFooterProvider>
        </AppShell.Main>

      </AppShell>

      <ProfileDrawer open={profileOpen} onClose={() => setProfileOpen(false)} />
    </>
  );
}
