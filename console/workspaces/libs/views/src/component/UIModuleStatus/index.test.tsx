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

import type { ReactNode } from 'react';
import { renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { UIModuleStatusProvider, useIsUIModuleInstalled, type UIModuleStatusMap } from '.';

const withPhases = (phases: UIModuleStatusMap) =>
  function Wrapper({ children }: { children: ReactNode }) {
    return <UIModuleStatusProvider value={phases}>{children}</UIModuleStatusProvider>;
  };

describe('useIsUIModuleInstalled', () => {
  it('treats every module as installed outside a module host', () => {
    const { result } = renderHook(() => useIsUIModuleInstalled('anything'));
    expect(result.current).toBe(true);
  });

  it.each<[string, boolean | undefined]>([
    ['installed', true],
    ['checking', undefined],
    ['installing', false],
    ['not-installed', false],
    ['unknown-module', false],
  ])('module %s -> %s', (moduleId, expected) => {
    const { result } = renderHook(() => useIsUIModuleInstalled(moduleId), {
      wrapper: withPhases({
        installed: 'installed',
        checking: 'checking',
        installing: 'installing',
        'not-installed': 'not-installed',
      }),
    });
    expect(result.current).toBe(expected);
  });
});
