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

import { useAsgardeo, useUser } from "@asgardeo/react";
import { UserInfo } from "../../types";
import { useCallback, useMemo } from "react";


export type AuthHooks = {
  isAuthenticated: boolean;
  userInfo: UserInfo;
  isLoadingUserInfo: boolean;
  isLoadingIsAuthenticated: boolean;
  getToken: () => Promise<string>;
  login: () => void;
  logout: () => Promise<void>;
  trySignInSilently: () => Promise<unknown>;
};

export const useAuthHooks = (): AuthHooks => {
  const {
    signIn,
    getAccessToken,
    signInSilently,
    signOut,
    isSignedIn = false,
    isLoading = false,
    isInitialized = false,
  } = useAsgardeo() ?? {};

  const { flattenedProfile } = useUser();
  const userInfo = useMemo(() => {
    return {
      ...flattenedProfile,
    } as UserInfo;
  }, [flattenedProfile]);

  const customLogin = () => {
    void signIn?.();
  };

  const handleLogout = useCallback(async () => {
    try {
      await signOut?.();
    } catch (error) {
      window.location.assign("/login");
      console.error("Error during signOut:", error);
    }
  }, [signOut]);

  const safeGetToken: () => Promise<string> = getAccessToken
    ?? (() => Promise.reject(new Error("getAccessToken is not available")));

  const safeSignInSilently: () => Promise<unknown> = signInSilently
    ?? (() => Promise.reject(new Error("signInSilently is not available")));

  return {
    isAuthenticated: isSignedIn && isInitialized,
    userInfo,
    isLoadingUserInfo: isLoading,
    isLoadingIsAuthenticated: !isInitialized || isLoading,
    getToken: safeGetToken,
    login: customLogin,
    logout: handleLogout,
    trySignInSilently: safeSignInSilently,
  };
};
