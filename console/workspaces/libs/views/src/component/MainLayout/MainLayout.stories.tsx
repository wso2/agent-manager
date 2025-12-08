import type { Meta, StoryObj } from '@storybook/react';
import { Box, Button, IconButton, Badge } from '@wso2/oxygen-ui';
import { Notifications, Search, Brightness4 } from '@wso2/oxygen-ui-icons-react';
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

// MainLayout with custom navigation items
export const WithCustomNavigation: Story = {
  args: {
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

// MainLayout with collapsed sidebar
export const CollapsedSidebar: Story = {
  args: {
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

// Logo visibility test story removed because logo is now always shown by default.
