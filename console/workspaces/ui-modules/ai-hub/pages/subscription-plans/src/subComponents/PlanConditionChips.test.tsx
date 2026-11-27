/*
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

import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { conditionChips, PlanConditionChips } from "./PlanConditionChips";
import type { PlanCondition } from "./planConditions";

const requests = (...limits: Array<[number, number | undefined, string]>): PlanCondition => ({
  id: "requests",
  params: {
    limits: limits.map(([count, timeAmount, timeUnit]) => ({ count, timeAmount, timeUnit })),
    onExceed: "reject",
  },
  type: "request-count",
});

const toolCalls = (count: number, onExceed = "reject"): PlanCondition => ({
  id: "tool-calls",
  params: { limits: [{ count, timeAmount: 1, timeUnit: "HOUR" }], onExceed },
  type: "tool-call-count",
});

const tokens = (count: number, tokenType?: string): PlanCondition => ({
  id: "tokens",
  params: { limits: [{ count, timeAmount: 2, timeUnit: "DAY" }], onExceed: "reject", tokenType },
  type: "token-count",
});

const custom = (id: string | undefined, params?: Record<string, unknown>): PlanCondition =>
  ({ id, params, type: "custom" }) as PlanCondition;

describe("conditionChips", () => {
  it("makes one chip per window of a built-in condition", () => {
    expect(conditionChips([requests([1000, 1, "MINUTE"], [5000, 2, "HOUR"])])).toEqual([
      { custom: false, details: ["1,000 / min"], first: "1,000 / min", label: "Requests", more: 0 },
      { custom: false, details: ["5,000 / 2 hr"], first: "5,000 / 2 hr", label: "Requests", more: 0 },
    ]);
  });

  it("treats a window without a length as one unit", () => {
    expect(conditionChips([requests([10, undefined, "MONTH"])])[0].first).toBe("10 / mo");
  });

  it("labels tool calls and token types, and notes an overage that is allowed", () => {
    const [tool] = conditionChips([toolCalls(50, "allow")]);
    expect(tool).toMatchObject({ label: "Tool calls", first: "50 / hr" });
    expect(tool.details).toEqual(["50 / hr", "Allowed past the limit"]);
    expect(conditionChips([tokens(9)])[0]).toMatchObject({ label: "Tokens", first: "9 / 2 day" });
    expect(conditionChips([tokens(9, "prompt")])[0].label).toBe("Prompt tokens");
    expect(conditionChips([tokens(9, "completion")])[0].label).toBe("Completion tokens");
    expect(conditionChips([tokens(9, "other")])[0].label).toBe("Custom");
  });

  it("shows a built-in condition without windows as No limit", () => {
    const chips = conditionChips([{ id: "requests", params: {}, type: "request-count" }]);
    expect(chips).toEqual([
      { custom: false, details: ["No limit"], first: "No limit", label: "Requests", more: 0 },
    ]);
  });

  it("makes one chip for a custom condition, its values for the tooltip", () => {
    expect(conditionChips([custom("region", { region: "eu", max: 3, tags: ["a"] })])).toEqual([
      {
        custom: true,
        details: ["region = eu", "max = 3", 'tags = ["a"]'],
        first: "region",
        label: "Custom",
        more: 0,
      },
    ]);
    expect(conditionChips([custom(undefined)])[0]).toMatchObject({
      details: ["No values"],
      first: "",
    });
  });
});

describe("PlanConditionChips", () => {
  it("shows a plan without conditions as Unlimited", () => {
    render(<PlanConditionChips conditions={undefined} />);
    expect(screen.getByText("Unlimited")).toBeInTheDocument();
  });

  it("shows each chip when there are no more than the maximum", () => {
    render(<PlanConditionChips conditions={[requests([1000, 1, "MINUTE"]), toolCalls(5)]} />);
    expect(screen.getByText("1,000 / min")).toBeInTheDocument();
    expect(screen.getByText("Tool calls")).toBeInTheDocument();
    expect(screen.queryByText(/^\+\d/)).not.toBeInTheDocument();
  });

  it("puts the chips past the maximum into +N, listed in its tooltip", async () => {
    render(
      <PlanConditionChips
        conditions={[
          requests([1, 1, "MINUTE"], [2, 1, "HOUR"]),
          toolCalls(3, "allow"),
          custom("region", { region: "eu" }),
          tokens(4),
        ]}
        max={2}
      />,
    );
    expect(screen.getByText("1 / min")).toBeInTheDocument();
    expect(screen.getByText("2 / hr")).toBeInTheDocument();
    expect(screen.queryByText("3 / hr")).not.toBeInTheDocument();
    const more = screen.getByLabelText("3 more conditions");
    expect(more).toHaveTextContent("+3");

    fireEvent.mouseOver(more);
    const tip = await screen.findByRole("tooltip");
    expect(within(tip).getByText("Tool calls")).toBeInTheDocument();
    expect(within(tip).getByText("3 / hr, Allowed past the limit")).toBeInTheDocument();
    expect(within(tip).getByText("Custom · region")).toBeInTheDocument();
    expect(within(tip).getByText("region = eu")).toBeInTheDocument();
    expect(within(tip).getByText("4 / 2 day")).toBeInTheDocument();
  });

  it("says one more condition for a single hidden chip", () => {
    render(<PlanConditionChips conditions={[requests([1, 1, "MINUTE"], [2, 1, "HOUR"])]} max={1} />);
    expect(screen.getByLabelText("1 more condition")).toHaveTextContent("+1");
  });
});

describe("PlanConditionChips on one line", () => {
  let cellWidth = 0;
  const chipWidth = 100;
  let observed: Array<() => void> = [];
  const disconnect = vi.fn();

  beforeEach(() => {
    observed = [];
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(callback: () => void) {
          observed.push(callback);
        }
        observe() {}
        disconnect = disconnect;
      },
    );
    vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockImplementation(() => chipWidth);
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(() => cellWidth);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    Object.defineProperty(document, "fonts", { configurable: true, value: undefined });
  });

  const four = [
    requests([1, 1, "MINUTE"], [2, 1, "HOUR"]),
    toolCalls(3),
    tokens(4),
  ];

  it("shows the most chips that fit whole, the rest in +N", () => {
    cellWidth = 400;
    render(<PlanConditionChips conditions={four} singleLine />);
    expect(screen.getByLabelText("1 more condition")).toBeInTheDocument();
  });

  it("moves chips that don't fit into +N, keeping room for it", () => {
    cellWidth = 250;
    render(<PlanConditionChips conditions={four} singleLine />);
    expect(screen.getByLabelText("3 more conditions")).toBeInTheDocument();
  });

  it("always shows the first chip, even when it doesn't fit", () => {
    cellWidth = 50;
    render(<PlanConditionChips conditions={four} singleLine />);
    expect(screen.getByLabelText("3 more conditions")).toBeInTheDocument();
  });

  it("shows every chip when all fit without +N", () => {
    cellWidth = 210;
    render(<PlanConditionChips conditions={[requests([1, 1, "MINUTE"], [2, 1, "HOUR"])]} singleLine />);
    expect(screen.queryByLabelText(/more condition/)).not.toBeInTheDocument();
  });

  it("measures again when the cell is resized and once the fonts load", async () => {
    let loaded: () => void = () => undefined;
    const ready = new Promise<void>((resolve) => {
      loaded = resolve;
    });
    Object.defineProperty(document, "fonts", { configurable: true, value: { ready } });
    cellWidth = 400;
    const { unmount } = render(<PlanConditionChips conditions={four} singleLine />);
    expect(screen.getByLabelText("1 more condition")).toBeInTheDocument();

    cellWidth = 250;
    act(() => observed[0]());
    expect(screen.getByLabelText("3 more conditions")).toBeInTheDocument();

    cellWidth = 400;
    await act(async () => {
      loaded();
      await ready;
    });
    expect(screen.getByLabelText("1 more condition")).toBeInTheDocument();

    unmount();
    expect(disconnect).toHaveBeenCalled();
  });

  it("doesn't measure after it is gone, when the fonts load late", async () => {
    let loaded: () => void = () => undefined;
    const ready = new Promise<void>((resolve) => {
      loaded = resolve;
    });
    Object.defineProperty(document, "fonts", { configurable: true, value: { ready } });
    cellWidth = 400;
    const { unmount } = render(<PlanConditionChips conditions={four} singleLine />);
    unmount();
    const widths = vi.mocked(Object.getOwnPropertyDescriptor(HTMLElement.prototype, "clientWidth")!
      .get!);
    const calls = widths.mock.calls.length;
    await act(async () => {
      loaded();
      await ready;
    });
    expect(widths.mock.calls.length).toBe(calls);
  });
});
