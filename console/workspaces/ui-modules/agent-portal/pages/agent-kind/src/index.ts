import type { PageMetadata } from "@agent-management-platform/types";
import { Layers as KindIcon } from "@wso2/oxygen-ui-icons-react";
import { PublishedList } from "./Publish.List";
import { PublishComponent } from "./Publish.Component";
import { CatalogOrganization } from "./Catalog.Organization";
import { CatalogKindDetails } from "./Catalog.KindDetails";

export const metaData: PageMetadata = {
  title: "Agent Kind",
  description: "Agent Kind pages",
  icon: KindIcon,
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
// The listing lives in shared-component (ACP Core's create-agent flow uses it
// too); re-exported here for existing importers.
export {
  CatalogKindListing,
  type CatalogKindListingProps,
} from "@agent-management-platform/shared-component";

export default PublishComponent;
