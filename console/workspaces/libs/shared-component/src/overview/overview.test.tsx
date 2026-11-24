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
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import type { ExternalComponentModule } from "@agent-management-platform/views";

const panelsByMount: Record<string, ExternalComponentModule[]> = {};
vi.mock("@agent-management-platform/views", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@agent-management-platform/views")>()),
  useExternalComponentModulesByMountPoint: (mount: string) => panelsByMount[mount] ?? [],
}));

import { DonutIcon, getDonutColorForPercent } from "./DonutIcon";
import { OverviewPanels } from "./OverviewPanels";
import { SectionHeader, UppercaseCaptionLabel } from "./SectionHeader";

const panel = (name: string, width?: "half" | "full"): ExternalComponentModule =>
  ({
    moduleName: name,
    kind: "component",
    width,
    component: ((props: { agentId: string }) => (
      <div data-testid="panel">{`${name}:${props.agentId}`}</div>
    )) as never,
  }) as unknown as ExternalComponentModule;

describe("OverviewPanels", () => {
  it("pairs consecutive half-width panels into rows and passes props", () => {
    panelsByMount.overview = [
      panel("a", "half"),
      panel("b", "half"),
      panel("c", "full"),
      panel("d", "half"),
      panel("e"),
    ];
    const { container } = render(<OverviewPanels mountPoint="overview" props={{ agentId: "ag" }} />);
    expect(screen.getAllByTestId("panel").map((p) => p.textContent)).toEqual([
      "a:ag",
      "b:ag",
      "c:ag",
      "d:ag",
      "e:ag",
    ]);
    // a+b share a row at half width; the lone half panel d gets a full row.
    const halfCells = container.querySelectorAll(".MuiGrid-grid-md-6");
    expect(halfCells).toHaveLength(2);
    expect(halfCells[0]).toHaveTextContent("a:ag");
    expect(container.querySelectorAll(".MuiGrid-grid-md-12")[0]).toHaveTextContent("d:ag");
    // Full-width panels render without a grid wrapper.
    expect(screen.getByText("c:ag").parentElement).toBe(container);
  });

  it("renders nothing for an empty mount point", () => {
    const { container } = render(<OverviewPanels mountPoint="nothing" props={{}} />);
    expect(container).toBeEmptyDOMElement();
  });
});

describe("SectionHeader", () => {
  it("shows the title, info and view-all link", async () => {
    render(
      <MemoryRouter>
        <SectionHeader title="Capabilities" titleInfo="What the agent can do" viewAllHref="/caps" viewAllLabel="See all" />
      </MemoryRouter>,
    );
    expect(screen.getByText("Capabilities")).toBeInTheDocument();
    fireEvent.mouseOver(screen.getByRole("button", { name: "More information about Capabilities" }));
    expect(await screen.findByRole("tooltip")).toHaveTextContent("What the agent can do");
    expect(screen.getByRole("link", { name: "See all" })).toHaveAttribute("href", "/caps");
  });

  it("can render just the link, or just the title", () => {
    const { rerender } = render(
      <MemoryRouter>
        <SectionHeader viewAllHref="/x" />
      </MemoryRouter>,
    );
    expect(screen.getByRole("link", { name: "View all" })).toBeInTheDocument();
    rerender(
      <MemoryRouter>
        <SectionHeader title="Only title" />
      </MemoryRouter>,
    );
    expect(screen.getByText("Only title")).toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("renders an uppercase caption label", () => {
    render(<UppercaseCaptionLabel>status</UppercaseCaptionLabel>);
    expect(screen.getByText("status")).toHaveStyle({ textTransform: "uppercase" });
  });
});

describe("getDonutColorForPercent", () => {
  it.each([
    [null, "primary"],
    [100, "success"],
    [70, "success"],
    [69.9, "warning"],
    [40, "warning"],
    [39, "error"],
    [0, "error"],
  ])("%s -> %s", (percent, color) => expect(getDonutColorForPercent(percent)).toBe(color));
});

describe("DonutIcon", () => {
  const arcOffset = (percent: number, size?: number) => {
    const { container } = render(
      <ThemeProvider theme={createTheme()}>
        <DonutIcon percent={percent} color="success" size={size} />
      </ThemeProvider>,
    );
    const arc = container.querySelectorAll("circle")[1];
    return {
      svg: container.querySelector("svg")!,
      dash: Number(arc.getAttribute("stroke-dasharray")),
      offset: Number(arc.getAttribute("stroke-dashoffset")),
    };
  };

  it("draws the arc proportional to the percent", () => {
    const { svg, dash, offset } = arcOffset(25, 100);
    expect(svg).toHaveAttribute("width", "100");
    expect(offset).toBeCloseTo(dash * 0.75);
  });

  it.each([
    [150, 0],
    [-10, 1],
    [Number.NaN, 1],
  ])("clamps %s", (percent, fraction) => {
    const { dash, offset } = arcOffset(percent);
    expect(offset).toBeCloseTo(dash * fraction);
  });
});
