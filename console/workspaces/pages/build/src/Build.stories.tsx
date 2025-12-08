/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
import { 
  BuildComponent,
  BuildProject,
  BuildOrganization,
} from './index';

// Component Level Stories
const metaComponent: Meta<typeof BuildComponent> = {
  title: 'Pages/Build/Component',
  component: BuildComponent,
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
    title: 'Build - Component Level',
    description: 'A page component for Build',
  },
};

export const ComponentCustom: StoryComponent = {
  args: {
    title: 'Custom Component Title',
    description: 'This is a custom description for the component level page.',
  },
};

// Project Level Stories
const metaProject: Meta<typeof BuildProject> = {
  title: 'Pages/Build/Project',
  component: BuildProject,
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
    title: 'Build - Project Level',
    description: 'A page component for Build',
  },
};

export const ProjectCustom: StoryObj<typeof metaProject> = {
  args: {
    title: 'Custom Project Title',
    description: 'This is a custom description for the project level page.',
  },
};

// Organization Level Stories
const metaOrganization: Meta<typeof BuildOrganization> = {
  title: 'Pages/Build/Organization',
  component: BuildOrganization,
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
    title: 'Build - Organization Level',
    description: 'A page component for Build',
  },
};

export const OrganizationCustom: StoryObj<typeof metaOrganization> = {
  args: {
    title: 'Custom Organization Title',
    description: 'This is a custom description for the organization level page.',
  },
};
