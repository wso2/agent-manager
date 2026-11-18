/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { FeatureManagementProvider } from "@agent-management-platform/feature-management";
import { SelfHostedGatewayTokensCard } from "./SelfHostedGatewayTokensCard";

const tokenId = "3fa85f64-5717-4562-b3fc-2c963f66afa6";

vi.mock("@agent-management-platform/api-client", () => ({
  useSelfHostedGatewayTokens: () => ({
    data: {
      count: 1,
      list: [
        { id: tokenId, prefix: "9b2c41d0", status: "active", createdAt: new Date().toISOString() },
      ],
    },
    isLoading: false,
    error: null,
  }),
  useCreateSelfHostedGatewayToken: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRevokeSelfHostedGatewayToken: () => ({ mutateAsync: vi.fn() }),
}));

vi.mock("@agent-management-platform/shared-component", () => ({
  copyToClipboard: vi.fn(),
  getErrorMessage: () => "",
  useConfirmationDialog: () => ({ addConfirmation: vi.fn() }),
}));

describe("SelfHostedGatewayTokensCard", () => {
  it("lists a token by its masked start, not by its record id", () => {
    render(
      <FeatureManagementProvider>
        <SelfHostedGatewayTokensCard orgName="acme" gatewayId="gw-1" />
      </FeatureManagementProvider>,
    );

    expect(screen.getByText("9b2c41d0-••••••••")).toBeInTheDocument();
    expect(screen.queryByText(tokenId)).not.toBeInTheDocument();
  });
});
