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

import { act, createElement, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

const auth = vi.hoisted(() => ({ signIn: vi.fn<(u: string, p: string) => Promise<void>>() }));

vi.mock("../FileAuthContext", () => ({ useFileAuth: () => auth }));
vi.mock("@agent-management-platform/views", () => ({ Logo: () => null }));
// @wso2/oxygen-ui (and the icon package it pulls in) ships dist files that
// import raw .css, which Node's loader rejects without a Vite inline rule this
// package does not configure. The form's behaviour lives in LoginForm, not in
// the design system, so plain DOM stand-ins carry the same props through.
vi.mock("@wso2/oxygen-ui-icons-react", () =>
  Object.fromEntries(
    [
      "Activity",
      "ArrowRight",
      "Eye",
      "EyeOff",
      "Lock",
      "PencilRuler",
      "Rocket",
      "ShieldCheck",
      "Sparkles",
      "User",
    ].map((name) => [name, () => null]),
  ),
);
vi.mock("@wso2/oxygen-ui", async () => {
  const { createElement: h } = await import("react");
  type Props = Record<string, unknown> & { children?: ReactNode };
  const passthrough = ({ children }: Props) => h("div", null, children);
  return {
    alpha: () => "transparent",
    Alert: ({ children, severity }: Props) =>
      h("div", { role: "alert", "data-severity": severity }, children),
    Avatar: passthrough,
    Box: passthrough,
    Button: ({ children, disabled, type }: Props) =>
      h("button", { disabled, type }, children),
    Card: ({ children, component, onSubmit }: Props) =>
      h(component as string, { onSubmit }, children),
    CardContent: passthrough,
    Divider: () => h("hr"),
    FormLabel: ({ children, htmlFor }: Props) => h("label", { htmlFor }, children),
    IconButton: ({ children, onClick, ...rest }: Props) =>
      h("button", { type: "button", onClick, "aria-label": rest["aria-label"] }, children),
    InputAdornment: passthrough,
    List: passthrough,
    ListItem: passthrough,
    ListItemAvatar: passthrough,
    ListItemText: ({ primary }: Props) => h("span", null, primary as ReactNode),
    Stack: passthrough,
    TextField: ({ disabled, id, name, onChange, onKeyDown, slotProps, type, value }: Props) => {
      const adornments = (slotProps as { input?: Record<string, ReactNode> } | undefined)?.input;
      return h(
        "div",
        null,
        adornments?.startAdornment,
        h("input", { disabled, id, name, onChange, onKeyDown, type, value }),
        adornments?.endAdornment,
      );
    },
    Typography: ({ children }: Props) => h("span", null, children),
  };
});

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root | undefined;
let container: HTMLDivElement;
let LoginForm: typeof import("./LoginForm").LoginForm;

// The first import transforms @wso2/oxygen-ui, which can outlast the default
// per-test timeout. Pay that once here.
beforeAll(async () => {
  ({ LoginForm } = await import("./LoginForm"));
}, 60_000);

const input = (id: string) => container.querySelector<HTMLInputElement>(`#${id}`)!;
const submitButton = () =>
  container.querySelector<HTMLButtonElement>('button[type="submit"]')!;
const alertText = () => container.querySelector('[role="alert"]')?.textContent ?? null;

/** Sets a controlled input's value the way a keystroke would. */
const type = async (el: HTMLInputElement, value: string) => {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  await act(async () => {
    setter.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
};

const submitForm = () =>
  act(async () => {
    container
      .querySelector("form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });

const pressEnter = (el: HTMLInputElement) =>
  act(async () => {
    el.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
  });

beforeEach(async () => {
  auth.signIn.mockReset();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  await act(async () => root!.render(createElement(LoginForm)));
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  root = undefined;
});

describe("LoginForm", () => {
  it("renders the credential form with sign-in disabled until both fields are filled", async () => {
    expect(container.textContent).toContain("Use the credentials configured for this deployment.");
    expect(submitButton().disabled).toBe(true);

    await type(input("login-username"), "   ");
    await type(input("login-password"), "pw");
    // A whitespace-only username is not a username.
    expect(submitButton().disabled).toBe(true);

    await type(input("login-username"), "admin");
    expect(submitButton().disabled).toBe(false);
  });

  it("does not call signIn when submitted with empty fields", async () => {
    await submitForm();
    await pressEnter(input("login-username"));
    expect(auth.signIn).not.toHaveBeenCalled();
  });

  it("submits the trimmed username and the password as typed", async () => {
    auth.signIn.mockResolvedValue(undefined);
    await type(input("login-username"), "  admin  ");
    await type(input("login-password"), " p w ");

    await submitForm();

    expect(auth.signIn).toHaveBeenCalledWith("admin", " p w ");
    expect(alertText()).toBeNull();
  });

  it("submits on Enter from the password field", async () => {
    auth.signIn.mockResolvedValue(undefined);
    await type(input("login-username"), "admin");
    await type(input("login-password"), "pw");

    await pressEnter(input("login-password"));

    expect(auth.signIn).toHaveBeenCalledWith("admin", "pw");
  });

  it("ignores keys other than Enter", async () => {
    await type(input("login-username"), "admin");
    await type(input("login-password"), "pw");
    await act(async () => {
      input("login-password").dispatchEvent(
        new KeyboardEvent("keydown", { key: "a", bubbles: true }),
      );
    });
    expect(auth.signIn).not.toHaveBeenCalled();
  });

  it("shows a loading state and disables the fields while signing in", async () => {
    let resolve!: () => void;
    auth.signIn.mockReturnValue(new Promise<void>((r) => (resolve = r)));
    await type(input("login-username"), "admin");
    await type(input("login-password"), "pw");

    await submitForm();

    expect(submitButton().textContent).toContain("Signing in…");
    expect(submitButton().disabled).toBe(true);
    expect(input("login-username").disabled).toBe(true);
    expect(input("login-password").disabled).toBe(true);

    // A second submit while one is in flight is ignored.
    await submitForm();
    expect(auth.signIn).toHaveBeenCalledTimes(1);

    await act(async () => resolve());
    expect(submitButton().textContent).toContain("Sign in");
    expect(submitButton().textContent).not.toContain("Signing in");
  });

  it("shows the error message and clears only the password on failure", async () => {
    auth.signIn.mockRejectedValue(new Error("Incorrect username or password."));
    await type(input("login-username"), "admin");
    await type(input("login-password"), "wrong");

    await submitForm();

    expect(alertText()).toContain("Incorrect username or password.");
    expect(input("login-username").value).toBe("admin");
    expect(input("login-password").value).toBe("");
    expect(submitButton().disabled).toBe(true);
  });

  it("falls back to a generic message for a non-Error rejection and clears it on retry", async () => {
    auth.signIn.mockRejectedValueOnce("boom");
    await type(input("login-username"), "admin");
    await type(input("login-password"), "pw");
    await submitForm();
    expect(alertText()).toContain("Could not sign in.");

    auth.signIn.mockResolvedValueOnce(undefined);
    await type(input("login-password"), "pw");
    await submitForm();
    expect(alertText()).toBeNull();
  });

  it("toggles password visibility", async () => {
    expect(input("login-password").type).toBe("password");
    const toggle = () =>
      container.querySelector<HTMLButtonElement>('button[aria-label$="password"]')!;
    expect(toggle().getAttribute("aria-label")).toBe("Show password");

    await act(async () => toggle().click());
    expect(input("login-password").type).toBe("text");
    expect(toggle().getAttribute("aria-label")).toBe("Hide password");

    await act(async () => toggle().click());
    expect(input("login-password").type).toBe("password");
  });
});
