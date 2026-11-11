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

export interface StreamContentPart {
  type: string;
  text?: string;
  [key: string]: unknown;
}

export interface StreamChunk {
  node: string;
  content: StreamContentPart[];
}

/**
 * Reads a `text/event-stream` body and yields each event's joined `data:`
 * payload as a raw string, in arrival order. Buffers partial lines across
 * network chunk boundaries, since those don't align with SSE event
 * boundaries (blank-line-delimited, per the SSE spec).
 */
export async function* readSSEStream(
  body: ReadableStream<Uint8Array>,
): AsyncGenerator<string> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      let boundary = buffer.indexOf("\n\n");
      while (boundary !== -1) {
        const rawEvent = buffer.slice(0, boundary);
        buffer = buffer.slice(boundary + 2);
        const data = extractDataField(rawEvent);
        if (data !== null) {
          yield data;
        }
        boundary = buffer.indexOf("\n\n");
      }
    }

    const trailing = buffer.trim();
    if (trailing) {
      const data = extractDataField(trailing);
      if (data !== null) {
        yield data;
      }
    }
  } finally {
    reader.releaseLock();
  }
}

function extractDataField(rawEvent: string): string | null {
  const dataLines: string[] = [];
  for (const line of rawEvent.split("\n")) {
    if (line.startsWith(":")) continue;
    if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).replace(/^ /, ""));
    }
  }
  return dataLines.length > 0 ? dataLines.join("\n") : null;
}

/**
 * Parses a raw SSE `data:` payload into a StreamChunk. Returns null for
 * the "[DONE]" sentinel, malformed JSON, or a payload missing the fields
 * this UI relies on — callers should skip (not crash) on null.
 */
export function parseStreamChunk(raw: string): StreamChunk | null {
  if (raw === "[DONE]") return null;

  try {
    const parsed = JSON.parse(raw);
    if (
      parsed &&
      typeof parsed === "object" &&
      typeof parsed.node === "string" &&
      Array.isArray(parsed.content)
    ) {
      return parsed as StreamChunk;
    }
    return null;
  } catch {
    return null;
  }
}
