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

import { render } from "@testing-library/react";
import { describe, expect, it, vi, beforeEach } from "vitest";
import { MonitorRunDrawer } from "./MonitorRunDrawer";

const { useMonitorRunLogs } = vi.hoisted(() => {
  // The real api-client reads the runtime config when it is first imported.
  (window as unknown as { __RUNTIME_CONFIG__: unknown }).__RUNTIME_CONFIG__ = {
    disableAuth: true,
  };
  return {
    useMonitorRunLogs: vi.fn(() => ({
      data: { logs: [] },
      isLoading: false,
      error: null,
    })),
  };
});

vi.mock("@agent-management-platform/api-client", async (orig) => ({
  ...(await orig<typeof import("@agent-management-platform/api-client")>()),
  useMonitorRunLogs: (...args: unknown[]) =>
    (useMonitorRunLogs as (...a: unknown[]) => unknown)(...args),
  isObserverConfigured: () => true,
}));

const baseRun = {
  id: "run-1",
  runName: "quality-monitor-abc12345",
  evaluators: [],
  traceStart: "2026-01-01T00:00:00Z",
  traceEnd: "2026-04-01T00:00:00Z",
  startedAt: "2026-04-01T10:00:00Z",
  status: "success" as const,
};

const props = {
  orgName: "acme",
  onClose: () => {},
  traceWindowLabel: "",
  durationLabel: "",
};

beforeEach(() => useMonitorRunLogs.mockClear());

describe("MonitorRunDrawer log range", () => {
  it("sends the absolute startedAt..completedAt range for a completed run", () => {
    const run = { ...baseRun, completedAt: "2026-04-01T10:05:00Z" };
    render(<MonitorRunDrawer run={run} {...props} />);

    expect(useMonitorRunLogs).toHaveBeenCalledWith(
      {
        organization: "acme",
        runName: "quality-monitor-abc12345",
        startTime: "2026-04-01T10:00:00Z",
        endTime: "2026-04-01T10:05:00Z",
      },
      "success",
    );
  });

  it("clamps the end bound to startedAt+1h when completedAt is later", () => {
    const run = { ...baseRun, completedAt: "2026-04-01T12:30:00Z" };
    render(<MonitorRunDrawer run={run} {...props} />);

    expect(useMonitorRunLogs).toHaveBeenCalledWith(
      {
        organization: "acme",
        runName: "quality-monitor-abc12345",
        startTime: "2026-04-01T10:00:00Z",
        endTime: "2026-04-01T11:00:00.000Z",
      },
      "success",
    );
  });

  it("sends a fixed startedAt+1h range for an in-flight run", () => {
    const run = { ...baseRun, completedAt: undefined, status: "running" as const };
    render(<MonitorRunDrawer run={run} {...props} />);

    expect(useMonitorRunLogs).toHaveBeenCalledWith(
      {
        organization: "acme",
        runName: "quality-monitor-abc12345",
        startTime: "2026-04-01T10:00:00Z",
        endTime: "2026-04-01T11:00:00.000Z",
      },
      "running",
    );
  });

  it("omits the range when startedAt is missing", () => {
    const run = { ...baseRun, startedAt: undefined, status: "pending" as const };
    render(<MonitorRunDrawer run={run} {...props} />);

    expect(useMonitorRunLogs).toHaveBeenCalledWith(
      { organization: "acme", runName: "quality-monitor-abc12345" },
      "pending",
    );
  });

  // A range computed from "now" at render time would change the query key on
  // every render and make TanStack Query refetch forever.
  it("passes an identical range across re-renders", () => {
    const run = { ...baseRun, completedAt: undefined, status: "running" as const };
    const { rerender } = render(<MonitorRunDrawer run={run} {...props} />);
    rerender(<MonitorRunDrawer run={run} {...props} />);

    const calls = useMonitorRunLogs.mock.calls as unknown[][];
    expect(calls.length).toBeGreaterThan(1);
    expect(calls[calls.length - 1][0]).toEqual(calls[0][0]);
  });
});
