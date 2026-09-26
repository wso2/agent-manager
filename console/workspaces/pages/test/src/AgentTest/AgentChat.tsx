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

import React, { useEffect, useMemo, useState, useRef } from "react";
import {
  Box,
  Button,
  TextField,
  Typography,
  Alert,
  CircularProgress,
} from "@wso2/oxygen-ui";
import { MessageCircle, Send } from "@wso2/oxygen-ui-icons-react";
import {
  useGetAgentConfigurations,
  useGetAgentEndpoints,
  useTestAgentAPIKey,
  ConsoleAction,
  useTrack,
} from "@agent-management-platform/api-client";
import { useParams } from "react-router-dom";
import { ChatMessage } from "./subComponents/ChatMessage";
import { FadeIn, useSnackBar } from "@agent-management-platform/views";
import { readSSEStream, parseStreamChunk } from "./utils/sse";
import { INPUT_LIMITS } from "@agent-management-platform/types";

interface ChatMessage {
  id: string;
  role: "user" | "assistant";
  content: string;
  timestamp: Date;
}

// A freshly issued test key takes a couple of seconds to reach the gateway's
// policy engine (control plane -> event hub -> websocket -> policy snapshot),
// so a single delayed retry with the same key rides out that window.
const TEST_KEY_PROPAGATION_RETRY_DELAY_MS = 2000;

const FORMAT_MISMATCH_SNACKBAR_DURATION_MS = 15000;

const FORMAT_MISMATCH_SNACKBAR_TEXT: Record<"streaming" | "http", string> = {
  streaming:
    "Streamed response didn't match the expected format",
  http: "Response didn't match the expected format ",
};


const FORMAT_HINT_DETAIL: Record<"streaming" | "http", string> = {
  streaming:
    "Expected an SSE data:{node: string, content: [{type: string, text?: string}]}",
  http: "Expected JSON body: {response: string}",
};

export function AgentChat() {
  const { pushSnackBar } = useSnackBar();
  const [endpoint, setEndpoint] = useState("");
  const [message, setMessage] = useState("");
  const defaultBody = useMemo(() => {
    return {
      session_id: `session-${Math.floor(Math.random() * 1000)}`,
      message: "Hi, How can you help me?",
      context: {},
    };
  }, []);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isRetryingAuth, setIsRetryingAuth] = useState(false);
  const [keyAlert, setKeyAlert] = useState<"unauthorized" | "refreshed" | null>(
    null,
  );
  const [isRefreshingKey, setIsRefreshingKey] = useState(false);
  const [isStreaming, setIsStreaming] = useState(false);
  const abortControllerRef = useRef<AbortController | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const { agentId, orgId, projectId, envId } = useParams();
  const { data: endpoints, isLoading: isEndpointsLoading } =
    useGetAgentEndpoints(
      {
        projName: projectId,
        orgName: orgId,
        agentName: agentId,
      },
      {
        environment: envId ?? "",
      },
    );
  // GetAgent returns only the lowest environment's config, so read per-env here.
  const { data: envConfig } = useGetAgentConfigurations(
    {
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
    },
    {
      environment: envId ?? "",
    },
  );
  const securityEnabled = envConfig?.enableApiKeySecurity ?? true;
  const oauthOnly = !!(
    envConfig?.enableOAuthSecurity && !envConfig?.enableApiKeySecurity
  );
  const {
    data: testKey,
    isLoading: isLoadingTestKey,
    error: testKeyError,
    refetch: refetchTestKey,
  } = useTestAgentAPIKey(
    { orgName: orgId, projName: projectId, agentName: agentId, envId },
    { enabled: securityEnabled && !oauthOnly },
  );
  const endpointOptions = useMemo(() => {
    return Object.entries(endpoints ?? {}).map(([key, value]) => ({
      label: key,
      value: value.url,
    }));
  }, [endpoints]);

  useEffect(() => {
    if (endpointOptions.length > 0) {
      setEndpoint(endpointOptions[0].value + "/chat");
    }
  }, [endpointOptions]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const handleStreamingResponse = async (body: ReadableStream<Uint8Array>) => {
    const assistantMessageId = (Date.now() + 1).toString();
    let started = false;

    try {
      for await (const raw of readSSEStream(body)) {
        const chunk = parseStreamChunk(raw);
        if (!chunk || chunk.node !== "answer") continue;

        const text = chunk.content
          .filter((part) => part.type === "text" && typeof part.text === "string")
          .map((part) => part.text as string)
          .join("");
        if (!text) continue;

        if (!started) {
          started = true;
          setMessages((prev) => [
            ...prev,
            {
              id: assistantMessageId,
              role: "assistant",
              content: text,
              timestamp: new Date(),
            },
          ]);
        } else {
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantMessageId ? { ...m, content: m.content + text } : m,
            ),
          );
        }
      }

      // Surface a hint instead of leaving the reply blank.
      if (!started) {
        setMessages((prev) => [
          ...prev,
          {
            id: assistantMessageId,
            role: "assistant",
            content: `${FORMAT_HINT_DETAIL.streaming}`,
            timestamp: new Date(),
          },
        ]);
        pushSnackBar({
          message: FORMAT_MISMATCH_SNACKBAR_TEXT.streaming,
          type: "info",
          duration: FORMAT_MISMATCH_SNACKBAR_DURATION_MS,
        });
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") {
        return;
      }
      const errorMsg =
        err instanceof Error
          ? err.message
          : "An error occurred while streaming the response";
      setError(errorMsg);
    }
  };

  const { track } = useTrack();

  const handleStopStreaming = () => {
    abortControllerRef.current?.abort();
  };

  const handleSendMessage = async () => {
    if (!message.trim() || isLoading || oauthOnly) return;
    // Testing an agent from the console is a read path as far as this service
    // is concerned, so nothing else records that the playground is used.
    // The prompt itself is never reported.
    track(ConsoleAction.TestInvoke, { invoke_mode: "chat" });
    if (securityEnabled && !testKey?.apiKey) {
      setError("API key security is enabled, but a test API key is not available yet.");
      return;
    }

    const userMessage: ChatMessage = {
      id: Date.now().toString(),
      role: "user",
      content: message.trim(),
      timestamp: new Date(),
    };

    setMessages((prev) => [...prev, userMessage]);
    setMessage("");
    setError(null);
    setKeyAlert(null);
    setIsLoading(true);
    setIsStreaming(false);

    const abortController = new AbortController();
    abortControllerRef.current = abortController;

    try {
      const requestBody = {
        ...defaultBody,
        message: userMessage.content,
      };

      const sendChatRequest = (apiKey?: string) => {
        const headers: Record<string, string> = {
          "Content-Type": "application/json",
        };
        if (securityEnabled && apiKey) {
          headers["X-API-Key"] = apiKey;
        }
        return fetch(endpoint, {
          method: "POST",
          headers,
          body: JSON.stringify(requestBody),
          referrerPolicy: "",
          signal: abortController.signal,
        });
      };
      // The gateway strips CORS headers from auth-rejected responses, so a
      // cross-origin 401 surfaces as a thrown TypeError ("Failed to fetch")
      // rather than a readable status. While security is enabled, treat both
      // shapes as a possible auth failure (null = auth-like failure).
      const attemptSend = async (): Promise<Response | null> => {
        try {
          const res = await sendChatRequest(testKey?.apiKey);
          return securityEnabled && res.status === 401 ? null : res;
        } catch (err) {
          if (securityEnabled && err instanceof TypeError) {
            return null;
          }
          throw err;
        }
      };

      let apiResponse = await attemptSend();

      // Retry once with the same key: an auth failure right after key
      // issuance usually just means the gateway has not finished loading the
      // key. Refetching here would rotate the key and restart that
      // propagation window.
      if (apiResponse === null) {
        setIsRetryingAuth(true);
        try {
          await new Promise((resolve) =>
            setTimeout(resolve, TEST_KEY_PROPAGATION_RETRY_DELAY_MS),
          );
          apiResponse = await attemptSend();
        } finally {
          setIsRetryingAuth(false);
        }
      }

      // Persistent auth failure: hand control back to the user instead of
      // retrying blindly — restore the message so it can be resent with one
      // click after the key is refreshed. When the gateway is offline the
      // standing warning banner already explains the failure and refreshing
      // the key cannot help, so no additional alert is shown.
      if (apiResponse === null) {
        setMessages((prev) => prev.filter((m) => m.id !== userMessage.id));
        setMessage(userMessage.content);
        if (testKey?.gatewayConnected !== false) {
          setKeyAlert("unauthorized");
        }
        return;
      }

      const contentType = apiResponse.headers.get("content-type");

      if (apiResponse.ok && contentType?.includes("text/event-stream") && apiResponse.body) {
        setIsStreaming(true);
        await handleStreamingResponse(apiResponse.body);
        return;
      }

      let responseData: any;
      if (contentType && contentType.includes("application/json")) {
        responseData = await apiResponse.json();
      } else {
        responseData = await apiResponse.text();
      }

      if (!apiResponse.ok) {
        const errorMessage =
          typeof responseData === "string"
            ? responseData
            : JSON.stringify(responseData, null, 2);
        setError(
          `Request failed with status ${apiResponse.status}: ${errorMessage}`,
        );

        const errorMessageObj: ChatMessage = {
          id: (Date.now() + 1).toString(),
          role: "assistant",
          content: `Error: ${errorMessage}`,
          timestamp: new Date(),
        };
        setMessages((prev) => [...prev, errorMessageObj]);
      } else {
        const hasExpectedShape = typeof responseData?.response === "string";
        const responseText = hasExpectedShape
          ? (responseData.response as string)
          : `${JSON.stringify(responseData, null, 4)}\n\n${FORMAT_HINT_DETAIL.http}`;

        const assistantMessage: ChatMessage = {
          id: (Date.now() + 1).toString(),
          role: "assistant",
          content: responseText,
          timestamp: new Date(),
        };
        setMessages((prev) => [...prev, assistantMessage]);
        if (!hasExpectedShape) {
          pushSnackBar({
            message: FORMAT_MISMATCH_SNACKBAR_TEXT.http,
            type: "info",
            duration: FORMAT_MISMATCH_SNACKBAR_DURATION_MS,
          });
        }
      }
    } catch (err) {
      const errorMsg =
        err instanceof Error
          ? err.message
          : "An error occurred while making the request";
      setError(errorMsg);

      const errorMessageObj: ChatMessage = {
        id: (Date.now() + 1).toString(),
        role: "assistant",
        content: `Error: ${errorMsg}`,
        timestamp: new Date(),
      };
      setMessages((prev) => [...prev, errorMessageObj]);
    } finally {
      setIsLoading(false);
      setIsStreaming(false);
    }
  };

  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      handleSendMessage();
    }
  };

  const handleRefreshTestKey = async () => {
    setIsRefreshingKey(true);
    try {
      await refetchTestKey();
      setKeyAlert("refreshed");
    } finally {
      setIsRefreshingKey(false);
    }
  };

  const keyAlertBanner = (
    <>
      {securityEnabled && testKey?.gatewayConnected === false && (
        <Alert severity="warning" sx={{ borderRadius: 1 }}>
          The gateway is not connected to the control plane right now. The
          test API key has been stored but will only work once the gateway
          reconnects.
        </Alert>
      )}
      {keyAlert === "unauthorized" && (
        <Alert
          severity="error"
          action={
            <Button
              color="inherit"
              size="small"
              onClick={handleRefreshTestKey}
              disabled={isRefreshingKey}
            >
              {isRefreshingKey ? "Refreshing..." : "Refresh test key"}
            </Button>
          }
          sx={{ borderRadius: 1 }}
        >
          The request was not authorized. The test API key may not be active
          on the gateway yet.
        </Alert>
      )}
      {keyAlert === "refreshed" && (
        <Alert
          severity="info"
          onClose={() => setKeyAlert(null)}
          sx={{ borderRadius: 1 }}
        >
          Test key refreshed. Send your message again.
        </Alert>
      )}
    </>
  );

  const inputDisabled =
    oauthOnly ||
    isLoading ||
    isEndpointsLoading ||
    isLoadingTestKey ||
    (securityEnabled && !testKey?.apiKey);
  const sendDisabled = inputDisabled || !message.trim();

  if (oauthOnly) {
    return (
      <Alert severity="info">
        <Typography variant="caption">
          Testing is unavailable while OAuth is enabled. Test this endpoint out-of-band with a
          valid token.
        </Typography>
      </Alert>
    );
  }

  if (messages.length === 0) {
    return (
      <FadeIn>
        <Box
          display="flex"
          justifyContent="center"
          alignItems="center"
          width="100%"
          flexDirection="column"
          minHeight="calc(100vh - 550px)"
          gap={2}
        >
          <Box
            display="flex"
            flexDirection="column"
            gap={1}
            alignItems="center"
          >
            <MessageCircle size={40} />
            <Typography variant="h5">Start a conversation</Typography>
            <Typography variant="body2">
              Send a message to begin chatting with the agent
            </Typography>
          </Box>
          <Box width="100%" maxWidth={600} display="flex" flexDirection="column" gap={1}>
            {keyAlertBanner}
            <Box
              width="100%"
              display="flex"
              justifyContent="flex-end"
              alignItems="center"
              gap={1}
            >
              <TextField
                slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.LONG_TEXT } }}
                fullWidth
                value={message}
                onChange={(e) => setMessage(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder="Type your message..."
                variant="outlined"
                disabled={inputDisabled}
              />
              <Button
                variant="contained"
                color="primary"
                onClick={handleSendMessage}
                disabled={sendDisabled}
                startIcon={
                  isLoading || isEndpointsLoading ? (
                    <CircularProgress size={16} />
                  ) : (
                    <Send size={16} />
                  )
                }
              >
                {isLoading ? "Sending" : "Send"}
              </Button>
            </Box>
          </Box>
        </Box>
      </FadeIn>
    );
  }
  return (
    <FadeIn>
      <Box
        display="flex"
        flexDirection="column"
        height="calc(100vh - 320px)"
        width="100%"
      >
        <Box
          flex={1}
          display="flex"
          flexDirection="column"
          justifyContent="flex-end"
          gap={2}
          p={2}
          sx={{
            flexGrow: 1,
          }}
        >
          {messages.map((msg) => (
            <ChatMessage
              key={msg.id}
              id={msg.id}
              role={msg.role}
              content={msg.content}
            />
          ))}

          {isLoading && (
            <Box display="flex" justifyContent="flex-start" width="100%">
              <Box display="flex" gap={1} alignItems="flex-start">
                <CircularProgress size={16} />
                <Typography
                  variant="body2"
                  color="text.secondary"
                  sx={{ fontSize: "0.875rem" }}
                >
                  {isRetryingAuth
                    ? "Waiting for the test API key to become active..."
                    : "Loading..."}
                </Typography>
              </Box>
            </Box>
          )}
          <div ref={messagesEndRef} />
        </Box>

        {/* Error Display */}
        {keyAlertBanner}
        {error && (
          <Alert
            severity="error"
            onClose={() => setError(null)}
            sx={{
              borderRadius: 1,
            }}
          >
            {error}
          </Alert>
        )}
        {!!testKeyError && (
          <Alert severity="error" sx={{ borderRadius: 1 }}>
            Failed to obtain a test API key. Send may fail until this is resolved.
          </Alert>
        )}

        {/* Message Input Area */}
        <Box
          display="flex"
          justifyContent="flex-end"
          alignItems="center"
          gap={1}
          py={2}
        >
          <TextField
            slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.LONG_TEXT } }}
            fullWidth
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type your message..."
            variant="outlined"
            size="small"
            disabled={inputDisabled}
          />
          {isStreaming ? (
            <Button variant="outlined" color="primary" onClick={handleStopStreaming}>
              Stop
            </Button>
          ) : (
            <Button
              variant="contained"
              color="primary"
              onClick={handleSendMessage}
              disabled={sendDisabled}
              startIcon={
                isLoading || isEndpointsLoading ? (
                  <CircularProgress size={16} />
                ) : (
                  <Send size={16} />
                )
              }
            >
              {isLoading || isEndpointsLoading ? "Sending" : "Send"}
            </Button>
          )}
        </Box>
      </Box>
    </FadeIn>
  );
}
