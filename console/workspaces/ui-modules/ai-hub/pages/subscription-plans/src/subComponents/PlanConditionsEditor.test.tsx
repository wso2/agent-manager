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

import { useState } from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { PlanConditionsEditor } from "./PlanConditionsEditor";
import {
  checkConditions,
  newCondition,
  newPair,
  newWindow,
  type ConditionDraft,
} from "./planConditions";

/** The editor with its own state, as the plan form holds it. */
function Editor({
  initial = [],
  readOnly = false,
  disabled = false,
}: {
  initial?: ConditionDraft[];
  readOnly?: boolean;
  disabled?: boolean;
}) {
  const [value, setValue] = useState(initial);
  return (
    <PlanConditionsEditor
      check={checkConditions(value)}
      disabled={disabled}
      onChange={setValue}
      readOnly={readOnly}
      showAllErrors={false}
      value={value}
    />
  );
}

const typeCards = () => screen.queryByRole("group", { name: "Condition types" });
const addCondition = () => screen.queryByRole("button", { name: "Add condition" });

describe("PlanConditionsEditor: the condition type cards", () => {
  it("offers the four condition types as cards while the plan has no conditions", () => {
    render(<Editor />);
    expect(typeCards()).toBeInTheDocument();
    for (const name of ["Request count", "Tool-call count", "Token count", "Custom"]) {
      expect(screen.getByRole("button", { name: new RegExp(`^${name}`) })).toBeInTheDocument();
    }
    expect(addCondition()).not.toBeInTheDocument();
  });

  it("adds the picked type, then offers Add condition for more", () => {
    render(<Editor />);
    fireEvent.click(screen.getByRole("button", { name: /^Request count/ }));
    expect(typeCards()).not.toBeInTheDocument();
    expect(addCondition()).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove Request count condition" })).toBeInTheDocument();
  });

  it("offers the cards again once the last condition is removed", () => {
    render(<Editor initial={[newCondition("token-count")]} />);
    expect(typeCards()).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Remove Token count condition" }));
    expect(typeCards()).toBeInTheDocument();
    expect(addCondition()).not.toBeInTheDocument();
  });

  it("shows a read-only plan without conditions as unlimited, with no cards", () => {
    render(<Editor readOnly />);
    expect(typeCards()).not.toBeInTheDocument();
    expect(addCondition()).not.toBeInTheDocument();
    expect(screen.getByText("No conditions: this plan is unlimited.")).toBeInTheDocument();
  });

  it("disables the cards while the form is busy", () => {
    render(<Editor disabled />);
    expect(screen.getByRole("button", { name: /^Custom/ })).toBeDisabled();
  });
});

/** The editor with its own state, showing every problem when asked, reporting each change. */
function Watched({
  initial,
  showAllErrors = false,
  readOnly = false,
  onValue,
}: {
  initial: ConditionDraft[];
  showAllErrors?: boolean;
  readOnly?: boolean;
  onValue?: (next: ConditionDraft[]) => void;
}) {
  const [value, setValue] = useState(initial);
  return (
    <PlanConditionsEditor
      check={checkConditions(value)}
      onChange={(next) => {
        setValue(next);
        onValue?.(next);
      }}
      readOnly={readOnly}
      showAllErrors={showAllErrors}
      value={value}
    />
  );
}

const lastValue = (spy: ReturnType<typeof vi.fn>) =>
  spy.mock.calls[spy.mock.calls.length - 1][0] as ConditionDraft[];

const expand = (name: string) =>
  fireEvent.click(screen.getByRole("button", { name: `Show ${name} settings` }));

describe("PlanConditionsEditor: the Add condition menu", () => {
  it("lists every type, marks a built-in one the plan has as Added, and adds the picked one", async () => {
    const onValue = vi.fn();
    render(<Watched initial={[newCondition("request-count")]} onValue={onValue} />);
    fireEvent.click(addCondition()!);
    const menu = await screen.findByRole("menu", { name: "Condition types" });
    const items = within(menu).getAllByRole("menuitem");
    expect(items).toHaveLength(4);
    const requestItem = items.find((item) => item.textContent?.startsWith("Request count"))!;
    expect(requestItem).toHaveAttribute("aria-disabled", "true");
    expect(within(requestItem).getByText("Added")).toBeInTheDocument();

    fireEvent.click(within(menu).getByRole("menuitem", { name: /^Tool-call count/ }));
    // Added once the menu has closed.
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Remove Tool-call count condition" }),
      ).toBeInTheDocument(),
    );
    expect(lastValue(onValue).map((draft) => draft.type)).toEqual([
      "request-count",
      "tool-call-count",
    ]);
    // A new condition opens, so its limit can be typed at once.
    expect(screen.getByRole("button", { name: "Hide Tool-call count settings" })).toBeInTheDocument();
  });

  it("adds nothing when the menu is closed without a pick", async () => {
    const onValue = vi.fn();
    render(<Watched initial={[newCondition("request-count")]} onValue={onValue} />);
    fireEvent.click(addCondition()!);
    const menu = await screen.findByRole("menu");
    fireEvent.keyDown(menu, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("menu")).not.toBeInTheDocument());
    expect(onValue).not.toHaveBeenCalled();
  });

  it("can't add past the most conditions a plan holds, and says so", () => {
    const many = Array.from({ length: 21 }, () => newCondition("custom"));
    render(<Watched initial={many} />);
    expect(addCondition()).toBeDisabled();
    expect(screen.getByText("A plan can have at most 20 conditions.")).toBeInTheDocument();
  });
});

describe("PlanConditionsEditor: a built-in condition", () => {
  it("opens and closes its settings, summing up its limits in the header", () => {
    const draft = newCondition("request-count");
    draft.windows[0].count = "1000";
    render(<Watched initial={[draft]} />);
    expect(screen.getByText("1,000 requests per minute")).toBeInTheDocument();
    expect(screen.queryByLabelText("Count")).not.toBeInTheDocument();
    expand("Request count");
    expect(screen.getByLabelText("Count")).toHaveValue(1000);
    fireEvent.click(screen.getByRole("button", { name: "Hide Request count settings" }));
    expect(screen.getByRole("button", { name: "Show Request count settings" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });

  it("edits a limit's count, length and unit, and adds and removes limits", async () => {
    const onValue = vi.fn();
    render(<Watched initial={[newCondition("tool-call-count")]} onValue={onValue} />);
    expect(screen.getByText("No limit yet")).toBeInTheDocument();
    expand("Tool-call count");
    // A lone limit can't be removed: remove the condition instead.
    expect(screen.getByRole("button", { name: "Remove limit" })).toBeDisabled();

    fireEvent.change(screen.getByLabelText("Count"), { target: { value: "5000" } });
    fireEvent.change(screen.getByLabelText("Window length"), { target: { value: "2" } });
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Window unit" }));
    fireEvent.click(await screen.findByRole("option", { name: "Hour" }));
    expect(lastValue(onValue)[0].windows[0]).toMatchObject({
      count: "5000",
      timeAmount: "2",
      timeUnit: "HOUR",
    });
    expect(screen.getByText("5,000 tool calls per 2 hours")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Add limit" }));
    expect(screen.getAllByLabelText("Count")).toHaveLength(2);
    expect(lastValue(onValue)[0].windows).toHaveLength(2);
    fireEvent.click(screen.getAllByRole("button", { name: "Remove limit" })[1]);
    expect(screen.getAllByLabelText("Count")).toHaveLength(1);
  });

  it("lets requests past the limit through when blocking is turned off", () => {
    const onValue = vi.fn();
    const draft = newCondition("request-count");
    draft.windows[0].count = "10";
    render(<Watched initial={[draft]} onValue={onValue} />);
    expand("Request count");
    const block = screen.getByRole("switch", { name: "Block requests when quota is reached" });
    expect(block).toBeChecked();
    fireEvent.click(block);
    expect(lastValue(onValue)[0].onExceed).toBe("allow");
    expect(screen.getByText("10 requests per minute · overage allowed")).toBeInTheDocument();
    fireEvent.click(block);
    expect(lastValue(onValue)[0].onExceed).toBe("reject");
  });

  it("picks which tokens a token condition counts", async () => {
    const onValue = vi.fn();
    const draft = newCondition("token-count");
    draft.windows[0].count = "7";
    render(<Watched initial={[draft]} onValue={onValue} />);
    expect(screen.getByText("7 tokens per minute")).toBeInTheDocument();
    expand("Token count");
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Tokens counted" }));
    fireEvent.click(await screen.findByRole("option", { name: "Prompt tokens" }));
    expect(lastValue(onValue)[0].tokenType).toBe("prompt");
    expect(screen.getByText("7 prompt tokens per minute")).toBeInTheDocument();
  });

  it("shows a limit's problem once the field is left, not before", () => {
    render(<Watched initial={[newCondition("request-count")]} />);
    expand("Request count");
    const count = screen.getByLabelText("Count");
    fireEvent.change(count, { target: { value: "0" } });
    expect(screen.queryByText("Enter a whole number of at least 1.")).not.toBeInTheDocument();
    fireEvent.blur(count);
    expect(screen.getByText("Enter a whole number of at least 1.")).toBeInTheDocument();
  });

  it("shows a window another limit already covers once the unit is picked", async () => {
    const draft = newCondition("request-count");
    draft.windows = [newWindow({ count: 1, timeAmount: 60 }), newWindow({ count: 2 })];
    render(<Watched initial={[draft]} />);
    expand("Request count");
    fireEvent.mouseDown(screen.getAllByRole("combobox", { name: "Window unit" })[1]);
    fireEvent.click(await screen.findByRole("option", { name: "Hour" }));
    expect(screen.getByText(/Another limit already covers this window/)).toBeInTheDocument();
  });

  it("shows every problem, and keeps a condition with one open, when asked to", () => {
    const draft = newCondition("request-count");
    draft.windows = [];
    render(<Watched initial={[draft]} showAllErrors />);
    expect(screen.getByRole("button", { name: "Hide Request count settings" })).toBeInTheDocument();
    expect(
      screen.getByText("Add at least one limit, or remove the condition."),
    ).toBeInTheDocument();
  });

  it("shows a read-only condition's limits without editing controls", () => {
    const draft = newCondition("request-count");
    draft.windows[0].count = "3";
    render(<Watched initial={[draft]} readOnly />);
    expect(
      screen.queryByRole("button", { name: "Remove Request count condition" }),
    ).not.toBeInTheDocument();
    expand("Request count");
    expect(screen.getByLabelText("Count")).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Add limit" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Remove limit" })).not.toBeInTheDocument();
    expect(addCondition()).not.toBeInTheDocument();
  });
});

describe("PlanConditionsEditor: a custom condition", () => {
  it("takes a name, values and an optional policy", () => {
    const onValue = vi.fn();
    render(<Watched initial={[newCondition("custom")]} onValue={onValue} />);
    expand("Custom");
    expect(screen.getByText("No values yet")).toBeInTheDocument();
    expect(
      screen.getByText("Lowercase letters, digits and hyphens, for example region-limit"),
    ).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("e.g. region-limit"), {
      target: { value: "region-limit" },
    });
    fireEvent.change(screen.getByLabelText("Key"), { target: { value: "region" } });
    fireEvent.change(screen.getByLabelText("Value"), { target: { value: "eu" } });
    fireEvent.change(screen.getByLabelText("Policy name"), { target: { value: "region-guard" } });
    fireEvent.change(screen.getByLabelText("Policy version"), { target: { value: "v2" } });

    expect(lastValue(onValue)[0]).toMatchObject({
      id: "region-limit",
      policyName: "region-guard",
      policyVersion: "v2",
    });
    expect(lastValue(onValue)[0].pairs[0]).toMatchObject({ name: "region", value: "eu" });
    expect(screen.getByText("region = eu")).toBeInTheDocument();
    // The name shows beside the header.
    expect(screen.getByText("region-limit")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: /The policy that enforces this condition/ }))
      .toBeInTheDocument();
  });

  it("adds and removes values", () => {
    const onValue = vi.fn();
    render(<Watched initial={[newCondition("custom")]} onValue={onValue} />);
    expand("Custom");
    fireEvent.click(screen.getByRole("button", { name: "Add value" }));
    expect(screen.getAllByLabelText("Key")).toHaveLength(2);
    fireEvent.click(screen.getAllByRole("button", { name: "Remove value" })[0]);
    expect(screen.getAllByLabelText("Key")).toHaveLength(1);
    expect(lastValue(onValue)[0].pairs).toHaveLength(1);
  });

  it("shows the problems of its name, policy and values once each field is left", () => {
    render(<Watched initial={[newCondition("custom")]} />);
    expand("Custom");
    const name = screen.getByPlaceholderText("e.g. region-limit");
    fireEvent.change(name, { target: { value: "requests" } });
    fireEvent.blur(name);
    expect(
      screen.getByText("requests is the name of a built-in condition; choose another."),
    ).toBeInTheDocument();

    const version = screen.getByLabelText("Policy version");
    fireEvent.change(version, { target: { value: "latest" } });
    fireEvent.blur(version);
    expect(screen.getByText("Enter a version such as v1.")).toBeInTheDocument();
    const policy = screen.getByLabelText("Policy name");
    // Both wrong: the name's problem shows first, once its field is left.
    fireEvent.change(policy, { target: { value: "-bad" } });
    fireEvent.blur(policy);
    expect(
      screen.getByText("Enter the policy name, for example region-guard."),
    ).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Value"), { target: { value: "eu" } });
    fireEvent.blur(screen.getByLabelText("Value"));
    expect(screen.getByText("Each value needs a key.")).toBeInTheDocument();
  });

  it("shows values too large in all once a value is left", () => {
    render(<Watched initial={[newCondition("custom")]} />);
    expand("Custom");
    fireEvent.change(screen.getByLabelText("Key"), { target: { value: "big" } });
    const value = screen.getByLabelText("Value");
    fireEvent.change(value, { target: { value: "x".repeat(5000) } });
    expect(screen.queryByText(/bytes in all/)).not.toBeInTheDocument();
    fireEvent.blur(value);
    expect(
      screen.getByText("The values must be at most 4,096 bytes in all."),
    ).toBeInTheDocument();
  });

  it("removes the condition", () => {
    const onValue = vi.fn();
    render(<Watched initial={[newCondition("custom"), newCondition("request-count")]} onValue={onValue} />);
    fireEvent.click(screen.getByRole("button", { name: "Remove Custom condition" }));
    expect(lastValue(onValue).map((draft) => draft.type)).toEqual(["request-count"]);
  });

  it("shows a read-only custom condition without editing controls", () => {
    const draft = newCondition("custom");
    draft.id = "region";
    draft.pairs = [newPair("region", "eu")];
    render(<Watched initial={[draft]} readOnly />);
    expand("Custom");
    expect(screen.getByLabelText("Key")).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Add value" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Remove value" })).not.toBeInTheDocument();
  });
});
