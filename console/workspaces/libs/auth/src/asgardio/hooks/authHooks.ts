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
import { globalConfig } from "@agent-management-platform/types";
import { useQuery } from "@tanstack/react-query";

const decodeJWTPart = (part: string): Record<string, unknown> | null => {
  try {
    const normalized = part.replace(/-/g, "+").replace(/_/g, "/");
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, "=");
    return JSON.parse(window.atob(padded)) as Record<string, unknown>;
  } catch {
    return null;
  }
};

const decodeJWT = (token: string) => {
  const [header, payload] = token.split(".");
  if (!header || !payload) return null;

  return {
    header: decodeJWTPart(header),
    payload: decodeJWTPart(payload),
  };
};

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
  const { data: tokenInfo } = useQuery({
    queryKey: ["tokenInfo", getAccessToken],
    queryFn: async (): Promise<string> => {
      const token = await getAccessToken?.();
      if (!token) {
        throw new Error("Access token is not available");
      }
      return token;
    },
    select: (data) => decodeJWT(data as string),
  });

  const userInfo = useMemo(() => {
    return {
      ...flattenedProfile,
      familyName: flattenedProfile?.family_name,
      givenName: flattenedProfile?.given_name,
      ...tokenInfo?.payload,
    } as UserInfo;
  }, [flattenedProfile, tokenInfo]);

  const customLogin = () => {
    void signIn?.();
  };

  const handleLogout = useCallback(async () => {
    try {
      await signOut?.();
    } catch (error) {
      console.error("Error during signOut:", error);
    } finally {
      window.location.assign(
        globalConfig.authConfig.afterSignOutUrl ?? "/login",
      );
    }
  }, [signOut]);

  const safeGetToken: () => Promise<string> =
    getAccessToken ??
    (() => Promise.reject(new Error("getAccessToken is not available")));

  const safeSignInSilently: () => Promise<unknown> =
    signInSilently ??
    (() => Promise.reject(new Error("signInSilently is not available")));

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
