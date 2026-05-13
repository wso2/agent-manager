import React from "react";
import { generatePath, useParams } from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap, type AgentKindResponse } from "@agent-management-platform/types";
import { CatalogKindListing } from "./subComponents/CatalogKindListing";
import { useListAgentKinds } from "@agent-management-platform/api-client";

export const CatalogList: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const { data, isLoading } = useListAgentKinds({ orgName: orgId ?? "" });

  const getViewPath = (item: AgentKindResponse) =>
    generatePath(absoluteRouteMap.children.org.children.catalog.children.kindDetails.path, {
      orgId: orgId ?? "",
      kindId: item.name,
    });

  return (
    <PageLayout
      title="Agent Catalog"
      description="Browse cataloged agent kinds of the organization."
      disableIcon
    >
      <CatalogKindListing items={data?.kinds ?? []} isLoading={isLoading} getViewPath={getViewPath} />
    </PageLayout>
  );
};

export default CatalogList;
