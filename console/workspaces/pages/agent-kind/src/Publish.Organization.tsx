import React from "react";
import { Navigate, Route, Routes, useParams } from "react-router-dom";
import { generatePath } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { PublishComponent } from "./Publish.Component";
import { PublishCreateVersion } from "./Publish.CreateVersion";
import { PublishVersionDetails } from "./Publish.VersionDetails";

export const PublishOrganization: React.FC = () => {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  return (
    <Routes>
      <Route index element={<PublishComponent />} />
      <Route path="create-new-version" element={<PublishCreateVersion />} />
      <Route path="version-details/:versionId" element={<PublishVersionDetails />} />
      <Route
        path="*"
        element={
          <Navigate
            to={generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents.children.publish.path,
              { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "" },
            )}
          />
        }
      />
    </Routes>
  );
};

export default PublishOrganization;
