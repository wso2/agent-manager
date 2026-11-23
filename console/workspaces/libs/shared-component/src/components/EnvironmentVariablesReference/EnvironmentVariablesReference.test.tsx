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
import { describe, expect, it, vi } from "vitest";

// MUI transitions and first renders are slow on a busy machine.
vi.setConfig({ testTimeout: 30000 });
import { EnvironmentVariablesReference } from "./EnvironmentVariablesReference";

const rows = [
  { key: "url", name: "AMP_URL", description: "Gateway URL" },
  { key: "key", name: "AMP_KEY" },
];

describe("EnvironmentVariablesReference", () => {
  it("shows the variables read-only inside an info alert by default", () => {
    render(<EnvironmentVariablesReference description="Injected at deploy." rows={rows} />);
    expect(screen.getByRole("alert")).toHaveTextContent("Environment Variables References");
    expect(screen.getByText("Injected at deploy.")).toBeInTheDocument();
    expect(screen.getByDisplayValue("AMP_URL")).toHaveAttribute("readonly");
    expect(screen.getByText("Gateway URL")).toBeInTheDocument();
    expect(screen.queryByText("(editable)")).not.toBeInTheDocument();
  });

  it("lets the names be edited and renders extra content as plain content", () => {
    const onNameChange = vi.fn();
    render(
      <EnvironmentVariablesReference
        title="LLM variables"
        description="Rename to match your code."
        rows={rows}
        variant="plain"
        onNameChange={onNameChange}
      >
        <pre>snippet</pre>
      </EnvironmentVariablesReference>,
    );
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByText("LLM variables")).toBeInTheDocument();
    expect(screen.getByText("(editable)")).toBeInTheDocument();
    expect(screen.getByText("snippet")).toBeInTheDocument();
    const input = screen.getByDisplayValue("AMP_KEY");
    expect(input).not.toHaveAttribute("readonly");
    fireEvent.change(input, { target: { value: "MY_KEY" } });
    expect(onNameChange).toHaveBeenCalledWith("key", "MY_KEY");
  });
});
