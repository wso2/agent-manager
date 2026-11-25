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

// Small presentational components that share a single file to keep the test
// suite fast; each describe block covers one component's visible contract.

import type { ComponentProps } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ListingTable } from "@wso2/oxygen-ui";
import { AgentTypeChips } from "./AgentTypeChips";
import { BackButton } from "./BackButton";
import { CollapsibleSection } from "./CollapsibleSection";
import { EditFormSkeleton } from "./EditFormSkeleton";
import { EntityHeader } from "./EntityHeader";
import { ErrorPages } from "./ErrorPages";
import { GatewayHostingChip } from "./GatewayHostingChip";
import { GatewayTypeChip } from "./GatewayTypeChip";
import { ListingSkeletonRows } from "./ListingSkeletonRows";
import { MCPLogo } from "./MCPLogo";
import { OverviewSectionCard } from "./OverviewSectionCard";
import { ResilienceTimeoutFields } from "./ResilienceTimeoutFields";

describe("AgentTypeChips", () => {
  it.each([
    [{ provisioning: { type: "external" } }, "External"],
    [{}, "Unknown"],
    [{ provisioning: { type: "internal" }, agentType: { type: "agent-api", subType: "chat-api" } }, "Chat"],
  ])("labels %j as %s", (props, label) => {
    render(<AgentTypeChips {...(props as ComponentProps<typeof AgentTypeChips>)} />);
    expect(screen.getByText(label)).toBeInTheDocument();
  });

  it("renders nothing for a platform agent without a sub-type", () => {
    const { container } = render(<AgentTypeChips provisioning={{ type: "internal" } as never} />);
    expect(container).toBeEmptyDOMElement();
  });
});

describe("BackButton", () => {
  it("links back to the given route", () => {
    render(
      <MemoryRouter>
        <BackButton to="/agents" label="Back to agents" />
      </MemoryRouter>,
    );
    expect(screen.getByRole("link", { name: "Back to agents" })).toHaveAttribute("href", "/agents");
  });
});

describe("CollapsibleSection", () => {
  it("keeps content collapsed until shown", () => {
    const { container, rerender } = render(
      <CollapsibleSection show={false}>
        <span>body</span>
      </CollapsibleSection>,
    );
    expect(container.querySelector(".MuiCollapse-hidden")).toBeInTheDocument();
    rerender(
      <CollapsibleSection show>
        <span>body</span>
      </CollapsibleSection>,
    );
    expect(screen.getByText("body")).toBeInTheDocument();
  });
});

describe("EditFormSkeleton", () => {
  it.each([
    [undefined, 2],
    [4, 4],
  ])("renders %s tab placeholders", (tabs, count) => {
    const { container } = render(<EditFormSkeleton tabs={tabs} />);
    // tabs + title + description + input + three list rows
    expect(container.querySelectorAll(".MuiSkeleton-root")).toHaveLength(count + 6);
  });
});

describe("EntityHeader", () => {
  it("shows the name initial, subtitle, id and badge", () => {
    render(
      <EntityHeader fallback="U" name="alice" subtitle="alice@example.com" id="u-1" badge={<span>Read-only</span>} />,
    );
    expect(screen.getByText("A")).toBeInTheDocument();
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    expect(screen.getByText("u-1")).toBeInTheDocument();
    expect(screen.getByText("Read-only")).toBeInTheDocument();
  });

  it("falls back to the given letter when the name is empty", () => {
    render(<EntityHeader fallback="G" name="" id="g-1" />);
    expect(screen.getByText("G")).toBeInTheDocument();
  });
});

describe("ErrorPages", () => {
  const reload = vi.fn();
  const originalLocation = window.location;
  afterEach(() => {
    Object.defineProperty(window, "location", { configurable: true, value: originalLocation });
  });

  it("renders the 404 page with a home link", () => {
    render(<ErrorPages.NotFound />);
    expect(screen.getByText("404")).toBeInTheDocument();
    expect(screen.getByText("Page Not Found")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Go to Home" })).toHaveAttribute("href", "/");
  });

  it("renders the generic error page", () => {
    render(<ErrorPages.Oops />);
    expect(screen.getByText("Something went wrong.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Go to Home" })).toBeInTheDocument();
  });

  it("renders a custom error with sign-out and retry", () => {
    Object.defineProperty(window, "location", {
      configurable: true,
      value: { ...originalLocation, reload },
    });
    const onLogout = vi.fn();
    render(
      <ErrorPages.CustomError title="No access" subTitle="403" message="Ask an admin." onLogout={onLogout} />,
    );
    expect(screen.getByText("No access")).toBeInTheDocument();
    expect(screen.getByText("403")).toBeInTheDocument();
    expect(screen.getByText("Ask an admin.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Sign out" }));
    expect(onLogout).toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(reload).toHaveBeenCalled();
  });

  it("omits sign-out when no handler is given", () => {
    render(<ErrorPages.CustomError title="t" message="m" />);
    expect(screen.queryByRole("button", { name: "Sign out" })).not.toBeInTheDocument();
  });
});

describe("GatewayHostingChip", () => {
  it.each([
    ["SELF_HOSTED", "Self-hosted"],
    ["MANAGED", "Managed"],
    [undefined, "Managed"],
  ])("labels %s as %s", (hosting, label) => {
    render(<GatewayHostingChip gateway={{ hosting: hosting as never }} />);
    expect(screen.getByText(label)).toBeInTheDocument();
  });
});

describe("GatewayTypeChip", () => {
  it.each([
    ["INGRESS", "Ingress"],
    ["egress", "Egress"],
    ["BOTH", "Ingress + Egress"],
    ["custom", "CUSTOM"],
  ])("labels %s as %s", (type, label) => {
    render(<GatewayTypeChip type={type} />);
    expect(screen.getByText(label)).toBeInTheDocument();
  });
});

describe("ListingSkeletonRows", () => {
  it.each([
    [undefined, undefined, 5, 2],
    [3, 1 as const, 3, 1],
  ])("rows=%s columns=%s", (rows, columns, expectedRows, contentCells) => {
    const { container } = render(
      <ListingTable>
        <ListingTable.Body>
          <ListingSkeletonRows rows={rows} columns={columns} />
        </ListingTable.Body>
      </ListingTable>,
    );
    const trs = container.querySelectorAll("tr");
    expect(trs).toHaveLength(expectedRows);
    // Content cells plus the trailing actions cell.
    expect(trs[0].querySelectorAll("td")).toHaveLength(contentCells + 1);
  });
});

describe("MCPLogo", () => {
  it("renders a decorative masked glyph at the given size", () => {
    const { container } = render(<MCPLogo size={20} className="logo" />);
    const span = container.querySelector("span")!;
    expect(span).toHaveAttribute("aria-hidden", "true");
    expect(span).toHaveClass("logo");
    expect(span.style.width).toBe("20px");
  });
});

describe("OverviewSectionCard", () => {
  it("renders the title, adornment, header action and link", () => {
    render(
      <MemoryRouter>
        <OverviewSectionCard
          title="Invoke URL"
          titleAdornment={<span>adorn</span>}
          headerAction={<button type="button">refresh</button>}
          actionHref="/agents/a/invoke"
          actionLabel="Open"
        >
          <span>body</span>
        </OverviewSectionCard>
      </MemoryRouter>,
    );
    expect(screen.getByText("Invoke URL")).toBeInTheDocument();
    expect(screen.getByText("adorn")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "refresh" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open" })).toHaveAttribute("href", "/agents/a/invoke");
    expect(screen.getByText("body").closest(".MuiCard-root")).toBeInTheDocument();
  });

  it("drops the card chrome and link in plain mode", () => {
    render(
      <OverviewSectionCard title="Caps" variant="plain">
        <span>body</span>
      </OverviewSectionCard>,
    );
    expect(screen.getByText("body").closest(".MuiCard-root")).toBeNull();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });
});

describe("ResilienceTimeoutFields", () => {
  it("edits both timeouts and shows their errors", () => {
    const onRequestTimeoutChange = vi.fn();
    const onIdleTimeoutChange = vi.fn();
    const onRequestTimeoutBlur = vi.fn();
    render(
      <ResilienceTimeoutFields
        defaultExpanded
        requestTimeout="5s"
        onRequestTimeoutChange={onRequestTimeoutChange}
        onRequestTimeoutBlur={onRequestTimeoutBlur}
        requestTimeoutError="bad request timeout"
        idleTimeout=""
        onIdleTimeoutChange={onIdleTimeoutChange}
        idleTimeoutError="bad idle timeout"
      />,
    );
    const request = screen.getByDisplayValue("5s");
    fireEvent.change(request, { target: { value: "10s" } });
    fireEvent.blur(request);
    expect(onRequestTimeoutChange).toHaveBeenCalledWith("10s");
    expect(onRequestTimeoutBlur).toHaveBeenCalled();
    fireEvent.change(screen.getByPlaceholderText("300s"), { target: { value: "1m" } });
    expect(onIdleTimeoutChange).toHaveBeenCalledWith("1m");
    expect(screen.getByText("bad request timeout")).toBeInTheDocument();
    expect(screen.getByText("bad idle timeout")).toBeInTheDocument();
  });
});
