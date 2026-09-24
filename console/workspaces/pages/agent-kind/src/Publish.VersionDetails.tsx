/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useMemo } from "react";
import { generatePath, useParams } from "react-router-dom";
import {
  Alert,
  Box,
  Chip,
  ListingTable,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { useGetAgentKind, useGetAgentKindVersion, useGetBuild } from "@agent-management-platform/api-client";
import { SwaggerSpecViewer, parseOpenApiSpecContent } from "@agent-management-platform/shared-component";
import { SectionCard } from "./SectionCard";

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

  const { data: kind } = useGetAgentKind({ orgName: orgId!, kindName: agentId! });
  const { data: version, isLoading: isVersionLoading, isError: isVersionError }
    = useGetAgentKindVersion({
      orgName: orgId!,
      kindName: agentId!,
      versionTag: versionId!,
    });

  const { data: build, isLoading: isBuildLoading } = useGetBuild({
    orgName: orgId!,
    projName: version?.sourceProjectName,
    agentName: version?.sourceAgentName,
    buildName: version?.buildName,
  });

  const apiSpec = useMemo(
    () => parseOpenApiSpecContent(build?.inputInterface?.schema?.content),
    [build],
  );

  const formattedDate = useMemo(
    () =>
      version
        ? new Date(version.createdAt).toLocaleDateString("en-US", {
          year: "numeric",
          month: "long",
          day: "numeric",
        })
        : undefined,
    [version],
  );

  return (
    <PageLayout docs={["agentKindAndCatalog", "agentLifecycle"]}
      title={`${kind?.displayName || agentId} ${versionId}`}
      description={version ? `Build Id: ${version.buildName ?? "—"}` : ""}
      disableIcon
      backHref={backHref}
      backLabel="Back to Publish"
    >
      {isVersionLoading ? (
        <Box sx={{ p: 2 }}>
          <Skeleton variant="rounded" height={32} sx={{ mb: 2, maxWidth: 320 }} />
          <Skeleton variant="rounded" height={48} sx={{ mb: 1 }} />
          <Skeleton variant="rounded" height={48} sx={{ mb: 1 }} />
          <Skeleton variant="rounded" height={48} />
        </Box>
      ) : isVersionError ? (
        <Alert severity="error">Failed to load this version. Please try again.</Alert>
      ) : !version ? (
        <Alert severity="error">Version not found.</Alert>
      ) : (
        <Stack spacing={3}>
          {/* Metadata chips */}
          <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
            {version.agentSubType && (
              <Chip label={version.agentSubType} size="small" variant="outlined" />
            )}
            {formattedDate && (
              <Typography variant="body2" color="text.secondary">
                Published on {formattedDate}
              </Typography>
            )}
          </Stack>

          {/* Config Schema */}
          <SectionCard title="Configuration Schema">
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
              Set when this version was published — publish a new version to change it.
            </Typography>
            {version.configSchema.length > 0 ? (
              <ListingTable.Container>
                <ListingTable>
                  <ListingTable.Head>
                    <ListingTable.Row>
                      <ListingTable.Cell width="25%">Name</ListingTable.Cell>
                      <ListingTable.Cell width="30%">Description</ListingTable.Cell>
                      <ListingTable.Cell width="15%">Mandatory</ListingTable.Cell>
                      <ListingTable.Cell width="15%">Secret</ListingTable.Cell>
                      <ListingTable.Cell width="15%">Default Value</ListingTable.Cell>
                    </ListingTable.Row>
                  </ListingTable.Head>
                  <ListingTable.Body>
                    {version.configSchema.map((item) => (
                      <ListingTable.Row key={item.name}>
                        <ListingTable.Cell>
                          <Typography variant="body2" fontWeight={500}>{item.name}</Typography>
                        </ListingTable.Cell>
                        <ListingTable.Cell>
                          <Typography variant="body2" color="text.secondary">
                            {item.description ?? "—"}
                          </Typography>
                        </ListingTable.Cell>
                        <ListingTable.Cell>
                          <Typography variant="body2" color="text.secondary">
                            {item.isMandatory ? "Yes" : "No"}
                          </Typography>
                        </ListingTable.Cell>
                        <ListingTable.Cell>
                          <Typography variant="body2" color="text.secondary">
                            {item.isSecret ? "Yes" : "No"}
                          </Typography>
                        </ListingTable.Cell>
                        <ListingTable.Cell>
                          <Typography variant="body2" color="text.secondary">
                            {item.defaultValue ? (item.isSecret ? "••••••••••••••••" : item.defaultValue) : "—"}
                          </Typography>
                        </ListingTable.Cell>
                      </ListingTable.Row>
                    ))}
                  </ListingTable.Body>
                </ListingTable>
              </ListingTable.Container>
            ) : (
              <Alert severity="info">No configuration schema defined for this version.</Alert>
            )}
          </SectionCard>

          {/* API Specification */}
          <SectionCard title="API Specification">
            {isBuildLoading ? (
              <Skeleton variant="rounded" height={300} />
            ) : apiSpec ? (
              <SwaggerSpecViewer
                spec={apiSpec}
                docExpansion="list"
                hideInfoSection
                hideServers
                hideAuthorizeButton
              />
            ) : (
              <Alert severity="info">No API specification available for this version.</Alert>
            )}
          </SectionCard>
        </Stack>
      )}
    </PageLayout>
  );
};

export default PublishVersionDetails;
