/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useEffect, useSyncExternalStore } from "react";

/**
 * Holds a gateway's one-time registration token for the pages that show it: the add
 * page leaves it for the setup page, and the setup steps leave a regenerated one.
 *
 * The token is shown only once, so it is kept in memory only: never in the URL, in
 * router state (which the browser keeps in session history), or in storage. A reload
 * loses it, and the setup steps then offer Reconfigure to generate a new one.
 */
interface NewGatewayToken {
  token: string;
  /** The token record's id, so revoking that record drops the token. */
  tokenId?: string;
}

const tokens = new Map<string, NewGatewayToken>();
/** Mounted pages showing each gateway's token; see useForgetNewGatewayTokenOnLeave. */
const holders = new Map<string, number>();
const listeners = new Set<() => void>();

const notify = () => listeners.forEach((listener) => listener());
const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

export function keepNewGatewayToken(gatewayId: string, token: string, tokenId?: string): void {
  tokens.set(gatewayId, { token, tokenId });
  notify();
}

/** The latest token request started from a gateway page, per gateway. */
const requests = new Map<string, number>();

/**
 * Starts a token request from one of the gateway's pages and returns its ticket for
 * keepRequestedGatewayToken. A request started later supersedes it.
 */
export function beginGatewayTokenRequest(gatewayId: string): number {
  const ticket = (requests.get(gatewayId) ?? 0) + 1;
  requests.set(gatewayId, ticket);
  return ticket;
}

/**
 * Keeps the token a gateway page requested, unless:
 * - a newer request for the gateway started meanwhile (two pages each reconfiguring):
 *   the newer one's token must not be overwritten by this one finishing late; or
 * - no page for the gateway is open any more: the leave check already ran, nothing
 *   would forget the token, and a later visit must not offer it.
 * Moving from the setup page to the view page keeps a page open, so the token
 * survives that. Returns whether the token was kept.
 */
export function keepRequestedGatewayToken(
  gatewayId: string,
  ticket: number,
  token: string,
  tokenId?: string,
): boolean {
  if (requests.get(gatewayId) !== ticket || !holders.has(gatewayId)) return false;
  keepNewGatewayToken(gatewayId, token, tokenId);
  return true;
}

export function newGatewayToken(gatewayId: string): string | null {
  return tokens.get(gatewayId)?.token ?? null;
}

export function forgetNewGatewayToken(gatewayId: string): void {
  if (tokens.delete(gatewayId)) notify();
}

/** Drops the gateway's token if it is the one just revoked; revoking another leaves it. */
export function forgetRevokedGatewayToken(gatewayId: string, tokenId: string): void {
  if (tokens.get(gatewayId)?.tokenId === tokenId) forgetNewGatewayToken(gatewayId);
}

/**
 * The gateway's token, following the store: one that arrives after the page opened
 * (the add page's request finishing late) shows up, and a revoked one goes away.
 */
export function useNewGatewayToken(gatewayId: string | undefined): string | null {
  return useSyncExternalStore(subscribe, () => (gatewayId ? newGatewayToken(gatewayId) : null));
}

/**
 * Forgets the gateway's token once no page showing it is left: it may be revoked
 * afterwards, and a later visit must not offer it again. Moving straight from one of
 * the gateway's pages to another (setup to view) keeps it, since the next page takes
 * over before the check runs. So does an effect re-run that mounts again at once.
 */
export function useForgetNewGatewayTokenOnLeave(gatewayId: string | undefined): void {
  useEffect(() => {
    if (!gatewayId) return undefined;
    holders.set(gatewayId, (holders.get(gatewayId) ?? 0) + 1);
    return () => {
      const remaining = (holders.get(gatewayId) ?? 1) - 1;
      if (remaining > 0) holders.set(gatewayId, remaining);
      else holders.delete(gatewayId);
      queueMicrotask(() => {
        if (!holders.has(gatewayId)) forgetNewGatewayToken(gatewayId);
      });
    };
  }, [gatewayId]);
}
