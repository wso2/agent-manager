/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { afterEach, describe, expect, it } from "vitest";
import { getPromptGuideUrl, getPromptsBaseUrl } from "./promptGuides";

const config = (window as unknown as { __RUNTIME_CONFIG__: Record<string, string | undefined> })
  .__RUNTIME_CONFIG__;

afterEach(() => {
  delete config.promptsBaseUrl;
  delete config.ampVersion;
});

describe("prompt guide URLs", () => {
  it("defaults to the GitHub raw copy at the release ref", () => {
    config.ampVersion = "1.4.0";
    expect(getPromptGuideUrl("agent.md")).toBe(
      "https://raw.githubusercontent.com/wso2/agent-manager/amp/v1.4.0/console/apps/web-ui/public/prompts/agent.md",
    );
  });

  it("follows main without a release version", () => {
    expect(getPromptsBaseUrl()).toContain("/agent-manager/main/console/");
  });

  it("uses the configured base URL without trailing slashes", () => {
    config.promptsBaseUrl = "  https://docs.example.com/prompts// ";
    expect(getPromptGuideUrl("x.md")).toBe("https://docs.example.com/prompts/x.md");
  });
});
