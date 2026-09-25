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

import {
    formatBytes,
    getMaxRequestBodyBytes,
    globalConfig,
    utf8ByteLength,
} from '@agent-management-platform/types';

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

type HttpErrorWithStatus = Error & { status: number; body?: unknown; code?: string };

/**
 * Set on the error serializeRequestBody throws, so error handlers can tell a
 * client-side size refusal from a server response and keep its message.
 */
export const REQUEST_TOO_LARGE_CODE = 'REQUEST_TOO_LARGE';

export function isRequestTooLargeError(error: unknown): error is Error {
    return error instanceof Error
        && (error as { code?: unknown }).code === REQUEST_TOO_LARGE_CODE;
}

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
    await sleep(DEFAULT_TIMEOUT);
    if (!response.ok) {
        await throwIfHttpWriteNotOk(response);
    }
    return response;
}

/**
 * Serialise a write body, refusing to send one larger than the deployment's
 * configured limit (MAX_REQUEST_BODY_BYTES).
 *
 * Where a WAF sits in front of the platform, an oversized body is rejected
 * with a 403 that never reaches the service, so the console cannot tell it
 * apart from a permission error. Checking here turns that into a message that
 * names the cause. Deployments without such a limit set it to 0.
 */
export function serializeRequestBody(body: object): string {
    const serialized = JSON.stringify(body);
    const limit = getMaxRequestBodyBytes();
    if (limit === 0) {
        return serialized;
    }
    const byteLength = utf8ByteLength(serialized);
    if (byteLength > limit) {
        // Kept to one short sentence: this surfaces in the single-line,
        // non-wrapping snackbar, which truncates anything longer.
        const err = new Error(
            `Request is too large (${formatBytes(byteLength)} of ${formatBytes(limit)}). `
            + 'Shorten the longest fields, such as file contents or descriptions.'
        ) as HttpErrorWithStatus;
        err.status = 413;
        err.code = REQUEST_TOO_LARGE_CODE;
        throw err;
    }
    return serialized;
}

export async function httpGET(
    context: string,
    params:{
        searchParams?: Record<string, string> | string[][],
        token?: string,
        options?: HttpOptions,
        timeoutMs?: number,
        // Overrides globalConfig.apiBaseUrl for this request. Used by the
        // unauthenticated runtime-config discovery call, which must target a
        // public host rather than the JWT-locked apiBaseUrl.
        baseUrl?: string,
    }) {
    const {searchParams, token, timeoutMs, baseUrl: baseUrlOverride} = params;
    const baseUrl = baseUrlOverride ?? globalConfig.apiBaseUrl;
    // Optional AbortController timeout so a hung endpoint rejects (as an
    // AbortError) instead of leaving the request pending forever. Callers that
    // gate app bootstrap on a GET (e.g. runtime config) must pass timeoutMs.
    const controller = timeoutMs ? new AbortController() : undefined;
    const timer = controller ? setTimeout(() => controller.abort(), timeoutMs) : undefined;
    try {
        const response = await fetch(`${baseUrl}${context}?${new URLSearchParams(searchParams).toString()}`, {
            method: 'GET',
            headers:  token ? {
                  'Content-Type': 'application/json',
                  'Authorization': `Bearer ${token}`
                } : {
                  'Content-Type': 'application/json'
                },
            signal: controller?.signal
        });
        if (!response.ok) {
            const err = new Error(`HTTP error! status: ${response.status}`) as HttpErrorWithStatus;
            err.status = response.status;
            throw err;
        }
        await sleep(DEFAULT_TIMEOUT);
        return response;
    } finally {
        if (timer) clearTimeout(timer);
    }
}

let observerBaseUrl: string | undefined;

/** Set by the runtime-config bootstrap once GET /api/v1/config resolves. */
export function setObserverBaseUrl(url: string | undefined): void {
  // Strip trailing slashes: requests are built as `${observerBaseUrl}${context}`
  // where context already starts with "/", so a trailing slash here would
  // produce "//api/v1/..." and 404 on gateways that don't collapse "//".
  observerBaseUrl = url?.trim().replace(/\/+$/, "") || undefined;
}

export function isObserverConfigured(): boolean {
  return !!observerBaseUrl;
}

/**
 * Same as httpGET but calls the observer service directly using the
 * observer base URL discovered at runtime via GET /api/v1/config.
 * Throws if the observer is not configured — the agent-manager no longer
 * serves traces routes, so silently falling back would produce opaque 404s.
 */
export async function httpGETObserver(
    context: string,
    params: {searchParams?: Record<string, string>, token?: string}) {
    const {searchParams, token} = params;
    const obsUrl = observerBaseUrl;
    if (!obsUrl) {
        throw new Error(
            'Observer is not configured. Set AM_OBSERVER_PUBLIC_URL on the agent-manager service.'
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
        body: serializeRequestBody(body)
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
        body: serializeRequestBody(body)
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
        body: serializeRequestBody(body)
    });
    return finalizeHttpWriteResponse(response);
}
