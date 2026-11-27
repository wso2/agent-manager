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

import { useEffect } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const track = vi.fn();
vi.mock("@agent-management-platform/api-client", () => ({
  useTrack: () => ({ track }),
  ConsoleAction: { ConfirmationCancelled: "confirmation-cancelled" },
}));

import {
  ConfirmationDialogProvider,
  useConfirmationDialog,
  type ConfirmationEvent,
} from "./ConfirmationDialogProvider";

function Requester({ events }: { events: ConfirmationEvent[] }) {
  const { addConfirmation } = useConfirmationDialog();
  return (
    <button type="button" onClick={() => events.forEach(addConfirmation)}>
      ask
    </button>
  );
}

const renderWith = (events: ConfirmationEvent[]) =>
  render(
    <ConfirmationDialogProvider>
      <Requester events={events} />
    </ConfirmationDialogProvider>,
  );

describe("ConfirmationDialogProvider", () => {
  beforeEach(() => track.mockClear());

  it("confirms with custom labels and runs onConfirm", () => {
    const onConfirm = vi.fn();
    renderWith([
      {
        title: "Delete agent",
        description: "This cannot be undone.",
        confirmButtonText: "Delete",
        cancelButtonText: "Keep",
        confirmButtonColor: "error",
        onConfirm,
      },
    ]);
    fireEvent.click(screen.getByText("ask"));
    expect(screen.getByText("Delete agent")).toBeInTheDocument();
    expect(screen.getByText("This cannot be undone.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Keep" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(onConfirm).toHaveBeenCalled();
    expect(track).not.toHaveBeenCalled();
  });

  it("records a cancel labelled from the button text when no analytics are given", () => {
    const onCancel = vi.fn();
    const onConfirm = vi.fn();
    renderWith([{ title: "Undeploy", description: "d", confirmButtonText: "Undeploy", onConfirm, onCancel }]);
    fireEvent.click(screen.getByText("ask"));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalled();
    expect(onConfirm).not.toHaveBeenCalled();
    expect(track).toHaveBeenCalledWith("confirmation-cancelled", {
      entity: "unspecified",
      destructive_action: "Undeploy",
    });
  });

  it("falls back to default labels and an unspecified action", () => {
    renderWith([{ title: "Sure?", description: "d", onConfirm: vi.fn() }]);
    fireEvent.click(screen.getByText("ask"));
    expect(screen.getByRole("button", { name: "Confirm" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(track).toHaveBeenCalledWith("confirmation-cancelled", {
      entity: "unspecified",
      destructive_action: "unspecified",
    });
  });

  it("shows stacked confirmations newest first", () => {
    const first = vi.fn();
    const second = vi.fn();
    renderWith([
      { title: "First", description: "", onConfirm: first },
      { title: "Second", description: "", onConfirm: second },
    ]);
    fireEvent.click(screen.getByText("ask"));
    expect(screen.getByText("Second")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));
    expect(second).toHaveBeenCalled();
    expect(screen.getByText("First")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));
    expect(first).toHaveBeenCalled();
  });

  it("enables Confirm only once the confirmation text is typed exactly", () => {
    const onConfirm = vi.fn();
    renderWith([
      {
        title: "Delete plan",
        description: "Gone for good.",
        confirmButtonText: "Delete",
        confirmationInputLabel: "Type the plan name to confirm",
        confirmationText: "Gold",
        onConfirm,
      },
    ]);
    fireEvent.click(screen.getByText("ask"));
    expect(screen.getByText("Type the plan name to confirm")).toBeInTheDocument();
    const confirm = screen.getByRole("button", { name: "Delete" });
    expect(confirm).toBeDisabled();
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "gold" } });
    expect(confirm).toBeDisabled();
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "Gold" } });
    expect(confirm).toBeEnabled();
    fireEvent.click(confirm);
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("shows Confirm disabled, with no confirmation field, when the action can't be taken", () => {
    const onConfirm = vi.fn();
    renderWith([
      {
        title: "Delete plan",
        description: '"Gold" is in use and can’t be deleted.',
        confirmButtonText: "Delete",
        confirmationText: "Gold",
        confirmDisabled: true,
        onConfirm,
      },
    ]);
    fireEvent.click(screen.getByText("ask"));
    expect(screen.getByText('"Gold" is in use and can’t be deleted.')).toBeInTheDocument();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("is a no-op outside a provider", () => {
    function Outside() {
      const { addConfirmation } = useConfirmationDialog();
      useEffect(() => addConfirmation({ title: "x", description: "", onConfirm: vi.fn() }), [addConfirmation]);
      return <span>outside</span>;
    }
    render(<Outside />);
    expect(screen.getByText("outside")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
