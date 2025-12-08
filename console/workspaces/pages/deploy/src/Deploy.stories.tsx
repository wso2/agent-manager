import type { Meta, StoryObj } from '@storybook/react';
import { 
  DeployComponent,
  DeployProject,
  DeployOrganization,
} from './index';

// Component Level Stories
const metaComponent: Meta<typeof DeployComponent> = {
  title: 'Pages/Deploy/Component',
  component: DeployComponent,
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
    title: 'Deploy - Component Level',
    description: 'A page component for Deploy',
  },
};

export const ComponentCustom: StoryComponent = {
  args: {
    title: 'Custom Component Title',
    description: 'This is a custom description for the component level page.',
  },
};

// Project Level Stories
const metaProject: Meta<typeof DeployProject> = {
  title: 'Pages/Deploy/Project',
  component: DeployProject,
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
    title: 'Deploy - Project Level',
    description: 'A page component for Deploy',
  },
};

export const ProjectCustom: StoryObj<typeof metaProject> = {
  args: {
    title: 'Custom Project Title',
    description: 'This is a custom description for the project level page.',
  },
};

// Organization Level Stories
const metaOrganization: Meta<typeof DeployOrganization> = {
  title: 'Pages/Deploy/Organization',
  component: DeployOrganization,
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
    title: 'Deploy - Organization Level',
    description: 'A page component for Deploy',
  },
};

export const OrganizationCustom: StoryObj<typeof metaOrganization> = {
  args: {
    title: 'Custom Organization Title',
    description: 'This is a custom description for the organization level page.',
  },
};
