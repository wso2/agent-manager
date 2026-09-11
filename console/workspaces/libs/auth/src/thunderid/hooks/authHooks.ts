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

import { useThunderID, useUser } from "@thunderid/react";
import type { ThunderIDUserProfile, UserInfo } from "../../types";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { globalConfig } from "@agent-management-platform/types";

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
  /**
   * True until the access token's payload has settled, one way or the other.
   *
   * The token is decoded asynchronously after sign-in, so the claims it carries
   * — `scope` above all — are absent from userInfo for a beat during which
   * isLoadingUserInfo is already false. A caller reading a claim needs to tell
   * that beat apart from a token that genuinely carries nothing, or it acts on
   * an empty answer and then changes its mind.
   */
  isLoadingAccessToken: boolean;
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
  } = useThunderID() ?? {};

  const { flattenedProfile } = useUser();
  const profileAttributes = (flattenedProfile as ThunderIDUserProfile | null)
    ?.attributes;

  // undefined = the decode has not settled yet; null = it settled with nothing
  // usable (signed out, no token, or a token that would not decode). Collapsing
  // the two would make "still loading" and "no claims" the same value, which is
  // the distinction isLoadingAccessToken exists to keep.
  const [accessTokenPayload, setAccessTokenPayload] = useState<
    Record<string, unknown> | null | undefined
  >(undefined);

  const getAccessTokenRef = useRef(getAccessToken);
  getAccessTokenRef.current = getAccessToken;

  useEffect(() => {
    if (!isSignedIn || !isInitialized) {
      if (!isSignedIn) setAccessTokenPayload(null);
      return;
    }
    let cancelled = false;
    const tokenPromise = getAccessTokenRef.current?.();
    if (!tokenPromise) {
      setAccessTokenPayload(null);
      return;
    }
    tokenPromise
      .then((token) => {
        if (cancelled) return;
        if (!token) {
          setAccessTokenPayload(null);
          return;
        }
        const decoded = decodeJWT(token);
        if (!cancelled) setAccessTokenPayload(decoded?.payload ?? null);
      })
      .catch(() => {
        if (!cancelled) setAccessTokenPayload(null);
      });
    return () => {
      cancelled = true;
    };
  }, [isSignedIn, isInitialized]);

  const userInfo = useMemo(() => {
    return {
      email: profileAttributes?.email,
      familyName: profileAttributes?.family_name,
      givenName: profileAttributes?.given_name,
      username: profileAttributes?.username,
      ...(accessTokenPayload ?? {}),
    } as UserInfo;
  }, [profileAttributes, accessTokenPayload]);

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
    isLoadingAccessToken: accessTokenPayload === undefined,
    getToken: safeGetToken,
    login: customLogin,
    logout: handleLogout,
    trySignInSilently: safeSignInSilently,
  };
};
