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
import {
  availabilityLabel,
  DeliveryStatusChip,
  errorMessageOf,
  EventTypeChips,
  filterLabel,
  HealthChip,
  Last24hChip,
  lookOf,
  SecretReveal,
  timeAgo,
} from "./eventsUi";

describe("eventsUi helpers", () => {
  it("looks up a destination's service label and letter", () => {
    expect(lookOf("webhook")).toMatchObject({ label: "Webhook", letter: "W" });
    expect(lookOf("notify", "slack")).toMatchObject({ label: "Slack", letter: "S" });
    expect(
      lookOf("notify", "slack", [
        { id: "slack", label: "Slack (workspace)", category: "Chat", urlFormat: "", docsPath: "" },
      ]).label,
    ).toBe("Slack (workspace)");
    expect(
      lookOf("notify", "matrix", [
        { id: "matrix", label: "Matrix", category: "Chat", urlFormat: "", docsPath: "" },
      ]),
    ).toEqual({ label: "Matrix", letter: "M", color: "#64748B" });
    expect(lookOf("notify", "unknown").label).toBe("unknown");
    expect(lookOf("notify").label).toBe("Notification");
  });

  it("describes availability", () => {
    expect(availabilityLabel("all", [])).toBe("All projects");
    expect(availabilityLabel("org", [])).toBe("Org only");
    expect(availabilityLabel("projects", ["a", "b"])).toBe("a, b");
    expect(availabilityLabel("projects", ["a", "b", "c", "d"])).toBe("a, b +2");
  });

  it("describes an environment filter and a missing time", () => {
    expect(filterLabel([])).toBe("all envs");
    expect(filterLabel(["dev", "prod"])).toBe("env: dev, prod");
    expect(timeAgo(undefined)).toBe("never");
  });

  it("prefers the service's error message over a generic HTTP one", () => {
    expect(errorMessageOf({ body: { message: "Name taken" }, message: "x" }, "fallback")).toBe(
      "Name taken",
    );
    expect(errorMessageOf(new Error("Network down"), "fallback")).toBe("Network down");
    expect(errorMessageOf(new Error("HTTP error! status: 500"), "fallback")).toBe("fallback");
    expect(errorMessageOf(undefined, "fallback")).toBe("fallback");
  });
});

describe("eventsUi chips", () => {
  it("shows destination health", () => {
    const { rerender } = render(<HealthChip health="healthy" />);
    expect(screen.getByText("Healthy")).toBeInTheDocument();
    rerender(<HealthChip health="failing" failures={4} />);
    expect(screen.getByText("Failing, 4 deliveries")).toBeInTheDocument();
    rerender(<HealthChip health="failing" failures={1} />);
    expect(screen.getByText("Failing")).toBeInTheDocument();
    rerender(<HealthChip health="idle" />);
    expect(screen.getByText("No deliveries")).toBeInTheDocument();
  });

  it("shows a delivery's status", () => {
    const { rerender } = render(<DeliveryStatusChip status="delivered" attempts={1} />);
    expect(screen.getByText("Delivered")).toBeInTheDocument();
    rerender(<DeliveryStatusChip status="failed" attempts={5} />);
    expect(screen.getByText("Failed")).toBeInTheDocument();
    rerender(<DeliveryStatusChip status="skipped" attempts={0} />);
    expect(screen.getByText("Skipped")).toBeInTheDocument();
    rerender(<DeliveryStatusChip status="pending" attempts={2} />);
    expect(screen.getByText("Retrying (2)")).toBeInTheDocument();
    rerender(<DeliveryStatusChip status="pending" attempts={0} />);
    expect(screen.getByText("Queued")).toBeInTheDocument();
  });

  it("summarises the last 24 hours", () => {
    const { rerender } = render(
      <Last24hChip counts={{ delivered: 3, failed: 1, pending: 0, skipped: 0 }} />,
    );
    expect(screen.getByText("1 failed")).toBeInTheDocument();
    rerender(<Last24hChip counts={{ delivered: 0, failed: 0, pending: 0, skipped: 2 }} />);
    expect(screen.getByText("0 sent")).toBeInTheDocument();
  });

  it("caps event chips and shows the rest as +N", () => {
    render(<EventTypeChips types={["a", "b", "c", "d"]} max={2} />);
    expect(screen.getByText("a")).toBeInTheDocument();
    expect(screen.getByText("b")).toBeInTheDocument();
    expect(screen.queryByText("c")).not.toBeInTheDocument();
    expect(screen.getByText("+2")).toBeInTheDocument();
  });

  it("reveals a signing secret once with a copy button", () => {
    const writeText = vi.fn();
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    const onClose = vi.fn();
    render(<SecretReveal name="CI hook" secret="whsec_abc" onClose={onClose} />);

    expect(
      screen.getByText("Signing secret for CI hook. Copy it now; it is not shown again."),
    ).toBeInTheDocument();
    expect(screen.getByText("whsec_abc")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Copy signing secret" }));
    expect(writeText).toHaveBeenCalledWith("whsec_abc");
  });
});
