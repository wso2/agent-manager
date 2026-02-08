/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import * as React from 'react';
import type { Meta, StoryObj } from '@storybook/react';
import { Sidebar, Box } from '@wso2/oxygen-ui';
import {
  Home,
  BarChart3,
  Users,
  Settings,
  FolderOpen,
  Layers,
  Shield,
  HelpCircle,
  Bell,
} from '@wso2/oxygen-ui-icons-react';

/**
 * Sidebar is a compound component for building application navigation sidebars.
 * It provides a flexible, composable API for creating sidebars with navigation
 * items, categories, badges, and user sections.
 */
const meta: Meta<typeof Sidebar> = {
  title: 'App Elements/Sidebar',
  component: Sidebar,
  tags: ['autodocs'],
  parameters: {
    layout: 'centered',
    docs: {
      description: {
        component: `
The Sidebar component is a compound component for building application navigation.
It supports collapsible states, hierarchical menus, and user profile sections.

### Sub-components
- \`Sidebar.Nav\` - Main navigation container
- \`Sidebar.Category\` - Group of navigation items
- \`Sidebar.CategoryLabel\` - Label for a category
- \`Sidebar.Item\` - Navigation item (supports nesting)
- \`Sidebar.ItemIcon\` - Icon for a navigation item
- \`Sidebar.ItemLabel\` - Label for a navigation item
- \`Sidebar.ItemBadge\` - Badge/count for a navigation item
- \`Sidebar.Footer\` - Footer section
- \`Sidebar.User\` - User profile section
- \`Sidebar.UserAvatar\` - User avatar
- \`Sidebar.UserName\` - User name
- \`Sidebar.UserEmail\` - User email

### Usage
\`\`\`tsx
import { Sidebar } from '@wso2/oxygen-ui';

<Sidebar
  collapsed={collapsed}
  activeItem={activeId}
  expandedMenus={expandedMenus}
  onSelect={handleSelect}
  onToggleExpand={handleToggle}
>
  <Sidebar.Nav>
    <Sidebar.Category>
      <Sidebar.Item id="home">
        <Sidebar.ItemIcon><Home size={20} /></Sidebar.ItemIcon>
        <Sidebar.ItemLabel>Home</Sidebar.ItemLabel>
      </Sidebar.Item>
    </Sidebar.Category>
  </Sidebar.Nav>
</Sidebar>
\`\`\`
        `,
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof Sidebar>;

/**
 * Basic sidebar with navigation items.
 */
export const Default: Story = {
  render: () => {
    const [activeItem, setActiveItem] = React.useState('dashboard');
    const [expandedMenus, setExpandedMenus] = React.useState<Record<string, boolean>>({});

    const handleToggleExpand = (id: string) => {
      setExpandedMenus((prev) => ({ ...prev, [id]: !prev[id] }));
    };

    return (
      <Box sx={{ height: 400, width: 280 }}>
        <Sidebar
          collapsed={false}
          activeItem={activeItem}
          expandedMenus={expandedMenus}
          onSelect={setActiveItem}
          onToggleExpand={handleToggleExpand}
        >
          <Sidebar.Nav>
            <Sidebar.Category>
              <Sidebar.Item id="dashboard">
                <Sidebar.ItemIcon><Home size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Dashboard</Sidebar.ItemLabel>
              </Sidebar.Item>
              <Sidebar.Item id="analytics">
                <Sidebar.ItemIcon><BarChart3 size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Analytics</Sidebar.ItemLabel>
              </Sidebar.Item>
              <Sidebar.Item id="users">
                <Sidebar.ItemIcon><Users size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Users</Sidebar.ItemLabel>
              </Sidebar.Item>
            </Sidebar.Category>
          </Sidebar.Nav>
        </Sidebar>
      </Box>
    );
  },
};

/**
 * Collapsed sidebar showing only icons.
 */
export const Collapsed: Story = {
  render: () => {
    const [activeItem, setActiveItem] = React.useState('dashboard');

    return (
      <Box sx={{ height: 400, width: 80 }}>
        <Sidebar
          collapsed={true}
          activeItem={activeItem}
          expandedMenus={{}}
          onSelect={setActiveItem}
        >
          <Sidebar.Nav>
            <Sidebar.Category>
              <Sidebar.Item id="dashboard">
                <Sidebar.ItemIcon><Home size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Dashboard</Sidebar.ItemLabel>
              </Sidebar.Item>
              <Sidebar.Item id="analytics">
                <Sidebar.ItemIcon><BarChart3 size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Analytics</Sidebar.ItemLabel>
              </Sidebar.Item>
              <Sidebar.Item id="users">
                <Sidebar.ItemIcon><Users size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Users</Sidebar.ItemLabel>
              </Sidebar.Item>
            </Sidebar.Category>
          </Sidebar.Nav>
        </Sidebar>
      </Box>
    );
  },
};

/**
 * Sidebar with categorized navigation.
 */
export const WithCategories: Story = {
  render: () => {
    const [activeItem, setActiveItem] = React.useState('dashboard');
    const [expandedMenus, setExpandedMenus] = React.useState<Record<string, boolean>>({});

    const handleToggleExpand = (id: string) => {
      setExpandedMenus((prev) => ({ ...prev, [id]: !prev[id] }));
    };

    return (
      <Box sx={{ height: 500, width: 280 }}>
        <Sidebar
          collapsed={false}
          activeItem={activeItem}
          expandedMenus={expandedMenus}
          onSelect={setActiveItem}
          onToggleExpand={handleToggleExpand}
        >
          <Sidebar.Nav>
            <Sidebar.Category>
              <Sidebar.Item id="dashboard">
                <Sidebar.ItemIcon><Home size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Dashboard</Sidebar.ItemLabel>
              </Sidebar.Item>
            </Sidebar.Category>

            <Sidebar.Category>
              <Sidebar.CategoryLabel>Management</Sidebar.CategoryLabel>
              <Sidebar.Item id="users">
                <Sidebar.ItemIcon><Users size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Users</Sidebar.ItemLabel>
              </Sidebar.Item>
              <Sidebar.Item id="projects">
                <Sidebar.ItemIcon><FolderOpen size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Projects</Sidebar.ItemLabel>
              </Sidebar.Item>
            </Sidebar.Category>

            <Sidebar.Category>
              <Sidebar.CategoryLabel>Infrastructure</Sidebar.CategoryLabel>
              <Sidebar.Item id="security">
                <Sidebar.ItemIcon><Shield size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Security</Sidebar.ItemLabel>
              </Sidebar.Item>
              <Sidebar.Item id="integrations">
                <Sidebar.ItemIcon><Layers size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Integrations</Sidebar.ItemLabel>
              </Sidebar.Item>
            </Sidebar.Category>
          </Sidebar.Nav>
        </Sidebar>
      </Box>
    );
  },
};

/**
 * Sidebar with badges on navigation items.
 */
export const WithBadges: Story = {
  render: () => {
    const [activeItem, setActiveItem] = React.useState('dashboard');

    return (
      <Box sx={{ height: 400, width: 280 }}>
        <Sidebar
          collapsed={false}
          activeItem={activeItem}
          expandedMenus={{}}
          onSelect={setActiveItem}
        >
          <Sidebar.Nav>
            <Sidebar.Category>
              <Sidebar.Item id="dashboard">
                <Sidebar.ItemIcon><Home size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Dashboard</Sidebar.ItemLabel>
              </Sidebar.Item>
              <Sidebar.Item id="users">
                <Sidebar.ItemIcon><Users size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Users</Sidebar.ItemLabel>
                <Sidebar.ItemBadge>12</Sidebar.ItemBadge>
              </Sidebar.Item>
              <Sidebar.Item id="notifications">
                <Sidebar.ItemIcon><Bell size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Notifications</Sidebar.ItemLabel>
                <Sidebar.ItemBadge color="error">3</Sidebar.ItemBadge>
              </Sidebar.Item>
            </Sidebar.Category>
          </Sidebar.Nav>
        </Sidebar>
      </Box>
    );
  },
};

/**
 * Sidebar with nested/hierarchical navigation items.
 */
export const WithNestedItems: Story = {
  render: () => {
    const [activeItem, setActiveItem] = React.useState('dashboard');
    const [expandedMenus, setExpandedMenus] = React.useState<Record<string, boolean>>({
      analytics: true,
    });

    const handleToggleExpand = (id: string) => {
      setExpandedMenus((prev) => ({ ...prev, [id]: !prev[id] }));
    };

    return (
      <Box sx={{ height: 500, width: 280 }}>
        <Sidebar
          collapsed={false}
          activeItem={activeItem}
          expandedMenus={expandedMenus}
          onSelect={setActiveItem}
          onToggleExpand={handleToggleExpand}
        >
          <Sidebar.Nav>
            <Sidebar.Category>
              <Sidebar.Item id="dashboard">
                <Sidebar.ItemIcon><Home size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Dashboard</Sidebar.ItemLabel>
              </Sidebar.Item>
              <Sidebar.Item id="analytics">
                <Sidebar.ItemIcon><BarChart3 size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Analytics</Sidebar.ItemLabel>
                <Sidebar.Item id="analytics-overview">
                  <Sidebar.ItemIcon><BarChart3 size={20} /></Sidebar.ItemIcon>
                  <Sidebar.ItemLabel>Overview</Sidebar.ItemLabel>
                </Sidebar.Item>
                <Sidebar.Item id="analytics-reports">
                  <Sidebar.ItemIcon><FolderOpen size={20} /></Sidebar.ItemIcon>
                  <Sidebar.ItemLabel>Reports</Sidebar.ItemLabel>
                </Sidebar.Item>
              </Sidebar.Item>
            </Sidebar.Category>
          </Sidebar.Nav>
        </Sidebar>
      </Box>
    );
  },
};

/**
 * Sidebar with footer section.
 */
export const WithFooter: Story = {
  render: () => {
    const [activeItem, setActiveItem] = React.useState('dashboard');
    const [expandedMenus, setExpandedMenus] = React.useState<Record<string, boolean>>({});

    const handleToggleExpand = (id: string) => {
      setExpandedMenus((prev) => ({ ...prev, [id]: !prev[id] }));
    };

    return (
      <Box sx={{ height: 500, width: 280 }}>
        <Sidebar
          collapsed={false}
          activeItem={activeItem}
          expandedMenus={expandedMenus}
          onSelect={setActiveItem}
          onToggleExpand={handleToggleExpand}
        >
          <Sidebar.Nav>
            <Sidebar.Category>
              <Sidebar.Item id="dashboard">
                <Sidebar.ItemIcon><Home size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Dashboard</Sidebar.ItemLabel>
              </Sidebar.Item>
              <Sidebar.Item id="analytics">
                <Sidebar.ItemIcon><BarChart3 size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Analytics</Sidebar.ItemLabel>
              </Sidebar.Item>
            </Sidebar.Category>
          </Sidebar.Nav>

          <Sidebar.Footer>
            <Sidebar.Category>
              <Sidebar.Item id="settings">
                <Sidebar.ItemIcon><Settings size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Settings</Sidebar.ItemLabel>
              </Sidebar.Item>
              <Sidebar.Item id="help">
                <Sidebar.ItemIcon><HelpCircle size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Help & Support</Sidebar.ItemLabel>
              </Sidebar.Item>
            </Sidebar.Category>
          </Sidebar.Footer>
        </Sidebar>
      </Box>
    );
  },
};

/**
 * Sidebar with user profile section.
 */
export const WithUser: Story = {
  render: () => {
    const [activeItem, setActiveItem] = React.useState('dashboard');

    return (
      <Box sx={{ height: 500, width: 280 }}>
        <Sidebar
          collapsed={false}
          activeItem={activeItem}
          expandedMenus={{}}
          onSelect={setActiveItem}
        >
          <Sidebar.Nav>
            <Sidebar.Category>
              <Sidebar.Item id="dashboard">
                <Sidebar.ItemIcon><Home size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Dashboard</Sidebar.ItemLabel>
              </Sidebar.Item>
              <Sidebar.Item id="settings">
                <Sidebar.ItemIcon><Settings size={20} /></Sidebar.ItemIcon>
                <Sidebar.ItemLabel>Settings</Sidebar.ItemLabel>
              </Sidebar.Item>
            </Sidebar.Category>
          </Sidebar.Nav>

          <Sidebar.Footer>
            <Sidebar.User onClick={() => console.log('User clicked')}>
              <Sidebar.UserAvatar>JD</Sidebar.UserAvatar>
              <Sidebar.UserName>John Doe</Sidebar.UserName>
              <Sidebar.UserEmail>john@example.com</Sidebar.UserEmail>
            </Sidebar.User>
          </Sidebar.Footer>
        </Sidebar>
      </Box>
    );
  },
};
