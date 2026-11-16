/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it } from 'vitest';
import { identityProviderLabel } from './identityProviderLabel';

describe('identityProviderLabel', () => {
  it('shows the built-in token issuer as DefaultKeyManager', () => {
    expect(identityProviderLabel('ThunderKeyManager')).toBe('DefaultKeyManager');
  });

  it('leaves any other name as it is', () => {
    expect(identityProviderLabel('my-idp')).toBe('my-idp');
  });

  it('is empty when there is no name', () => {
    expect(identityProviderLabel(undefined)).toBe('');
    expect(identityProviderLabel('')).toBe('');
  });
});
