import type { Meta, StoryObj } from '@storybook/react';
import { 
  TracesComponent,
  TracesProject,
  TracesOrganization,
} from './index';

// Component Level Stories
const metaComponent: Meta<typeof TracesComponent> = {
  title: 'Pages/Traces/Component',
  component: TracesComponent,
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

export default metaComponent;
type StoryComponent = StoryObj<typeof metaComponent>;

export const ComponentDefault: StoryComponent = {
  args: {
    title: 'Traces - Component Level',
    description: 'A page component for Traces',
  },
};

export const ComponentCustom: StoryComponent = {
  args: {
    title: 'Custom Component Title',
    description: 'This is a custom description for the component level page.',
  },
};

// Project Level Stories
const metaProject: Meta<typeof TracesProject> = {
  title: 'Pages/Traces/Project',
  component: TracesProject,
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

export const ProjectDefault: StoryObj<typeof metaProject> = {
  args: {
    title: 'Traces - Project Level',
    description: 'A page component for Traces',
  },
};

export const ProjectCustom: StoryObj<typeof metaProject> = {
  args: {
    title: 'Custom Project Title',
    description: 'This is a custom description for the project level page.',
  },
};

// Organization Level Stories
const metaOrganization: Meta<typeof TracesOrganization> = {
  title: 'Pages/Traces/Organization',
  component: TracesOrganization,
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

export const OrganizationDefault: StoryObj<typeof metaOrganization> = {
  args: {
    title: 'Traces - Organization Level',
    description: 'A page component for Traces',
  },
};

export const OrganizationCustom: StoryObj<typeof metaOrganization> = {
  args: {
    title: 'Custom Organization Title',
    description: 'This is a custom description for the organization level page.',
  },
};
