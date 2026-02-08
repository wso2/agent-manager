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
import { UserMenu, Box, Typography } from '@wso2/oxygen-ui';

/**
 * UserMenu is a component for displaying user profile dropdown menu
 * with navigation options like Profile, Settings, Billing, and Logout.
 */
const meta: Meta<typeof UserMenu> = {
  title: 'App Elements/UserMenu',
  component: UserMenu,
  tags: ['autodocs'],
  parameters: {
    layout: 'centered',
    docs: {
      description: {
        component: `
The UserMenu component provides a user profile dropdown menu.

### Features
- User avatar button that opens dropdown
- User info header with name, email, and role badge
- Menu items for Profile, Settings, Billing, and Logout
- Destructive styling for logout action
- Role/plan badge display (e.g., "Pro")

### Usage
\`\`\`tsx
import { UserMenu } from '@wso2/oxygen-ui';

<UserMenu
  user={{
    name: 'John Doe',
    email: 'john@example.com',
    avatar: 'JD',
    role: 'Pro'
  }}
  onProfileClick={() => navigate('/profile')}
  onSettingsClick={() => navigate('/settings')}
  onBillingClick={() => navigate('/billing')}
  onLogout={() => handleLogout()}
/>
\`\`\`
        `,
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof UserMenu>;

/**
 * Basic user menu with all options.
 */
export const Default: Story = {
  render: () => (
    <Box sx={{ p: 4 }}>
      <Typography variant="caption" color="text.secondary" sx={{ mb: 2, display: 'block' }}>
        Click the avatar to open menu
      </Typography>
      <UserMenu
        user={{
          name: 'John Doe',
          email: 'john@example.com',
          avatar: 'JD',
        }}
        onProfileClick={() => console.log('Profile clicked')}
        onSettingsClick={() => console.log('Settings clicked')}
        onBillingClick={() => console.log('Billing clicked')}
        onLogout={() => console.log('Logout clicked')}
      />
    </Box>
  ),
};

/**
 * User menu with role badge.
 */
export const WithRole: Story = {
  render: () => (
    <Box sx={{ p: 4 }}>
      <Typography variant="caption" color="text.secondary" sx={{ mb: 2, display: 'block' }}>
        User with Pro role badge
      </Typography>
      <UserMenu
        user={{
          name: 'Jane Smith',
          email: 'jane@example.com',
          avatar: 'JS',
          role: 'Pro',
        }}
        onProfileClick={() => console.log('Profile clicked')}
        onSettingsClick={() => console.log('Settings clicked')}
        onBillingClick={() => console.log('Billing clicked')}
        onLogout={() => console.log('Logout clicked')}
      />
    </Box>
  ),
};

/**
 * Admin user with admin role.
 */
export const AdminUser: Story = {
  render: () => (
    <Box sx={{ p: 4 }}>
      <Typography variant="caption" color="text.secondary" sx={{ mb: 2, display: 'block' }}>
        Admin user
      </Typography>
      <UserMenu
        user={{
          name: 'Admin User',
          email: 'admin@company.com',
          avatar: 'AU',
          role: 'Admin',
        }}
        onProfileClick={() => console.log('Profile clicked')}
        onSettingsClick={() => console.log('Settings clicked')}
        onBillingClick={() => console.log('Billing clicked')}
        onLogout={() => console.log('Logout clicked')}
      />
    </Box>
  ),
};

/**
 * User without avatar initials (uses first letter of name).
 */
export const WithoutAvatar: Story = {
  render: () => (
    <Box sx={{ p: 4 }}>
      <Typography variant="caption" color="text.secondary" sx={{ mb: 2, display: 'block' }}>
        Avatar uses first letter of name
      </Typography>
      <UserMenu
        user={{
          name: 'Michael Johnson',
          email: 'michael.johnson@example.com',
        }}
        onProfileClick={() => console.log('Profile clicked')}
        onSettingsClick={() => console.log('Settings clicked')}
        onBillingClick={() => console.log('Billing clicked')}
        onLogout={() => console.log('Logout clicked')}
      />
    </Box>
  ),
};

/**
 * Enterprise user with long email.
 */
export const LongEmail: Story = {
  render: () => (
    <Box sx={{ p: 4 }}>
      <Typography variant="caption" color="text.secondary" sx={{ mb: 2, display: 'block' }}>
        Long email gets truncated
      </Typography>
      <UserMenu
        user={{
          name: 'Enterprise User',
          email: 'enterprise.user.with.very.long.email@corporation.example.com',
          avatar: 'EU',
          role: 'Enterprise',
        }}
        onProfileClick={() => console.log('Profile clicked')}
        onSettingsClick={() => console.log('Settings clicked')}
        onBillingClick={() => console.log('Billing clicked')}
        onLogout={() => console.log('Logout clicked')}
      />
    </Box>
  ),
};

/**
 * Multiple user menus (showing different users).
 */
export const MultipleUsers: Story = {
  render: () => (
    <Box sx={{ p: 4, display: 'flex', gap: 4 }}>
      <Box sx={{ textAlign: 'center' }}>
        <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: 'block' }}>
          Free User
        </Typography>
        <UserMenu
          user={{
            name: 'Free User',
            email: 'free@example.com',
            avatar: 'FU',
          }}
          onProfileClick={() => console.log('Profile clicked')}
          onLogout={() => console.log('Logout clicked')}
        />
      </Box>
      <Box sx={{ textAlign: 'center' }}>
        <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: 'block' }}>
          Pro User
        </Typography>
        <UserMenu
          user={{
            name: 'Pro User',
            email: 'pro@example.com',
            avatar: 'PU',
            role: 'Pro',
          }}
          onProfileClick={() => console.log('Profile clicked')}
          onLogout={() => console.log('Logout clicked')}
        />
      </Box>
      <Box sx={{ textAlign: 'center' }}>
        <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: 'block' }}>
          Admin
        </Typography>
        <UserMenu
          user={{
            name: 'Admin',
            email: 'admin@example.com',
            avatar: 'AD',
            role: 'Admin',
          }}
          onProfileClick={() => console.log('Profile clicked')}
          onLogout={() => console.log('Logout clicked')}
        />
      </Box>
    </Box>
  ),
};
