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

