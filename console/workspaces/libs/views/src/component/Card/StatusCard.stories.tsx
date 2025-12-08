import type { Meta, StoryObj } from '@storybook/react';
import { StatusCard, StatusCardPresets } from './StatusCard';
import { 
  Build as BuildIcon, 
  CheckCircle as CheckCircleIcon, 
  Timer as TimerIcon,
  Error as ErrorIcon,
  Warning as WarningIcon 
} from '@wso2/oxygen-ui-icons-react';

const meta: Meta<typeof StatusCard> = {
  title: 'Components/Card/StatusCard',
  component: StatusCard,
  parameters: {
    layout: 'centered',
  },
  argTypes: {
    title: {
      control: 'text',
    },
    value: {
      control: 'text',
    },
    subtitle: {
      control: 'text',
    },
    icon: {
      control: false,
    },
    iconVariant: {
      control: 'select',
      options: ['primary', 'secondary', 'success', 'warning', 'error', 'info'],
    },
    tag: {
      control: 'text',
    },
    tagVariant: {
      control: 'select',
      options: ['success', 'warning', 'error', 'info', 'default'],
    },
    clickable: {
      control: 'boolean',
    },
    onClick: {
      action: 'clicked',
    },
  },
};

export default meta;
type Story = StoryObj<typeof StatusCard>;

export const Default: Story = {
  args: {
    title: 'Latest Build',
    value: 'v2.1.3',
    subtitle: '3 days ago',
    icon: <BuildIcon />,
    tag: 'Active',
    tagVariant: 'default',
  },
};

export const BuildStatus: Story = {
  args: {
    title: 'Latest Build',
    value: 'v2.1.3',
    subtitle: '3 days ago',
    icon: <BuildIcon />,
    iconVariant: StatusCardPresets.buildStatus.iconVariant,
    tag: 'Active',
    tagVariant: StatusCardPresets.buildStatus.tagVariant,
  },
};

export const SuccessRate: Story = {
  args: {
    title: 'Success Rate',
    value: '47/47',
    subtitle: 'last 30 days',
    icon: <CheckCircleIcon />,
    iconVariant: StatusCardPresets.successRate.iconVariant,
    tag: '100%',
    tagVariant: StatusCardPresets.successRate.tagVariant,
  },
};

export const BuildTime: Story = {
  args: {
    title: 'Build Time',
    value: '3m 42s',
    subtitle: 'avg duration',
    icon: <TimerIcon />,
    iconVariant: StatusCardPresets.buildTime.iconVariant,
    tag: 'Avg',
    tagVariant: StatusCardPresets.buildTime.tagVariant,
  },
};

export const ErrorStatus: Story = {
  args: {
    title: 'Failed Builds',
    value: '5',
    subtitle: 'last 7 days',
    icon: <ErrorIcon />,
    iconVariant: StatusCardPresets.error.iconVariant,
    tag: 'Critical',
    tagVariant: StatusCardPresets.error.tagVariant,
  },
};

export const WarningStatus: Story = {
  args: {
    title: 'Pending Reviews',
    value: '12',
    subtitle: 'requires attention',
    icon: <WarningIcon />,
    iconVariant: StatusCardPresets.warning.iconVariant,
    tag: 'Pending',
    tagVariant: StatusCardPresets.warning.tagVariant,
  },
};

export const ClickableCard: Story = {
  args: {
    title: 'Deployments',
    value: '24',
    subtitle: 'this month',
    icon: <BuildIcon />,
    tag: 'Live',
    tagVariant: 'success',
    clickable: true,
    onClick: () => alert('Card clicked!'),
  },
};

export const NoTag: Story = {
  args: {
    title: 'Total Users',
    value: '1,234',
    subtitle: 'registered users',
    icon: <CheckCircleIcon />,
  },
};

export const MultipleCards: Story = {
  render: () => (
    <div style={{ display: 'flex', gap: '16px', flexWrap: 'wrap' }}>
      <StatusCard
        title="Latest Build"
        value="v2.1.3"
        subtitle="3 days ago"
        icon={<BuildIcon />}
        iconVariant={StatusCardPresets.buildStatus.iconVariant}
        tag="Active"
        tagVariant={StatusCardPresets.buildStatus.tagVariant}
      />
      <StatusCard
        title="Success Rate"
        value="47/47"
        subtitle="last 30 days"
        icon={<CheckCircleIcon />}
        iconVariant={StatusCardPresets.successRate.iconVariant}
        tag="100%"
        tagVariant={StatusCardPresets.successRate.tagVariant}
      />
      <StatusCard
        title="Build Time"
        value="3m 42s"
        subtitle="avg duration"
        icon={<TimerIcon />}
        iconVariant={StatusCardPresets.buildTime.iconVariant}
        tag="Avg"
        tagVariant={StatusCardPresets.buildTime.tagVariant}
      />
    </div>
  ),
};

export const DarkTheme: Story = {
  args: {
    title: 'System Health',
    value: '98.5%',
    subtitle: 'uptime',
    icon: <CheckCircleIcon />,
    tag: 'Excellent',
    tagVariant: 'success',
  },
  parameters: {
    backgrounds: {
      default: 'dark',
    },
  },
};
