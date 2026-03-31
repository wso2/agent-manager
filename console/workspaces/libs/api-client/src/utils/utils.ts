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

import { refreshToken } from '@agent-management-platform/auth';
import { globalConfig } from '@agent-management-platform/types';

export function sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
}
export const SERVICE_BASE = '/api/v1';

export function encodeRequired(value: string | undefined, label: string): string {
  if (!value) {
    throw new Error(`Missing required parameter: ${label}`);
  }
  return encodeURIComponent(value);
}
export const OBS_SERVICE_BASE = '/api';
export const POLL_INTERVAL = 5000;
export const SLOW_POLL_INTERVAL = 15000;

const DEFAULT_TIMEOUT = 1000;

export interface HttpOptions {
   useObsPlaneHostApi?: boolean;
}

/**
 * Triggers a token refresh only when the response indicates an expired/invalid
 * token (HTTP 401). Intentionally skips refresh for client errors such as 404
 * (resource not found) or 400 (bad request) which are not auth-related.
 */
async function handleTokenExpiry(response: Response): Promise<void> {
    if (response.status === 401) {
        await refreshToken();
    }
}

type HttpErrorWithStatus = Error & { status: number; body?: unknown };

async function throwIfHttpWriteNotOk(response: Response): Promise<void> {
    let body: unknown;
    try {
        body = await response.json();
    } catch {
        body = undefined;
    }
    let message = `HTTP error! status: ${response.status}`;
    if (
        body !== null &&
        typeof body === "object" &&
        "message" in body &&
        typeof (body as { message: unknown }).message === "string"
    ) {
        message = (body as { message: string }).message;
    }
    const err = new Error(message) as HttpErrorWithStatus;
    err.status = response.status;
    err.body = body;
    throw err;
}

async function finalizeHttpWriteResponse(response: Response): Promise<Response> {
    if (!response.ok) {
        await handleTokenExpiry(response);
    }
    await sleep(DEFAULT_TIMEOUT);
    if (!response.ok) {
        await throwIfHttpWriteNotOk(response);
    }
    return response;
}

export async function httpGET(
    context: string, 
    params:{searchParams?: Record<string, string>, token?: string, options?: HttpOptions}) {
    const {searchParams, token} = params;
    const baseUrl = globalConfig.apiBaseUrl;
    const response = await fetch(`${baseUrl}${context}?${new URLSearchParams(searchParams).toString()}`, {
        method: 'GET',
        headers:  token ? {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${token}`
            } : {
              'Content-Type': 'application/json'
            }
    });
    if (!response.ok) {
        await handleTokenExpiry(response);
        const err = new Error(`HTTP error! status: ${response.status}`) as HttpErrorWithStatus;
        err.status = response.status;
        throw err;
    }
    await sleep(DEFAULT_TIMEOUT);
    return response;
}

/**
 * Same as httpGET but calls the traces-observer-service directly using obsApiBaseUrl.
 * Throws if obsApiBaseUrl is not configured — the agent-manager no longer serves
 * traces routes, so silently falling back would produce opaque 404 errors.
 */
export async function httpGETObserver(
    context: string,
    params: {searchParams?: Record<string, string>, token?: string}) {
    const {searchParams, token} = params;
    const obsUrl = globalConfig.obsApiBaseUrl?.trim();
    if (!obsUrl || obsUrl === '$OBS_API_BASE_URL') {
        throw new Error(
            'obsApiBaseUrl is not configured. Set OBS_API_BASE_URL to the traces-observer-service URL.'
        );
    }
    const baseUrl = obsUrl;
    const response = await fetch(`${baseUrl}${context}?${new URLSearchParams(searchParams).toString()}`, {
        method: 'GET',
        headers: token ? {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`
        } : {
            'Content-Type': 'application/json'
        }
    });
    if (!response.ok) {
        await handleTokenExpiry(response);
        const err = new Error(`HTTP error! status: ${response.status}`) as HttpErrorWithStatus;
        err.status = response.status;
        throw err;
    }
    await sleep(DEFAULT_TIMEOUT);
    return response;
}

export async function httpPOST(
    context: string, 
    body: object, 
    params: {searchParams?: Record<string, string>, token?: string, options?: HttpOptions}) {
    const {searchParams, token} = params;
    const baseUrl = globalConfig.apiBaseUrl;
    const response = await fetch(`${baseUrl}${context}?${new URLSearchParams(searchParams).toString()}`, {
        method: 'POST',
        headers: token ? {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`
        } : {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify(body)
    });
    return finalizeHttpWriteResponse(response);
}

export async function httpPUT(
    context: string, 
    body: object, 
    params: {searchParams?: Record<string, string>, token?: string, options?: HttpOptions}) {
    const {searchParams, token} = params;
    const baseUrl = globalConfig.apiBaseUrl;
    const response = await fetch(`${baseUrl}${context}?${new URLSearchParams(searchParams).toString()}`, {
        method: 'PUT',
        headers: token ? {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`
        } : {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify(body)
    });
    return finalizeHttpWriteResponse(response);
}

export async function httpDELETE(
    context: string, 
    params: {searchParams?: Record<string, string>, token?: string, options?: HttpOptions}) {
    const {searchParams, token} = params;
    const baseUrl = globalConfig.apiBaseUrl;
    const response = await fetch(`${baseUrl}${context}?${new URLSearchParams(searchParams).toString()}`, {
        method: 'DELETE',
        headers: token ? {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`
        } : {
            'Content-Type': 'application/json'
        }
    });
    return finalizeHttpWriteResponse(response);
}

export async function httpPATCH(
    context: string, 
    body: object, 
    params: {searchParams?: Record<string, string>, token?: string, options?: HttpOptions}) {
    const {searchParams, token} = params;
    const baseUrl = globalConfig.apiBaseUrl;
    const response = await fetch(`${baseUrl}${context}?${new URLSearchParams(searchParams).toString()}`, {
        method: 'PATCH',
        headers: token ? {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`
        } : {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify(body)
    });
    return finalizeHttpWriteResponse(response);
}
