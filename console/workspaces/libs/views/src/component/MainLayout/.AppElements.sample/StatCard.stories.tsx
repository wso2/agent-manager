/*
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

import type { Meta, StoryObj } from '@storybook/react';
import { StatCard, Box, Grid } from '@wso2/oxygen-ui';
import {
  Users,
  TrendingUp,
  TrendingDown,
  DollarSign,
  Activity,
  ShoppingCart,
  Eye,
  Clock,
} from '@wso2/oxygen-ui-icons-react';
import React from 'react';

/**
 * StatCard is a specialized card component for displaying statistics and metrics.
 * It provides a consistent layout with an icon, value, and label.
 */
const meta: Meta<typeof StatCard> = {
  title: 'App Elements/Stat Card',
  component: StatCard,
  tags: ['autodocs'],
  parameters: {
    layout: 'centered',
    docs: {
      description: {
        component: `
The StatCard component is designed for displaying key metrics and statistics in a visually
appealing and consistent format. It extends the MUI Card component, so you can use all Card props.

### Features
- **Icon Support**: Display an optional icon with the statistic
- **Color Variants**: Icon can use theme color variants (primary, secondary, success, error, info, warning)
- **Flexible Values**: Accepts string or number values
- **Card Extension**: Inherits all Card props for full customization
- **Responsive**: Works well in grid layouts for dashboard statistics

### Usage
\`\`\`tsx
import { StatCard } from '@wso2/oxygen-ui';
import { Users } from '@wso2/oxygen-ui-icons-react';

<StatCard
  value={1234}
  label="Total Users"
  icon={<Users size={24} />}
  iconColor="primary"
/>
\`\`\`
        `,
      },
    },
  },
  argTypes: {
    value: {
      control: 'text',
      description: 'The statistic value to display',
      table: {
        type: { summary: 'string | number' },
      },
    },
    label: {
      control: 'text',
      description: 'The label/description for the statistic',
    },
    icon: {
      control: false,
      description: 'Optional icon element to display',
    },
    iconColor: {
      control: 'select',
      options: ['primary', 'secondary', 'success', 'error', 'info', 'warning'],
      description: 'Color variant for the icon',
      table: {
        type: { summary: 'primary | secondary | success | error | info | warning' },
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof StatCard>;

/**
 * Default usage with a primary icon.
 */
export const Default: Story = {
  args: {
    value: 1234,
    label: 'Total Users',
    icon: <Users size={24} />,
    iconColor: 'primary',
  },
};

/**
 * Grid layout showing multiple stat cards with different color variants.
 */
export const ColorVariants: Story = {
  render: () => (
    <Grid container spacing={3} sx={{ width: 900 }}>
      <Grid size={{ xs: 12, sm: 6, md: 4 }}>
        <StatCard
          value="2,451"
          label="Total Users"
          icon={<Users size={24} />}
          iconColor="primary"
        />
      </Grid>
      <Grid size={{ xs: 12, sm: 6, md: 4 }}>
        <StatCard
          value="$45,231"
          label="Revenue"
          icon={<DollarSign size={24} />}
          iconColor="success"
        />
      </Grid>
      <Grid size={{ xs: 12, sm: 6, md: 4 }}>
        <StatCard
          value="156"
          label="Active Sessions"
          icon={<Activity size={24} />}
          iconColor="info"
        />
      </Grid>
      <Grid size={{ xs: 12, sm: 6, md: 4 }}>
        <StatCard
          value="89"
          label="Pending Orders"
          icon={<ShoppingCart size={24} />}
          iconColor="warning"
        />
      </Grid>
      <Grid size={{ xs: 12, sm: 6, md: 4 }}>
        <StatCard
          value="23"
          label="Critical Alerts"
          icon={<TrendingDown size={24} />}
          iconColor="error"
        />
      </Grid>
      <Grid size={{ xs: 12, sm: 6, md: 4 }}>
        <StatCard
          value="12.5K"
          label="Page Views"
          icon={<Eye size={24} />}
          iconColor="secondary"
        />
      </Grid>
    </Grid>
  ),
};

/**
 * Stat cards without icons for a minimal look.
 */
export const WithoutIcon: Story = {
  render: () => (
    <Grid container spacing={3} sx={{ width: 600 }}>
      <Grid size={{ xs: 12, sm: 6 }}>
        <StatCard value="98.5%" label="Uptime" />
      </Grid>
      <Grid size={{ xs: 12, sm: 6 }}>
        <StatCard value="2.3s" label="Avg Response Time" />
      </Grid>
      <Grid size={{ xs: 12, sm: 6 }}>
        <StatCard value="847" label="API Calls" />
      </Grid>
      <Grid size={{ xs: 12, sm: 6 }}>
        <StatCard value="15" label="Active Projects" />
      </Grid>
    </Grid>
  ),
};

/**
 * Dashboard example with growth metrics.
 */
export const DashboardMetrics: Story = {
  render: () => (
    <Grid container spacing={3} sx={{ width: 900 }}>
      <Grid size={{ xs: 12, sm: 6, md: 3 }}>
        <StatCard
          value="+23%"
          label="Growth Rate"
          icon={<TrendingUp size={24} />}
          iconColor="success"
        />
      </Grid>
      <Grid size={{ xs: 12, sm: 6, md: 3 }}>
        <StatCard
          value="3.2K"
          label="New Users"
          icon={<Users size={24} />}
          iconColor="primary"
        />
      </Grid>
      <Grid size={{ xs: 12, sm: 6, md: 3 }}>
        <StatCard
          value="$128K"
          label="Total Revenue"
          icon={<DollarSign size={24} />}
          iconColor="success"
        />
      </Grid>
      <Grid size={{ xs: 12, sm: 6, md: 3 }}>
        <StatCard
          value="24/7"
          label="Support Available"
          icon={<Clock size={24} />}
          iconColor="info"
        />
      </Grid>
    </Grid>
  ),
};

/**
 * Custom styled stat card with elevation and custom props.
 */
export const CustomStyled: Story = {
  render: () => (
    <Grid container spacing={3} sx={{ width: 600 }}>
      <Grid size={{ xs: 12, sm: 6 }}>
        <StatCard
          value="1,845"
          label="Downloads"
          icon={<TrendingUp size={24} />}
          iconColor="primary"
          elevation={4}
          sx={{ bgcolor: 'primary.light', color: 'primary.contrastText' }}
        />
      </Grid>
      <Grid size={{ xs: 12, sm: 6 }}>
        <StatCard
          value="$45,231"
          label="Revenue"
          icon={<DollarSign size={24} />}
          iconColor="success"
          variant="outlined"
          sx={{ borderWidth: 2, borderColor: 'success.main' }}
        />
      </Grid>
    </Grid>
  ),
};

/**
 * Large format stat cards for key metrics.
 */
export const LargeFormat: Story = {
  render: () => (
    <Grid container spacing={3} sx={{ width: 600 }}>
      <Grid size={{ xs: 12 }}>
        <StatCard
          value="$1,234,567"
          label="Total Revenue This Year"
          icon={<DollarSign size={32} />}
          iconColor="success"
          sx={{
            '& .MuiTypography-h5': {
              fontSize: '2rem',
            },
          }}
        />
      </Grid>
      <Grid size={{ xs: 12 }}>
        <StatCard
          value="15,843"
          label="Active Customers Worldwide"
          icon={<Users size={32} />}
          iconColor="primary"
          sx={{
            '& .MuiTypography-h5': {
              fontSize: '2rem',
            },
          }}
        />
      </Grid>
    </Grid>
  ),
};
