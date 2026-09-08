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

import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";

vi.mock("@agent-management-platform/api-client", () => ({
  useUpdateLLMProvider: vi.fn(),
}));

import { useUpdateLLMProvider } from "@agent-management-platform/api-client";
import {
  RenameLLMProviderDialog,
  type RenameLLMProviderDialogProps,
} from "./RenameLLMProviderDialog";

const mockUseUpdateLLMProvider = vi.mocked(useUpdateLLMProvider);

const renderDialog = (props: Partial<RenameLLMProviderDialogProps> = {}) => {
  const defaultProps: RenameLLMProviderDialogProps = {
    open: true,
    onClose: vi.fn(),
    orgName: "test-org",
    providerId: "provider-123",
    currentName: "Original Provider Name",
  };
  const merged = { ...defaultProps, ...props };
  return {
    ...render(
      <ThemeProvider theme={createTheme()}>
        <RenameLLMProviderDialog {...merged} />
      </ThemeProvider>,
    ),
    props: merged,
  };
};

describe("RenameLLMProviderDialog", () => {
  const mockMutateAsync = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    mockUseUpdateLLMProvider.mockReturnValue({
      mutateAsync: mockMutateAsync,
      isPending: false,
    } as unknown as ReturnType<typeof useUpdateLLMProvider>);
  });

  it("renders with the prefilled current provider name", () => {
    renderDialog({ currentName: "My Provider" });
    const input = screen.getByRole("textbox", { name: /display name/i });
    expect(input).toHaveValue("My Provider");
    expect(screen.getByRole("button", { name: /save/i })).toBeDisabled();
  });

  it("keeps Save button disabled when input is unchanged or whitespace identical", () => {
    renderDialog({ currentName: "My Provider" });
    const input = screen.getByRole("textbox", { name: /display name/i });
    fireEvent.change(input, { target: { value: "   My Provider   " } });
    expect(screen.getByRole("button", { name: /save/i })).toBeDisabled();
  });

  it("enables Save button when input is modified to a new valid name", () => {
    renderDialog({ currentName: "My Provider" });
    const input = screen.getByRole("textbox", { name: /display name/i });
    fireEvent.change(input, { target: { value: "Updated Provider" } });
    expect(screen.getByRole("button", { name: /save/i })).toBeEnabled();
  });

  it("shows validation error if name is cleared or under 2 characters", async () => {
    renderDialog({ currentName: "My Provider" });
    const input = screen.getByRole("textbox", { name: /display name/i });
    fireEvent.change(input, { target: { value: "A" } });
    const saveButton = screen.getByRole("button", { name: /save/i });
    expect(saveButton).toBeEnabled();
    fireEvent.click(saveButton);

    expect(
      await screen.findByText("Display name must be at least 2 characters"),
    ).toBeInTheDocument();
    expect(mockMutateAsync).not.toHaveBeenCalled();
  });

  it("shows validation error if name exceeds 120 characters", async () => {
    renderDialog({ currentName: "My Provider" });
    const input = screen.getByRole("textbox", { name: /display name/i });
    fireEvent.change(input, { target: { value: "A".repeat(121) } });
    const saveButton = screen.getByRole("button", { name: /save/i });
    fireEvent.click(saveButton);

    expect(
      await screen.findByText("Display name must be at most 120 characters"),
    ).toBeInTheDocument();
    expect(mockMutateAsync).not.toHaveBeenCalled();
  });

  it("submits the update mutation with trimmed name and calls onClose upon success", async () => {
    mockMutateAsync.mockResolvedValueOnce({
      uuid: "provider-123",
      name: "Renamed Provider",
    });
    const onClose = vi.fn();
    renderDialog({
      orgName: "test-org",
      providerId: "provider-123",
      currentName: "Original Provider",
      onClose,
    });

    const input = screen.getByRole("textbox", { name: /display name/i });
    fireEvent.change(input, { target: { value: "  Renamed Provider  " } });

    const saveButton = screen.getByRole("button", { name: /save/i });
    fireEvent.click(saveButton);

    await waitFor(() => expect(mockMutateAsync).toHaveBeenCalledTimes(1));
    expect(mockMutateAsync).toHaveBeenCalledWith({
      params: { orgName: "test-org", providerId: "provider-123" },
      body: { name: "Renamed Provider" },
    });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("submits on Enter key press in the text field", async () => {
    mockMutateAsync.mockResolvedValueOnce({
      uuid: "provider-123",
      name: "Enter Provider",
    });
    const onClose = vi.fn();
    renderDialog({
      currentName: "Original Provider",
      onClose,
    });

    const input = screen.getByRole("textbox", { name: /display name/i });
    fireEvent.change(input, { target: { value: "Enter Provider" } });
    fireEvent.keyDown(input, { key: "Enter", code: "Enter" });

    await waitFor(() => expect(mockMutateAsync).toHaveBeenCalledTimes(1));
    expect(mockMutateAsync).toHaveBeenCalledWith({
      params: { orgName: "test-org", providerId: "provider-123" },
      body: { name: "Enter Provider" },
    });
    expect(onClose).toHaveBeenCalled();
  });

  it("displays error message if mutation fails", async () => {
    mockMutateAsync.mockRejectedValueOnce(new Error("Network connection error"));
    renderDialog({
      currentName: "Original Provider",
    });

    const input = screen.getByRole("textbox", { name: /display name/i });
    fireEvent.change(input, { target: { value: "New Provider Name" } });
    fireEvent.click(screen.getByRole("button", { name: /save/i }));

    expect(
      await screen.findByText("Network connection error"),
    ).toBeInTheDocument();
  });

  it("calls onClose when Cancel button is clicked", () => {
    const onClose = vi.fn();
    renderDialog({ onClose });
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
