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
