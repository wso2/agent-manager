/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 */

import { useMemo } from "react";
import { generatePath, Navigate, useNavigate, useParams } from "react-router-dom";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import { PageLayout, useReturnTo } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { OrgAddMCPProxyForm } from "./subComponents/AddMCPProxyForm";

export const AddMCPProxyOrganization = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();
  const { canWrite } = useResourceAccess("mcp_server");
  const listHref = useMemo(
    () =>
      generatePath(absoluteRouteMap.children.org.children.mcpProxies.path, {
        orgId: orgId ?? "",
      }),
    [orgId],
  );
  // Back, Cancel and a successful save all return to the page that opened this one.
  const returnTo = useReturnTo(listHref);

  if (!canWrite) {
    return <Navigate to={listHref} replace />;
  }

  return (
    <PageLayout
      title="Register MCP Server"
      backHref={returnTo}
      backLabel={returnTo === listHref ? "Back to MCP server list" : "Back"}
      disableIcon
    >
      <OrgAddMCPProxyForm returnTo={returnTo} onCancel={() => navigate(returnTo)} />
    </PageLayout>
  );
};

export default AddMCPProxyOrganization;
