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

import { withBasePath } from '@agent-management-platform/types';
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react';

import type { AuthProviderProps, UserInfo } from '../types';

import {
  clearSession,
  decodeClaims,
  readSession,
  requestLogin,
  userInfoFromClaims,
  writeSession,
} from './session';

type FileAuthContextValue = {
  isAuthenticated: boolean;
  userInfo: UserInfo;
  isLoading: boolean;
  getToken: () => Promise<string>;
  /** Exchanges credentials for a token. Throws a human-readable message. */
  signIn: (username: string, password: string) => Promise<void>;
  signOut: () => Promise<void>;
};

/**
 * The largest delay setTimeout accepts: it stores the value in a signed 32-bit
 * integer (about 24.8 days), and anything larger wraps round and fires at once.
 */
const MAX_TIMEOUT_MS = 2_147_483_647;

const FileAuthContext = createContext<FileAuthContextValue | null>(null);

export const AuthProvider = ({ children }: AuthProviderProps) => {
  // undefined = the stored session has not been read yet. Collapsing it into
  // "signed out" would flash the login form on every reload for a user who is
  // in fact signed in.
  const [session, setSession] = useState<ReturnType<typeof readSession> | undefined>(
    undefined,
  );

  useEffect(() => {
    setSession(readSession());
  }, []);

  // File mode issues no refresh token, so an expired session cannot be
  // renewed — it can only be noticed. A timer set for the moment of expiry
  // signs the user out then, rather than leaving the UI looking signed in
  // until the next request happens to 401.
  useEffect(() => {
    if (!session) {
      return undefined;
    }
    let timer = 0;
    const signOutWhenExpired = () => {
      const remaining = session.expiresAt - Date.now();
      if (remaining <= 0) {
        clearSession();
        setSession(null);
        return;
      }
      // setTimeout holds its delay in a signed 32-bit integer, so anything
      // past MAX_TIMEOUT_MS wraps and fires immediately: a long
      // FILE_AUTH_TOKEN_TTL would sign the user out the moment they signed
      // in. Wait in bounded steps and re-check instead of trusting one long
      // sleep — which also means a laptop that suspends wakes up to a fresh
      // comparison against the clock rather than a timer that drifted.
      timer = window.setTimeout(signOutWhenExpired, Math.min(remaining, MAX_TIMEOUT_MS));
    };
    signOutWhenExpired();
    return () => window.clearTimeout(timer);
  }, [session]);

  const signIn = useCallback(async (username: string, password: string) => {
    const { accessToken, expiresInSeconds } = await requestLogin(username, password);
    setSession(writeSession(accessToken, expiresInSeconds));
  }, []);

  const signOut = useCallback(async () => {
    clearSession();
    setSession(null);
    // A hard navigation, matching the ThunderID provider: it clears any
    // in-memory state held by pages that never unmount on a router navigation.
    window.location.assign(withBasePath('/login'));
  }, []);

  const getToken = useCallback(
    () => Promise.resolve(session?.accessToken ?? ''),
    [session],
  );

  const value = useMemo<FileAuthContextValue>(() => {
    const claims = session ? decodeClaims(session.accessToken) : null;
    return {
      isAuthenticated: Boolean(session),
      userInfo: userInfoFromClaims(claims),
      isLoading: session === undefined,
      getToken,
      signIn,
      signOut,
    };
  }, [session, getToken, signIn, signOut]);

  return (
    <FileAuthContext.Provider value={value}>{children}</FileAuthContext.Provider>
  );
};

export const useFileAuth = (): FileAuthContextValue => {
  const context = useContext(FileAuthContext);
  if (!context) {
    throw new Error('useFileAuth must be used within the file-mode AuthProvider');
  }
  return context;
};
