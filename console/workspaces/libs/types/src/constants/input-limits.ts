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

import { globalConfig } from '../config';

/**
 * Request body cap used when MAX_REQUEST_BODY_BYTES is unset: 0, no limit.
 * Deployments behind a WAF with a body limit set it (e.g. 57344, 56 KB, under
 * a 64 KB WAF limit) so an oversized save is refused with a readable error.
 */
export const DEFAULT_MAX_REQUEST_BODY_BYTES = 0;

/**
 * Parses a byte limit from runtime config. The template substitutes an unset
 * variable with an empty string, which means "use the default". Anything else
 * that is not a non-negative integer also falls back, with a warning: for the
 * request-body limit the default is no limit, so a mistyped value would
 * otherwise switch the check off without any sign.
 */
const readByteLimit = (
  name: string,
  raw: string | number | undefined,
  fallback: number,
): number => {
  if (raw === undefined) return fallback;
  const text = typeof raw === 'number' ? null : raw.trim();
  if (text === '') return fallback;
  const parsed = text === null ? raw as number : Number(text);
  if (Number.isInteger(parsed) && parsed >= 0) return parsed;
  // eslint-disable-next-line no-console
  console.warn(`Ignoring invalid ${name} "${String(raw)}"; expected a byte count. Using ${fallback}.`);
  return fallback;
};

/**
 * Largest request body, in bytes, the console sends on a write. 0 means no
 * limit, which is the default; a deployment turns the check on by setting
 * MAX_REQUEST_BODY_BYTES.
 */
export const getMaxRequestBodyBytes = (): number =>
  readByteLimit(
    'MAX_REQUEST_BODY_BYTES',
    globalConfig?.maxRequestBodyBytes,
    DEFAULT_MAX_REQUEST_BODY_BYTES,
  );

/** UTF-8 size of a string, which is what the backend and a WAF measure. */
export const utf8ByteLength = (value: string): number => new TextEncoder().encode(value).length;

/**
 * Formats a byte count for messages. Whole units read as units (1000000 →
 * "1 MB", 57344 → "56 KB"); anything else is exact (57345 → "57,345 bytes"),
 * so a value just over a limit never rounds to the limit itself.
 */
export const formatBytes = (bytes: number): string => {
  if (bytes >= 1_000_000 && bytes % 1_000_000 === 0) return `${bytes / 1_000_000} MB`;
  if (bytes >= 1024 && bytes % 1024 === 0) return `${bytes / 1024} KB`;
  return `${bytes.toLocaleString('en-US')} bytes`;
};

/**
 * Per-field character limits for console form inputs.
 *
 * These exist so a single oversized field cannot push a request past the
 * 64 KB body limit of the AWS WAF in front of the cloud and on-prem
 * deployments: the WAF drops such a request before the service sees it, and
 * the user gets a bare 403 with no field to blame.
 * Enforcing the limit at the input means the user sees the cap while typing
 * instead of discovering it on submit.
 *
 * Counts are characters, not bytes. A non-ASCII character can serialise to
 * up to 4 bytes, so a field's byte cost can be several times its character
 * count. The caps are per field, so a form that fills several large fields
 * can still exceed the WAF limit.
 */
export const INPUT_LIMITS = {
  /** Generated/URL-safe handles (agent name, scope name, role handle). */
  HANDLE: 50,
  /** Human-facing names and display names. */
  NAME: 100,
  /** Single-line free text: titles, labels, summaries. */
  SHORT_TEXT: 255,
  /**
   * Description fields across every create/edit form, including the markdown
   * ones. Generous enough for a few paragraphs of prose with formatting, and
   * still far below the WAF body limit.
   */
  DESCRIPTION: 2_000,
  /** Multi-line free text that is expected to be long: README, instructions. */
  LONG_TEXT: 8_000,
  /** Prompts and markdown documents authored in the console. */
  PROMPT: 16_000,
  /** Source code authored in the console (evaluator bodies, config editors). */
  SOURCE: 32_000,
  /** URLs and endpoints. */
  URL: 2_048,
  /** Environment variable / header / parameter keys. */
  KEY: 128,
  /** Environment variable / header / parameter values. */
  VALUE: 4_096,
  /** Filesystem-ish paths handed to the build container. */
  PATH: 512,
  /** Secrets, API keys and tokens pasted into the console. */
  SECRET: 4_096,
  /** Passwords typed into the console. */
  PASSWORD: 128,
} as const;

export type InputLimit = (typeof INPUT_LIMITS)[keyof typeof INPUT_LIMITS];
