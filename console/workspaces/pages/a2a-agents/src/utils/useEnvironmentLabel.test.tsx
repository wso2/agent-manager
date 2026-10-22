/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { renderHook } from '@testing-library/react';
import { useListEnvironments } from '@agent-management-platform/api-client';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useEnvironmentLabel } from './useEnvironmentLabel';

vi.mock('@agent-management-platform/api-client', () => ({
  useListEnvironments: vi.fn(),
}));

const mockedUseListEnvironments = vi.mocked(useListEnvironments);

function listReturning(data: unknown) {
  mockedUseListEnvironments.mockReturnValue({ data } as ReturnType<
    typeof useListEnvironments
  >);
}

describe('useEnvironmentLabel', () => {
  beforeEach(() => mockedUseListEnvironments.mockReset());

  it('prefers a non-empty display name', () => {
    listReturning([
      { name: 'prod', displayName: 'Production' },
      { name: 'blank', displayName: '' },
    ]);
    const { result } = renderHook(() => useEnvironmentLabel('acme'));

    expect(mockedUseListEnvironments).toHaveBeenCalledWith({ orgName: 'acme' });
    expect(result.current('prod')).toBe('Production');
    expect(result.current('blank')).toBe('blank');
    expect(result.current('dev')).toBe('dev');
  });

  it('returns names unchanged while the list is missing', () => {
    listReturning(undefined);
    const { result } = renderHook(() => useEnvironmentLabel('acme'));

    expect(result.current('prod')).toBe('prod');
  });
});
