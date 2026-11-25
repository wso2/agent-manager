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
import { Inbox } from '@wso2/oxygen-ui-icons-react';
import { describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../test-utils/render';
import { LoadErrorNotice, StateNotice } from '.';

describe('StateNotice', () => {
  it('renders the title, description, extra content and actions', () => {
    renderWithProviders(
      <StateNotice
        icon={Inbox}
        title="No agents yet"
        description="Create one to get started."
        actions={<button type="button">Create agent</button>}
      >
        <code>amp agents create</code>
      </StateNotice>,
    );
    expect(screen.getByRole('heading', { level: 2, name: 'No agents yet' })).toBeInTheDocument();
    expect(screen.getByText('Create one to get started.')).toBeInTheDocument();
    expect(screen.getByText('amp agents create')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Create agent' })).toBeInTheDocument();
  });

  it('numbers pending steps and checks off finished ones', () => {
    renderWithProviders(
      <StateNotice
        icon={Inbox}
        tone="warning"
        title="Finish setup"
        description="Two more steps."
        steps={[
          { label: 'Install', state: 'done' },
          { label: 'Configure', state: 'current' },
          { label: 'Deploy', state: 'todo' },
        ]}
      />,
    );
    const steps = screen.getByLabelText('Steps');
    expect(steps).toHaveTextContent('Install');
    expect(steps).toHaveTextContent('2Configure');
    expect(steps).toHaveTextContent('3Deploy');
    // The finished step shows a check instead of its number.
    expect(steps).not.toHaveTextContent('1Install');
  });

  it.each([
    ['contained', {}, true],
    ['uncontained', { contained: false }, false],
    ['compact', { compact: true, tone: 'error' as const }, false],
  ])('%s variant', (_label, props, hasContainer) => {
    const { container } = renderWithProviders(
      <StateNotice icon={Inbox} title="T" description="D" {...props} />,
    );
    expect(screen.getByRole('heading', { name: 'T' })).toBeInTheDocument();
    expect(screen.queryByLabelText('Steps')).not.toBeInTheDocument();
    // Without a container the notice's own stack is the root element.
    expect(container.firstElementChild?.classList.contains('MuiStack-root')).toBe(!hasContainer);
  });
});

describe('LoadErrorNotice', () => {
  it('shows the error message, a retry button and a back link', () => {
    const onRetry = vi.fn();
    renderWithProviders(
      <LoadErrorNotice
        entityLabel="project"
        error={new Error('Server exploded')}
        onRetry={onRetry}
        backHref="/projects"
        backLabel="Back to projects"
      />,
    );
    expect(screen.getByRole('heading', { name: "Couldn't load this project" })).toBeInTheDocument();
    expect(screen.getByText('Server exploded')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(onRetry).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('link', { name: 'Back to projects' })).toHaveAttribute(
      'href',
      '/projects',
    );
  });

  it.each([undefined, 'string error', new Error('')])(
    'falls back to a generic explanation for %j',
    (error) => {
      renderWithProviders(<LoadErrorNotice entityLabel="agent" error={error} backHref="/x" />);
      expect(
        screen.getByText('The agent may have been deleted, or you may not have access to it.'),
      ).toBeInTheDocument();
      expect(screen.queryByRole('button', { name: 'Try again' })).not.toBeInTheDocument();
      expect(screen.getByRole('link', { name: 'Go back' })).toBeInTheDocument();
    },
  );
});
