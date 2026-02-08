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
import { NotificationBanner, Box, Button } from '@wso2/oxygen-ui';

/**
 * NotificationBanner is a dismissible system alert banner for displaying
 * important announcements or notifications at the top of the application.
 */
const meta: Meta<typeof NotificationBanner> = {
  title: 'App Elements/NotificationBanner',
  component: NotificationBanner,
  tags: ['autodocs'],
  parameters: {
    layout: 'fullscreen',
    docs: {
      description: {
        component: `
The NotificationBanner component displays system-wide notifications at the top of the application.

### Features
- Multiple severity levels (info, warning, error, success)
- Optional title for more prominent messages
- Optional action button
- Smooth collapse animation on dismiss
- Full-width design that sits at the top of the app

### Usage
\`\`\`tsx
import { NotificationBanner } from '@wso2/oxygen-ui';

<NotificationBanner
  visible={showBanner}
  severity="info"
  title="Scheduled Maintenance"
  message="The system will be undergoing maintenance on Dec 20th."
  actionLabel="Learn More"
  onAction={() => openMaintenanceInfo()}
  onDismiss={() => setShowBanner(false)}
/>
\`\`\`
        `,
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof NotificationBanner>;

/**
 * Info banner (default).
 */
export const Info: Story = {
  render: () => (
    <NotificationBanner
      severity="info"
      message="A new version of the application is available. Refresh to update."
      onDismiss={() => console.log('Dismissed')}
    />
  ),
};

/**
 * Info banner with title.
 */
export const InfoWithTitle: Story = {
  render: () => (
    <NotificationBanner
      severity="info"
      title="System Update"
      message="A new version of the application is available. Refresh to update."
      onDismiss={() => console.log('Dismissed')}
    />
  ),
};

/**
 * Warning banner.
 */
export const Warning: Story = {
  render: () => (
    <NotificationBanner
      severity="warning"
      title="Scheduled Maintenance"
      message="The system will undergo maintenance on January 15th from 2:00 AM to 4:00 AM UTC."
      onDismiss={() => console.log('Dismissed')}
    />
  ),
};

/**
 * Error banner.
 */
export const Error: Story = {
  render: () => (
    <NotificationBanner
      severity="error"
      title="Service Disruption"
      message="Some features may be unavailable due to ongoing issues. Our team is working on a fix."
      onDismiss={() => console.log('Dismissed')}
    />
  ),
};

/**
 * Success banner.
 */
export const Success: Story = {
  render: () => (
    <NotificationBanner
      severity="success"
      title="Upgrade Complete"
      message="Your account has been successfully upgraded to the Pro plan."
      onDismiss={() => console.log('Dismissed')}
    />
  ),
};

/**
 * Banner with action button.
 */
export const WithAction: Story = {
  render: () => (
    <NotificationBanner
      severity="info"
      title="New Feature Available"
      message="Check out our new dashboard analytics feature."
      actionLabel="Learn More"
      onAction={() => console.log('Learn more clicked')}
      onDismiss={() => console.log('Dismissed')}
    />
  ),
};

/**
 * Controllable banner with visibility toggle.
 */
export const Controllable: Story = {
  render: () => {
    const [visible, setVisible] = React.useState(true);

    return (
      <Box>
        <NotificationBanner
          visible={visible}
          severity="warning"
          title="Trial Ending Soon"
          message="Your trial period ends in 3 days. Upgrade now to keep access to all features."
          actionLabel="Upgrade Now"
          onAction={() => console.log('Upgrade clicked')}
          onDismiss={() => setVisible(false)}
        />
        <Box sx={{ p: 2 }}>
          {!visible && (
            <Button variant="outlined" onClick={() => setVisible(true)}>
              Show Banner Again
            </Button>
          )}
        </Box>
      </Box>
    );
  },
};

/**
 * Multiple banners stacked.
 */
export const MultipleBanners: Story = {
  render: () => {
    const [showInfo, setShowInfo] = React.useState(true);
    const [showWarning, setShowWarning] = React.useState(true);

    return (
      <Box>
        <NotificationBanner
          visible={showInfo}
          severity="info"
          message="New feature: Dark mode is now available in settings."
          onDismiss={() => setShowInfo(false)}
        />
        <NotificationBanner
          visible={showWarning}
          severity="warning"
          message="Your payment method will expire soon. Please update your billing information."
          actionLabel="Update"
          onAction={() => console.log('Update clicked')}
          onDismiss={() => setShowWarning(false)}
        />
        <Box sx={{ p: 2 }}>
          <Button
            variant="outlined"
            onClick={() => {
              setShowInfo(true);
              setShowWarning(true);
            }}
            disabled={showInfo && showWarning}
          >
            Reset Banners
          </Button>
        </Box>
      </Box>
    );
  },
};

/**
 * Simple message without title.
 */
export const SimpleMessage: Story = {
  render: () => (
    <NotificationBanner
      severity="success"
      message="Your changes have been saved successfully."
      onDismiss={() => console.log('Dismissed')}
    />
  ),
};
