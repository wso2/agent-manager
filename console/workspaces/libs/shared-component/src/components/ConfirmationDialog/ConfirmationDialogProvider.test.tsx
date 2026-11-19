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
  ConfirmationDialogProvider,
  useConfirmationDialog,
  type ConfirmationEvent,
} from "./ConfirmationDialogProvider";

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: { ConfirmationCancelled: "confirmation-cancelled" },
  useTrack: () => ({ track: vi.fn() }),
}));

function Opener({ event }: { event: ConfirmationEvent }) {
  const { addConfirmation } = useConfirmationDialog();
  return <button onClick={() => addConfirmation(event)}>open</button>;
}

const open = (event: Partial<ConfirmationEvent>) => {
  const onConfirm = vi.fn();
  render(
    <ConfirmationDialogProvider>
      <Opener
        event={{ description: "Gone for good.", onConfirm, title: "Delete plan", ...event }}
      />
    </ConfirmationDialogProvider>,
  );
  fireEvent.click(screen.getByText("open"));
  return onConfirm;
};

const confirmButton = () => screen.getByRole("button", { name: "Delete" });

describe("ConfirmationDialogProvider", () => {
  it("confirms at once without a confirmation text", () => {
    const onConfirm = open({ confirmButtonText: "Delete" });
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    fireEvent.click(confirmButton());
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("enables Confirm only once the confirmation text is typed", () => {
    const onConfirm = open({
      confirmButtonText: "Delete",
      confirmationInputLabel: "Type the plan name to confirm",
      confirmationText: "Gold",
    });
    expect(screen.getByText("Type the plan name to confirm")).toBeInTheDocument();
    expect(confirmButton()).toBeDisabled();

    fireEvent.change(screen.getByRole("textbox"), { target: { value: "gold" } });
    expect(confirmButton()).toBeDisabled();

    fireEvent.change(screen.getByRole("textbox"), { target: { value: "Gold" } });
    expect(confirmButton()).toBeEnabled();
    fireEvent.click(confirmButton());
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });
});
