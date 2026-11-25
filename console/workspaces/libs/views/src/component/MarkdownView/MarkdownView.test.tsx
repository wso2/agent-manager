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
import { DescriptionCard } from '../DescriptionCard';
import { MarkdownView } from './MarkdownView';

describe('MarkdownView', () => {
  it('renders markdown, including GFM tables and links', () => {
    renderWithProviders(
      <MarkdownView
        content={[
          '# Title',
          '',
          'Some **bold** text and a [link](https://example.com).',
          '',
          '| a | b |',
          '| - | - |',
          '| 1 | 2 |',
        ].join('\n')}
      />,
    );
    expect(screen.getByRole('heading', { level: 1, name: 'Title' })).toBeInTheDocument();
    expect(screen.getByText('bold').tagName).toBe('STRONG');
    expect(screen.getByRole('link', { name: 'link' })).toHaveAttribute('href', 'https://example.com');
    expect(screen.getByRole('table')).toBeInTheDocument();
    expect(screen.getByRole('cell', { name: '2' })).toBeInTheDocument();
  });

  it('keeps safe raw HTML images but strips scripts and event handlers', () => {
    const { container } = renderWithProviders(
      <MarkdownView
        content={
          '<img src="https://example.com/a.png" alt="diagram" width="10" onerror="alert(1)">' +
          '\n\n<script>window.pwned = true</script>'
        }
      />,
    );
    const img = screen.getByRole('img', { name: 'diagram' });
    expect(img).toHaveAttribute('src', 'https://example.com/a.png');
    expect(img).toHaveAttribute('width', '10');
    expect(img).not.toHaveAttribute('onerror');
    expect(container.querySelector('script')).toBeNull();
  });
});

describe('DescriptionCard', () => {
  it('renders the title header above the markdown content', () => {
    renderWithProviders(<DescriptionCard title="About" content="Hello *world*" />);
    expect(screen.getByText('About')).toBeInTheDocument();
    expect(screen.getByText('world').tagName).toBe('EM');
  });

  it('renders only the content when no title is given', () => {
    const { container } = renderWithProviders(<DescriptionCard content="Just text" />);
    expect(screen.getByText('Just text')).toBeInTheDocument();
    expect(container.querySelector('.MuiCardHeader-root')).toBeNull();
  });
});
