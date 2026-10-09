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

import type { Build } from "@agent-management-platform/types";

/**
 * The request/response shape a chat agent's POST /chat speaks:
 * - standard: the platform's chat interface (Python agents)
 *   {session_id, message, context} -> {response}
 * - ballerina: a ballerina/ai ai:Listener service
 *   {message, sessionId} -> {message}
 */
export type ChatContract = "standard" | "ballerina";

/** A Ballerina buildpack agent's chat agent is an ai:Listener service. */
export function chatContractFor(build: Build | null | undefined): ChatContract {
  return build?.type === "buildpack" && build.buildpack?.language === "ballerina"
    ? "ballerina"
    : "standard";
}

export function buildChatRequestBody(
  contract: ChatContract,
  sessionId: string,
  message: string,
): Record<string, unknown> {
  return contract === "ballerina"
    ? { message, sessionId }
    : { session_id: sessionId, message, context: {} };
}

/** The agent's reply text, or undefined when the body isn't the contract's shape. */
export function readChatReply(contract: ChatContract, body: unknown): string | undefined {
  const field = contract === "ballerina" ? "message" : "response";
  const value = (body as Record<string, unknown> | null)?.[field];
  return typeof value === "string" ? value : undefined;
}

export const CHAT_REPLY_FORMAT_HINT: Record<ChatContract, string> = {
  standard: "Expected JSON body: {response: string}",
  ballerina: "Expected JSON body: {message: string}",
};
