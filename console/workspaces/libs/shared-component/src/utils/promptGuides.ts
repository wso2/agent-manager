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

import { globalConfig } from "@agent-management-platform/types";
import { getScriptRef } from "./releaseRef";

/**
 * Base URL of the console's prompts/ directory. The guides are read by external
 * AI assistants, not the browser, so the default is the public
 * raw.githubusercontent.com copy at the current release ref;
 * `globalConfig.promptsBaseUrl` overrides it.
 */
export function getPromptsBaseUrl(): string {
  const configured = globalConfig.promptsBaseUrl?.trim();
  if (configured) return configured.replace(/\/+$/, "");
  return `https://raw.githubusercontent.com/wso2/agent-manager/${getScriptRef()}/console/apps/web-ui/public/prompts`;
}

/** Full URL for a prompt guide, pinned to the current release ref. */
export function getPromptGuideUrl(fileName: string): string {
  return `${getPromptsBaseUrl()}/${fileName}`;
}
