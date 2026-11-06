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

import type { ThunderIDProviderProps } from "@thunderid/react";
import { globalConfig } from "@agent-management-platform/types";

type AuthConfig = ThunderIDProviderProps;
type BackendEndpoints = NonNullable<NonNullable<AuthConfig["endpoints"]>["backend"]>;

const REQUESTED_WITH_HEADER = "X-Requested-With";
const REQUESTED_WITH_VALUE = "XMLHttpRequest";
const OPTIONAL_CONTEXT_ENDPOINTS = ["refresh", "signOut", "token"] as const;

export const isLocalPath = (value: unknown): value is string =>
  typeof value === "string" && /^\/(?![/\\])/.test(value);

export function resolveAuthConfig(config: AuthConfig): AuthConfig {
  const backend = config.endpoints?.backend;
  if (!backend) return config;
  const contextIsLocal =
    isLocalPath(backend.session) &&
    isLocalPath(backend.checkSession) &&
    OPTIONAL_CONTEXT_ENDPOINTS.every(
      (key) => backend[key] === undefined || isLocalPath(backend[key]),
    );
  if (!contextIsLocal) {
    return { ...config, endpoints: { ...config.endpoints, backend: undefined } };
  }
  if (isLocalPath(backend.signIn)) return config;
  const session = backend.session as string;
  return {
    ...config,
    endpoints: {
      ...config.endpoints,
      backend: {
        ...backend,
        signIn: `${session.slice(0, session.lastIndexOf("/"))}/signin`,
      },
    },
  };
}

export function backendEndpoints(): BackendEndpoints | undefined {
  if (globalConfig.disableAuth) return undefined;
  return resolveAuthConfig(globalConfig.authConfig).endpoints?.backend;
}

let sessionEnded = false;
const sessionEndedListeners = new Set<() => void>();

export function endBackendSession(): void {
  if (sessionEnded) return;
  sessionEnded = true;
  sessionEndedListeners.forEach((listener) => listener());
}

export function isBackendSessionEnded(): boolean {
  return sessionEnded;
}

export function subscribeBackendSessionEnded(listener: () => void): () => void {
  sessionEndedListeners.add(listener);
  return () => {
    sessionEndedListeners.delete(listener);
  };
}

export async function isBackendSignedIn(): Promise<boolean> {
  const checkSession = backendEndpoints()?.checkSession;
  if (!checkSession) return true;
  try {
    const response = await fetch(checkSession, {
      credentials: "include",
      headers: {
        Accept: "application/json",
        [REQUESTED_WITH_HEADER]: REQUESTED_WITH_VALUE,
      },
    });
    if (!response.ok) return true;
    const { isSignedIn } = (await response.json()) as { isSignedIn?: boolean };
    return isSignedIn !== false;
  } catch {
    return true;
  }
}

const isSameOrigin = (url: string): boolean =>
  new URL(url, window.location.href).origin === window.location.origin;

export async function backendFetch(
  url: string,
  init: RequestInit = {},
): Promise<Response> {
  if (!backendEndpoints() || !isSameOrigin(url)) return fetch(url, init);
  const send = () => {
    const headers = new Headers(init.headers);
    headers.set(REQUESTED_WITH_HEADER, REQUESTED_WITH_VALUE);
    return fetch(url, { ...init, headers });
  };
  const response = await send();
  if (response.status !== 401) return response;
  if (await isBackendSignedIn()) {
    return init.body instanceof ReadableStream ? response : send();
  }
  endBackendSession();
  return response;
}

export function clearBrowserSignInSession(clientId: string | undefined): void {
  if (!clientId) return;
  const suffix = `-${clientId}`;
  const isOwnKey = (key: string | null): key is string =>
    key !== null &&
    key.endsWith(suffix) &&
    /-instance_\d+$/.test(key.slice(0, -suffix.length));
  try {
    [window.localStorage, window.sessionStorage].forEach((storage) => {
      Array.from({ length: storage.length }, (_, index) => storage.key(index))
        .filter(isOwnKey)
        .forEach((key) => storage.removeItem(key));
    });
  } catch {
    return;
  }
}
