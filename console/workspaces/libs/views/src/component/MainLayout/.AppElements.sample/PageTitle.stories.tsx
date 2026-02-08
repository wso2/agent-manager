/*
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

import type { Meta, StoryObj } from '@storybook/react';
import { PageTitle, colors, Box } from '@wso2/oxygen-ui';
import { Link as LinkIcon, User, Folder } from '@wso2/oxygen-ui-icons-react';
import React from 'react';

/**
 * PageTitle is a compound component for displaying page headers with optional subheaders.
 * It provides a consistent layout and styling for page titles throughout your application.
 */
const meta: Meta<typeof PageTitle> = {
  title: 'App Elements/Page Title',
  component: PageTitle,
  tags: ['autodocs'],
  parameters: {
    docs: {
      description: {
        component: `
The PageTitle component is a flexible compound component that displays a page title with an optional subheader.
It uses the compound component pattern, providing PageTitle.Header and PageTitle.SubHeader as subcomponents.

### Features
- **Compound Pattern**: Compose Avatar, Header, SubHeader, and Link as needed
- **Avatar Support**: Optional avatar on the left side
- **Link Component**: Add links with icon support below the subheader
- **Theme Integration**: Automatically adapts to the current theme
- **Flexible Styling**: Accepts all standard HTML div props and the sx prop for Material-UI theming
- **Typography Control**: Full control over variant, color, and other Typography props

### Usage
\`\`\`tsx
import { PageTitle } from '@wso2/oxygen-ui';
import { Link as LinkIcon } from '@wso2/oxygen-ui-icons-react';

<PageTitle sx={{ mb: 4 }}>
  <PageTitle.Avatar src="/avatar.png" />
  <PageTitle.Header>Dashboard</PageTitle.Header>
  <PageTitle.SubHeader>Overview of your workspace</PageTitle.SubHeader>
  <PageTitle.Link href="/docs" icon={<LinkIcon />}>View Documentation</PageTitle.Link>
</PageTitle>
\`\`\`
        `,
      },
    },
  },
  argTypes: {
    children: {
      control: false,
      description: 'The content of the PageTitle (typically Header and SubHeader components)',
    },
  },
};

export default meta;
type Story = StoryObj<typeof PageTitle>;

/**
 * Basic usage with both header and subheader.
 */
export const Default: Story = {
  render: () => (
    <PageTitle>
      <PageTitle.Header>Dashboard</PageTitle.Header>
      <PageTitle.SubHeader>Overview of your workspace and recent activity</PageTitle.SubHeader>
    </PageTitle>
  ),
};

/**
 * Page title with avatar on the left side.
 */
export const WithAvatar: Story = {
  render: () => (
    <PageTitle>
      <PageTitle.Avatar src="https://i.pravatar.cc/150?img=12" alt="User Avatar" />
      <PageTitle.Header>John Doe</PageTitle.Header>
      <PageTitle.SubHeader>Software Engineer</PageTitle.SubHeader>
    </PageTitle>
  ),
};

/**
 * Page title with avatar using an icon.
 */
export const WithAvatarIcon: Story = {
  render: () => (
    <PageTitle>
      <PageTitle.Avatar>
        <User size={30} />
      </PageTitle.Avatar>
      <PageTitle.Header>User Profile</PageTitle.Header>
      <PageTitle.SubHeader>Manage your account settings</PageTitle.SubHeader>
    </PageTitle>
  ),
};

/**
 * Page title with a link below the subheader.
 */
export const WithLink: Story = {
  render: () => (
    <PageTitle>
      <PageTitle.Header>Documentation</PageTitle.Header>
      <PageTitle.SubHeader>Comprehensive guides and API references</PageTitle.SubHeader>
      <PageTitle.Link href="#" icon={<LinkIcon size={14} />}>
        View all docs
      </PageTitle.Link>
    </PageTitle>
  ),
};

/**
 * Complete example with avatar, header, subheader, and link.
 */
export const Complete: Story = {
  render: () => (
    <PageTitle>
      <PageTitle.Avatar sx={{ bgcolor: colors.deepPurple[500] }}>
        <Folder size={30} />
      </PageTitle.Avatar>
      <PageTitle.Header>Project Dashboard</PageTitle.Header>
      <PageTitle.SubHeader>Track progress and manage your team</PageTitle.SubHeader>
      <PageTitle.Link href="#" icon={<LinkIcon size={14} />}>
        View project details
      </PageTitle.Link>
    </PageTitle>
  ),
};

/**
 * Page title with custom styling using the sx prop.
 */
export const WithSxProp: Story = {
  render: () => (
    <PageTitle sx={{ mb: 4, p: 2, bgcolor: 'action.hover', borderRadius: 1 }}>
      <PageTitle.Header>Activity Logs</PageTitle.Header>
      <PageTitle.SubHeader>View and monitor authentication events and system activities</PageTitle.SubHeader>
      <PageTitle.Link href="#" icon={<LinkIcon size={14} />}>
        View detailed logs
      </PageTitle.Link>
    </PageTitle>
  ),
};

/**
 * Full example with BackButton, Avatar, Header, SubHeader, Link, and Actions.
 */
export const FullExample: Story = {
  render: () => (
    <PageTitle>
      <PageTitle.BackButton component={<a href="#" />} />
      <PageTitle.Avatar sx={{ bgcolor: colors.deepPurple[500] }}>
        E
      </PageTitle.Avatar>
      <PageTitle.Header>E-Commerce Platform</PageTitle.Header>
      <PageTitle.SubHeader>Complete authentication and user management system for e-commerce</PageTitle.SubHeader>
      <PageTitle.Link href="#" icon={<LinkIcon size={14} />}>
        Link a Repository
      </PageTitle.Link>
      <PageTitle.Actions>
        <Box sx={{ display: 'flex', gap: 1 }}>
          <button style={{ padding: '8px 16px', borderRadius: '4px', border: 'none', backgroundColor: '#1976d2', color: 'white', cursor: 'pointer' }}>
            Create
          </button>
        </Box>
      </PageTitle.Actions>
    </PageTitle>
  ),
};

/**
 * Page title with custom styling on the container.
 */
export const CustomStyling: Story = {
  render: () => (
    <PageTitle style={{ borderLeft: '4px solid #1976d2', paddingLeft: '16px' }}>
      <PageTitle.Header>Projects</PageTitle.Header>
      <PageTitle.SubHeader>Manage your development projects</PageTitle.SubHeader>
      <PageTitle.Link href="#" icon={<LinkIcon size={14} />}>
        Create new project
      </PageTitle.Link>
    </PageTitle>
  ),
};
