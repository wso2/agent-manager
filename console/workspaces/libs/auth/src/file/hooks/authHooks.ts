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

import type { AuthHooks } from '../../thunderid/hooks/authHooks';
import { useFileAuth } from '../FileAuthContext';

/**
 * The file-mode half of the AuthHooks contract.
 *
 * Every caller in the console sees this shape and not the mode behind it, so
 * the differences are absorbed here:
 *
 * - `login()` does nothing. In the ThunderID provider it starts a redirect to
 *   the identity provider; in file mode the credential form is already on
 *   screen by the time anything would call it, and navigating would only
 *   reload the page under the user's typing.
 * - `trySignInSilently()` resolves to nothing. There is no refresh token to
 *   trade, so there is no silent sign-in to attempt.
 * - The claims are decoded synchronously from the stored token, so there is no
 *   window where the user is authenticated but their scopes are not yet known.
 *   `isLoadingAccessToken` is therefore only ever true while the stored
 *   session itself is being read.
 */
export const useAuthHooks = (): AuthHooks => {
  const { isAuthenticated, userInfo, isLoading, getToken, signOut } =
    useFileAuth();

  return {
    isAuthenticated,
    userInfo,
    isLoadingUserInfo: isLoading,
    isLoadingIsAuthenticated: isLoading,
    isLoadingAccessToken: isLoading,
    getToken,
    login: () => undefined,
    logout: signOut,
    trySignInSilently: () => Promise.resolve(undefined),
  };
};
