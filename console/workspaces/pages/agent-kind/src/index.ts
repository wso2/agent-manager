import type { PageMetadata } from "@agent-management-platform/types";
import { Package as PackageIcon } from "@wso2/oxygen-ui-icons-react";
import { PublishComponent } from "./Publish.Component";
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
  },
};

export { PublishComponent, CatalogOrganization, CatalogKindDetails };
export { CatalogKindListing } from "./subComponents/CatalogKindListing";
export type { CatalogKindListingProps } from "./subComponents/CatalogKindListing";
export type { CatalogItem, CatalogItemVersion, LatestVersion } from "./catalog.mock";
export { getLatestVersion, DUMMY_CATALOG_LIST } from "./catalog.mock";

export default PublishComponent;
