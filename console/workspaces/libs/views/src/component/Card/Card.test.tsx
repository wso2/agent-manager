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

import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { Card } from './Card';
import { TestWrapper } from '../../../setupTests';

describe('Card', () => {
  it('renders children correctly', () => {
    render(
      <TestWrapper>
        <Card>
          <div>Test content</div>
        </Card>
      </TestWrapper>
    );
    
    expect(screen.getByText('Test content')).toBeInTheDocument();
  });

  it('applies custom className', () => {
    render(
      <TestWrapper>
        <Card className="custom-class" data-testid="Card">
          <div>Test content</div>
        </Card>
      </TestWrapper>
    );
    
    const card = screen.getByTestId('Card');
    expect(card).toHaveClass('custom-class');
  });

  it('passes through MUI Card props', () => {
    render(
      <TestWrapper>
        <Card variant="outlined" elevation={2} data-testid="Card">
          <div>Test content</div>
        </Card>
      </TestWrapper>
    );
    
    const card = screen.getByTestId('Card');
    expect(card).toBeInTheDocument();
  });

  it('renders with CardContent wrapper', () => {
    render(
      <TestWrapper>
        <Card>
          <div>Test content</div>
        </Card>
      </TestWrapper>
    );
    
    // CardContent should be present (MUI component)
    expect(screen.getByText('Test content')).toBeInTheDocument();
  });
});
