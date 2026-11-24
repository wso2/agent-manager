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

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { PermissionTree } from "./PermissionTree";

const renderTree = (props: Partial<React.ComponentProps<typeof PermissionTree>> = {}) =>
  render(
    <ThemeProvider theme={createTheme()}>
      <PermissionTree items={[]} selectedIds={[]} onChange={vi.fn()} {...props} />
    </ThemeProvider>,
  );

describe("PermissionTree display", () => {
  it("shows the empty message when there are no permissions", () => {
    renderTree({ emptyMessage: "Nothing to grant." });
    expect(screen.getByText("Nothing to grant.")).toBeInTheDocument();
  });

  it("shows the default empty message", () => {
    renderTree();
    expect(screen.getByText("No permissions available.")).toBeInTheDocument();
  });

  it("shows item descriptions and collapses an expanded group", async () => {
    renderTree({
      items: [
        { id: "agents:read", path: ["agents", "read"], description: "View agents" },
        { id: "agents:write", path: ["agents", "write"] },
      ],
    });

    // Top-level groups start expanded.
    expect(screen.getByText("View agents")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Collapse" }));
    await waitFor(() => expect(screen.queryByText("View agents")).not.toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Expand" }));
    expect(screen.getByText("View agents")).toBeInTheDocument();
  });
});
