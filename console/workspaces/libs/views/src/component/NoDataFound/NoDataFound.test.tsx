/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import { fireEvent, screen } from '@testing-library/react';
import { SearchX } from '@wso2/oxygen-ui-icons-react';
import { describe, expect, it } from 'vitest';
import { renderWithProviders } from '../../../test-utils/render';
import { NoDataFound } from './NoDataFound';

describe('NoDataFound', () => {
  it('shows the default message and icon on an outlined panel', () => {
    const { container } = renderWithProviders(<NoDataFound />);
    expect(screen.getByRole('heading', { name: 'No data found' })).toBeInTheDocument();
    expect(container.querySelector('svg')).toBeInTheDocument();
    expect(container.querySelector('.MuiPaper-outlined')).toBeInTheDocument();
  });

  it('renders the message, subtitle, custom icon and action without a background', () => {
    let clicked = false;
    const { container } = renderWithProviders(
      <NoDataFound
        message="No agents"
        subtitle="Create your first agent"
        icon={<span data-testid="custom-icon" />}
        action={
          <button type="button" onClick={() => (clicked = true)}>
            Create
          </button>
        }
        disableBackground
      />,
    );
    expect(screen.getByRole('heading', { name: 'No agents' })).toBeInTheDocument();
    expect(screen.getByText('Create your first agent')).toBeInTheDocument();
    expect(screen.getByTestId('custom-icon')).toBeInTheDocument();
    expect(container.querySelector('.MuiPaper-root')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Create' }));
    expect(clicked).toBe(true);
  });

  it('renders an icon component when given iconElement', () => {
    const { container } = renderWithProviders(<NoDataFound iconElement={SearchX} />);
    expect(container.querySelector('svg')).toHaveAttribute('width', '100');
  });
});
