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
import { useState } from "react";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { describe, expect, it, vi } from "vitest";

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
}));

import { ExpressionChips } from "./BudgetForm";

function Harness() {
  const [values, setValues] = useState<string[]>([]);
  return (
    <MemoryRouter>
      <ThemeProvider theme={createTheme()}>
        <ExpressionChips
          values={values}
          onChange={setValues}
          placeholder="expression"
          helper="helper"
          draftKey="f0"
          onDraft={() => undefined}
          connector="AND"
        />
      </ThemeProvider>
    </MemoryRouter>
  );
}

describe("ExpressionChips", () => {
  it("takes several expressions, one per Enter", () => {
    render(<Harness />);
    const type = (text: string) => {
      const input = screen.getByRole("combobox");
      fireEvent.change(input, { target: { value: text } });
      fireEvent.keyDown(input, { key: "Enter", code: "Enter" });
    };
    type("user.team == 'a'");
    type("user.region == 'b'");
    type("user.tier == 'c'");
    expect(screen.getByText("user.team == 'a'")).toBeInTheDocument();
    expect(screen.getByText("user.region == 'b'")).toBeInTheDocument();
    expect(screen.getByText("user.tier == 'c'")).toBeInTheDocument();
    expect(screen.getAllByText("AND")).toHaveLength(2);
  });
});
