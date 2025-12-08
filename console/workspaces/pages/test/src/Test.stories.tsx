import type { Meta, StoryObj } from '@storybook/react';
import { 
  TestComponent,
  TestProject,
  TestOrganization,
} from './index';

// Component Level Stories
const metaComponent: Meta<typeof TestComponent> = {
  title: 'Pages/Test/Component',
  component: TestComponent,
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
    title: 'Test - Component Level',
    description: 'A page component for Test',
  },
};

export const ComponentCustom: StoryComponent = {
  args: {
    title: 'Custom Component Title',
    description: 'This is a custom description for the component level page.',
  },
};

// Project Level Stories
const metaProject: Meta<typeof TestProject> = {
  title: 'Pages/Test/Project',
  component: TestProject,
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
    title: 'Test - Project Level',
    description: 'A page component for Test',
  },
};

export const ProjectCustom: StoryObj<typeof metaProject> = {
  args: {
    title: 'Custom Project Title',
    description: 'This is a custom description for the project level page.',
  },
};

// Organization Level Stories
const metaOrganization: Meta<typeof TestOrganization> = {
  title: 'Pages/Test/Organization',
  component: TestOrganization,
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
    title: 'Test - Organization Level',
    description: 'A page component for Test',
  },
};

export const OrganizationCustom: StoryObj<typeof metaOrganization> = {
  args: {
    title: 'Custom Organization Title',
    description: 'This is a custom description for the organization level page.',
  },
};
