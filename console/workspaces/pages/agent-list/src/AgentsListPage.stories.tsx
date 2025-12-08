import type { Meta, StoryObj } from '@storybook/react';
import { fn } from '@storybook/test';
import { AgentsListPage } from './AgentsListPage';

const meta: Meta<typeof AgentsListPage> = {
  title: 'Pages/AgentsListPage',
  component: AgentsListPage,
  parameters: {
    layout: 'fullscreen',
  },
  tags: ['autodocs'],
  args: {
    onCreateAgent: fn(),
  },
};

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    title: 'Agents',
  },
};

export const CustomTitle: Story = {
  args: {
    title: 'My AI Agents',
  },
};

export const WithAgents: Story = {
  args: {
    title: 'Agents',
    agents: [
      { id: '1', name: 'Customer Support Agent' },
      { id: '2', name: 'Data Analysis Agent' },
      { id: '3', name: 'Code Review Agent' },
    ],
  },
};

export const EmptyState: Story = {
  args: {
    title: 'Agents',
    agents: [],
  },
};

export const CustomBackHref: Story = {
  args: {
    title: 'Agents',
    backHref: '/dashboard',
  },
};

