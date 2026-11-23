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
  PageContent,
  ColorSchemeToggle,
  UserMenu,
} from "@wso2/oxygen-ui";
import { generatePath, Outlet, useNavigate, useParams } from "react-router-dom";
import { LogOut, Settings } from "@wso2/oxygen-ui-icons-react";
import { ProfileDrawer } from "./ProfileDrawer";
import { useAuthHooks } from "@agent-management-platform/auth";
import { Logo, useExternalComponentModules } from "@agent-management-platform/views";
import { globalConfig, absoluteRouteMap } from "@agent-management-platform/types";
import { LeftNavigation, combineNavItems, flattenWithChildren } from "./LeftNavigation";
import { useNavigationItems } from "./navigationItems";
import { TopNavigation } from "./TopNavigation";
import { useFooterLinkModules } from "./useFooterLinkModules";
import { ConsoleAction, orgRouteId, useGetUserProfile, useListOrganizations, useTrack } from "@agent-management-platform/api-client";
import { MountPoints } from "../../types";
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
  const { track } = useTrack();
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
  const { links: externalFooterLinks, sources: footerLinkSources } = useFooterLinkModules();
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
          {/* The footer follows the page: at the bottom of a short page, after a long one.
              No padding here: each page lays out its own PageContent. */}
          <PageContent
            fullWidth
            noPadding
            footer={
              // The padding moves from the footer onto its content, so the divider runs
              // edge to edge and meets the sidebar's while the text keeps its inset.
              <Footer variant="plain" sx={{ px: 0, "& > *": { px: 3 } }}>
                <Footer.Copyright>
                  © {new Date().getFullYear()} Choreo Labs LLC.
                </Footer.Copyright>
                {
                  // Plain array, not a Fragment: Footer's React.Children.toArray()
                  // wouldn't unwrap it.
                  globalConfig.ampVersion && [
                    <Footer.Divider key="footer-divider" />,
                    <Footer.Version key="footer-version">
                      {`${globalConfig.ampVersion}`}
                    </Footer.Version>,
                  ]
                }
                {globalConfig.docsUrl && (
                  <Footer.Link
                    href={globalConfig.docsUrl + "/get-started/what-is-amp/"}
                    target="_blank"
                    rel="noopener noreferrer"
                    onClick={() => track(ConsoleAction.DocsLinkClick, { doc_target: "getting-started" })}
                  >
                    Documentation
                  </Footer.Link>
                )}
                {globalConfig.footerLinks?.supportUrl && (
                  <Footer.Link href={globalConfig.footerLinks.supportUrl} target="_blank" rel="noopener noreferrer">Support</Footer.Link>
                )}
                {externalFooterLinks}
                {globalConfig.footerLinks?.termsOfUseUrl && (
                  <Footer.Link href={globalConfig.footerLinks.termsOfUseUrl} target="_blank" rel="noopener noreferrer">Terms of Use</Footer.Link>
                )}
                {globalConfig.footerLinks?.privacyPolicyUrl && (
                  <Footer.Link href={globalConfig.footerLinks.privacyPolicyUrl} target="_blank" rel="noopener noreferrer">Privacy Policy</Footer.Link>
                )}
              </Footer>
            }
          >
            <Outlet />
          </PageContent>
        </AppShell.Main>
      </AppShell>

      <ProfileDrawer open={profileOpen} onClose={() => setProfileOpen(false)} />
      {/* Report the footer's extra links; mounted out here, as the footer would drop them. */}
      {footerLinkSources}
    </>
  );
}
