import { Button } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { PageLayout } from "@agent-management-platform/views";
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

  return (
    <Routes>
      <Route
        index
        element={
          <PageLayout
            title="MCP Servers"
            disableIcon
            actions={
              <Button
                variant="contained"
                color="primary"
                component={Link}
                to={generatePath(
                  absoluteRouteMap.children.org.children.mcpProxies.children.add
                    .path,
                  routeParams,
                )}
                startIcon={<Plus size={16} />}
              >
                Register MCP Server
              </Button>
            }
          >
            <MCPProxyTable />
          </PageLayout>
        }
      />
      <Route path="add" element={<AddMCPProxyOrganization />} />
      <Route path="view/:proxyId" element={<ViewMCPProxy />} />
      <Route path="view/:proxyId/edit" element={<EditMCPProxy />} />
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
