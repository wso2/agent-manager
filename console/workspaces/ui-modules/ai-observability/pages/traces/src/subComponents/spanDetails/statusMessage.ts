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

// Helpers for the payload embedded in a span status message. Gateway/LLM errors
// arrive as e.g.
//   "Error code: 422 - {'message': {'action': 'GUARDRAIL_INTERVENED',
//     'actionReason': 'Violation of ...', ...}, 'type': 'CONTENT_LENGTH_GUARDRAIL'}"
// i.e. a human prefix followed by a Python dict literal (single quotes,
// None/True/False). These helpers extract and normalize that payload.

// Converts a Python dict literal into JSON text. A char-by-char tokenizer
// re-encodes each string so apostrophes inside values (e.g. "someone's") don't
// break, and only the non-string parts get None/True/False translated.
function pythonDictToJson(input: string): string {
  const ESCAPES: Record<string, string> = {
    n: "\n",
    t: "\t",
    r: "\r",
    "\\": "\\",
    "'": "'",
    '"': '"',
  };
  let out = "";
  let buf = ""; // pending non-string run
  const flush = () => {
    out += buf
      .replace(/\bNone\b/g, "null")
      .replace(/\bTrue\b/g, "true")
      .replace(/\bFalse\b/g, "false");
    buf = "";
  };
  let i = 0;
  while (i < input.length) {
    const ch = input[i];
    if (ch === "'" || ch === '"') {
      flush();
      const quote = ch;
      i++;
      let str = "";
      while (i < input.length) {
        const c = input[i];
        if (c === "\\") {
          const next = input[i + 1] ?? "";
          str += next in ESCAPES ? ESCAPES[next] : next;
          i += 2;
          continue;
        }
        if (c === quote) {
          i++;
          break;
        }
        str += c;
        i++;
      }
      out += JSON.stringify(str); // re-encode with proper JSON escaping
      continue;
    }
    buf += ch;
    i++;
  }
  flush();
  return out;
}

// Extracts and parses the object embedded in a status message. Scans for the
// balanced object (ignoring braces inside strings), then tries plain JSON first
// and a Python-dict normalization. Returns the parsed object, or undefined when
// nothing parseable is present.
export function parseEmbeddedObject(
  message: string
): Record<string, unknown> | undefined {
  const start = message.indexOf("{");
  if (start === -1) {
    return undefined;
  }
  let depth = 0;
  let quote: string | null = null;
  let end = -1;
  for (let i = start; i < message.length; i++) {
    const ch = message[i];
    if (quote) {
      if (ch === "\\") {
        i++;
        continue;
      }
      if (ch === quote) quote = null;
      continue;
    }
    if (ch === "'" || ch === '"') {
      quote = ch;
      continue;
    }
    if (ch === "{") depth++;
    else if (ch === "}") {
      depth--;
      if (depth === 0) {
        end = i;
        break;
      }
    }
  }
  if (end === -1) {
    return undefined;
  }
  const candidate = message.slice(start, end + 1);
  for (const text of [candidate, pythonDictToJson(candidate)]) {
    try {
      const parsed = JSON.parse(text);
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
        return parsed as Record<string, unknown>;
      }
    } catch {
      // try the next candidate
    }
  }
  return undefined;
}

// Recursively finds the first non-empty string value stored under `key`.
function deepFindString(value: unknown, key: string): string | undefined {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    for (const [k, v] of Object.entries(value)) {
      if (k === key && typeof v === "string" && v.trim()) {
        return v.trim();
      }
      const found = deepFindString(v, key);
      if (found) return found;
    }
  } else if (Array.isArray(value)) {
    for (const item of value) {
      const found = deepFindString(item, key);
      if (found) return found;
    }
  }
  return undefined;
}

// Returns the guardrail actionReason embedded in a status message (e.g.
// "Violation of applied content length constraints detected."), or undefined
// when the message carries no parseable actionReason.
export function extractActionReason(message: string): string | undefined {
  const obj = parseEmbeddedObject(message);
  if (!obj) {
    return undefined;
  }
  return deepFindString(obj, "actionReason");
}

// Structured view of a status message, used by the Status tab. 
export interface StatusMessageDetails {
  action?: string;
  actionReason?: string;
  direction?: string;
  interveningGuardrail?: string;
  type?: string;
  parsed?: Record<string, unknown>;
  raw: string;
}

export function getStatusMessageDetails(message: string): StatusMessageDetails {
  const parsed = parseEmbeddedObject(message);
  const pick = (key: string) =>
    parsed ? deepFindString(parsed, key) : undefined;
  return {
    action: pick("action"),
    actionReason: pick("actionReason"),
    direction: pick("direction"),
    interveningGuardrail: pick("interveningGuardrail"),
    type: pick("type"),
    parsed,
    raw: message,
  };
}
