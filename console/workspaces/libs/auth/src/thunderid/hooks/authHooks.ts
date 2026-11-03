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
import { globalConfig, withBasePath } from "@agent-management-platform/types";

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

// One function for every render: callers list getToken in effect dependencies, so a new
// function per render re-runs those effects (and their fetches) on every render.
const noBackendToken = (): Promise<string> => Promise.resolve("");

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
    signInOptions,
    getAccessToken,
    getDecodedIdToken,
    getGrantedPermissions,
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

  // Sign-in on the backend (BFF): the gateway's oauth2-session holds the tokens
  // and the browser only an HttpOnly session cookie, so there is no access token
  // here to read claims from or to attach to API calls.
  const isBackendAuth = Boolean(globalConfig.authConfig.endpoints?.backend);

  const getAccessTokenRef = useRef(getAccessToken);
  getAccessTokenRef.current = getAccessToken;
  const getDecodedIdTokenRef = useRef(getDecodedIdToken);
  getDecodedIdTokenRef.current = getDecodedIdToken;
  const getGrantedPermissionsRef = useRef(getGrantedPermissions);
  getGrantedPermissionsRef.current = getGrantedPermissions;

  useEffect(() => {
    if (!isSignedIn || !isInitialized) {
      if (!isSignedIn) setAccessTokenPayload(null);
      return;
    }
    let cancelled = false;
    // On the backend the same claims come from the session: the id_token's,
    // with the granted permissions standing in for the access token's scope.
    const payloadPromise = isBackendAuth
      ? Promise.all([
          getDecodedIdTokenRef.current?.(),
          getGrantedPermissionsRef.current?.(),
        ]).then(([idToken, permissions]) => ({
          ...((idToken as Record<string, unknown> | undefined) ?? {}),
          scope: (permissions ?? []).join(" "),
        }))
      : getAccessTokenRef.current?.()?.then((token) =>
          token ? (decodeJWT(token)?.payload ?? null) : null,
        );
    if (!payloadPromise) {
      setAccessTokenPayload(null);
      return;
    }
    payloadPromise
      .then((payload) => {
        if (!cancelled) setAccessTokenPayload(payload);
      })
      .catch(() => {
        if (!cancelled) setAccessTokenPayload(null);
      });
    return () => {
      cancelled = true;
    };
  }, [isSignedIn, isInitialized, isBackendAuth]);

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
    // signInOptions has to be passed explicitly. The provider prop is only
    // applied automatically by <SignInButton>; this console redirects to
    // sign-in from the Login page instead, and a bare signIn() sends no custom
    // authorize params at all, so the RFC 8707 resource indicator would be
    // silently dropped.
    //
    // Without it ThunderID evaluates the requested permission scopes against
    // the deployment's default resource server rather than urn:wso2:amp, and
    // drops every amp: scope it cannot resolve there — the token comes back
    // with aud=amp-console-client and openid profile email only, so every
    // scope-gated surface reads as "no permissions" once RBAC is enforced.
    void signIn?.(signInOptions);
  };

  const handleLogout = useCallback(async () => {
    try {
      await signOut?.();
    } catch (error) {
      console.error("Error during signOut:", error);
    } finally {
      window.location.assign(
        globalConfig.authConfig.afterSignOutUrl ?? withBasePath("/login"),
      );
    }
  }, [signOut]);

  // On the backend an API call carries the session cookie and the gateway adds
  // the token, so callers get an empty one and the HTTP helpers, which only set
  // Authorization for a token, send none.
  const safeGetToken: () => Promise<string> = isBackendAuth
    ? noBackendToken
    : (getAccessToken ??
      (() => Promise.reject(new Error("getAccessToken is not available"))));

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
