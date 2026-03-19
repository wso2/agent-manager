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

import { useAuthContext } from "@asgardeo/auth-react";
import { useQuery } from "@tanstack/react-query";
import { UserInfo } from "../../types";
import { globalConfig } from "@agent-management-platform/types";

/**
 * Module-level ref populated by `initRefreshToken` (called from AuthProvider).
 * Lets the plain `refreshToken` utility reach the provider-managed session
 * without needing to be a hook itself.
 */
let _refreshAccessToken: (() => Promise<unknown>) | null = null;

export const initRefreshToken = (fn: () => Promise<unknown>): void => {
  _refreshAccessToken = fn;
};

export const refreshToken = async (): Promise<void> => {
  if (_refreshAccessToken) {
    await _refreshAccessToken();
  }
};

export const useAuthHooks = () => {
  const {
    signIn,
    getIDToken,
    getBasicUserInfo,
    isAuthenticated,
    trySignInSilently,
    revokeAccessToken,
  } = useAuthContext() ?? {};
  const { authConfig } = globalConfig;

  const { data: userInfo, isLoading: isLoadingUserInfo } = useQuery({
    queryKey: ["auth", "userInfo", getBasicUserInfo],
    queryFn: async () => {
      return getBasicUserInfo();
    },
    enabled: !!getBasicUserInfo,
  });

  const {
    data: isAuthenticatedState,
    isLoading: isLoadingIsAuthenticated,
    refetch: refetchIsAuthenticated,
  } = useQuery({
    queryKey: ["isAuthenticated", isAuthenticated],
    queryFn: () => {
      return isAuthenticated();
    },
  });

  const customLogin = () => {
    signIn();
    refetchIsAuthenticated();
  };

  return {
    isAuthenticated: isAuthenticatedState,
    userInfo: userInfo as UserInfo,
    isLoadingUserInfo: isLoadingUserInfo,
    isLoadingIsAuthenticated: isLoadingIsAuthenticated,
    getToken: () => getIDToken(),
    login: () => customLogin(),
    logout: async () => {
      try {
        await revokeAccessToken();
      } catch (error) {
        // Preserve error visibility while guaranteeing navigation.
        // eslint-disable-next-line no-console
        console.error("Error during signOut:", error);
      }
      window.location.assign(authConfig?.signOutRedirectURL ?? "/logout");
    },
    trySignInSilently: () => trySignInSilently(),
  };
};
