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

import React, { type ReactNode } from "react";
import {
  Box,
  Card,
  Divider,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  ListSubheader,
  Typography,
} from "@wso2/oxygen-ui";
import { Folder, Shield, Users } from "@wso2/oxygen-ui-icons-react";
import {
  generatePath,
  matchPath,
  useLocation,
  useParams,
  Link,
} from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import { settingsRoute, useIdentityVisibility } from "./settingsRoutes";

interface SubNavItem {
  label: string;
  href: string;
  wildPath: string;
  icon: ReactNode;
}

interface SubNavSection {
  title: string;
  items: SubNavItem[];
}

const ICON_SX_ACTIVE = { minWidth: 36, color: "primary.main" } as const;
const ICON_SX_INACTIVE = { minWidth: 36, color: "inherit" } as const;
const TEXT_SX_ACTIVE = { color: "text.secondary" } as const;
const TEXT_SX_INACTIVE = { color: "inherit" } as const;

export const SettingsLayout: React.FC<{ children: ReactNode }> = ({
  children,
}) => {
  const { orgId } = useParams<{ orgId: string }>();
  const { pathname } = useLocation();
  const identityVisibility = useIdentityVisibility();

  const identityNode = settingsRoute.children.identities.children;

  const userMgmtItems: SubNavItem[] = [];
  if (identityVisibility.users) {
    userMgmtItems.push({
      label: "Users",
      href: generatePath(identityNode.users.path, { orgId }),
      wildPath: identityNode.users.wildPath,
      icon: <Users size={18} />,
    });
  }
  if (identityVisibility.groups) {
    userMgmtItems.push({
      label: "Groups",
      href: generatePath(identityNode.groups.path, { orgId }),
      wildPath: identityNode.groups.wildPath,
      icon: <Folder size={18} />,
    });
  }
  if (identityVisibility.roles) {
    userMgmtItems.push({
      label: "Roles",
      href: generatePath(identityNode.roles.path, { orgId }),
      wildPath: identityNode.roles.wildPath,
      icon: <Shield size={18} />,
    });
  }

  const sections: SubNavSection[] = userMgmtItems.length > 0
    ? [{ title: "User Management", items: userMgmtItems }]
    : [];

  return (
    <PageLayout docs={["authorization", "organization"]} title="Settings" disableIcon>
      <Card sx={{ display: "flex", overflow: "hidden", minHeight: "calc(70vh - 64px)" }}>
        <Box
          component="nav"
          sx={{ width: 200, flexShrink: 0, overflowY: "auto", p: 2 }}
        >
          {sections.map((section) => (
            <List
              key={section.title}
              subheader={
                <ListSubheader disableGutters disableSticky>
                  <Typography variant="overline">
                    {section.title}
                  </Typography>
                </ListSubheader>
              }
            >
              {section.items.map((item) => {
                const isActive = !!matchPath(item.wildPath, pathname);
                return (
                  <Link
                    key={item.label}
                    to={item.href}
                    style={{ textDecoration: "none", color: "inherit" }}
                  >
                    <ListItemButton selected={isActive}>
                      <ListItemIcon sx={isActive ? ICON_SX_ACTIVE : ICON_SX_INACTIVE}>
                        {item.icon}
                      </ListItemIcon>
                      <ListItemText
                        primary={item.label}
                        sx={isActive ? TEXT_SX_ACTIVE : TEXT_SX_INACTIVE}
                      />
                    </ListItemButton>
                  </Link>
                );
              })}
            </List>
          ))}
        </Box>
        <Divider orientation="vertical" flexItem />
        <Box sx={{ flex: 1, minWidth: 0, overflowY: "auto", p: 3 }}>
          {children}
        </Box>
      </Card>
    </PageLayout>
  );
};

export default SettingsLayout;
