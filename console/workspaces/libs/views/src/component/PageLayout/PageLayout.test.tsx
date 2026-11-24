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

import { useState } from 'react';
import { fireEvent, screen } from '@testing-library/react';
import { Route, Routes, useLocation } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../test-utils/render';
import { PageLayout } from './PageLayout';
import { PageErrorBoundary } from './PageErrorBoundary';

afterEach(() => {
  vi.restoreAllMocks();
});

function LocationProbe() {
  const location = useLocation();
  return (
    <div data-testid="location">
      {location.pathname}|{JSON.stringify(location.state)}
    </div>
  );
}

describe('PageLayout', () => {
  it('renders the title, avatar initial, description, meta, actions and content', () => {
    renderWithProviders(
      <PageLayout
        title="Customer Bot"
        description="Answers support questions"
        meta={<span>Created 2 days ago</span>}
        titleTail={<span>v2</span>}
        actions={<button type="button">Deploy</button>}
      >
        <p>Page body</p>
      </PageLayout>,
    );
    expect(screen.getByText('Customer Bot')).toBeInTheDocument();
    expect(screen.getByText('C')).toBeInTheDocument();
    expect(screen.getByText('Answers support questions')).toBeInTheDocument();
    expect(screen.getByText('Created 2 days ago')).toBeInTheDocument();
    expect(screen.getByText('v2')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Deploy' })).toBeInTheDocument();
    expect(screen.getByText('Page body')).toBeInTheDocument();
    expect(document.title).toMatch(/^Customer Bot \| /);
  });

  it('navigates back with the given state from the back link', () => {
    renderWithProviders(
      <Routes>
        <Route
          path="/edit"
          element={
            <PageLayout title="Edit" backHref="/list" backState={{ from: '/x' }} backLabel="Back to list">
              body
            </PageLayout>
          }
        />
        <Route path="/list" element={<LocationProbe />} />
      </Routes>,
      { initialEntries: ['/edit'] },
    );
    fireEvent.click(screen.getByText('Back to list'));
    expect(screen.getByTestId('location')).toHaveTextContent('/list|{"from":"/x"}');
  });

  it('labels the back link "Back" by default', () => {
    renderWithProviders(
      <PageLayout title="Edit" backHref="/list">
        body
      </PageLayout>,
    );
    expect(screen.getByText('Back')).toBeInTheDocument();
  });

  it('shows skeletons instead of the title, meta and actions while loading', () => {
    renderWithProviders(
      <PageLayout
        title="Hidden title"
        isLoading
        description="Hidden description"
        actions={<button type="button">Hidden action</button>}
      >
        body
      </PageLayout>,
    );
    expect(screen.queryByText('Hidden title')).not.toBeInTheDocument();
    expect(screen.queryByText('Hidden description')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Hidden action' })).not.toBeInTheDocument();
    expect(document.querySelectorAll('.MuiSkeleton-root').length).toBeGreaterThanOrEqual(4);
    expect(screen.getByText('body')).toBeInTheDocument();
  });

  it.each([
    ['avatar={null}', { avatar: null }],
    ['disableIcon', { disableIcon: true }],
  ])('drops the avatar with %s', (_label, props) => {
    renderWithProviders(
      <PageLayout title="Zed" {...props}>
        body
      </PageLayout>,
    );
    expect(screen.queryByText('Z')).not.toBeInTheDocument();
  });

  it('uses a custom avatar spec', () => {
    renderWithProviders(
      <PageLayout title="Zed" avatar={{ children: <span data-testid="logo" /> }}>
        body
      </PageLayout>,
    );
    expect(screen.getByTestId('logo')).toBeInTheDocument();
  });

  it('replaces a crashing page with a retryable error screen', () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    function Flaky() {
      const [crash] = useState(() => (window as { crashOnce?: boolean }).crashOnce);
      if (crash) {
        throw new Error('boom');
      }
      return <p>Recovered</p>;
    }
    (window as { crashOnce?: boolean }).crashOnce = true;
    renderWithProviders(
      <PageLayout title="Flaky">
        <Flaky />
      </PageLayout>,
    );
    expect(screen.getByText('Something went wrong.')).toBeInTheDocument();
    expect(errorSpy).toHaveBeenCalledWith(
      'PageLayout failed to render',
      expect.any(Error),
      expect.anything(),
    );

    (window as { crashOnce?: boolean }).crashOnce = false;
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(screen.getByText('Recovered')).toBeInTheDocument();
  });
});

describe('PageErrorBoundary', () => {
  it('renders its children when nothing throws', () => {
    renderWithProviders(
      <PageErrorBoundary>
        <p>fine</p>
      </PageErrorBoundary>,
    );
    expect(screen.getByText('fine')).toBeInTheDocument();
  });
});
