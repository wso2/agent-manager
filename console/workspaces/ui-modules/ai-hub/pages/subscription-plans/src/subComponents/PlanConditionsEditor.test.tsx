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
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PlanConditionsEditor } from "./PlanConditionsEditor";
import { checkConditions, newCondition, type ConditionDraft } from "./planConditions";

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
