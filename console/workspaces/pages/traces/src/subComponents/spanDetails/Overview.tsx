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

import { NoDataFound } from "@agent-management-platform/views";
import {
  Box,
  Card,
  CardContent,
  Chip,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { Info } from "@wso2/oxygen-ui-icons-react";
import { AmpAttributes, PromptMessage, ToolData, AgentData } from "@agent-management-platform/types";
import { memo, useCallback, useMemo } from "react";

interface OverviewProps {
  ampAttributes?: AmpAttributes;
}

interface MessageListProps {
  title: string;
  messages: Partial<PromptMessage>[];
  getRoleColor: (role: string) => "default" | "primary" | "success" | "info";
  "data-testid"?: string;
  showEmptyMessage?: boolean;
}

function formattedMessage(message: string) {
  try {
    return JSON.stringify(JSON.parse(message), null, 2);
  } catch {
    return message;
  }
}
const MessageList = memo(function MessageList({
  title,
  messages,
  getRoleColor,
  "data-testid": testId,
  showEmptyMessage = false,
}: MessageListProps) {
  if (messages.length === 0) {
    if (!showEmptyMessage) {
      return null;
    }
    
    return (
      <Box data-testid={testId}>
        <Typography variant="h6" sx={{ mb: 2 }}>
          {title}
        </Typography>
        <Card variant="outlined" sx={{ bgcolor: 'action.hover' }}>
          <CardContent>
            <Typography variant="body2" color="text.secondary">
              No data available
            </Typography>
          </CardContent>
        </Card>
      </Box>
    );
  }

  return (
    <Box data-testid={testId}>
      <Typography variant="h6" sx={{ mb: 2 }}>
        {title}
      </Typography>
      <Stack spacing={2}>
        {messages.map((message, index) => {
          const messageKey =
            (message as PromptMessage & { id?: string }).id ?? index;

          return (
            <Card key={messageKey} variant="outlined">
              <CardContent>
                <Stack spacing={1.5}>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    {message?.role && message.role !== "unknown" && (
                      <Chip
                        label={message.role}
                        size="small"
                        color={getRoleColor(message.role)}
                        variant="outlined"
                      />
                    )}
                  </Box>
                  {message.content && (
                    <Typography
                      variant="body2"
                      sx={{
                        whiteSpace: "pre-wrap",
                        wordBreak: "break-word",
                      }}
                    >
                      {formattedMessage(message.content)}
                    </Typography>
                  )}
                  {message.toolCalls && message.toolCalls.length > 0 && (
                    <Box>
                      <Stack spacing={1}>
                        {message.toolCalls.map((toolCall, toolIndex) => {
                          const toolCallKey = toolCall.id ?? toolIndex;

                          return (
                            <Card key={toolCallKey} variant="outlined">
                              <CardContent sx={{ "&:last-child": { pb: 1.5 } }}>
                                <Typography
                                  variant="caption"
                                  sx={{ fontWeight: "bold" }}
                                >
                                  {toolCall.name}
                                </Typography>
                                {toolCall.arguments && (
                                  <Typography
                                    variant="caption"
                                    sx={{
                                      display: "block",
                                      mt: 0.5,
                                      fontFamily: "monospace",
                                      whiteSpace: "pre-wrap",
                                      wordBreak: "break-word",
                                    }}
                                  >
                                    {formattedMessage(toolCall.arguments)}
                                  </Typography>
                                )}
                              </CardContent>
                            </Card>
                          );
                        })}
                      </Stack>
                    </Box>
                  )}
                </Stack>
              </CardContent>
            </Card>
          );
        })}
      </Stack>
    </Box>
  );
});

export function Overview({ ampAttributes }: OverviewProps) {
  const normalizeMessages = useCallback(
    (
      input: PromptMessage[] | string[] | string | undefined
    ): (Partial<PromptMessage> | { content: string })[] => {
      if (!input) return [];
      if (typeof input === "string") {
        return [{ content: input }];
      }
      // Handle string arrays (e.g., for embedding documents)
      if (Array.isArray(input) && input.length > 0 && typeof input[0] === "string") {
        return (input as string[]).map(doc => ({ content: doc }));
      }
      // Handle PromptMessage arrays
      return input as PromptMessage[];
    },
    []
  );

  const inputMessages = useMemo(
    () => normalizeMessages(ampAttributes?.input),
    [ampAttributes?.input, normalizeMessages]
  );

  const outputMessages = useMemo(
    () => normalizeMessages(ampAttributes?.output),
    [ampAttributes?.output, normalizeMessages]
  );

  // Extract name from data based on kind
  const name = useMemo(() => {
    const { kind, data } = ampAttributes || {};
    if (kind === 'tool' && data) {
      return (data as ToolData).name;
    } else if (kind === 'agent' && data) {
      return (data as AgentData).name;
    }
    return undefined;
  }, [ampAttributes]);

  // Extract system prompt for agent spans
  const systemPrompt = useMemo(() => {
    const { kind, data } = ampAttributes || {};
    if (kind === 'agent' && data) {
      return (data as AgentData).systemPrompt;
    }
    return undefined;
  }, [ampAttributes]);

  const hasContent = inputMessages.length > 0 || outputMessages.length > 0;

  // Check if this is a span type that should have input/output
  const shouldHaveInputOutput = useMemo(() => {
    const { kind } = ampAttributes || {};
    return kind === 'llm' || kind === 'tool' || kind === 'agent' || kind === 'embedding';
  }, [ampAttributes]);

  const getRoleColor = useCallback((role: string) => {
    switch (role) {
      case "system":
        return "default";
      case "user":
        return "primary";
      case "assistant":
        return "success";
      case "tool":
        return "info";
      default:
        return "default";
    }
  }, []);

  if (!hasContent && !name) {
    return (
      <NoDataFound
        message="Failed to extract span details"
        iconElement={Info}
        subtitle="Try selecting a different span"
        disableBackground
      />
    );
  }

  return (
    <Stack spacing={3}>
      {name && (
        <Stack>
          <Typography variant="h6">Name</Typography>
          <Card variant="outlined">
            <CardContent>
              <Typography variant="body2">{name}</Typography>
            </CardContent>
          </Card>
        </Stack>
      )}

      {systemPrompt && (
        <Stack>
          <Typography variant="h6">System Prompt</Typography>
          <Card variant="outlined">
            <CardContent>
              <Typography 
                variant="body2"
                sx={{
                  whiteSpace: "pre-wrap",
                  wordBreak: "break-word",
                }}
              >
                {formattedMessage(systemPrompt)}
              </Typography>
            </CardContent>
          </Card>
        </Stack>
      )}

      <MessageList
        title="Input Messages"
        messages={inputMessages}
        getRoleColor={getRoleColor}
        data-testid="input-messages"
        showEmptyMessage={shouldHaveInputOutput && name !== undefined}
      />
      <MessageList
        title="Output Messages"
        messages={outputMessages}
        getRoleColor={getRoleColor}
        data-testid="output-messages"
        showEmptyMessage={shouldHaveInputOutput && name !== undefined}
      />
    </Stack>
  );
}
