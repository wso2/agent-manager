/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 */

import { useMemo } from "react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { AddMCPProxyForm } from "./subComponents/AddMCPProxyForm";

export const AddMCPProxyOrganization = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();
  const backHref = useMemo(
    () =>
      generatePath(absoluteRouteMap.children.org.children.mcpProxies.path, {
        orgId: orgId ?? "",
      }),
    [orgId],
  );

  return (
    <PageLayout docs={["registerMcpProxy", "mcpProxy", "authorizeAgentMcpTools"]}
      title="Register MCP Server"
      backHref={backHref}
      backLabel="Back to MCP Server list"
      disableIcon
    >
      <AddMCPProxyForm onCancel={() => navigate(backHref)} />
    </PageLayout>
  );
};

export default AddMCPProxyOrganization;
