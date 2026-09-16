import { PageLayout } from "@agent-management-platform/views";
import { AddMCPProxyOrganization } from "./AddMCPProxy.Organization";
import { MCPProxyTable } from "./subComponents/MCPProxyTable";
import { ViewMCPProxy } from "./subComponents/ViewMCPProxy";
import {
  generatePath,
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
          <PageLayout docs={["mcpProxy", "registerMcpProxy", "authorizeAgentMcpTools"]} title="MCP Servers" disableIcon>
            <MCPProxyTable />
          </PageLayout>
        }
      />
      <Route path="add" element={<AddMCPProxyOrganization />} />
      <Route path="view/:proxyId" element={<ViewMCPProxy />} />
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
