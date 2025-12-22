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
import { MessageCircle } from "@wso2/oxygen-ui-icons-react";
import { AmpAttributes, PromptMessage } from "@agent-management-platform/types";
import { memo, useCallback, useMemo } from "react";

interface OverviewProps {
  ampAttributes?: AmpAttributes;
}

interface MessageListProps {
  title: string;
  messages: Partial<PromptMessage>[];
  getRoleColor: (role: string) => "default" | "primary" | "success" | "info";
  "data-testid"?: string;
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
}: MessageListProps) {
  if (messages.length === 0) {
    return null;
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
      input: PromptMessage[] | string | undefined
    ): (Partial<PromptMessage> | { content: string })[] => {
      if (!input) return [];
      if (typeof input === "string") {
        return [{ content: input }];
      }
      return input;
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

  const hasContent = inputMessages.length > 0 || outputMessages.length > 0;

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

  if (!hasContent) {
    return (
      <NoDataFound
        message="No input or output messages found"
        iconElement={MessageCircle}
        subtitle="No messages found"
        disableBackground
      />
    );
  }

  return (
    <Stack spacing={3}>
      <MessageList
        title="Input Messages"
        messages={inputMessages}
        getRoleColor={getRoleColor}
        data-testid="input-messages"
      />
      <MessageList
        title="Output Messages"
        messages={outputMessages}
        getRoleColor={getRoleColor}
        data-testid="output-messages"
      />
    </Stack>
  );
}
