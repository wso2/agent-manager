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
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import type { TraceOverview } from "@agent-management-platform/types";

// api-client and shared-component crash at import time outside an app shell.
vi.mock("@agent-management-platform/api-client", () => ({
  useListAgentDeployments: vi.fn(() => ({ data: undefined })),
  useTraceList: vi.fn(),
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  NESTED_CARD_SX: {},
  DonutIcon: () => null,
  getDonutColorForPercent: () => "success",
  SectionHeader: ({ title }: { title: string }) => <h2>{title}</h2>,
}));
vi.mock("@wso2/oxygen-ui-charts-react", () => ({ AreaChart: () => null }));

import { useTraceList } from "@agent-management-platform/api-client";
import { EnvObservabilitySection } from "./EnvObservabilitySection";

const trace = (traceId: string, extra: Partial<TraceOverview> = {}): TraceOverview => ({
  traceId,
  rootSpanId: `${traceId}-root`,
  rootSpanName: "agent",
  startTime: "2026-10-01T10:00:00Z",
  endTime: "2026-10-01T10:00:01Z",
  durationInNanos: 1_000_000_000,
  spanCount: 3,
  ...extra,
});

/** Renders the section over traces and returns the Success Rate value. */
function successRate(traces: TraceOverview[]) {
  vi.mocked(useTraceList).mockReturnValue({
    traceList: { traces, totalCount: traces.length },
    isLoading: false,
  } as unknown as ReturnType<typeof useTraceList>);
  render(
    <ThemeProvider theme={createTheme()}>
      <EnvObservabilitySection orgId="o" projectId="p" agentId="a" envId="dev" external />
    </ThemeProvider>,
  );
  return screen.getByText("Success Rate").previousElementSibling?.textContent;
}

beforeEach(() => vi.clearAllMocks());

describe("EnvObservabilitySection success rate", () => {
  it("shows no rate when no trace reports a status", () => {
    expect(successRate([trace("t1"), trace("t2")])).toBe("—");
  });

  it("counts the traces that report a status", () => {
    expect(
      successRate([
        trace("t1", { status: { errorCount: 0 } }),
        trace("t2", { status: { errorCount: 2 } }),
        trace("t3"),
      ]),
    ).toBe("50.0%");
  });
});
