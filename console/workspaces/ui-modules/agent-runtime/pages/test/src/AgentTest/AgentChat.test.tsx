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

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  useGetAgent,
  useGetAgentEndpoints,
  useTestAgentAPIKey,
} from "@agent-management-platform/api-client";
import { AgentChat } from "./AgentChat";

vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgent: vi.fn(),
  useGetAgentEndpoints: vi.fn(),
  useTestAgentAPIKey: vi.fn(),
}));

const pushSnackBar = vi.fn();

vi.mock("@agent-management-platform/views", () => ({
  FadeIn: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  MarkdownView: ({ content }: { content: string }) => <div>{content}</div>,
  useSnackBar: () => ({ pushSnackBar }),
}));

vi.mock("react-router-dom", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router-dom")>();
  return {
    ...actual,
    useParams: () => ({
      agentId: "agent-1",
      orgId: "org-1",
      projectId: "proj-1",
      envId: "env-1",
    }),
  };
});

function mockHooks() {
  vi.mocked(useGetAgentEndpoints).mockReturnValue({
    data: { dev: { url: "https://agent.example.test" } },
    isLoading: false,
  } as unknown as ReturnType<typeof useGetAgentEndpoints>);

  vi.mocked(useGetAgent).mockReturnValue({
    data: {
      displayName: "Test Agent",
      configurations: {
        enableApiKeySecurity: false,
        enableOAuthSecurity: false,
      },
    },
  } as ReturnType<typeof useGetAgent>);

  vi.mocked(useTestAgentAPIKey).mockReturnValue({
    data: undefined,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  } as unknown as ReturnType<typeof useTestAgentAPIKey>);
}

async function sendMessage(text: string) {
  const input = await screen.findByPlaceholderText("Type your message...");
  fireEvent.change(input, { target: { value: text } });
  const sendButton = screen.getByRole("button", { name: /send/i });
  fireEvent.click(sendButton);
}

describe("AgentChat", () => {
  beforeEach(() => {
    mockHooks();
    pushSnackBar.mockClear();
  });

  it("renders the assistant reply from a buffered JSON response", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ response: "Hello from agent" }), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      ),
    );

    render(<AgentChat />);
    await sendMessage("Hi");

    await waitFor(() =>
      expect(screen.getByText("Hello from agent")).toBeInTheDocument(),
    );
  });

  it("renders and accumulates the assistant reply as SSE chunks stream in", async () => {
    const encoder = new TextEncoder();
    let controller!: ReadableStreamDefaultController<Uint8Array>;
    const stream = new ReadableStream<Uint8Array>({
      start(c) {
        controller = c;
      },
    });

    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(stream, {
          status: 200,
          headers: { "content-type": "text/event-stream" },
        }),
      ),
    );

    render(<AgentChat />);
    await sendMessage("Hi");

    await waitFor(() =>
      expect(screen.getByRole("button", { name: /stop/i })).toBeInTheDocument(),
    );

    controller.enqueue(
      encoder.encode(
        'data: {"node":"answer","content":[{"type":"text","text":"Hel"}]}\n\n',
      ),
    );
    await waitFor(() => expect(screen.getByText("Hel")).toBeInTheDocument());

    controller.enqueue(
      encoder.encode(
        'data: {"node":"answer","content":[{"type":"text","text":"lo"}]}\n\n',
      ),
    );
    await waitFor(() => expect(screen.getByText("Hello")).toBeInTheDocument());

    controller.close();
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: /stop/i })).not.toBeInTheDocument(),
    );
  });

  it("aborts the stream and keeps the partial reply when Stop is clicked", async () => {
    const encoder = new TextEncoder();
    let controller!: ReadableStreamDefaultController<Uint8Array>;
    const stream = new ReadableStream<Uint8Array>({
      start(c) {
        controller = c;
      },
    });

    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
        init?.signal?.addEventListener("abort", () => {
          controller.error(new DOMException("Aborted", "AbortError"));
        });
        return Promise.resolve(
          new Response(stream, {
            status: 200,
            headers: { "content-type": "text/event-stream" },
          }),
        );
      }),
    );

    render(<AgentChat />);
    await sendMessage("Hi");

    await waitFor(() =>
      expect(screen.getByRole("button", { name: /stop/i })).toBeInTheDocument(),
    );

    controller.enqueue(
      encoder.encode(
        'data: {"node":"answer","content":[{"type":"text","text":"Partial"}]}\n\n',
      ),
    );
    await waitFor(() => expect(screen.getByText("Partial")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: /stop/i }));

    await waitFor(() =>
      expect(screen.queryByRole("button", { name: /stop/i })).not.toBeInTheDocument(),
    );
    expect(screen.getByText("Partial")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("shows a hint and no crash when a buffered JSON response is missing `response`", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ unexpected: "shape" }), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      ),
    );

    render(<AgentChat />);
    await sendMessage("Hi");

    await waitFor(() =>
      expect(screen.getByText(/Expected JSON body/i)).toBeInTheDocument(),
    );
    expect(pushSnackBar).toHaveBeenCalledWith(
      expect.objectContaining({ type: "info", message: expect.stringContaining("Response didn't match") }),
    );
  });

  it("shows a hint when the stream ends without a recognized answer chunk", async () => {
    const encoder = new TextEncoder();
    let controller!: ReadableStreamDefaultController<Uint8Array>;
    const stream = new ReadableStream<Uint8Array>({
      start(c) {
        controller = c;
      },
    });

    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(stream, {
          status: 200,
          headers: { "content-type": "text/event-stream" },
        }),
      ),
    );

    render(<AgentChat />);
    await sendMessage("Hi");

    await waitFor(() =>
      expect(screen.getByRole("button", { name: /stop/i })).toBeInTheDocument(),
    );

    controller.enqueue(encoder.encode('data: {"unexpected":"shape"}\n\n'));
    controller.close();

    await waitFor(() =>
      expect(screen.getByText(/Expected an SSE data:/i)).toBeInTheDocument(),
    );
    expect(pushSnackBar).toHaveBeenCalledWith(
      expect.objectContaining({ type: "info", message: expect.stringContaining("Streamed response didn't match") }),
    );
  });
});
