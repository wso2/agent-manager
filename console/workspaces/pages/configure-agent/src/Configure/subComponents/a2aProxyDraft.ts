/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { A2AProxyResponse, SetA2AProxyRequest } from "@agent-management-platform/types";

/** What a save does with a stored upstream secret. */
type StoredSecretAction = "keep" | "replace" | "clear";

export interface A2AProxyDraft {
  enabled: boolean;
  header: string;
  value: string;
  secret: StoredSecretAction;
}

export const UPSTREAM_HEADER_MAX = 255;
export const UPSTREAM_VALUE_MAX_BYTES = 8192;
const HEADER_PATTERN = /^[A-Za-z0-9_-]+$/;
// These frame or route the request, so the gateway owns them (as do all Proxy-* headers).
const RESERVED_HEADERS = new Set([
  "host", "content-length", "transfer-encoding", "connection", "upgrade", "te", "keep-alive", "trailer",
]);

const isReservedHeader = (header: string) => {
  const name = header.toLowerCase();
  return name.startsWith("proxy-") || RESERVED_HEADERS.has(name);
};

/** A non-empty header means the server stores a secret for it. */
export const hasStoredSecret = (proxy?: A2AProxyResponse) => !!proxy?.upstreamAuthHeader;

export function draftFromProxy(proxy?: A2AProxyResponse): A2AProxyDraft {
  return {
    enabled: !!proxy,
    header: proxy?.upstreamAuthHeader ?? "",
    value: "",
    secret: hasStoredSecret(proxy) ? "keep" : "replace",
  };
}

/** C0, DEL and C1, as Go's unicode.IsControl sees them. */
function hasControlCharacter(value: string): boolean {
  return [...value].some((c) => {
    const code = c.codePointAt(0) ?? 0;
    return code < 0x20 || (code >= 0x7f && code <= 0x9f);
  });
}

/** Mirrors the server: the gateway pastes into quoted YAML, doubles '%' and renders {{ }}. */
export function upstreamAuthError(draft: A2AProxyDraft, proxy?: A2AProxyResponse): string | null {
  const header = draft.header.trim();
  if (!draft.enabled || draft.secret === "clear" || (header === "" && draft.value === "")) return null;
  if (!HEADER_PATTERN.test(header)) return "Use only letters, digits, '-' and '_' in the header name";
  if (header.length > UPSTREAM_HEADER_MAX) return `The header name can be at most ${UPSTREAM_HEADER_MAX} characters`;
  if (isReservedHeader(header)) return `The ${header} header cannot be set`;
  if (draft.secret === "keep" && hasStoredSecret(proxy)) return null;
  if (draft.value === "") return "Enter the header value";
  if (new TextEncoder().encode(draft.value).length > UPSTREAM_VALUE_MAX_BYTES) {
    return `The value can be at most ${UPSTREAM_VALUE_MAX_BYTES} bytes`;
  }
  if (/['\r\n]/.test(draft.value)) return "The value cannot contain quotes or line breaks";
  if (draft.value.includes("%")) return "The value cannot contain '%'";
  if (hasControlCharacter(draft.value)) return "The value cannot contain control characters";
  if (draft.value.includes("{{")) return "The value cannot contain '{{'";
  return null;
}

/** The PUT body: no upstreamAuth clears the stored credential, a header alone keeps its secret. */
export function proxyRequestOf(draft: A2AProxyDraft, proxy?: A2AProxyResponse): SetA2AProxyRequest {
  const header = draft.header.trim();
  if (draft.secret === "clear" || header === "") return {};
  if (draft.secret === "keep" && hasStoredSecret(proxy)) return { upstreamAuth: { header } };
  return { upstreamAuth: { header, value: draft.value } };
}

/** A save that drops the stored secret is confirmed first. */
export function dropsStoredSecret(draft: A2AProxyDraft, proxy?: A2AProxyResponse): boolean {
  return hasStoredSecret(proxy) && (!draft.enabled || draft.secret === "clear" || draft.header.trim() === "");
}

export function draftChanged(draft: A2AProxyDraft, proxy?: A2AProxyResponse): boolean {
  if (draft.enabled !== !!proxy) return true;
  if (!draft.enabled) return false;
  if (draft.header.trim() !== (proxy?.upstreamAuthHeader ?? "")) return true;
  return draft.secret === "clear" || (draft.secret === "replace" && draft.value !== "");
}
