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
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { DestinationKindListResponse } from "@agent-management-platform/types";
import { destination, kinds } from "./__test__/fixtures";

const hoisted = vi.hoisted(() => {
  // The console sets its runtime config before any module loads.
  (window as unknown as { __RUNTIME_CONFIG__: unknown }).__RUNTIME_CONFIG__ = {
    docsUrl: "https://docs.example.com",
  };
  return {
  kinds: undefined as unknown,
  };
});

vi.mock("@agent-management-platform/api-client", () => ({
  useDestinationKinds: () => ({ data: hoisted.kinds }),
  useListAllProjects: () => ({
    data: { list: [{ metadata: { name: "shop" } }, { metadata: { name: "billing" } }] },
  }),
}));

import { DestinationForm, type DestinationFormProps } from "./DestinationForm";

const props = (over: Partial<DestinationFormProps> = {}): DestinationFormProps => ({
  orgName: "acme",
  saving: false,
  submitLabel: "Add destination",
  onCancel: vi.fn(),
  onSubmit: vi.fn(),
  onDirtyChange: vi.fn(),
  ...over,
});

const type = (label: string, value: string) =>
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
const submit = (label = "Add destination") =>
  fireEvent.click(screen.getByRole("button", { name: label }));

// Full MUI pages are slow to render on a busy machine.
vi.setConfig({ testTimeout: 30000 });

beforeEach(() => {
  hoisted.kinds = kinds;
});

describe("DestinationForm", () => {
  it("validates a new Slack destination", () => {
    const p = props();
    render(<DestinationForm {...p} />);

    expect(screen.getByRole("button", { name: "Slack" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("link", { name: "Set up this destination" })).toHaveAttribute(
      "href",
      "https://docs.example.com/slack",
    );
    submit();
    expect(screen.getByText("Enter a name")).toBeInTheDocument();
    expect(screen.getByText("Enter the service URL")).toBeInTheDocument();

    type("Slack URL", "https://hooks.slack.com/x");
    expect(screen.getByText("A Slack URL starts with slack://")).toBeInTheDocument();
    expect(p.onSubmit).not.toHaveBeenCalled();
  });

  it("asks for a project when availability is limited to projects", () => {
    const p = props();
    render(<DestinationForm {...p} />);

    type("Slack URL", "slack://token@channel");
    type("Name", "#alerts");
    fireEvent.click(screen.getByLabelText("Selected projects"));
    submit();

    expect(screen.getByText("Select at least one project")).toBeInTheDocument();
    expect(p.onSubmit).not.toHaveBeenCalled();
  });

  // The project picker cannot be opened (TextInput replaces the Autocomplete's
  // InputProps), so the projects come from the starting availability.
  it("submits a Slack destination for selected projects", () => {
    const p = props({ defaultAvailability: { availability: "projects", projects: ["shop"] } });
    render(<DestinationForm {...p} />);

    expect(screen.getByLabelText("Selected projects")).toBeChecked();
    type("Slack URL", " slack://token@channel ");
    type("Name", " #alerts ");
    type("Description (optional)", "acme workspace");
    submit();

    expect(p.onSubmit).toHaveBeenCalledWith({
      name: "#alerts",
      description: "acme workspace",
      kind: "notify",
      config: { service: "slack", url: "slack://token@channel" },
      availability: "projects",
      projects: ["shop"],
    });
    expect(p.onDirtyChange).toHaveBeenLastCalledWith(true);
  });

  it("submits a webhook after checking its URL", () => {
    const p = props();
    render(<DestinationForm {...p} />);

    fireEvent.click(screen.getByRole("button", { name: "Webhook" }));
    type("Endpoint URL", "ftp://ci.example.com");
    type("Name", "CI");
    submit();
    expect(screen.getByText("Enter an http(s) URL")).toBeInTheDocument();

    type("Endpoint URL", "https://ci.example.com/hook");
    fireEvent.click(screen.getByLabelText("Organization only"));
    submit();

    expect(p.onSubmit).toHaveBeenCalledWith({
      name: "CI",
      description: "",
      kind: "webhook",
      config: { url: "https://ci.example.com/hook" },
      availability: "org",
      projects: [],
    });
  });

  it("picks a service from More services", () => {
    const p = props();
    render(<DestinationForm {...p} />);

    fireEvent.click(screen.getByRole("button", { name: "More services" }));
    expect(screen.getByText("Choose a service first.")).toBeInTheDocument();
    type("Name", "Gamers");
    submit();
    expect(screen.getByText("Choose a service")).toBeInTheDocument();

    fireEvent.mouseDown(screen.getByRole("combobox"));
    expect(screen.getByText("Paging")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("option", { name: /Discord/ }));
    type("Discord URL", "discord://id/token");
    submit();

    expect(p.onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: "notify",
        config: { service: "discord", url: "discord://id/token" },
      }),
    );
  });

  it("keeps an edited destination's type and its stored URL", () => {
    const p = props({
      submitLabel: "Save",
      destination: destination({ name: "Alerts", description: "old", service: "slack" }),
    });
    render(<DestinationForm {...p} />);

    expect(screen.getByText("The type of a destination cannot be changed.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Webhook" })).toBeDisabled();
    expect(screen.getByLabelText("Slack URL")).toHaveAttribute(
      "placeholder",
      "Leave blank to keep the current URL",
    );
    expect(screen.getByLabelText("Name")).toHaveValue("Alerts");
    expect(p.onDirtyChange).toHaveBeenLastCalledWith(false);

    type("Description (optional)", "new");
    submit("Save");

    expect(p.onSubmit).toHaveBeenCalledWith({
      name: "Alerts",
      description: "new",
      kind: "notify",
      config: { service: "slack" },
      availability: "all",
      projects: [],
    });
  });

  it("opens an edited destination on a more-services service", () => {
    render(
      <DestinationForm
        {...props({ destination: destination({ service: "opsgenie", name: "Genie" }) })}
      />,
    );

    expect(screen.getByRole("combobox")).toHaveTextContent("Opsgenie");
    expect(screen.getByLabelText("Opsgenie URL")).toBeInTheDocument();
  });

  it("offers webhooks only when the service has no notification kind", () => {
    hoisted.kinds = { kinds: [kinds.kinds[1]] } satisfies DestinationKindListResponse;
    render(<DestinationForm {...props()} />);

    expect(screen.queryByRole("button", { name: "Slack" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Webhook" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByLabelText("Endpoint URL")).toBeInTheDocument();
  });

  it("cancels", () => {
    const p = props();
    render(<DestinationForm {...p} />);

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(p.onCancel).toHaveBeenCalled();
  });

  it("disables the buttons while saving", () => {
    const p = props({ saving: true });
    render(<DestinationForm {...p} />);

    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Add destination" })).toBeDisabled();
  });
});
