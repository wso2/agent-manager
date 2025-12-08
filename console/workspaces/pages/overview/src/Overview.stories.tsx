import type { Meta, StoryObj } from '@storybook/react';
import { 
  OverviewComponent,
  OverviewProject,
  OverviewOrganization,
} from './index';

// Component Level Stories
const metaComponent: Meta<typeof OverviewComponent> = {
  title: 'Pages/Overview/Component',
  component: OverviewComponent,
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
    title: 'Overview - Component Level',
    description: 'A page component for Overview',
  },
};

export const ComponentCustom: StoryComponent = {
  args: {
    title: 'Custom Component Title',
    description: 'This is a custom description for the component level page.',
  },
};

// Project Level Stories
const metaProject: Meta<typeof OverviewProject> = {
  title: 'Pages/Overview/Project',
  component: OverviewProject,
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
    title: 'Overview - Project Level',
    description: 'A page component for Overview',
  },
};

export const ProjectCustom: StoryObj<typeof metaProject> = {
  args: {
    title: 'Custom Project Title',
    description: 'This is a custom description for the project level page.',
  },
};

// Organization Level Stories
const metaOrganization: Meta<typeof OverviewOrganization> = {
  title: 'Pages/Overview/Organization',
  component: OverviewOrganization,
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
    title: 'Overview - Organization Level',
    description: 'A page component for Overview',
  },
};

export const OrganizationCustom: StoryObj<typeof metaOrganization> = {
  args: {
    title: 'Custom Organization Title',
    description: 'This is a custom description for the organization level page.',
  },
};
