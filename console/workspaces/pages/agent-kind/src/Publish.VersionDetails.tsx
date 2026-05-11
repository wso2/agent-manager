import React, { useMemo } from "react";
import { generatePath, useParams } from "react-router-dom";
import { Alert, Box, Chip, Divider, Stack, Typography } from "@wso2/oxygen-ui";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { SwaggerSpecViewer } from "@agent-management-platform/shared-component";
import { DUMMY_CATALOG_LIST, type CatalogItemVersion } from "./catalog.mock";

const MOCK_ITEM = DUMMY_CATALOG_LIST[0];

function buildOpenApiSpec(versionId: string, apiSpecs: Record<string, unknown>): Record<string, unknown> {
  return {
    openapi: "3.0.0",
    info: {
      title: `${MOCK_ITEM.title} — v${versionId}`,
      version: versionId,
    },
    paths: {
      "/invoke": {
        post: {
          summary: "Invoke the agent",
          operationId: "invokeAgent",
          requestBody: {
            required: true,
            content: {
              "application/json": {
                schema: apiSpecs.input as Record<string, unknown>,
              },
            },
          },
          responses: {
            "200": {
              description: "Successful response",
              content: {
                "application/json": {
                  schema: apiSpecs.output as Record<string, unknown>,
                },
              },
            },
          },
        },
      },
    },
  };
}

export const PublishVersionDetails: React.FC = () => {
  const { orgId, projectId, agentId, versionId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    versionId: string;
  }>();

  const backHref = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents.children.publish.path,
    { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "" },
  );

  const version: CatalogItemVersion | undefined = versionId ? MOCK_ITEM.versions[versionId] : undefined;

  const openApiSpec = useMemo(() => {
    if (!version?.apiSpecs || !versionId) return null;
    return buildOpenApiSpec(versionId, version.apiSpecs as Record<string, unknown>);
  }, [version, versionId]);

  const formattedDate = version
    ? new Date(version.releaseDate).toLocaleDateString("en-US", {
      year: "numeric",
      month: "long",
      day: "numeric",
    })
    : undefined;

  return (
    <PageLayout
      title={`v${versionId}`}
      description={version?.description ?? "Version details"}
      disableIcon
      backHref={backHref}
      backLabel="Back to Publish"
    >
      <Stack spacing={3}>
        {/* Metadata */}
        <Stack direction="row" spacing={1} alignItems="center">
          <Chip label={`v${versionId}`} size="small" color="primary" variant="outlined" />
          {formattedDate && (
            <Typography variant="body2" color="text.secondary">
              Released on {formattedDate}
            </Typography>
          )}
        </Stack>

        <Divider />

        {/* Changelog */}
        <Stack spacing={1}>
          <Typography variant="subtitle1" fontWeight={600}>
            Changelog
          </Typography>
          {version?.changes && version.changes.length > 0 ? (
            <Stack component="ul" spacing={0.5} sx={{ m: 0, pl: 2.5 }}>
              {version.changes.map((change, i) => (
                <Typography component="li" variant="body2" key={i}>
                  {change}
                </Typography>
              ))}
            </Stack>
          ) : (
            <Typography variant="body2" color="text.secondary">
              No changes listed.
            </Typography>
          )}
        </Stack>

        <Divider />

        {/* API Spec */}
        <Stack spacing={1.5}>
          <Typography variant="subtitle1" fontWeight={600}>
            API Specification
          </Typography>
          {openApiSpec ? (
            <SwaggerSpecViewer
              spec={openApiSpec}
              docExpansion="full"
              defaultModelsExpandDepth={2}
              hideInfoSection
              hideServers
              hideAuthorizeButton
            />
          ) : (
            <Alert severity="info">No API specification available for this version.</Alert>
          )}
        </Stack>
      </Stack>
    </PageLayout>
  );
};

export default PublishVersionDetails;

