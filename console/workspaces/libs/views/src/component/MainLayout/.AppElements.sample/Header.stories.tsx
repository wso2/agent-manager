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
import {
  Header,
  ColorSchemeToggle,
  IconButton,
  Badge,
  Divider,
  Tooltip,
  Box,
} from '@wso2/oxygen-ui';
import {
  Zap,
  Bell,
  HelpCircle,
  Menu,
} from '@wso2/oxygen-ui-icons-react';

/**
 * Logo component for the header.
 */
const Logo: React.FC = () => (
  <Box
    sx={{
      width: 32,
      height: 32,
      borderRadius: 1,
      bgcolor: 'primary.main',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      color: 'primary.contrastText',
    }}
  >
    <Zap size={20} />
  </Box>
);

/**
 * Header is a compound component for building application top navigation bars.
 * It provides a flexible, composable API for creating headers with various elements
 * like logos, navigation toggles, context switchers, and action buttons.
 */
const meta: Meta<typeof Header> = {
  title: 'App Elements/Header',
  component: Header,
  tags: ['autodocs'],
  parameters: {
    layout: 'fullscreen',
    docs: {
      description: {
        component: `
The Header component is a compound component for building application navigation bars.
It uses the compound component pattern, providing sub-components for maximum flexibility.

### Sub-components
- \`Header.Toggle\` - Sidebar toggle button
- \`Header.Brand\` - Container for logo and title
- \`Header.BrandLogo\` - Logo element
- \`Header.BrandTitle\` - Title text
- \`Header.Switchers\` - Container for context switchers
- \`Header.Actions\` - Container for action buttons
- \`Header.Spacer\` - Flexible spacer

### Usage
\`\`\`tsx
import { Header } from '@wso2/oxygen-ui';

<Header>
  <Header.Toggle collapsed={collapsed} onToggle={toggle} />
  <Header.Brand>
    <Header.BrandLogo><Logo /></Header.BrandLogo>
    <Header.BrandTitle>Dashboard</Header.BrandTitle>
  </Header.Brand>
  <Header.Spacer />
  <Header.Actions>
    <ColorSchemeToggle />
    <UserMenu />
  </Header.Actions>
</Header>
\`\`\`
        `,
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof Header>;

/**
 * Basic header with logo, title, and action buttons.
 */
export const Default: Story = {
  render: () => (
    <Header>
      <Header.Brand>
        <Header.BrandLogo><Logo /></Header.BrandLogo>
        <Header.BrandTitle>Oxygen UI</Header.BrandTitle>
      </Header.Brand>
      <Header.Spacer />
      <Header.Actions>
        <ColorSchemeToggle />
      </Header.Actions>
    </Header>
  ),
};

/**
 * Header with sidebar toggle button.
 */
export const WithToggle: Story = {
  render: () => {
    const [collapsed, setCollapsed] = React.useState(false);

    return (
      <Header>
        <Header.Toggle
          collapsed={collapsed}
          onToggle={() => setCollapsed(!collapsed)}
        />
        <Header.Brand>
          <Header.BrandLogo><Logo /></Header.BrandLogo>
          <Header.BrandTitle>Dashboard</Header.BrandTitle>
        </Header.Brand>
        <Header.Spacer />
        <Header.Actions>
          <ColorSchemeToggle />
        </Header.Actions>
      </Header>
    );
  },
};

/**
 * Header with multiple action buttons.
 */
export const WithActions: Story = {
  render: () => (
    <Header>
      <Header.Brand>
        <Header.BrandLogo><Logo /></Header.BrandLogo>
        <Header.BrandTitle>Oxygen UI</Header.BrandTitle>
      </Header.Brand>
      <Header.Spacer />
      <Header.Actions>
        <ColorSchemeToggle />
        <Tooltip title="Help & Support">
          <IconButton size="small" sx={{ color: 'text.secondary' }}>
            <HelpCircle size={20} />
          </IconButton>
        </Tooltip>
        <Tooltip title="Notifications">
          <IconButton size="small" sx={{ color: 'text.secondary' }}>
            <Badge badgeContent={3} color="error" max={99}>
              <Bell size={20} />
            </Badge>
          </IconButton>
        </Tooltip>
        <Divider orientation="vertical" flexItem sx={{ mx: 1 }} />
        <IconButton size="small" sx={{ color: 'text.secondary' }}>
          <Menu size={20} />
        </IconButton>
      </Header.Actions>
    </Header>
  ),
};

/**
 * Minimal header without switchers section.
 */
export const Minimal: Story = {
  render: () => {
    const [collapsed, setCollapsed] = React.useState(false);

    return (
      <Header minimal>
        <Header.Toggle
          collapsed={collapsed}
          onToggle={() => setCollapsed(!collapsed)}
        />
        <Header.Brand>
          <Header.BrandLogo><Logo /></Header.BrandLogo>
          <Header.BrandTitle>Minimal Header</Header.BrandTitle>
        </Header.Brand>
        <Header.Spacer />
        <Header.Actions>
          <ColorSchemeToggle />
        </Header.Actions>
      </Header>
    );
  },
};

/**
 * Header with switchers section for organization/project context.
 */
export const WithSwitchers: Story = {
  render: () => (
    <Header>
      <Header.Brand>
        <Header.BrandLogo><Logo /></Header.BrandLogo>
        <Header.BrandTitle>Oxygen UI</Header.BrandTitle>
      </Header.Brand>
      <Header.Switchers>
        <Box
          sx={{
            px: 1.5,
            py: 0.5,
            bgcolor: 'action.hover',
            borderRadius: 1,
            fontSize: 14,
          }}
        >
          Acme Corp
        </Box>
        <Box
          sx={{
            px: 1.5,
            py: 0.5,
            bgcolor: 'action.hover',
            borderRadius: 1,
            fontSize: 14,
          }}
        >
          Project Alpha
        </Box>
      </Header.Switchers>
      <Header.Spacer />
      <Header.Actions>
        <ColorSchemeToggle />
      </Header.Actions>
    </Header>
  ),
};

/**
 * Complete header with all elements.
 */
export const Complete: Story = {
  render: () => {
    const [collapsed, setCollapsed] = React.useState(false);

    return (
      <Header>
        <Header.Toggle
          collapsed={collapsed}
          onToggle={() => setCollapsed(!collapsed)}
        />
        <Header.Brand>
          <Header.BrandLogo><Logo /></Header.BrandLogo>
          <Header.BrandTitle>Oxygen UI</Header.BrandTitle>
        </Header.Brand>
        <Header.Switchers>
          <Box
            sx={{
              px: 1.5,
              py: 0.5,
              bgcolor: 'action.hover',
              borderRadius: 1,
              fontSize: 14,
            }}
          >
            Acme Corp
          </Box>
        </Header.Switchers>
        <Header.Spacer />
        <Header.Actions>
          <ColorSchemeToggle />
          <Tooltip title="Help">
            <IconButton size="small" sx={{ color: 'text.secondary' }}>
              <HelpCircle size={20} />
            </IconButton>
          </Tooltip>
          <Tooltip title="Notifications">
            <IconButton size="small" sx={{ color: 'text.secondary' }}>
              <Badge badgeContent={5} color="error">
                <Bell size={20} />
              </Badge>
            </IconButton>
          </Tooltip>
        </Header.Actions>
      </Header>
    );
  },
};
