import type { Meta, StoryObj } from '@storybook/react';
import { Card } from './Card';
import { 
  Typography, 
  Button, 
  Chip, 
  Box, 
  Avatar, 
  CardActions, 
  CardHeader,
  IconButton,
  Stack
} from '@wso2/oxygen-ui';
import { 
  PlayArrow, 
  Settings, 
  MoreVert,
  CheckCircle,
  Error
} from '@wso2/oxygen-ui-icons-react';

const meta: Meta<typeof Card> = {
  title: 'AI Agent Management/Views/Card',
  component: Card,
  argTypes: {
    className: {
      control: 'text',
      description: 'CSS class name for styling',
    },
    children: {
      control: 'text',
      description: 'Content to display inside the card',
    },
    variant: {
      control: { type: 'select' },
      options: ['elevation', 'outlined'],
      description: 'The variant to use',
    },
    elevation: {
      control: { type: 'number', min: 0, max: 24 },
      description: 'Shadow depth, corresponds to dp in the Material Design specification',
    },
    sx: {
      control: 'object',
      description: 'The system prop that allows defining system overrides as well as additional CSS styles',
    },
  },
};

export default meta;
type Story = StoryObj<typeof Card>;

export const Default: Story = {
  args: {
    children: (
      <Typography variant="body1">
        This is a basic card with some content.
      </Typography>
    ),
  },
};

export const WithCustomClassName: Story = {
  args: {
    className: 'max-w-md',
    children: (
      <Typography variant="body1">
        This card has custom styling applied via className.
      </Typography>
    ),
  },
};

export const AIAgentCard: Story = {
  args: {
    variant: 'outlined',
    children: (
      <Box>
        <CardHeader
          avatar={
            <Avatar sx={{ bgcolor: 'primary.main' }}>
              <Settings />
            </Avatar>
          }
          title="AI Agent - Customer Support Bot"
          subheader="GPT-4 Powered Assistant"
          action={
            <IconButton aria-label="settings">
              <MoreVert />
            </IconButton>
          }
        />
        <Box sx={{ px: 2, pb: 1 }}>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            This AI agent handles customer inquiries and provides automated support responses.
          </Typography>
          <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
            <Chip 
              icon={<CheckCircle />} 
              label="Active" 
              color="success" 
              size="small" 
            />
            <Chip 
              label="GPT-4" 
              variant="outlined" 
              size="small" 
            />
          </Stack>
        </Box>
        <CardActions>
          <Button size="small" startIcon={<PlayArrow />}>
            Start
          </Button>
          <Button size="small" startIcon={<Settings />}>
            Configure
          </Button>
        </CardActions>
      </Box>
    ),
  },
};

export const AgentStatusCard: Story = {
  args: {
    variant: 'elevation',
    elevation: 3,
    children: (
      <Box>
        <CardHeader
          avatar={
            <Avatar sx={{ bgcolor: 'success.main' }}>
              <CheckCircle />
            </Avatar>
          }
          title="Agent Status"
          subheader="System Health Monitor"
        />
        <Box sx={{ px: 2, pb: 1 }}>
          <Stack direction="row" spacing={2} alignItems="center" sx={{ mb: 2 }}>
            <Chip 
              icon={<CheckCircle />} 
              label="Healthy" 
              color="success" 
              variant="filled"
            />
            <Typography variant="body2" color="text.secondary">
              Uptime: 99.9%
            </Typography>
          </Stack>
          <Typography variant="body2" color="text.secondary">
            Last updated: {new Date().toLocaleString()}
          </Typography>
        </Box>
      </Box>
    ),
  },
};

export const AgentErrorCard: Story = {
  args: {
    variant: 'outlined',
    sx: { borderColor: 'error.main' },
    children: (
      <Box>
        <CardHeader
          avatar={
            <Avatar sx={{ bgcolor: 'error.main' }}>
              <Error />
            </Avatar>
          }
          title="Agent Error"
          subheader="Connection Failed"
        />
        <Box sx={{ px: 2, pb: 1 }}>
          <Typography variant="body2" color="error.main" sx={{ mb: 2 }}>
            Unable to connect to the AI service. Please check your configuration.
          </Typography>
          <Stack direction="row" spacing={1}>
            <Chip 
              icon={<Error />} 
              label="Error" 
              color="error" 
              size="small" 
            />
            <Chip 
              label="Retry Available" 
              variant="outlined" 
              size="small" 
            />
          </Stack>
        </Box>
        <CardActions>
          <Button size="small" color="error" startIcon={<PlayArrow />}>
            Retry Connection
          </Button>
          <Button size="small" startIcon={<Settings />}>
            Fix Configuration
          </Button>
        </CardActions>
      </Box>
    ),
  },
};

export const AgentMetricsCard: Story = {
  args: {
    variant: 'elevation',
    elevation: 2,
    children: (
      <Box>
        <CardHeader
          title="Performance Metrics"
          subheader="Last 24 hours"
        />
        <Box sx={{ px: 2, pb: 1 }}>
          <Stack spacing={2}>
            <Box>
              <Typography variant="h6" color="primary">
                1,247
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Requests Processed
              </Typography>
            </Box>
            <Box>
              <Typography variant="h6" color="success.main">
                98.5%
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Success Rate
              </Typography>
            </Box>
            <Box>
              <Typography variant="h6" color="warning.main">
                245ms
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Average Response Time
              </Typography>
            </Box>
          </Stack>
        </Box>
        <CardActions>
          <Button size="small">
            View Detailed Report
          </Button>
        </CardActions>
      </Box>
    ),
  },
};

export const ThemeShowcaseCard: Story = {
  args: {
    variant: 'outlined',
    children: (
      <Box>
        <CardHeader
          title="Custom Theme Showcase"
          subheader="AI Agent Management Platform Colors"
        />
        <Box sx={{ px: 2, pb: 1 }}>
          <Stack spacing={2}>
            <Box>
              <Typography variant="h6" sx={{ color: 'primary.main' }}>
                Primary: #cd00ef
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Main brand color for AI agents
              </Typography>
            </Box>
            <Box>
              <Typography variant="h6" sx={{ color: 'secondary.main' }}>
                Secondary: #f4009e
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Accent color for highlights
              </Typography>
            </Box>
            <Stack direction="row" spacing={1}>
              <Chip 
                label="Primary" 
                sx={{ bgcolor: 'primary.main', color: 'white' }}
                size="small"
              />
              <Chip 
                label="Secondary" 
                sx={{ bgcolor: 'secondary.main', color: 'white' }}
                size="small"
              />
              <Chip 
                label="Success" 
                color="success"
                size="small"
              />
              <Chip 
                label="Warning" 
                color="warning"
                size="small"
              />
            </Stack>
          </Stack>
        </Box>
        <CardActions>
          <Button 
            size="small" 
            variant="contained" 
            sx={{ 
              bgcolor: 'primary.main',
              '&:hover': { bgcolor: 'primary.dark' }
            }}
          >
            Primary Action
          </Button>
          <Button 
            size="small" 
            variant="contained" 
            sx={{ 
              bgcolor: 'secondary.main',
              '&:hover': { bgcolor: 'secondary.dark' }
            }}
          >
            Secondary Action
          </Button>
        </CardActions>
      </Box>
    ),
  },
};
