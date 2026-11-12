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

import { globalConfig } from '@agent-management-platform/types';

import type { UserInfo } from '../types';

/**
 * File-mode sessions keep their own storage key rather than reusing the
 * ThunderID SDK's `session_data-instance_<n>-<clientId>` blob: that shape is
 * the SDK's to change, and a half-written imitation of it would be read back
 * by SDK code that assumes fields we never set.
 */
const STORAGE_KEY = 'amp.file-auth.session';

/**
 * Treat a token as expired slightly early, so one that passes the check here
 * does not expire in flight on the request it was read for.
 */
const EXPIRY_SKEW_MS = 30_000;

export type StoredSession = {
  accessToken: string;
  /** Epoch milliseconds. */
  expiresAt: number;
};

/**
 * Storage throws in a private window and with site data blocked, and returns
 * nothing in previews. Every access is guarded: a session that cannot be read
 * is a signed-out user, never a crash on boot.
 */
const safeStorage = {
  read(): string | null {
    try {
      return window.localStorage.getItem(STORAGE_KEY);
    } catch {
      return null;
    }
  },
  write(value: string): void {
    try {
      window.localStorage.setItem(STORAGE_KEY, value);
    } catch {
      // A session that cannot be persisted still works for this tab.
    }
  },
  remove(): void {
    try {
      window.localStorage.removeItem(STORAGE_KEY);
    } catch {
      // Nothing to do: the caller is signing out regardless.
    }
  },
};

/** Reads the stored session, or null when absent, unreadable or expired. */
export const readSession = (): StoredSession | null => {
  const raw = safeStorage.read();
  if (!raw) {
    return null;
  }

  let parsed: Partial<StoredSession>;
  try {
    parsed = JSON.parse(raw) as Partial<StoredSession>;
  } catch {
    // Corrupt entry: drop it rather than leave it to fail on every read.
    safeStorage.remove();
    return null;
  }

  if (
    typeof parsed.accessToken !== 'string' ||
    !parsed.accessToken ||
    typeof parsed.expiresAt !== 'number'
  ) {
    safeStorage.remove();
    return null;
  }

  if (Date.now() + EXPIRY_SKEW_MS >= parsed.expiresAt) {
    // File mode issues no refresh token, so an expired session is simply over.
    safeStorage.remove();
    return null;
  }

  return { accessToken: parsed.accessToken, expiresAt: parsed.expiresAt };
};

export const writeSession = (
  accessToken: string,
  expiresInSeconds: number,
): StoredSession => {
  const session: StoredSession = {
    accessToken,
    expiresAt: Date.now() + expiresInSeconds * 1000,
  };
  safeStorage.write(JSON.stringify(session));
  return session;
};

export const clearSession = (): void => safeStorage.remove();

/** Decodes a JWT payload. Never verifies: the server is the only verifier. */
export const decodeClaims = (
  token: string,
): Record<string, unknown> | null => {
  try {
    const payload = token.split('.')[1];
    if (!payload) {
      return null;
    }
    const normalised = payload.replace(/-/g, '+').replace(/_/g, '/');
    const padded = normalised.padEnd(
      normalised.length + ((4 - (normalised.length % 4)) % 4),
      '=',
    );
    return JSON.parse(atob(padded)) as Record<string, unknown>;
  } catch {
    return null;
  }
};

const asString = (value: unknown): string | undefined =>
  typeof value === 'string' && value ? value : undefined;

/**
 * Maps the login token's claims onto the UserInfo the rest of the console
 * consumes. The claim names are the server's (`ouId`, `ouHandle`, `scope`);
 * the UserInfo field names are the app's.
 */
export const userInfoFromClaims = (
  claims: Record<string, unknown> | null,
): UserInfo => {
  if (!claims) {
    return {};
  }

  const username = asString(claims.sub);
  const scope = asString(claims.scope);

  return {
    sub: username,
    username,
    // File mode has no profile directory, so the username is the only name
    // there is. Showing the raw subject beats showing nothing.
    displayName: username,
    orgId: asString(claims.ouId),
    orgHandle: asString(claims.ouHandle),
    orgName: asString(claims.ouHandle),
    scope,
    allowedScopes: scope,
  };
};

/**
 * The login endpoint is mounted at the server root, beside the JWKS and
 * well-known routes — not under the `/api/v1` prefix the resource API uses.
 */
export const loginEndpoint = (): string =>
  `${(globalConfig?.apiBaseUrl ?? '').replace(/\/+$/, '')}/auth/login`;
