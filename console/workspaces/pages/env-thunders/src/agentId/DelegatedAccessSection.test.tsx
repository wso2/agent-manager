/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { DelegatedAccessSection } from "./DelegatedAccessSection";

const agent = {
  enabled: true,
  orgName: "agent-manager-team",
  projectName: "yashed",
  identityId: "identity-1",
};

describe("DelegatedAccessSection", () => {
  it("hides the section in legacy mode even when the console URL is configured", () => {
    render(
      <DelegatedAccessSection
        {...agent}
        enabled={false}
        consoleUrl="/iam"
        isExternal={false}
      />,
    );
    expect(
      screen.queryByRole("heading", { name: "Delegated Access (OBO)" }),
    ).not.toBeInTheDocument();
  });
  it.each([
    "https://console.dev.example.com/iam/",
    "https://console.stage.example.com/iam/",
    "https://console.example.com/iam/",
    "https://console.eu.example.com/iam/",
  ])(
    "opens the agent in the configured Identity Platform console at %s",
    (consoleUrl) => {
      render(
        <DelegatedAccessSection
          {...agent}
          consoleUrl={consoleUrl}
          isExternal={false}
        />,
      );
      const link = screen.getByRole("link", {
        name: "Configure in Identity Platform",
      });
      expect(link).toHaveAttribute(
        "href",
        `${consoleUrl}organizations/agent-manager-team/projects/yashed/agents/identity-1`,
      );
      expect(link).toHaveAttribute("rel", "noopener noreferrer");
      expect(
        screen.getByText(/credentials available to your hosted agent/),
      ).toBeInTheDocument();
    },
  );

  it("guides external agents and uses the selected identity", () => {
    render(
      <DelegatedAccessSection
        {...agent}
        consoleUrl="/iam"
        identityId="identity-2"
        isExternal
      />,
    );
    expect(screen.getByText(/in your external runtime/)).toBeInTheDocument();
    expect(screen.getByRole("link")).toHaveAttribute(
      "href",
      `${window.location.origin}/iam/organizations/agent-manager-team/projects/yashed/agents/identity-2`,
    );
  });

  it("waits for an active agent identity before showing the link", () => {
    render(
      <DelegatedAccessSection
        {...agent}
        consoleUrl="/iam"
        identityId={undefined}
        isExternal
      />,
    );
    expect(
      screen.getByText(/once the agent has an active Agent ID/),
    ).toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it.each([
    undefined,
    "$IDENTITY_PLATFORM_URL",
    "javascript:alert(1)",
    "//example.com/iam",
  ])(
    "hides the section when the console URL is unavailable or invalid (%s)",
    (consoleUrl) => {
      render(
        <DelegatedAccessSection
          {...agent}
          consoleUrl={consoleUrl}
          isExternal={false}
        />,
      );
      expect(
        screen.queryByRole("heading", { name: "Delegated Access (OBO)" }),
      ).not.toBeInTheDocument();
    },
  );
});
