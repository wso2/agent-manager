/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 */

import { AddMCPProxyOrganization } from "./AddMCPProxy.Organization";
import { MCPLogo } from "./components/MCPLogo";
import { MCPProxiesOrganization } from "./MCPProxies.Organization";
import { ViewMCPProxy } from "./subComponents/ViewMCPProxy";
import {
  getCapabilityId,
  isToolBlockedByAcl,
  type CapabilityKind,
} from "./subComponents/mcpEndpoints";
import type { PageMetadata } from "@agent-management-platform/types";

export const metaData: PageMetadata = {
  title: "MCP Servers",
  description: "A page component for MCP Server management",
  icon: MCPLogo,
  path: "/mcp-proxies",
  component: MCPProxiesOrganization,
  levels: {
    organization: MCPProxiesOrganization,
    addMCPProxyOrganization: AddMCPProxyOrganization,
    viewMCPProxyOrganization: ViewMCPProxy,
  },
};

export {
  AddMCPProxyOrganization,
  MCPLogo,
  MCPProxiesOrganization,
  ViewMCPProxy,
  getCapabilityId,
  isToolBlockedByAcl,
  type CapabilityKind,
};

export default MCPProxiesOrganization;
