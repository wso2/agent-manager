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
import { NoDataFound } from "@agent-management-platform/views";

export interface AgentTestProps {
  defaultBody?: Record<string, unknown>;
}

interface ChatMessage {
  id: string;
  role: "user" | "assistant";
  content: string;
  timestamp: Date;
}

export function AgentTest({
  defaultBody = {
    thread_id: 123,
    passenger_id: "2021 652719",
    question: "Hi, How can you help me?",
  },
}: AgentTestProps) {
  const [endpoint, setEndpoint] = useState("");
  const [message, setMessage] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const { agentId, orgId, projectId, envId } = useParams();
  const { data: endpoints } = useGetAgentEndpoints(
    {
      projName: projectId ?? "",
      orgName: orgId ?? "",
      agentName: agentId ?? "",
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

      let responseData: unknown;
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
          typeof responseData === "string"
            ? responseData
            : JSON.stringify(responseData, null, 2);

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

  return (
    <Box
      display="flex"
      flexDirection="column"
      minHeight="calc(100vh - 200px)"
      width="100%"
    >
      {/* Chat Messages Area */}
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
        {messages.length === 0 && (
          <NoDataFound
            message="Start a conversation"
            subtitle="Send a message to begin chatting with the agent"
            icon={<MessageCircle />}
          />
        )}

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
  );
}
