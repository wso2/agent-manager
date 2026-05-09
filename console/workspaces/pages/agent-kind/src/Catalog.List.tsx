import React from "react";
import { generatePath, useParams } from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { DUMMY_CATALOG_LIST, type CatalogItem } from "./catalog.mock";
import { CatalogKindListing } from "./subComponents/CatalogKindListing";

export const CatalogList: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();

  const getViewPath = (item: CatalogItem) =>
    generatePath(absoluteRouteMap.children.org.children.catalog.children.kindDetails.path, {
      orgId: orgId ?? "",
      kindId: item.id,
    });

  return (
    <PageLayout
      title="Agent Catalog"
      description="Browse cataloged agent kinds of the organization."
      disableIcon
    >
      <CatalogKindListing items={DUMMY_CATALOG_LIST} getViewPath={getViewPath} />
    </PageLayout>
  );
};

export default CatalogList;
