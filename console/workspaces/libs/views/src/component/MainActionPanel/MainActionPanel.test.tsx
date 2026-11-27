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
import { describe, expect, it } from 'vitest';
import { renderWithProviders } from '../../../test-utils/render';
import { ExternalAgentIcon } from '../../icons';
import { FadeIn } from '../FadeIn';
import { FormGridSpacer } from '../FormGridSpacer';
import { FullPageLoader } from '../FullPageLoader';
import { IconTile } from '../IconTile';
import { Logo } from '../Logo';
import { RadioMark } from '../RadioMark';
import { MainActionPanel } from './MainActionPanel';

describe('MainActionPanel', () => {
  it.each(['elevated', 'outlined', 'filled'] as const)(
    'renders its children in the %s variant',
    (variant) => {
      renderWithProviders(
        <MainActionPanel variant={variant} className="extra" aria-label="actions">
          <button type="button">Save</button>
        </MainActionPanel>,
      );
      const panel = screen.getByTestId('MainActionPanel');
      expect(panel).toHaveClass('main-action-panel', 'extra');
      expect(panel).toHaveAttribute('aria-label', 'actions');
      expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument();
    },
  );
});

describe('small presentational components', () => {
  it('FullPageLoader shows the Oxygen UI page loader', () => {
    renderWithProviders(<FullPageLoader />);
    expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'true');
    expect(screen.getByText('Loading')).toBeInTheDocument();
  });

  it('FadeIn renders its children', () => {
    renderWithProviders(<FadeIn>faded</FadeIn>);
    expect(screen.getByText('faded')).toBeInTheDocument();
  });

  it('Logo renders the themed wordmark image', () => {
    renderWithProviders(<Logo width={40} />);
    const img = screen.getByRole('img', { name: /Choreo AI Control Plane/ });
    expect(img).toHaveAttribute('src', expect.stringMatching(/choreo-ai-control-plane-(dark|white)\.svg$/));
    expect(img).toHaveAttribute('width', '40');
    // A caller-supplied size replaces the 28px default instead of fighting it.
    expect(img.style.height).toBe('');
    expect(img.style.width).toBe('');
  });

  it('Logo defaults to 28px high when no size is given', () => {
    renderWithProviders(<Logo />);
    expect(screen.getByRole('img', { name: /Choreo AI Control Plane/ }).style.height).toBe('28px');
  });

  it('ExternalAgentIcon forwards svg props', () => {
    const { container } = renderWithProviders(<ExternalAgentIcon data-testid="ext" height={10} />);
    expect(screen.getByTestId('ext')).toHaveAttribute('height', '10');
    expect(container.querySelector('svg')).toBeInTheDocument();
  });

  it.each([true, false])('IconTile and RadioMark are decorative (selected=%s)', (selected) => {
    const { container } = renderWithProviders(
      <>
        <IconTile selected={selected} size={48}>
          <svg data-testid="tile-icon" />
        </IconTile>
        <RadioMark selected={selected} />
      </>,
    );
    expect(screen.getByTestId('tile-icon').parentElement).toHaveAttribute('aria-hidden', 'true');
    const radioMark = container.lastElementChild;
    expect(radioMark).toHaveAttribute('aria-hidden', 'true');
    // Only a selected radio mark carries the check icon.
    expect(radioMark?.querySelector('svg') !== null).toBe(selected);
  });

  it('FormGridSpacer renders an empty grid cell', () => {
    const { container } = renderWithProviders(<FormGridSpacer columns={8} />);
    expect(container.querySelector('.MuiGrid-root')).toBeEmptyDOMElement();
  });
});
