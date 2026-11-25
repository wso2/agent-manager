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

import { fireEvent, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderRoute } from "./__test__/renderRoute";

const hoisted = vi.hoisted(() => ({
  inviteUser: vi.fn(),
  isPending: false,
  error: null as unknown,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useInviteUser: () => ({
    mutateAsync: hoisted.inviteUser,
    isPending: hoisted.isPending,
    error: hoisted.error,
  }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
}));

import { UserInvitePage } from "./UserInvitePage";

const LINK = "https://amp.example.com/invite/abc";

const renderPage = () =>
  renderRoute(
    "/org/acme/settings/identities/users/invite",
    "/org/:orgId/settings/identities/users/invite",
    <UserInvitePage />,
  );

const enterEmail = (value: string) =>
  fireEvent.change(screen.getByLabelText("Email Address"), { target: { value } });

const writeText = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.isPending = false;
  hoisted.error = null;
  writeText.mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {
    value: { writeText },
    configurable: true,
  });
});

describe("UserInvitePage", () => {
  it("validates the email as it is typed", () => {
    renderPage();

    expect(screen.getByRole("button", { name: "Get invitation link" })).toBeDisabled();
    enterEmail("nope");
    expect(screen.getByText("Enter a valid email address")).toBeInTheDocument();
  });

  it("does not send an invalid email", () => {
    renderPage();

    enterEmail("nope");
    fireEvent.click(screen.getByRole("button", { name: "Get invitation link" }));
    expect(hoisted.inviteUser).not.toHaveBeenCalled();
  });

  it("invites the trimmed email and shows the link to share", async () => {
    hoisted.inviteUser.mockResolvedValue({ inviteLink: LINK });
    renderPage();

    enterEmail(" jane@example.com ");
    fireEvent.click(screen.getByRole("button", { name: "Get invitation link" }));

    expect(await screen.findByDisplayValue(LINK)).toBeInTheDocument();
    expect(hoisted.inviteUser).toHaveBeenCalledWith({
      params: { orgName: "acme" },
      body: { email: "jane@example.com" },
    });
    expect(screen.getByText(/An invitation has been created for/)).toBeInTheDocument();
  });

  it("copies the link to the clipboard", async () => {
    hoisted.inviteUser.mockResolvedValue({ inviteLink: LINK });
    renderPage();

    enterEmail("jane@example.com");
    fireEvent.click(screen.getByRole("button", { name: "Get invitation link" }));
    fireEvent.click(await screen.findByRole("button", { name: "Copy link" }));

    expect(writeText).toHaveBeenCalledWith(LINK);
    expect(await screen.findByRole("button", { name: "Copied!" })).toBeInTheDocument();
  });

  it("resets the form to invite another user", async () => {
    hoisted.inviteUser.mockResolvedValue({ inviteLink: LINK });
    renderPage();

    enterEmail("jane@example.com");
    fireEvent.click(screen.getByRole("button", { name: "Get invitation link" }));
    fireEvent.click(await screen.findByRole("button", { name: "Invite another" }));

    expect(screen.getByLabelText("Email Address")).toHaveValue("");
    expect(screen.queryByDisplayValue(LINK)).not.toBeInTheDocument();
  });

  it("returns to the users list when done", async () => {
    hoisted.inviteUser.mockResolvedValue({ inviteLink: LINK });
    renderPage();

    enterEmail("jane@example.com");
    fireEvent.click(screen.getByRole("button", { name: "Get invitation link" }));
    fireEvent.click(await screen.findByRole("button", { name: "Done" }));

    expect(screen.getByText("at /org/acme/settings/identities/users")).toBeInTheDocument();
  });

  it("stays on the form when the invite fails", async () => {
    hoisted.inviteUser.mockRejectedValue(new Error("nope"));
    renderPage();

    enterEmail("jane@example.com");
    fireEvent.click(screen.getByRole("button", { name: "Get invitation link" }));

    await vi.waitFor(() => expect(hoisted.inviteUser).toHaveBeenCalled());
    expect(screen.getByLabelText("Email Address")).toBeInTheDocument();
  });

  it("shows the server error", () => {
    hoisted.error = new Error("Already a member");
    renderPage();

    expect(screen.getByText("Already a member")).toBeInTheDocument();
  });

  it("cancels back to the users list", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("at /org/acme/settings/identities/users")).toBeInTheDocument();
  });
});
