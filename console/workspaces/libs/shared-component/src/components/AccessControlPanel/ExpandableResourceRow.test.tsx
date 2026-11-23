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

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("swagger-ui-react", () => ({
  default: (props: { spec?: { paths?: Record<string, unknown> } }) => (
    <div data-testid="swagger">{JSON.stringify(props.spec?.paths ?? null)}</div>
  ),
}));

import ExpandableResourceRow from "./ExpandableResourceRow";

describe("ExpandableResourceRow", () => {
  it("shows method, path and summary and reports clicks separately from expand", () => {
    const onRowClick = vi.fn();
    const onToggleOpen = vi.fn();
    render(
      <ExpandableResourceRow
        resource={{ method: "post", path: "/orders", summary: "Create order" }}
        isOpen={false}
        onRowClick={onRowClick}
        onToggleOpen={onToggleOpen}
      />,
    );
    expect(screen.getByText("POST")).toBeInTheDocument();
    expect(screen.getByText("Create order")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Expand details" }));
    expect(onToggleOpen).toHaveBeenCalled();
    expect(onRowClick).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText("/orders"));
    expect(onRowClick).toHaveBeenCalled();
  });

  it("prefers a supplied operation spec", async () => {
    render(
      <ExpandableResourceRow
        resource={{ method: "GET", path: "/a" }}
        operationSpec={{ paths: { "/custom": {} } }}
        isOpen
        onToggleOpen={vi.fn()}
      />,
    );
    expect((await screen.findByTestId("swagger")).textContent).toBe('{"/custom":{}}');
  });

  it("falls back to plain text for non-HTTP items", () => {
    render(
      <ExpandableResourceRow
        resource={{ method: "PROMPT", path: "summarize" }}
        isOpen
        showSummary={false}
        noSummaryText="Nothing here"
        onToggleOpen={vi.fn()}
      />,
    );
    expect(screen.getByText("PROMPT summarize")).toBeInTheDocument();
    expect(screen.getByText("Nothing here")).toBeInTheDocument();
    expect(screen.queryByTestId("swagger")).not.toBeInTheDocument();
  });
});
