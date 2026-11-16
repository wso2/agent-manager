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

import { describe, it, expect } from "vitest";
import { readSSEStream, parseStreamChunk, type StreamChunk } from "./sse";

function streamFromChunks(chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(chunk));
      }
      controller.close();
    },
  });
}

async function collect(stream: ReadableStream<Uint8Array>): Promise<string[]> {
  const results: string[] = [];
  for await (const data of readSSEStream(stream)) {
    results.push(data);
  }
  return results;
}

describe("readSSEStream", () => {
  it("yields the data payload for a single complete event", async () => {
    const stream = streamFromChunks(['data: {"node":"answer"}\n\n']);
    await expect(collect(stream)).resolves.toEqual(['{"node":"answer"}']);
  });

  it("reassembles an event split across multiple network reads", async () => {
    const stream = streamFromChunks(['data: {"no', 'de":"answer"}\n\n']);
    await expect(collect(stream)).resolves.toEqual(['{"node":"answer"}']);
  });

  it("joins multi-line data fields with a newline", async () => {
    const stream = streamFromChunks(["data: line one\ndata: line two\n\n"]);
    await expect(collect(stream)).resolves.toEqual(["line one\nline two"]);
  });

  it("ignores comment lines and unrelated SSE fields", async () => {
    const stream = streamFromChunks([
      ": this is a comment\nevent: answer\nid: 1\ndata: hello\nretry: 3000\n\n",
    ]);
    await expect(collect(stream)).resolves.toEqual(["hello"]);
  });

  it("yields multiple events found in a single network read", async () => {
    const stream = streamFromChunks(["data: first\n\ndata: second\n\n"]);
    await expect(collect(stream)).resolves.toEqual(["first", "second"]);
  });

  it("flushes a trailing event that arrives without a final blank line", async () => {
    const stream = streamFromChunks(["data: no trailing newline"]);
    await expect(collect(stream)).resolves.toEqual(["no trailing newline"]);
  });
});

describe("parseStreamChunk", () => {
  it("parses a valid chunk payload", () => {
    const chunk = parseStreamChunk(
      '{"node":"answer","content":[{"type":"text","text":" including"}]}',
    );
    const expected: StreamChunk = {
      node: "answer",
      content: [{ type: "text", text: " including" }],
    };
    expect(chunk).toEqual(expected);
  });

  it("returns null for the [DONE] sentinel", () => {
    expect(parseStreamChunk("[DONE]")).toBeNull();
  });

  it("returns null for malformed JSON", () => {
    expect(parseStreamChunk("{not json")).toBeNull();
  });

  it("returns null when required fields are missing", () => {
    expect(parseStreamChunk('{"content":[]}')).toBeNull();
    expect(parseStreamChunk('{"node":"answer"}')).toBeNull();
  });
});
