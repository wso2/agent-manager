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

import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
  useGenerateAgentToken: () => ({
    data: undefined,
    error: null,
    isFetching: false,
    refetch: vi.fn(),
  }),
}));

vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureAccess: () => true,
}));

import { SetupAgentDrawer } from "./SetupAgentDrawer";

describe("SetupAgentDrawer", () => {
  it("offers only the agent's API key", () => {
    render(
      <SetupAgentDrawer open onClose={() => {}} orgName="org" projName="proj" agentName="agent" />,
    );

    expect(screen.getByText("Generate API Key")).toBeInTheDocument();
    expect(screen.getByText("Generate an API key for this agent.")).toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: /language/i })).not.toBeInTheDocument();
    const text = document.body.textContent ?? "";
    expect(text).not.toMatch(/trace|OTEL|amp-instrument/i);
  });
});
