import { globalConfig } from '@agent-management-platform/types';

export function sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
}
export const SERVICE_BASE = '/api/v1';
export const OBS_SERVICE_BASE = '/api';
export const POLL_INTERVAL = 5000;

const DEFAULT_TIMEOUT = 1000;

export interface HttpOptions {
   useObsPlaneHostApi?: boolean;
}

export async function httpGET(
    context: string, 
    params:{searchParams?: Record<string, string>, token?: string, options?: HttpOptions}) {
    const {searchParams, token, options} = params;
    const baseUrl = options?.useObsPlaneHostApi
     ? globalConfig.obsApiBaseUrl 
     : globalConfig.apiBaseUrl;
    const response = await fetch(`${baseUrl}${context}?${new URLSearchParams(searchParams).toString()}`, {
        method: 'GET',
        headers:  token ? {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${token}`
            } : {
              'Content-Type': 'application/json'
            }
    });
    await sleep(DEFAULT_TIMEOUT);
    return response;
}

export async function httpPOST(
    context: string, 
    body: object, 
    params: {searchParams?: Record<string, string>, token?: string, options?: HttpOptions}) {
    const {searchParams, token, options} = params;
    const baseUrl = options?.useObsPlaneHostApi
     ? globalConfig.obsApiBaseUrl 
     : globalConfig.apiBaseUrl;
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
    await sleep(DEFAULT_TIMEOUT);
    return response;
}

export async function httpPUT(
    context: string, 
    body: object, 
    params: {searchParams?: Record<string, string>, token?: string, options?: HttpOptions}) {
    const {searchParams, token, options} = params;
    const baseUrl = options?.useObsPlaneHostApi
     ? globalConfig.obsApiBaseUrl 
     : globalConfig.apiBaseUrl;
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
    await sleep(DEFAULT_TIMEOUT);
    return response;
}

export async function httpDELETE(
    context: string, 
    params: {searchParams?: Record<string, string>, token?: string, options?: HttpOptions}) {
    const {searchParams, token, options} = params;
    const baseUrl = options?.useObsPlaneHostApi
     ? globalConfig.obsApiBaseUrl 
     : globalConfig.apiBaseUrl;
    const response = await fetch(`${baseUrl}${context}?${new URLSearchParams(searchParams).toString()}`, {
        method: 'DELETE',
        headers: token ? {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`
        } : {
            'Content-Type': 'application/json'
        }
    });
    await sleep(DEFAULT_TIMEOUT);
    return response;
}

export async function httpPATCH(
    context: string, 
    body: object, 
    params: {searchParams?: Record<string, string>, token?: string, options?: HttpOptions}) {
    const {searchParams, token, options} = params;
    const baseUrl = options?.useObsPlaneHostApi
     ? globalConfig.obsApiBaseUrl 
     : globalConfig.apiBaseUrl;
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
    await sleep(DEFAULT_TIMEOUT);
    return response;
}

