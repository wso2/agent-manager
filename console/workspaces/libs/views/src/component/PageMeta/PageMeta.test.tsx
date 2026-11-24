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

import { screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../test-utils/render';
import { CreatedByMetadata, CreatedMetadata } from '../CreatedMetadata';
import { PageMetaItem, PageMetaList } from './PageMeta';

afterEach(() => {
  vi.useRealTimers();
});

describe('PageMetaItem', () => {
  it('uses the label as the icon tooltip for an icon row', () => {
    renderWithProviders(
      <PageMetaItem icon={<svg data-testid="icon" />} label="Owner">
        alice
      </PageMetaItem>,
    );
    expect(screen.getByText('alice')).toBeInTheDocument();
    expect(screen.queryByText('Owner:')).not.toBeInTheDocument();
    expect(screen.getByTestId('icon').parentElement).toHaveAttribute('aria-label', 'Owner');
  });

  it('shows "Label:" text when there is no icon', () => {
    renderWithProviders(<PageMetaItem label="Version">{3}</PageMetaItem>);
    expect(screen.getByText('Version:')).toBeInTheDocument();
    expect(screen.getByText('3')).toBeInTheDocument();
  });

  it('renders node values as given, with an icon but no label', () => {
    renderWithProviders(
      <PageMetaList>
        <PageMetaItem icon={<svg data-testid="icon" />}>
          <button type="button">tag</button>
        </PageMetaItem>
      </PageMetaList>,
    );
    expect(screen.getByRole('button', { name: 'tag' })).toBeInTheDocument();
    expect(screen.getByTestId('icon').parentElement).not.toHaveAttribute('aria-label');
  });
});

describe('CreatedMetadata', () => {
  it('renders nothing without a timestamp', () => {
    const { container } = renderWithProviders(<CreatedMetadata />);
    expect(container).toBeEmptyDOMElement();
  });

  it('renders the relative creation time', () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-01-10T00:00:00Z'));
    renderWithProviders(<CreatedMetadata createdAt="2026-01-08T00:00:00Z" />);
    expect(screen.getByText(/2 days ago/)).toBeInTheDocument();
  });
});

describe('CreatedByMetadata', () => {
  it('renders nothing when the creator is unknown', () => {
    const { container } = renderWithProviders(<CreatedByMetadata />);
    expect(container).toBeEmptyDOMElement();
  });

  it('renders the creator with a tooltip label', () => {
    renderWithProviders(<CreatedByMetadata createdBy="Alice" label="Owner" />);
    expect(screen.getByText('Alice')).toBeInTheDocument();
    expect(screen.getByLabelText('Owner')).toBeInTheDocument();
  });
});
