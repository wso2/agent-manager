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
import { useGetAgentEndpoints } from "@agent-management-platform/api-client";
import { useParams } from "react-router-dom";
import { ChatMessage } from "./subComponents/ChatMessage";
import { FadeIn } from "@agent-management-platform/views";

interface ChatMessage {
  id: string;
  role: "user" | "assistant";
  content: string;
  timestamp: Date;
}


export function AgentChat() {
  const [endpoint, setEndpoint] = useState("");
  const [message, setMessage] = useState("");
  const defaultBody = useMemo(() => {
    return {
      thread_id: Math.floor(Math.random() * 1000),
      passenger_id: "2021 652719",
      question: "Hi, How can you help me?",
    };
  }, []);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const { agentId, orgId, projectId, envId } = useParams();
  const { data: endpoints } = useGetAgentEndpoints(
    {
      projName: projectId,
      orgName: orgId,
      agentName: agentId,
    },
    {
      environment: envId ?? "",
    }
  );
  const endpointOptions = useMemo(() => {
    return Object.entries(endpoints ?? {}).map(([key, value]) => ({
      label: key,
      value: value.url,
    }));
  }, [endpoints]);

  useEffect(() => {
    if (endpointOptions.length > 0) {
      setEndpoint(endpointOptions[0].value + "/invocations");
    }
  }, [endpointOptions]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const handleSendMessage = async () => {
    if (!message.trim() || isLoading) return;

    const userMessage: ChatMessage = {
      id: Date.now().toString(),
      role: "user",
      content: message.trim(),
      timestamp: new Date(),
    };

    setMessages((prev) => [...prev, userMessage]);
    setMessage("");
    setError(null);
    setIsLoading(true);

    try {
      const requestBody = {
        ...defaultBody,
        question: userMessage.content,
      };

      const apiResponse = await fetch(endpoint, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(requestBody),
        referrerPolicy: "",
      });

      let responseData: any;
      const contentType = apiResponse.headers.get("content-type");
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
          `Request failed with status ${apiResponse.status}: ${errorMessage}`
        );

        const errorMessageObj: ChatMessage = {
          id: (Date.now() + 1).toString(),
          role: "assistant",
          content: `Error: ${errorMessage}`,
          timestamp: new Date(),
        };
        setMessages((prev) => [...prev, errorMessageObj]);
      } else {
        const responseText =
          typeof responseData?.results === "string"
            ? (responseData.results as string)
            : JSON.stringify(responseData?.result, null, 4);

        const assistantMessage: ChatMessage = {
          id: (Date.now() + 1).toString(),
          role: "assistant",
          content: responseText,
          timestamp: new Date(),
        };
        setMessages((prev) => [...prev, assistantMessage]);
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
    }
  };

  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      handleSendMessage();
    }
  };

  if (messages.length === 0) {
    return (
      <FadeIn>
        <Box
          display="flex"
          justifyContent="center"
          alignItems="center"
          width="100%"
          flexDirection="column"
          minHeight="calc(100vh - 350px)"
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
          <Box width="100%" maxWidth={600}>
            <Box
              width="100%"
              display="flex"
              justifyContent="flex-end"
              alignItems="center"
              gap={1}
            >
              <TextField
                fullWidth
                value={message}
                onChange={(e) => setMessage(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder="Type your message..."
                variant="outlined"
                disabled={isLoading}
              />
              <Button
                variant="contained"
                color="primary"
                onClick={handleSendMessage}
                disabled={isLoading || !message.trim()}
                startIcon={
                  isLoading ? (
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
        minHeight="calc(100vh - 250px)"
        width="100%"
      >
        <Box
          flex={1}
          overflow="auto"
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
                  Loading...
                </Typography>
              </Box>
            </Box>
          )}
          <div ref={messagesEndRef} />
        </Box>

        {/* Error Display */}
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

        {/* Message Input Area */}
        <Box
          display="flex"
          justifyContent="flex-end"
          alignItems="center"
          gap={1}
          pt={2}
        >
          <TextField
            fullWidth
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type your message..."
            variant="outlined"
            size="small"
            disabled={isLoading}
          />
          <Button
            variant="contained"
            color="primary"
            onClick={handleSendMessage}
            disabled={isLoading || !message.trim()}
            startIcon={
              isLoading ? <CircularProgress size={16} /> : <Send size={16} />
            }
          >
            {isLoading ? "Sending" : "Send"}
          </Button>
        </Box>
      </Box>
    </FadeIn>
  );
}
