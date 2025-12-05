/**
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
import { Box, Button, IconButton, Badge } from '@mui/material';
import { Notifications, Search, Brightness4 } from '@mui/icons-material';
import { MainLayout } from './MainLayout';

const meta: Meta<typeof MainLayout> = {
  title: 'Components/MainLayout',
  component: MainLayout,
  parameters: {
    layout: 'fullscreen',
  },
  decorators: [
    (Story) => {
      return (
        <Box sx={{ display: 'flex', height: '100vh' }}>
          <Story >
            <Box height={1000} width={1000} bgcolor="red" />
          </Story>
        </Box>
      );
    },
  ],
  argTypes: {
    appTitle: {
      control: 'text',
      description: 'The title of the application',
    },
    showAppTitle: {
      control: 'boolean',
      description: 'Whether to show the app title',
    },
    sidebarCollapsed: {
      control: 'boolean',
      description: 'Whether the sidebar is collapsed (icons only)',
    },
    onLogout: {
      action: 'logout',
      description: 'Callback when user logs out',
    },
  },
};

export default meta;
type Story = StoryObj<typeof MainLayout>;

// Basic MainLayout with default props
export const Default: Story = {
  args: {
    user: {
      name: 'John Doe',
      email: 'john.doe@example.com',
    },
  },
};

// MainLayout with custom logo
export const WithLogo: Story = {
  args: {
    logo: (
      <Box
        sx={{
          width: 40,
          height: 40,
          borderRadius: 1,
          background: 'linear-gradient(45deg, #cd00ef, #f4009e)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'white',
          fontWeight: 'bold',
          fontSize: '1.2rem',
        }}
      >
        AI
      </Box>
    ),
    user: {
      name: 'Jane Smith',
      email: 'jane.smith@example.com',
      avatar: 'https://i.pravatar.cc/150?img=1',
    },
  },
};

// MainLayout with custom navigation items
export const WithCustomNavigation: Story = {
  args: {
    logo: (
      <Box
        sx={{
          width: 40,
          height: 40,
          borderRadius: 1,
          background: 'linear-gradient(45deg, #cd00ef, #f4009e)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'white',
          fontWeight: 'bold',
          fontSize: '1.2rem',
        }}
      >
        AI
      </Box>
    ),
    user: {
      name: 'Alex Johnson',
      email: 'alex.johnson@example.com',
    },
    navigationItems: [
      { label: 'Home', icon: <Box>🏠</Box>, href: '/home' },
      { label: 'Projects', icon: <Box>📁</Box>, href: '/projects' },
      { label: 'Team', icon: <Box>👥</Box>, href: '/team' },
      { label: 'Reports', icon: <Box>📊</Box>, href: '/reports' },
    ],
  },
};

// MainLayout with custom user menu items
export const WithCustomUserMenu: Story = {
  args: {
    logo: (
      <Box
        sx={{
          width: 40,
          height: 40,
          borderRadius: 1,
          background: 'linear-gradient(45deg, #cd00ef, #f4009e)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'white',
          fontWeight: 'bold',
          fontSize: '1.2rem',
        }}
      >
        AI
      </Box>
    ),
    user: {
      name: 'Sarah Wilson',
      email: 'sarah.wilson@example.com',
      avatar: 'https://i.pravatar.cc/150?img=2',
    },
    userMenuItems: [
      { label: 'My Profile', icon: <Box>👤</Box>, onClick: () => { } },
      { label: 'Account Settings', icon: <Box>⚙️</Box>, onClick: () => { } },
      { label: 'Billing', icon: <Box>💳</Box>, onClick: () => { } },
      { label: 'Help & Support', icon: <Box>❓</Box>, onClick: () => { } },
      { label: 'Sign Out', icon: <Box>🚪</Box>, onClick: () => { }, divider: true },
    ],
  },
};

// MainLayout with external elements
export const WithExternalElements: Story = {
  args: {
    logo: (
      <Box
        sx={{
          width: 40,
          height: 40,
          borderRadius: 1,
          background: 'linear-gradient(45deg, #cd00ef, #f4009e)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'white',
          fontWeight: 'bold',
          fontSize: '1.2rem',
        }}
      >
        AI
      </Box>
    ),
    user: {
      name: 'Mike Chen',
      email: 'mike.chen@example.com',
    },
    leftElements: (
      <Button variant="outlined" size="small" sx={{ mr: 1 }}>
        New Project
      </Button>
    ),
    rightElements: (
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        <IconButton size="small">
          <Search />
        </IconButton>
        <IconButton size="small">
          <Badge badgeContent={4} color="error">
            <Notifications />
          </Badge>
        </IconButton>
        <IconButton size="small">
          <Brightness4 />
        </IconButton>
      </Box>
    ),
  },
};

// MainLayout without user menu
export const WithoutUser: Story = {
  args: {
    logo: (
      <Box
        sx={{
          width: 40,
          height: 40,
          borderRadius: 1,
          background: 'linear-gradient(45deg, #cd00ef, #f4009e)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'white',
          fontWeight: 'bold',
          fontSize: '1.2rem',
        }}
      >
        AI
      </Box>
    ),
    rightElements: (
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        <Button variant="outlined" size="small">
          Sign In
        </Button>
        <Button variant="contained" size="small">
          Sign Up
        </Button>
      </Box>
    ),
  },
};

// MainLayout with long title
export const WithLongTitle: Story = {
  args: {
    appTitle: 'AI Agent Management Platform - Advanced Dashboard',
    user: {
      name: 'Dr. Emily Rodriguez',
      email: 'emily.rodriguez@ai-platform.com',
    },
  },
};

// MainLayout without title
export const WithoutTitle: Story = {
  args: {
    showAppTitle: false,
    logo: (
      <Box
        sx={{
          width: 40,
          height: 40,
          borderRadius: 1,
          background: 'linear-gradient(45deg, #cd00ef, #f4009e)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'white',
          fontWeight: 'bold',
          fontSize: '1.2rem',
        }}
      >
        AI
      </Box>
    ),
    user: {
      name: 'Tom Anderson',
      email: 'tom.anderson@example.com',
    },
  },
};

// MainLayout with collapsed sidebar
export const CollapsedSidebar: Story = {
  args: {
    logo: (
      <Box
        sx={{
          width: 40,
          height: 40,
          borderRadius: 1,
          background: 'linear-gradient(45deg, #cd00ef, #f4009e)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'white',
          fontWeight: 'bold',
          fontSize: '1.2rem',
        }}
      >
        AI
      </Box>
    ),
    user: {
      name: 'Lisa Park',
      email: 'lisa.park@example.com',
    },
    sidebarCollapsed: true, // This should show logo in collapsed state
    navigationItems: [
      { label: 'Dashboard', icon: <Box>📊</Box>, href: '/dashboard' },
      { label: 'Projects', icon: <Box>📁</Box>, href: '/projects' },
      { label: 'Team', icon: <Box>👥</Box>, href: '/team' },
    ],
  },
};

// Logo visibility test - both states
export const LogoVisibilityTest: Story = {
  args: {
    logo: (
      <Box
        sx={{
          width: 40,
          height: 40,
          borderRadius: 1,
          background: 'linear-gradient(45deg, #cd00ef, #f4009e)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'white',
          fontWeight: 'bold',
          fontSize: '1.2rem',
        }}
      >
        AI
      </Box>
    ),
    user: {
      name: 'Test User',
      email: 'test@example.com',
    },
    appTitle: 'Logo Test',
  },
};
