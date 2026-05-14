import type { PageMetadata } from "@agent-management-platform/types";
import { Package as PackageIcon } from "@wso2/oxygen-ui-icons-react";
import { PublishedList } from "./Publish.List";
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
    component: PublishedList,
    organization: CatalogOrganization,
    kindDetails: CatalogKindDetails,
    publishOrganization: PublishComponent,
  },
};

export { PublishComponent, PublishedList, CatalogOrganization, CatalogKindDetails };
export { PublishVersionDetails } from "./Publish.VersionDetails";
export { CatalogKindListing } from "./subComponents/CatalogKindListing";
export type { CatalogKindListingProps } from "./subComponents/CatalogKindListing";

export default PublishComponent;
