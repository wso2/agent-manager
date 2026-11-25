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
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { LegacyCreateRedirect } from "./LegacyCreateRedirect";

const Destination = () => {
  const location = useLocation();
  return <div>{`at ${location.pathname} ${JSON.stringify(location.state)}`}</div>;
};

const renderAt = (entry: { pathname: string; state?: unknown }) =>
  render(
    <MemoryRouter initialEntries={[entry]} future={{ v7_startTransition: true }}>
      <Routes>
        {/* In the console the Add a New Agent page claims the exact path. */}
        <Route path="/org/:orgId/project/:projectId/newAgent" element={<Destination />} />
        <Route path="/org/:orgId/project/:projectId/newAgent/*" element={<LegacyCreateRedirect />} />
        <Route path="/org/:orgId/newAgent/*" element={<LegacyCreateRedirect />} />
        <Route path="*" element={<Destination />} />
      </Routes>
    </MemoryRouter>,
  );

describe("LegacyCreateRedirect", () => {
  it("sends an old create URL to Add a New Agent in the same project, keeping the wizard state", () => {
    renderAt({ pathname: "/org/acme/project/shop/newAgent/create", state: { from: "agents" } });

    expect(
      screen.getByText('at /org/acme/project/shop/newAgent {"from":"agents"}'),
    ).toBeInTheDocument();
  });

  it("uses the default project when the URL has none", () => {
    renderAt({ pathname: "/org/acme/newAgent/langgraph" });

    expect(screen.getByText("at /org/acme/project/default/newAgent null")).toBeInTheDocument();
  });
});
