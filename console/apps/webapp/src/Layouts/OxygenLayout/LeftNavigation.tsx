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

import { Sidebar } from "@wso2/oxygen-ui";
import type { ReactNode } from "react";

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

interface LeftNavigationProps {
  collapsed: boolean;
  activeItem: string;
  mainItems: NavigationItem[];
  groupedItems: NavigationSection[];
  onNavigationClick: (itemKey: string) => void;
}

export function LeftNavigation({
  collapsed,
  activeItem,
  mainItems,
  groupedItems,
  onNavigationClick,
}: LeftNavigationProps) {
  return (
    <Sidebar
      collapsed={collapsed}
      activeItem={activeItem}
      onSelect={onNavigationClick}
    >
      <Sidebar.Nav>
        <Sidebar.Category>
          {mainItems.map((item) => (
            <Sidebar.Item id={item.label} key={item.label}>
              <Sidebar.ItemIcon>{item.icon}</Sidebar.ItemIcon>
              <Sidebar.ItemLabel>{item.label}</Sidebar.ItemLabel>
            </Sidebar.Item>
          ))}
        </Sidebar.Category>
        {groupedItems.map((group) => (
          <Sidebar.Category key={group.title}>
            <Sidebar.CategoryLabel>{group.title}</Sidebar.CategoryLabel>
            {group.items.map((item) => (
              <Sidebar.Item id={item.label} key={item.label}>
                <Sidebar.ItemIcon>{item.icon}</Sidebar.ItemIcon>
                <Sidebar.ItemLabel>{item.label}</Sidebar.ItemLabel>
              </Sidebar.Item>
            ))}
          </Sidebar.Category>
        ))}
      </Sidebar.Nav>
    </Sidebar>
  );
}
