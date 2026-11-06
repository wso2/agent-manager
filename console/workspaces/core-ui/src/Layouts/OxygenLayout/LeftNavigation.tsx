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

import { Box, Divider, Sidebar, Tooltip } from "@wso2/oxygen-ui";
import { PanelLeftClose, PanelLeftOpen } from "@wso2/oxygen-ui-icons-react";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Link, useNavigate, type LinkProps } from "react-router-dom";

export interface NavigationItem {
  label: string;
  icon?: ReactNode;
  href?: string;
  isActive?: boolean;
  pinBottom?: boolean;
  /** Show after the regular navigation groups, separated by a divider. */
  dividerBefore?: boolean;
  /** A short label shown next to the item, such as "NEW". */
  badge?: string;
  type: 'item';
  children?: NavigationItem[];
}
export interface NavigationSection {
  title: string;
  items: Array<NavigationItem>;
  icon?: ReactNode;
  type: 'section';
  // When true, the group header becomes a collapsible nav item instead of a static title.
  collapsible?: boolean;
}

interface LeftNavigationProps {
  collapsed: boolean;
  /** Toggles the collapsed state from the control at the bottom of the sidebar. */
  onToggleCollapse: () => void;
  activeItem: string;
  mainItems: NavigationItem[];
  groupedItems: NavigationSection[];
}

const COLLAPSE_TOGGLE_ID = "sidebar-collapse-toggle";

// Shared by every sidebar tooltip so they all look alike.
const NAV_TOOLTIP_SLOT_PROPS = {
  tooltip: {
    sx: {
      typography: "body2",
      px: 1.5,
      py: 1,
      bgcolor: "background.default",
      color: "text.primary",
      border: 1,
      borderColor: "divider",
      borderRadius: 1,
      boxShadow: 1,
    },
  },
} as const;

/** Adds an accessible tooltip directly to a sidebar navigation link. */
function NavigationLink({ tooltip, ...props }: LinkProps & { tooltip: string }) {
  // Oxygen wraps links in a Fragment, so its built-in tooltip cannot attach
  // hover/focus handlers. Attach the tooltip directly to the router link.
  return (
    <Tooltip title={tooltip} placement="right" slotProps={NAV_TOOLTIP_SLOT_PROPS}>
      <Link {...props} />
    </Tooltip>
  );
}

/** Same tooltip as NavigationLink, for a sidebar item that isn't a link. */
function NavigationTooltip({ tooltip, children }: { tooltip: string; children?: ReactNode }) {
  return (
    <Tooltip title={tooltip} placement="right" slotProps={NAV_TOOLTIP_SLOT_PROPS}>
      <Box sx={{ display: "block" }}>{children}</Box>
    </Tooltip>
  );
}

/**
 * Renders linked leaf items with tooltips when collapsed, and parent items as
 * submenu toggles. Real anchors preserve middle-click and open-in-new-tab behavior.
 */
function renderItem(item: NavigationItem, collapsed = false) {
  const isLeaf = !item.children?.length;
  return (
    <Sidebar.Item
      id={item.label}
      key={item.label}
      link={isLeaf && item.href ? (
        <NavigationLink to={item.href} tooltip={collapsed ? item.label : ""} />
      ) : undefined}
    >
      <Sidebar.ItemIcon>{item.icon}</Sidebar.ItemIcon>
      <Sidebar.ItemLabel>{item.label}</Sidebar.ItemLabel>
      {item.badge && <Sidebar.ItemBadge color="primary">{item.badge}</Sidebar.ItemBadge>}
      {item.children?.map((child) => renderItem(child))}
    </Sidebar.Item>
  );
}

function findParentLabel(items: NavigationItem[], childLabel: string): string | undefined {
  for (const item of items) {
    if (item.children?.some((child) => child.label === childLabel)) {
      return item.label;
    }
    if (item.children) {
      const nestedParentLabel = findParentLabel(item.children, childLabel);
      if (nestedParentLabel) {
        return nestedParentLabel;
      }
    }
  }
  return undefined;
}

export const flattenWithChildren = (items: NavigationItem[]): NavigationItem[] =>
  items.flatMap((item) => [item, ...flattenWithChildren(item.children ?? [])]);

// Turns a section into a nav item with its items as children; Sidebar.Item handles expand/collapse.
export const sectionToItem = (section: NavigationSection): NavigationItem => ({
  label: section.title,
  type: 'item',
  icon: section.icon,
  children: section.items,
});

export const combineNavItems = (
  mainItems: NavigationItem[],
  groupedItems: NavigationSection[],
): NavigationItem[] => [
  ...mainItems,
  ...groupedItems.flatMap((group) => (group.collapsible ? [sectionToItem(group)] : group.items)),
];

/** Renders sidebar groups and routes selections from collapsed submenus. */
export function LeftNavigation({
  collapsed,
  onToggleCollapse,
  activeItem,
  mainItems,
  groupedItems,
}: LeftNavigationProps) {
  const topItems = mainItems.filter((item) => !item.pinBottom && !item.dividerBefore);
  const separatedItems = mainItems.filter((item) => !item.pinBottom && item.dividerBefore);
  const bottomItems = mainItems.filter((item) => item.pinBottom);
  const [expandedMenus, setExpandedMenus] = useState<Record<string, boolean>>({});
  const navigate = useNavigate();

  const allItems = useMemo(
    () => combineNavItems(mainItems, groupedItems),
    [mainItems, groupedItems],
  );
  const flatItems = useMemo(() => flattenWithChildren(allItems), [allItems]);

  // Only force-expand when activeItem itself changes, not on every allItems
  // reference change 
  const lastExpandedForRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (lastExpandedForRef.current === activeItem) {
      return;
    }
    lastExpandedForRef.current = activeItem;
    const parentLabel = findParentLabel(allItems, activeItem);
    if (parentLabel) {
      setExpandedMenus((prev) => (prev[parentLabel] ? prev : { ...prev, [parentLabel]: true }));
    }
  }, [activeItem, allItems]);

  // Collapsible groups start expanded, without resetting a group the user already toggled.
  useEffect(() => {
    setExpandedMenus((prev) => {
      const next = { ...prev };
      let changed = false;
      for (const group of groupedItems) {
        if (group.collapsible && !(group.title in next)) {
          next[group.title] = true;
          changed = true;
        }
      }
      return changed ? next : prev;
    });
  }, [groupedItems]);

  const handleToggleExpand = (id: string) => {
    setExpandedMenus((prev) => ({ ...prev, [id]: !prev[id] }));
  };

  // Collapsed sidebars show a child item's siblings in a hover popover instead
  // of the inline expansion used when expanded, and that popover only reports
  // the clicked item's id (its label) via onSelect rather than reusing its
  // `link` element — so route it to that item's href ourselves.
  const handleSelect = (id: string) => {
    if (id === COLLAPSE_TOGGLE_ID) {
      onToggleCollapse();
      return;
    }
    const item = flatItems.find((i) => i.label === id);
    if (item?.href) {
      navigate(item.href);
    }
  };

  return (
    <Sidebar
      collapsed={collapsed}
      activeItem={activeItem}
      expandedMenus={expandedMenus}
      onSelect={handleSelect}
      onToggleExpand={handleToggleExpand}
      width={280}
    >
      <Sidebar.Nav
        sx={{
          "& .MuiButtonBase-root": {
            animation: "leftNavFadeIn 320ms ease-out",
          },
          "@keyframes leftNavFadeIn": {
            from: {
              opacity: 0,
              transform: "translateX(6px)",
            },
            to: {
              opacity: 1,
              transform: "translateX(0)",
            },
          },
        }}
      >
        <Sidebar.Category>{topItems.map((item) => renderItem(item, collapsed))}</Sidebar.Category>
        {groupedItems.map((group) =>
          group.collapsible ? (
            renderItem(sectionToItem(group), collapsed)
          ) : (
            <Sidebar.Category key={group.title}>
              <Sidebar.CategoryLabel>{group.title}</Sidebar.CategoryLabel>
              {group.items.map((item) => renderItem(item, collapsed))}
            </Sidebar.Category>
          ),
        )}
        {separatedItems.map((item) => (
          <Sidebar.Category key={item.label}>
            {renderItem(item, collapsed)}
          </Sidebar.Category>
        ))}
      </Sidebar.Nav>
      <Sidebar.Footer>
        {bottomItems.length > 0 && (
          <>
            <Sidebar.Category>
              {bottomItems.map((item) => renderItem(item, collapsed))}
            </Sidebar.Category>
            <Divider sx={{ my: 1 }} />
          </>
        )}
        {/* A plain item rather than Sidebar.CollapseToggle, whose tooltip can't be styled. */}
        <Sidebar.Category>
          <Sidebar.Item
            id={COLLAPSE_TOGGLE_ID}
            link={<NavigationTooltip tooltip={collapsed ? "Expand" : ""} />}
          >
            <Sidebar.ItemIcon>
              {collapsed ? <PanelLeftOpen size={20} /> : <PanelLeftClose size={20} />}
            </Sidebar.ItemIcon>
            <Sidebar.ItemLabel>{collapsed ? "Expand" : "Collapse"}</Sidebar.ItemLabel>
          </Sidebar.Item>
        </Sidebar.Category>
      </Sidebar.Footer>
    </Sidebar>
  );
}
