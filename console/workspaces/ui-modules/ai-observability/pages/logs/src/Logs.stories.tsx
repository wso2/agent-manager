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

import type { Meta, StoryObj } from "@storybook/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { LogsComponent } from "./index";

const route = "/org/org1/project/proj1/agents/agent1/environment/env1/observability/logs";

const WithRouter = () => (
  <MemoryRouter initialEntries={[route]} initialIndex={0}>
    <Routes>
      <Route
        path="/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/logs"
        element={<LogsComponent />}
      />
    </Routes>
  </MemoryRouter>
);

const meta: Meta<typeof WithRouter> = {
  title: "Pages/Logs",
  component: WithRouter,
  parameters: {
    layout: "fullscreen",
  },
  tags: ["autodocs"],
};

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const WithTimeRangeAndSort: Story = {
  render: () => (
    <MemoryRouter
      initialEntries={[`${route}?timeRange=1h&sortOrder=asc`]}
      initialIndex={0}
    >
      <Routes>
        <Route
          path="/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/logs"
          element={<LogsComponent />}
        />
      </Routes>
    </MemoryRouter>
  ),
};
