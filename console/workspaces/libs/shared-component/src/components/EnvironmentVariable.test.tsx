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

import React, { useState } from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { EnvironmentVariable, type EnvVariableItem } from "./EnvironmentVariable";

function Harness({
  initial,
  onChange,
  ...props
}: Partial<React.ComponentProps<typeof EnvironmentVariable>> & {
  initial: EnvVariableItem[];
  onChange?: (v: EnvVariableItem[]) => void;
}) {
  const [vars, setVars] = useState(initial);
  return (
    <>
      <EnvironmentVariable
        envVariables={vars}
        setEnvVariables={(update) =>
          setVars((prev) => {
            const next = typeof update === "function" ? update(prev) : update;
            onChange?.(next);
            return next;
          })
        }
        {...props}
      />
    </>
  );
}

const byPlaceholder = (p: string) => screen.getByPlaceholderText(p) as HTMLInputElement;

describe("EnvironmentVariable", () => {
  it("lists variables, masking secret values and marking required keys", () => {
    render(
      <Harness
        initial={[
          { key: "API_URL", value: "https://x" },
          { key: "TOKEN", value: "s3cret", isSensitive: true },
        ]}
        lockedKeys={new Set(["API_URL"])}
        title="Env"
        description="Desc"
      />,
    );
    expect(screen.getByText("Env")).toBeInTheDocument();
    expect(screen.getByText("Desc")).toBeInTheDocument();
    expect(screen.getByText("https://x")).toBeInTheDocument();
    expect(screen.queryByText("s3cret")).not.toBeInTheDocument();
    expect(screen.getByText("••••••••")).toBeInTheDocument();
    expect(screen.getByText("Required")).toBeInTheDocument();
    // Only the unlocked variable can be deleted.
    expect(screen.getAllByTitle("Delete")).toHaveLength(1);
  });

  it("adds a variable, turning spaces in the name into underscores", () => {
    const onChange = vi.fn();
    render(<Harness initial={[]} onChange={onChange} hideTitle />);
    expect(screen.queryByText("Environment Variables (Optional)")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    const add = () => screen.getAllByRole("button", { name: "Add" }).slice(-1)[0];
    expect(add()).toBeDisabled();
    fireEvent.change(byPlaceholder("Enter a new key"), { target: { value: "MY VAR" } });
    expect(byPlaceholder("Enter a new key").value).toBe("MY_VAR");
    fireEvent.change(byPlaceholder("Enter a value"), { target: { value: "hidden" } });
    fireEvent.click(screen.getByRole("checkbox", { name: "Mark as a Secret" }));
    expect(byPlaceholder("Enter a value")).toHaveAttribute("type", "password");
    const valueBox = byPlaceholder("Enter a value").parentElement as HTMLElement;
    fireEvent.click(within(valueBox).getByRole("button"));
    expect(byPlaceholder("Enter a value")).toHaveAttribute("type", "text");
    fireEvent.click(add());
    expect(onChange).toHaveBeenLastCalledWith([{ key: "MY_VAR", value: "hidden", isSensitive: true }]);
    expect(screen.queryByPlaceholderText("Enter a new key")).not.toBeInTheDocument();
  });

  it("cancels an add without changing anything", () => {
    const onChange = vi.fn();
    render(<Harness initial={[]} onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    fireEvent.change(byPlaceholder("Enter a new key"), { target: { value: "X" } });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.queryByPlaceholderText("Enter a new key")).not.toBeInTheDocument();
  });

  it("removes a variable", () => {
    const onChange = vi.fn();
    render(<Harness initial={[{ key: "A", value: "1" }, { key: "B", value: "2" }]} onChange={onChange} />);
    fireEvent.click(screen.getAllByTitle("Delete")[0]);
    expect(onChange).toHaveBeenLastCalledWith([{ key: "B", value: "2" }]);
  });

  it("edits a plain value prefilled with the current one", () => {
    const onChange = vi.fn();
    render(<Harness initial={[{ key: "A", value: "old" }]} onChange={onChange} isExistingData />);
    fireEvent.click(screen.getByTitle("Edit value"));
    expect(byPlaceholder("Enter new value").value).toBe("old");
    fireEvent.change(byPlaceholder("Enter new value"), { target: { value: "new" } });
    fireEvent.click(screen.getByRole("button", { name: "Update" }));
    expect(onChange).toHaveBeenLastCalledWith([{ key: "A", value: "new", isSecretEdited: undefined }]);
  });

  it("edits a secret from blank, warns about overwriting, and flags it as edited", () => {
    const onChange = vi.fn();
    render(
      <Harness
        initial={[{ key: "TOKEN", value: "", isSensitive: true, secretRef: "ref" }]}
        onChange={onChange}
        isExistingData
        hideAddButton
      />,
    );
    expect(screen.queryByRole("button", { name: "Add" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByTitle("Edit value"));
    expect(
      screen.getByText(/Updating a Secret value removes the previous value/),
    ).toBeInTheDocument();
    const input = byPlaceholder("Enter new value");
    expect(input.value).toBe("");
    expect(input).toHaveAttribute("type", "password");
    expect(screen.getByRole("button", { name: "Update" })).toBeDisabled();
    fireEvent.click(within(input.parentElement as HTMLElement).getByRole("button"));
    expect(input).toHaveAttribute("type", "text");
    fireEvent.change(input, { target: { value: "rotated" } });
    fireEvent.click(screen.getByRole("button", { name: "Update" }));
    expect(onChange).toHaveBeenLastCalledWith([
      { key: "TOKEN", value: "rotated", isSensitive: true, secretRef: "ref", isSecretEdited: true },
    ]);
  });

  it("cancels an edit", () => {
    const onChange = vi.fn();
    render(<Harness initial={[{ key: "A", value: "old" }]} onChange={onChange} isExistingData />);
    fireEvent.click(screen.getByTitle("Edit value"));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByText("old")).toBeInTheDocument();
  });
});
