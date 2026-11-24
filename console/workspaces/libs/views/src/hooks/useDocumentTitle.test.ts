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

import { renderHook } from '@testing-library/react';
import { afterEach, beforeAll, describe, expect, it } from 'vitest';
import { useDocumentTitle } from './useDocumentTitle';

// The hook captures the first document title it sees as the module-wide base,
// so every test here shares "Base Console".
describe('useDocumentTitle', () => {
  beforeAll(() => {
    document.title = 'Base Console';
  });

  afterEach(() => {
    document.title = 'Base Console';
  });

  it('prefixes the page title onto the base title and restores the previous one on unmount', () => {
    const { unmount } = renderHook(() => useDocumentTitle('Agents'));
    expect(document.title).toBe('Agents | Base Console');

    unmount();
    expect(document.title).toBe('Base Console');
  });

  it('shows only the base title when no page title is given', () => {
    document.title = 'Something else';
    const { unmount } = renderHook(() => useDocumentTitle(undefined));
    expect(document.title).toBe('Base Console');

    unmount();
    expect(document.title).toBe('Something else');
  });

  it('updates when the title changes', () => {
    const { rerender } = renderHook(({ title }) => useDocumentTitle(title), {
      initialProps: { title: 'One' },
    });
    expect(document.title).toBe('One | Base Console');

    rerender({ title: 'Two' });
    expect(document.title).toBe('Two | Base Console');
  });
});
