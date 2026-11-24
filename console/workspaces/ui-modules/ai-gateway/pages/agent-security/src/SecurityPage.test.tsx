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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { MemoryRouter, Route, Routes, useNavigate, useParams } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

// Stands in for the page: keeps local state so a remount is visible.
vi.mock("./Security.Component", () => ({
  SecurityComponent: () => {
    const { agentId } = useParams();
    const [firstAgent] = useState(agentId);
    const [clicks, setClicks] = useState(0);
    const navigate = useNavigate();
    return (
      <div>
        <span>mounted for {firstAgent}</span>
        <span>clicks {clicks}</span>
        <button onClick={() => setClicks((c) => c + 1)}>click</button>
        <button onClick={() => navigate("/agents/b/security")}>switch agent</button>
      </div>
    );
  },
}));

import { SecurityPage } from "./SecurityPage";

describe("SecurityPage", () => {
  it("resets the page state when the agent in the URL changes", () => {
    render(
      <MemoryRouter
        initialEntries={["/agents/a/security"]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route path="/agents/:agentId/security" element={<SecurityPage />} />
        </Routes>
      </MemoryRouter>,
    );
    expect(screen.getByText("mounted for a")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "click" }));
    expect(screen.getByText("clicks 1")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "switch agent" }));
    expect(screen.getByText("mounted for b")).toBeInTheDocument();
    expect(screen.getByText("clicks 0")).toBeInTheDocument();
  });
});
