/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { OxygenUIThemeProvider } from "@wso2/oxygen-ui";
import { ProfileDrawer } from "./ProfileDrawer";


const mocks = vi.hoisted(() => ({
  enabled: true,
  profile: undefined as unknown,
  update: vi.fn(),
  isPending: false,
  error: null as unknown,
}));

vi.mock("@agent-management-platform/auth", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useAuthHooks: () => ({
    userInfo: { sub: "u1", username: "jdoe", givenName: "Jane", familyName: "Doe", email: "j@x.io" },
  }),
}));

vi.mock("@agent-management-platform/feature-management", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useFeatureFlag: () => mocks.enabled,
}));

vi.mock("@agent-management-platform/api-client", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useGetUserProfile: () => ({ data: mocks.profile }),
  useUpdateUserProfile: () => ({
    mutateAsync: mocks.update,
    isPending: mocks.isPending,
    error: mocks.error,
  }),
}));

const onClose = vi.fn();

function renderDrawer() {
  return render(
    <OxygenUIThemeProvider>
      <MemoryRouter initialEntries={["/org/acme"]}>
        <Routes>
          <Route path="/org/:orgId" element={<ProfileDrawer open onClose={onClose} />} />
        </Routes>
      </MemoryRouter>
    </OxygenUIThemeProvider>,
  );
}

const field = (label: RegExp) => screen.getByLabelText(label) as HTMLInputElement;

beforeEach(() => {
  mocks.enabled = true;
  mocks.profile = undefined;
  mocks.update.mockReset().mockResolvedValue({});
  mocks.isPending = false;
  mocks.error = null;
  onClose.mockReset();
});

describe("ProfilePage", () => {
  it("prefills from the sign-in claims and saves trimmed changes", async () => {
    renderDrawer();
    expect(screen.getByText("Profile Settings")).toBeInTheDocument();
    expect(field(/Username/).value).toBe("jdoe");
    expect(field(/Email/).value).toBe("j@x.io");
    fireEvent.change(field(/First Name/), { target: { value: "  Janet " } });
    fireEvent.change(field(/Last Name/), { target: { value: "Roe" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await screen.findByText("Profile updated successfully");
    expect(mocks.update).toHaveBeenCalledWith({
      params: { orgName: "acme", userId: "u1" },
      body: {
        attributes: { username: "jdoe", given_name: "Janet", family_name: "Roe", email: "j@x.io" },
      },
    });
  });

  it("prefers the stored profile attributes", () => {
    mocks.profile = {
      attributes: { username: "stored", given_name: "S", family_name: "T", email: "s@t.io" },
    };
    renderDrawer();
    expect(field(/Username/).value).toBe("stored");
    expect(field(/Email/).value).toBe("s@t.io");
  });

  it("validates the profile before saving and clears errors as the user types", () => {
    renderDrawer();
    fireEvent.change(field(/Username/), { target: { value: " " } });
    fireEvent.change(field(/Email/), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    expect(screen.getByText("Username is required")).toBeInTheDocument();
    expect(screen.getByText("Email is required")).toBeInTheDocument();
    expect(mocks.update).not.toHaveBeenCalled();

    fireEvent.change(field(/Username/), { target: { value: "ok" } });
    expect(screen.queryByText("Username is required")).not.toBeInTheDocument();
    fireEvent.change(field(/Email/), { target: { value: "not-an-email" } });
    expect(screen.queryByText("Email is required")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    expect(screen.getByText("Invalid email format")).toBeInTheDocument();
    expect(mocks.update).not.toHaveBeenCalled();
  });

  it("shows the update error", async () => {
    mocks.error = new Error("Server said no");
    mocks.update.mockRejectedValue(new Error("Server said no"));
    renderDrawer();
    expect(screen.getByText("Server said no")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(mocks.update).toHaveBeenCalled());
    expect(screen.queryByText("Profile updated successfully")).not.toBeInTheDocument();
  });

  it("falls back to a generic message for an error without one", () => {
    mocks.error = "weird";
    renderDrawer();
    expect(screen.getByText("An error occurred")).toBeInTheDocument();
  });

  it("is read-only when profile management is off", () => {
    mocks.enabled = false;
    renderDrawer();
    expect(field(/Username/)).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Change Password" })).toBeDisabled();
  });

  it("shows a saving state while the update runs", () => {
    mocks.isPending = true;
    renderDrawer();
    expect(screen.getByRole("button", { name: "Saving..." })).toBeDisabled();
  });

  it("validates and changes the password", async () => {
    renderDrawer();
    fireEvent.click(screen.getByRole("tab", { name: "Change Password" }));
    const submit = screen.getByRole("button", { name: "Update password" });
    expect(submit).toBeDisabled();

    fireEvent.change(field(/New Password/), { target: { value: "short" } });
    fireEvent.click(submit);
    expect(screen.getByText("Password must be at least 8 characters")).toBeInTheDocument();
    expect(screen.getByText("Passwords do not match")).toBeInTheDocument();

    fireEvent.change(field(/New Password/), { target: { value: "longenough" } });
    expect(screen.queryByText("Password must be at least 8 characters")).not.toBeInTheDocument();
    fireEvent.change(field(/Confirm Password/), { target: { value: "longenough" } });
    expect(screen.queryByText("Passwords do not match")).not.toBeInTheDocument();
    fireEvent.click(submit);

    await screen.findByText("Password updated successfully");
    expect(mocks.update).toHaveBeenCalledWith({
      params: { orgName: "acme", userId: "u1" },
      body: { attributes: { password: "longenough" } },
    });
    expect(field(/New Password/).value).toBe("");

    // Switching tabs clears the success message.
    fireEvent.click(screen.getByRole("tab", { name: "Profile Information" }));
    expect(screen.queryByText("Password updated successfully")).not.toBeInTheDocument();
  });

  it("requires a new password", () => {
    renderDrawer();
    fireEvent.click(screen.getByRole("tab", { name: "Change Password" }));
    fireEvent.change(field(/New Password/), { target: { value: "   " } });
    fireEvent.click(screen.getByRole("button", { name: "Update password" }));
    expect(screen.getByText("New password is required")).toBeInTheDocument();
    expect(mocks.update).not.toHaveBeenCalled();
  });

  it("dismisses the success message", async () => {
    renderDrawer();
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await screen.findByText("Profile updated successfully");
    const alert = screen.getByText("Profile updated successfully").closest("[role=alert]") as HTMLElement;
    fireEvent.click(within(alert).getByRole("button"));
    expect(screen.queryByText("Profile updated successfully")).not.toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
  });
});
