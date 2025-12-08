import type { Meta, StoryObj } from '@storybook/react';
import { AddNewAgent } from './AddNewAgent';

const meta: Meta<typeof AddNewAgent> = {
  title: 'Pages/AddNewAgent',
  component: AddNewAgent,
  parameters: {
    layout: 'fullscreen',
  },
  tags: ['autodocs'],
  argTypes: {
    title: {
      control: 'text',
      description: 'The title of the page',
    },
    description: {
      control: 'text',
      description: 'The description of the page',
    },
  },
};

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    title: 'Add New Agent',
    description: 'A page component for Add New Agent',
  },
};

export const WithCustomTitle: Story = {
  args: {
    title: 'Custom Page Title',
    description: 'A page component for Add New Agent',
  },
};

export const WithCustomDescription: Story = {
  args: {
    title: 'Add New Agent',
    description: 'This is a custom description for the page.',
  },
};
