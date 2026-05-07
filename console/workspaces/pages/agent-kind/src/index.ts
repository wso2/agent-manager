import type { PageMetadata } from "@agent-management-platform/types";
import { Package as PackageIcon } from "@wso2/oxygen-ui-icons-react";
import { PublishComponent } from "./Publish.Component";
import { CatalogOrganization } from "./Catalog.Organization";

export const metaData: PageMetadata = {
  title: "Agent Kind",
  description: "Agent Kind pages",
  icon: PackageIcon,
  path: "/agent-kind",
  component: PublishComponent,
  levels: {
    component: PublishComponent,
    organization: CatalogOrganization,
  },
};

export { PublishComponent, CatalogOrganization };

export default PublishComponent;
