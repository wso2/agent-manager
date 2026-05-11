import type { PageMetadata } from "@agent-management-platform/types";
import { Package as PackageIcon } from "@wso2/oxygen-ui-icons-react";
import { PublishComponent } from "./Publish.Component";
import { PublishOrganization } from "./Publish.Organization";
import { CatalogOrganization } from "./Catalog.Organization";
import { CatalogKindDetails } from "./Catalog.KindDetails";

export const metaData: PageMetadata = {
  title: "Agent Kind",
  description: "Agent Kind pages",
  icon: PackageIcon,
  path: "/agent-kind",
  component: PublishComponent,
  levels: {
    component: PublishComponent,
    organization: CatalogOrganization,
    kindDetails: CatalogKindDetails,
    publishOrganization: PublishOrganization,
  },
};

export { PublishComponent, PublishOrganization, CatalogOrganization, CatalogKindDetails };
export { PublishCreateVersion } from "./Publish.CreateVersion";
export { PublishVersionDetails } from "./Publish.VersionDetails";
export { CatalogKindListing } from "./subComponents/CatalogKindListing";
export type { CatalogKindListingProps } from "./subComponents/CatalogKindListing";
export type { CatalogItem, CatalogItemVersion, LatestVersion } from "./catalog.mock";
export { getLatestVersion, DUMMY_CATALOG_LIST } from "./catalog.mock";

export default PublishComponent;
