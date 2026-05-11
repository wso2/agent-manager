import React, { useMemo } from "react";
import { Button, Chip, ListingTable, Typography } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useNavigate, useParams } from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { DUMMY_CATALOG_LIST, getLatestVersion } from "./catalog.mock";

const MOCK_ITEM = DUMMY_CATALOG_LIST[0];

export const PublishComponent: React.FC = () => {
  const navigate = useNavigate();
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  const createVersionPath = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents.children.publish.children.createNewVersion.path,
    { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "" },
  );

  const versions = useMemo(
    () =>
      Object.entries(MOCK_ITEM.versions).sort(
        ([, a], [, b]) => new Date(b.releaseDate).getTime() - new Date(a.releaseDate).getTime(),
      ),
    [],
  );

  const latestVersionKey = useMemo(() => getLatestVersion(MOCK_ITEM)?.versionKey, []);

  const handleRowClick = (versionKey: string) => {
    navigate(
      generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents.children.publish.children.versionDetails.path,
        { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "", versionId: versionKey },
      ),
    );
  };

  return (
    <PageLayout
      title="Publish"
      description="Manage and publish versions of this agent kind to the catalog."
      disableIcon
      actions={
        <Button
          variant="contained"
          component={Link}
          to={createVersionPath}
          startIcon={<Plus />}
          color="primary"
        >
          Create Version
        </Button>
      }
    >
      <ListingTable.Container>
        <ListingTable>
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell width="12%">Version</ListingTable.Cell>
              <ListingTable.Cell width="18%">Release Date</ListingTable.Cell>
              <ListingTable.Cell>Description</ListingTable.Cell>
              <ListingTable.Cell width="15%">Changes</ListingTable.Cell>
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {versions.map(([versionKey, version]) => (
              <ListingTable.Row
                key={versionKey}
                hover
                clickable
                onClick={() => handleRowClick(versionKey)}
              >
                <ListingTable.Cell>
                  <Typography variant="body2" fontWeight={600}>
                    v{versionKey}
                    {versionKey === latestVersionKey && (
                      <Chip
                        label="Latest"
                        size="small"
                        color="primary"
                        sx={{ ml: 1, height: 18, fontSize: "0.65rem" }}
                      />
                    )}
                  </Typography>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Typography variant="body2" color="text.secondary">
                    {new Date(version.releaseDate).toLocaleDateString("en-US", {
                      year: "numeric",
                      month: "short",
                      day: "numeric",
                    })}
                  </Typography>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Typography variant="body2">{version.description}</Typography>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Typography variant="body2" color="text.secondary">
                    {version.changes.length} change{version.changes.length !== 1 ? "s" : ""}
                  </Typography>
                </ListingTable.Cell>
              </ListingTable.Row>
            ))}
          </ListingTable.Body>
        </ListingTable>
      </ListingTable.Container>
    </PageLayout>
  );
};

export default PublishComponent;
