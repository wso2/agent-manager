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

/**
 * A Ballerina chat agent is a ballerina/ai `ai:Listener` service: unlike the
 * platform's standard chat interface (fixed port 8000, base path "/"), it serves
 * `POST <basePath>/chat` on its own port, so the form asks for both. Mirrors
 * client.ChatAPIDefaultPort in agent-manager-service.
 */
export const BALLERINA_CHAT_DEFAULT_PORT = 9090;

/** Whether a Chat Agent of this language names its own port and base path. */
export function chatAgentHasOwnEndpoint(language: string | undefined): boolean {
  return language === "ballerina";
}
