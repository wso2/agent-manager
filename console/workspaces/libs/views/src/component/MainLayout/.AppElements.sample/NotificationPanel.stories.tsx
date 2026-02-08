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
import { NotificationPanel, Box, Button, Avatar, Typography } from '@wso2/oxygen-ui';
import {
  Bell,
  AlertTriangle,
  Info,
  CheckCircle,
  MessageSquare,
  User,
  Settings,
} from '@wso2/oxygen-ui-icons-react';

/**
 * NotificationPanel is a compound component for building notification drawers.
 * It provides a flexible, composable API for creating notification panels with
 * headers, tabs, action buttons, and notification lists.
 */
const meta: Meta<typeof NotificationPanel> = {
  title: 'App Elements/NotificationPanel',
  component: NotificationPanel,
  tags: ['autodocs'],
  parameters: {
    layout: 'centered',
    docs: {
      description: {
        component: `
The NotificationPanel component is a slide-out drawer for displaying notifications.
It uses the compound component pattern for maximum flexibility.

### Sub-components
- \`NotificationPanel.Header\` - Panel header container
- \`NotificationPanel.HeaderIcon\` - Icon in the header
- \`NotificationPanel.HeaderTitle\` - Title text
- \`NotificationPanel.HeaderBadge\` - Badge/count in the header
- \`NotificationPanel.HeaderClose\` - Close button
- \`NotificationPanel.Tabs\` - Tab navigation for filtering
- \`NotificationPanel.Actions\` - Bulk action buttons
- \`NotificationPanel.List\` - Scrollable notification list
- \`NotificationPanel.Item\` - Individual notification item
- \`NotificationPanel.ItemAvatar\` - Avatar for item
- \`NotificationPanel.ItemTitle\` - Title for item
- \`NotificationPanel.ItemMessage\` - Message for item
- \`NotificationPanel.ItemTimestamp\` - Timestamp for item
- \`NotificationPanel.ItemAction\` - Action button for item
- \`NotificationPanel.EmptyState\` - Empty state display

### Usage
\`\`\`tsx
import { NotificationPanel } from '@wso2/oxygen-ui';

<NotificationPanel open={open} onClose={handleClose}>
  <NotificationPanel.Header>
    <NotificationPanel.HeaderIcon><Bell size={20} /></NotificationPanel.HeaderIcon>
    <NotificationPanel.HeaderTitle>Notifications</NotificationPanel.HeaderTitle>
    <NotificationPanel.HeaderBadge>{count}</NotificationPanel.HeaderBadge>
    <NotificationPanel.HeaderClose />
  </NotificationPanel.Header>
  <NotificationPanel.List>
    {notifications.map(n => (
      <NotificationPanel.Item key={n.id} id={n.id} type={n.type}>
        <NotificationPanel.ItemAvatar>{n.avatar}</NotificationPanel.ItemAvatar>
        <NotificationPanel.ItemTitle>{n.title}</NotificationPanel.ItemTitle>
        <NotificationPanel.ItemMessage>{n.message}</NotificationPanel.ItemMessage>
        <NotificationPanel.ItemTimestamp>{n.timestamp}</NotificationPanel.ItemTimestamp>
      </NotificationPanel.Item>
    ))}
  </NotificationPanel.List>
</NotificationPanel>
\`\`\`
        `,
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof NotificationPanel>;

// Sample notification data
const sampleNotifications = [
  {
    id: '1',
    type: 'info' as const,
    title: 'System Update',
    message: 'A new version of the application is available. Please refresh to update.',
    timestamp: '5 minutes ago',
    avatar: 'SU',
    read: false,
  },
  {
    id: '2',
    type: 'warning' as const,
    title: 'API Rate Limit',
    message: 'You are approaching your API rate limit. Consider upgrading your plan.',
    timestamp: '1 hour ago',
    avatar: 'RL',
    read: false,
  },
  {
    id: '3',
    type: 'success' as const,
    title: 'Deployment Complete',
    message: 'Your application has been successfully deployed to production.',
    timestamp: '2 hours ago',
    avatar: 'DC',
    read: true,
  },
  {
    id: '4',
    type: 'error' as const,
    title: 'Build Failed',
    message: 'The latest build failed. Check the logs for more details.',
    timestamp: '3 hours ago',
    avatar: 'BF',
    read: true,
  },
];

/**
 * Basic notification panel with header and list.
 */
export const Default: Story = {
  render: () => {
    const [open, setOpen] = React.useState(false);

    return (
      <Box>
        <Button variant="outlined" onClick={() => setOpen(true)}>
          Open Notifications
        </Button>
        <NotificationPanel open={open} onClose={() => setOpen(false)}>
          <NotificationPanel.Header>
            <NotificationPanel.HeaderIcon><Bell size={20} /></NotificationPanel.HeaderIcon>
            <NotificationPanel.HeaderTitle>Notifications</NotificationPanel.HeaderTitle>
            <NotificationPanel.HeaderBadge>4</NotificationPanel.HeaderBadge>
            <NotificationPanel.HeaderClose />
          </NotificationPanel.Header>
          <NotificationPanel.List>
            {sampleNotifications.map((n) => (
              <NotificationPanel.Item key={n.id} id={n.id} type={n.type}>
                <NotificationPanel.ItemAvatar>
                  <Avatar sx={{ width: 32, height: 32, fontSize: 12 }}>{n.avatar}</Avatar>
                </NotificationPanel.ItemAvatar>
                <NotificationPanel.ItemTitle>{n.title}</NotificationPanel.ItemTitle>
                <NotificationPanel.ItemMessage>{n.message}</NotificationPanel.ItemMessage>
                <NotificationPanel.ItemTimestamp>{n.timestamp}</NotificationPanel.ItemTimestamp>
              </NotificationPanel.Item>
            ))}
          </NotificationPanel.List>
        </NotificationPanel>
      </Box>
    );
  },
};

/**
 * Notification panel with tabs for filtering.
 */
export const WithTabs: Story = {
  render: () => {
    const [open, setOpen] = React.useState(false);
    const [tabIndex, setTabIndex] = React.useState(0);

    const unreadNotifications = sampleNotifications.filter((n) => !n.read);
    const alertNotifications = sampleNotifications.filter((n) => n.type === 'warning' || n.type === 'error');

    const getFilteredNotifications = () => {
      switch (tabIndex) {
        case 1:
          return unreadNotifications;
        case 2:
          return alertNotifications;
        default:
          return sampleNotifications;
      }
    };

    return (
      <Box>
        <Button variant="outlined" onClick={() => setOpen(true)}>
          Open Notifications
        </Button>
        <NotificationPanel open={open} onClose={() => setOpen(false)}>
          <NotificationPanel.Header>
            <NotificationPanel.HeaderIcon><Bell size={20} /></NotificationPanel.HeaderIcon>
            <NotificationPanel.HeaderTitle>Notifications</NotificationPanel.HeaderTitle>
            <NotificationPanel.HeaderBadge>{sampleNotifications.length}</NotificationPanel.HeaderBadge>
            <NotificationPanel.HeaderClose />
          </NotificationPanel.Header>
          <NotificationPanel.Tabs
            tabs={[
              { label: 'All', count: sampleNotifications.length },
              { label: 'Unread', count: unreadNotifications.length, color: 'primary' },
              { label: 'Alerts', count: alertNotifications.length, color: 'warning' },
            ]}
            value={tabIndex}
            onChange={setTabIndex}
          />
          <NotificationPanel.List>
            {getFilteredNotifications().map((n) => (
              <NotificationPanel.Item key={n.id} id={n.id} type={n.type}>
                <NotificationPanel.ItemAvatar>
                  <Avatar sx={{ width: 32, height: 32, fontSize: 12 }}>{n.avatar}</Avatar>
                </NotificationPanel.ItemAvatar>
                <NotificationPanel.ItemTitle>{n.title}</NotificationPanel.ItemTitle>
                <NotificationPanel.ItemMessage>{n.message}</NotificationPanel.ItemMessage>
                <NotificationPanel.ItemTimestamp>{n.timestamp}</NotificationPanel.ItemTimestamp>
              </NotificationPanel.Item>
            ))}
          </NotificationPanel.List>
        </NotificationPanel>
      </Box>
    );
  },
};

/**
 * Notification panel with action buttons.
 */
export const WithActions: Story = {
  render: () => {
    const [open, setOpen] = React.useState(false);
    const [notifications, setNotifications] = React.useState(sampleNotifications);

    const hasUnread = notifications.some((n) => !n.read);

    const handleMarkAllRead = () => {
      setNotifications(notifications.map((n) => ({ ...n, read: true })));
    };

    const handleClearAll = () => {
      setNotifications([]);
    };

    return (
      <Box>
        <Button variant="outlined" onClick={() => setOpen(true)}>
          Open Notifications
        </Button>
        <NotificationPanel open={open} onClose={() => setOpen(false)}>
          <NotificationPanel.Header>
            <NotificationPanel.HeaderIcon><Bell size={20} /></NotificationPanel.HeaderIcon>
            <NotificationPanel.HeaderTitle>Notifications</NotificationPanel.HeaderTitle>
            <NotificationPanel.HeaderBadge>{notifications.length}</NotificationPanel.HeaderBadge>
            <NotificationPanel.HeaderClose />
          </NotificationPanel.Header>
          <NotificationPanel.Actions
            hasUnread={hasUnread}
            onMarkAllRead={handleMarkAllRead}
            onClearAll={handleClearAll}
          />
          {notifications.length === 0 ? (
            <NotificationPanel.EmptyState />
          ) : (
            <NotificationPanel.List>
              {notifications.map((n) => (
                <NotificationPanel.Item key={n.id} id={n.id} type={n.type}>
                  <NotificationPanel.ItemAvatar>
                    <Avatar sx={{ width: 32, height: 32, fontSize: 12 }}>{n.avatar}</Avatar>
                  </NotificationPanel.ItemAvatar>
                  <NotificationPanel.ItemTitle>{n.title}</NotificationPanel.ItemTitle>
                  <NotificationPanel.ItemMessage>{n.message}</NotificationPanel.ItemMessage>
                  <NotificationPanel.ItemTimestamp>{n.timestamp}</NotificationPanel.ItemTimestamp>
                </NotificationPanel.Item>
              ))}
            </NotificationPanel.List>
          )}
        </NotificationPanel>
      </Box>
    );
  },
};

/**
 * Notification items with action buttons.
 */
export const WithItemActions: Story = {
  render: () => {
    const [open, setOpen] = React.useState(false);

    return (
      <Box>
        <Button variant="outlined" onClick={() => setOpen(true)}>
          Open Notifications
        </Button>
        <NotificationPanel open={open} onClose={() => setOpen(false)}>
          <NotificationPanel.Header>
            <NotificationPanel.HeaderIcon><Bell size={20} /></NotificationPanel.HeaderIcon>
            <NotificationPanel.HeaderTitle>Notifications</NotificationPanel.HeaderTitle>
            <NotificationPanel.HeaderClose />
          </NotificationPanel.Header>
          <NotificationPanel.List>
            {sampleNotifications.map((n) => (
              <NotificationPanel.Item key={n.id} id={n.id} type={n.type}>
                <NotificationPanel.ItemAvatar>
                  <Avatar sx={{ width: 32, height: 32, fontSize: 12 }}>{n.avatar}</Avatar>
                </NotificationPanel.ItemAvatar>
                <NotificationPanel.ItemTitle>{n.title}</NotificationPanel.ItemTitle>
                <NotificationPanel.ItemMessage>{n.message}</NotificationPanel.ItemMessage>
                <NotificationPanel.ItemTimestamp>{n.timestamp}</NotificationPanel.ItemTimestamp>
                <NotificationPanel.ItemAction onClick={() => console.log('View', n.id)}>
                  View Details
                </NotificationPanel.ItemAction>
              </NotificationPanel.Item>
            ))}
          </NotificationPanel.List>
        </NotificationPanel>
      </Box>
    );
  },
};

/**
 * Empty notification panel.
 */
export const EmptyState: Story = {
  render: () => {
    const [open, setOpen] = React.useState(false);

    return (
      <Box>
        <Button variant="outlined" onClick={() => setOpen(true)}>
          Open Notifications
        </Button>
        <NotificationPanel open={open} onClose={() => setOpen(false)}>
          <NotificationPanel.Header>
            <NotificationPanel.HeaderIcon><Bell size={20} /></NotificationPanel.HeaderIcon>
            <NotificationPanel.HeaderTitle>Notifications</NotificationPanel.HeaderTitle>
            <NotificationPanel.HeaderClose />
          </NotificationPanel.Header>
          <NotificationPanel.EmptyState />
        </NotificationPanel>
      </Box>
    );
  },
};

/**
 * Left-anchored notification panel.
 */
export const LeftAnchored: Story = {
  render: () => {
    const [open, setOpen] = React.useState(false);

    return (
      <Box>
        <Button variant="outlined" onClick={() => setOpen(true)}>
          Open Left Panel
        </Button>
        <NotificationPanel open={open} onClose={() => setOpen(false)} anchor="left">
          <NotificationPanel.Header>
            <NotificationPanel.HeaderIcon><Bell size={20} /></NotificationPanel.HeaderIcon>
            <NotificationPanel.HeaderTitle>Notifications</NotificationPanel.HeaderTitle>
            <NotificationPanel.HeaderBadge>4</NotificationPanel.HeaderBadge>
            <NotificationPanel.HeaderClose />
          </NotificationPanel.Header>
          <NotificationPanel.List>
            {sampleNotifications.map((n) => (
              <NotificationPanel.Item key={n.id} id={n.id} type={n.type}>
                <NotificationPanel.ItemAvatar>
                  <Avatar sx={{ width: 32, height: 32, fontSize: 12 }}>{n.avatar}</Avatar>
                </NotificationPanel.ItemAvatar>
                <NotificationPanel.ItemTitle>{n.title}</NotificationPanel.ItemTitle>
                <NotificationPanel.ItemMessage>{n.message}</NotificationPanel.ItemMessage>
                <NotificationPanel.ItemTimestamp>{n.timestamp}</NotificationPanel.ItemTimestamp>
              </NotificationPanel.Item>
            ))}
          </NotificationPanel.List>
        </NotificationPanel>
      </Box>
    );
  },
};
