/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import type { AuthMode } from '@agent-management-platform/types';
import { globalConfig } from '@agent-management-platform/types';

import { AuthProvider as FileAuthProvider } from './file/FileAuthContext';
import { useAuthHooks as useFileAuthHooks } from './file/hooks/authHooks';
import { AuthProvider as NoAuthAuthProvider } from './no-auth/AuthProvider';
import { useAuthHooks as useNoAuthHooks } from './no-auth/hooks/authHooks';
import { AuthProvider as ThunderIDAuthProvider } from './thunderid/AuthProvider';
import { useAuthHooks as useThunderIDAuthHooks } from './thunderid/hooks/authHooks';

/**
 * The provider this console signs in with.
 *
 * Defaults to 'file', matching the service's own AUTH_MODE default, so the two
 * halves agree when neither is configured. A deployment that uses an identity
 * provider sets 'thunder' on both — the mismatch it would otherwise hit is a
 * login form posting to an endpoint that answers 404, or a redirect to an IdP
 * whose tokens the service will not accept, neither of which names the cause.
 *
 * `disableAuth` is still honoured when authMode is unset, so a deployment that
 * predates the mode setting keeps the behaviour it had. New deployments should
 * set authMode, which names what it selects rather than what it switches off.
 */
export const authMode: AuthMode = (() => {
  const configured = globalConfig?.authMode;
  if (configured === 'file' || configured === 'none' || configured === 'thunder') {
    return configured;
  }
  return globalConfig?.disableAuth ? 'none' : 'file';
})();

/** True when sign-in is a credential form this console renders itself. */
export const isFileAuthMode = authMode === 'file';

const providers = {
  file: FileAuthProvider,
  none: NoAuthAuthProvider,
  thunder: ThunderIDAuthProvider,
} as const;

const hooks = {
  file: useFileAuthHooks,
  none: useNoAuthHooks,
  thunder: useThunderIDAuthHooks,
} as const;

export const AuthProvider = providers[authMode];
export const useAuthHooks = hooks[authMode];

export { LoginForm } from './file/LoginForm/LoginForm';

export {
  backendEndpoints,
  backendFetch,
  fetchWithBackendToken,
  isBackendAuthConfig,
  resolveAuthConfig,
} from './thunderid/backendSession';
