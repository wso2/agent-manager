import type { Meta, StoryObj } from '@storybook/react';
import { MainActionPanel } from './MainActionPanel';
import { 
  Button, 
  Stack, 
  Typography, 
  Box,
  IconButton,
  Divider,
  Chip
} from '@wso2/oxygen-ui';
import { 
  Save, 
  Cancel, 
  Delete, 
  Edit, 
  Add,
  Close,
  CheckCircle,
  Warning
} from '@wso2/oxygen-ui-icons-react';

const meta: Meta<typeof MainActionPanel> = {
  title: 'AI Agent Management/Views/MainActionPanel',
  component: MainActionPanel,
  argTypes: {
    className: {
      control: 'text',
      description: 'CSS class name for styling',
    },
    children: {
      control: 'text',
      description: 'Content to display inside the action panel',
    },
    variant: {
      control: { type: 'select' },
      options: ['elevated', 'outlined', 'filled'],
      description: 'The visual variant of the panel',
    },
    elevation: {
      control: { type: 'number', min: 0, max: 24 },
      description: 'Shadow depth for elevated variant',
    },
    sx: {
      control: 'object',
      description: 'The system prop that allows defining system overrides as well as additional CSS styles',
    },
  },
  parameters: {
    layout: 'fullscreen',
    docs: {
      description: {
        component: 'A fixed bottom action panel component for displaying primary actions and controls. Perfect for forms, wizards, and action-heavy interfaces.',
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof MainActionPanel>;

export const Default: Story = {
  args: {
    children: (
      <Stack direction="row" spacing={2} justifyContent="flex-end">
        <Button variant="outlined" startIcon={<Cancel />}>
          Cancel
        </Button>
        <Button variant="contained" startIcon={<Save />}>
          Save Changes
        </Button>
      </Stack>
    ),
  },
};

export const WithTitle: Story = {
  args: {
    children: (
      <Box>
        <Typography variant="h6" sx={{ mb: 2 }}>
          Agent Configuration
        </Typography>
        <Stack direction="row" spacing={2} justifyContent="space-between" alignItems="center">
          <Typography variant="body2" color="text.secondary">
            3 agents selected
          </Typography>
          <Stack direction="row" spacing={2}>
            <Button variant="outlined" startIcon={<Cancel />}>
              Cancel
            </Button>
            <Button variant="contained" startIcon={<Save />}>
              Save Configuration
            </Button>
          </Stack>
        </Stack>
      </Box>
    ),
  },
};

export const AgentCreationPanel: Story = {
  args: {
    variant: 'elevated',
    elevation: 12,
    children: (
      <Box>
        <Stack direction="row" spacing={2} justifyContent="space-between" alignItems="center" sx={{ mb: 2 }}>
          <Box>
            <Typography variant="h6">
              Create New Agent
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Configure your AI agent settings
            </Typography>
          </Box>
          <IconButton size="small">
            <Close />
          </IconButton>
        </Stack>
        <Divider sx={{ mb: 2 }} />
        <Stack direction="row" spacing={2} justifyContent="space-between" alignItems="center">
          <Stack direction="row" spacing={1}>
            <Chip 
              icon={<CheckCircle />} 
              label="Configuration Complete" 
              color="success" 
              size="small" 
            />
            <Chip 
              label="Ready to Deploy" 
              variant="outlined" 
              size="small" 
            />
          </Stack>
          <Stack direction="row" spacing={2}>
            <Button variant="outlined" startIcon={<Cancel />}>
              Cancel
            </Button>
            <Button variant="outlined" startIcon={<Edit />}>
              Edit
            </Button>
            <Button variant="contained" startIcon={<Add />}>
              Create Agent
            </Button>
          </Stack>
        </Stack>
      </Box>
    ),
  },
};

export const OutlinedVariant: Story = {
  args: {
    variant: 'outlined',
    children: (
      <Stack direction="row" spacing={2} justifyContent="flex-end">
        <Button variant="text" startIcon={<Cancel />}>
          Cancel
        </Button>
        <Button variant="contained" startIcon={<Save />}>
          Save
        </Button>
      </Stack>
    ),
  },
};

export const FilledVariant: Story = {
  args: {
    variant: 'filled',
    children: (
      <Stack direction="row" spacing={2} justifyContent="space-between" alignItems="center">
        <Typography variant="body2" color="text.secondary">
          Changes will be saved automatically
        </Typography>
        <Stack direction="row" spacing={2}>
          <Button variant="outlined" size="small" startIcon={<Cancel />}>
            Discard
          </Button>
          <Button variant="contained" size="small" startIcon={<Save />}>
            Save Now
          </Button>
        </Stack>
      </Stack>
    ),
  },
};

export const DangerActions: Story = {
  args: {
    variant: 'elevated',
    elevation: 6,
    children: (
      <Box>
        <Stack direction="row" spacing={2} alignItems="center" sx={{ mb: 2 }}>
          <Warning color="warning" />
          <Typography variant="h6" color="warning.main">
            Delete Agent
          </Typography>
        </Stack>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          This action cannot be undone. The agent and all its data will be permanently deleted.
        </Typography>
        <Stack direction="row" spacing={2} justifyContent="flex-end">
          <Button variant="outlined" startIcon={<Cancel />}>
            Cancel
          </Button>
          <Button variant="contained" color="error" startIcon={<Delete />}>
            Delete Agent
          </Button>
        </Stack>
      </Box>
    ),
  },
};

export const MultiStepWizard: Story = {
  args: {
    variant: 'elevated',
    elevation: 8,
    children: (
      <Box>
        <Stack direction="row" spacing={2} justifyContent="space-between" alignItems="center" sx={{ mb: 2 }}>
          <Box>
            <Typography variant="h6">
              Agent Setup Wizard
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Step 2 of 4: Configure Environment
            </Typography>
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <Box sx={{ width: 8, height: 8, borderRadius: '50%', bgcolor: 'primary.main' }} />
            <Box sx={{ width: 8, height: 8, borderRadius: '50%', bgcolor: 'primary.main' }} />
            <Box sx={{ width: 8, height: 8, borderRadius: '50%', bgcolor: 'grey.300' }} />
            <Box sx={{ width: 8, height: 8, borderRadius: '50%', bgcolor: 'grey.300' }} />
          </Box>
        </Stack>
        <Divider sx={{ mb: 2 }} />
        <Stack direction="row" spacing={2} justifyContent="space-between" alignItems="center">
          <Button variant="outlined" startIcon={<Cancel />}>
            Cancel Setup
          </Button>
          <Stack direction="row" spacing={2}>
            <Button variant="outlined">
              Previous
            </Button>
            <Button variant="contained" startIcon={<Save />}>
              Next Step
            </Button>
          </Stack>
        </Stack>
      </Box>
    ),
  },
};

export const CompactActions: Story = {
  args: {
    variant: 'outlined',
    children: (
      <Stack direction="row" spacing={1} justifyContent="flex-end">
        <Button variant="text" size="small">
          Cancel
        </Button>
        <Button variant="contained" size="small" startIcon={<Save />}>
          Save
        </Button>
      </Stack>
    ),
  },
};

export const CustomStyling: Story = {
  args: {
    variant: 'elevated',
    elevation: 10,
    sx: {
      backgroundColor: 'primary.main',
      color: 'white',
      '& .MuiButton-root': {
        color: 'white',
        borderColor: 'white',
        '&:hover': {
          backgroundColor: 'rgba(255, 255, 255, 0.1)',
        },
      },
    },
    children: (
      <Stack direction="row" spacing={2} justifyContent="flex-end">
        <Button variant="outlined" startIcon={<Cancel />}>
          Cancel
        </Button>
        <Button 
          variant="contained" 
          startIcon={<Save />}
          sx={{ 
            backgroundColor: 'white', 
            color: 'primary.main',
            '&:hover': {
              backgroundColor: 'grey.100',
            },
          }}
        >
          Save Changes
        </Button>
      </Stack>
    ),
  },
};
