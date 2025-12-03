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

import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { AgentsListPage } from './AgentsListPage';

describe('AgentsListPage', () => {
  it('renders with default title', () => {
    render(<AgentsListPage />);
    expect(screen.getByText('Agents')).toBeInTheDocument();
  });

  it('renders page description', () => {
    render(<AgentsListPage />);
    expect(screen.getByText('Manage and monitor all your AI agents across environments')).toBeInTheDocument();
  });

  it('renders add new agent button', () => {
    render(<AgentsListPage />);
    expect(screen.getByText('Add New Agent')).toBeInTheDocument();
  });

  it('renders search input', () => {
    render(<AgentsListPage />);
    expect(screen.getByPlaceholderText('Search agents')).toBeInTheDocument();
  });

  it('renders agents table with sample data', () => {
    render(<AgentsListPage />);
    
    // Check for table headers
    expect(screen.getByText('Agent')).toBeInTheDocument();
    expect(screen.getByText('Framework')).toBeInTheDocument();
    expect(screen.getByText('Model')).toBeInTheDocument();
    expect(screen.getByText('Status')).toBeInTheDocument();
    expect(screen.getByText('Project')).toBeInTheDocument();
    
    // eslint-disable-next-line max-len
    // Check for sample agent data (there are multiple Agent 1 and Agent 2 entries in the sample data)
    expect(screen.getAllByText('Agent 1')).toHaveLength(2);
    expect(screen.getAllByText('Agent 2')).toHaveLength(2);
    expect(screen.getByText('Agent 3')).toBeInTheDocument();
  });

  it('renders framework and model information', () => {
    render(<AgentsListPage />);
    
    // Check for framework and model data (only 5 are visible due to pagination)
    expect(screen.getAllByText('OpenAI')).toHaveLength(5); // 5 agents with OpenAI framework (pagination shows 5)
    expect(screen.getAllByText('gpt-4o')).toHaveLength(5); // 5 agents with gpt-4o model (pagination shows 5)
  });
});


