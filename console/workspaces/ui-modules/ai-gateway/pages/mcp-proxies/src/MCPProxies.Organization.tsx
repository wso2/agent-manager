import { Button } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { PageLayout } from "@agent-management-platform/views";
import { useListMCPProxies } from "@agent-management-platform/api-client";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import { AddMCPProxyOrganization } from "./AddMCPProxy.Organization";
import { MCPProxyTable } from "./subComponents/MCPProxyTable";
import { ViewMCPProxy } from "./subComponents/ViewMCPProxy";
import { EditMCPProxy } from "./subComponents/EditMCPProxy";
import {
  generatePath,
  Link,
  Navigate,
  Route,
  Routes,
  useParams,
} from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";

export const MCPProxiesOrganization = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const routeParams = { orgId: orgId ?? "" };
  const { canWrite } = useResourceAccess("mcp_server");

  // Same query as the table (deduped); the header CTA only shows once the list has items.
  const { data } = useListMCPProxies({ orgName: orgId });
  const hasItems = (data?.list?.length ?? 0) > 0;

  // One definition, used both as the page action and as the empty state's CTA.
  const createButton = (
    <Button
      variant="contained"
      color="primary"
      component={Link}
      to={generatePath(
        absoluteRouteMap.children.org.children.mcpProxies.children.add.path,
        routeParams,
      )}
      startIcon={<Plus size={16} />}
    >
      Register MCP server
    </Button>
  );

  return (
    <Routes>
      <Route
        index
        element={
          <PageLayout
            title="MCP Servers"
            disableIcon
            actions={hasItems && canWrite ? createButton : undefined}
          >
            <MCPProxyTable createAction={canWrite ? createButton : undefined} />
          </PageLayout>
        }
      />
      {canWrite && <Route path="add" element={<AddMCPProxyOrganization />} />}
      <Route path="view/:proxyId" element={<ViewMCPProxy />} />
      {canWrite && <Route path="view/:proxyId/edit" element={<EditMCPProxy />} />}
      <Route
        path="*"
        element={
          <Navigate
            to={generatePath(
              absoluteRouteMap.children.org.children.mcpProxies.path,
              routeParams,
            )}
            replace
          />
        }
      />
    </Routes>
  );
};

export default MCPProxiesOrganization;
