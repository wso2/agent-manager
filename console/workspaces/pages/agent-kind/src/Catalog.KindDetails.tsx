/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
import { generatePath, useParams, useSearchParams } from "react-router-dom";
import { Alert, Box, Button, Chip, Divider, ListingTable, MenuItem, Select, SelectChangeEvent, Stack, Typography } from "@wso2/oxygen-ui";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { SwaggerSpecViewer } from "@agent-management-platform/shared-component";
import { DUMMY_CATALOG_LIST, getLatestVersion } from "./catalog.mock";
import { Plus } from "@wso2/oxygen-ui-icons-react";

export const CatalogKindDetails: React.FC = () => {
  const { kindId, orgId } = useParams<{ kindId: string; orgId: string }>();

  const item = useMemo(
    () => DUMMY_CATALOG_LIST.find((c) => c.id === kindId),
    [kindId],
  );

  const versionKeys = useMemo(
    () =>
      item
        ? Object.entries(item.versions)
          .sort(
            ([, a], [, b]) =>
              new Date(b.releaseDate).getTime() - new Date(a.releaseDate).getTime(),
          )
          .map(([key]) => key)
        : [],
    [item],
  );

  const [searchParams, setSearchParams] = useSearchParams();
  const defaultVersion = useMemo(() => getLatestVersion(item!)?.versionKey ?? "", [item]);
  const selectedVersion = searchParams.get("version") ?? defaultVersion;

  const versionData = item?.versions[selectedVersion];

  const backHref = generatePath(absoluteRouteMap.children.org.children.catalog.path, {
    orgId: orgId ?? "",
  });

  const versionSelector = versionKeys.length > 0 && (
    <Select
      size="small"
      value={selectedVersion}
      onChange={(e: SelectChangeEvent<string>) =>
        setSearchParams((prev) => { prev.set("version", e.target.value); return prev; })
      }
      sx={{ minWidth: 120 }}
    >
      {versionKeys.map((key) => (
        <MenuItem key={key} value={key}>
          v{key}
        </MenuItem>
      ))}
    </Select>
  );

  return (
    <PageLayout
      title={item?.title ?? "Agent Kind Details"}
      description={
        item && versionData
          ? `Released on ${new Date(versionData.releaseDate).toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" })}`
          : "View details of this agent kind."
      }
      backHref={backHref}
      backLabel="Back to Agent Catalog"
      actions={[versionSelector,
        <Button key="edit" variant="contained" startIcon={<Plus />} color="primary">
          Add "{item?.title}" Agent
        </Button>]}
      disableIcon
    >
      {!item && (
        <Typography color="error">Agent kind &quot;{kindId}&quot; was not found.</Typography>
      )}

      {item && versionData && (
        <Stack spacing={3}>
          {/* Tags */}
          <Stack direction="row" spacing={1} flexWrap="wrap">
            {item.tags.map((tag) => (
              <Chip key={tag} label={tag} size="small" />
            ))}
          </Stack>

          {/* Description */}
          <Box>
            <Typography variant="overline" color="text.secondary">
              Description
            </Typography>
            <Typography variant="body1">{item.description}</Typography>
          </Box>

          <Divider />

          {/* Runtime Configuration */}
          <Stack spacing={1.5}>
            <Typography variant="overline" color="text.secondary">
              Runtime Configuration
            </Typography>
            {versionData.runtimeConfig && Object.keys(versionData.runtimeConfig).length > 0 ? (
              <ListingTable.Container>
                <ListingTable>
                  <ListingTable.Head>
                    <ListingTable.Row>
                      <ListingTable.Cell width="40%">Key</ListingTable.Cell>
                      <ListingTable.Cell width="30%">Type</ListingTable.Cell>
                      <ListingTable.Cell width="30%">Secret</ListingTable.Cell>
                    </ListingTable.Row>
                  </ListingTable.Head>
                  <ListingTable.Body>
                    {Object.entries(versionData.runtimeConfig).map(([key, config]) => (
                      <ListingTable.Row key={key}>
                        <ListingTable.Cell>
                          <Typography variant="body2" fontWeight={500}>{key}</Typography>
                        </ListingTable.Cell>
                        <ListingTable.Cell>
                          <Typography variant="body2" color="text.secondary">
                            {typeof config.type === "boolean" ? "boolean" : typeof config.type === "number" ? "number" : "string"}
                          </Typography>
                        </ListingTable.Cell>
                        <ListingTable.Cell>
                          <Typography variant="body2" color="text.secondary">
                            {config.isSecrete ? "Yes" : "No"}
                          </Typography>
                        </ListingTable.Cell>
                      </ListingTable.Row>
                    ))}
                  </ListingTable.Body>
                </ListingTable>
              </ListingTable.Container>
            ) : (
              <Alert severity="info">No runtime config keys available for this version.</Alert>
            )}
          </Stack>

          <Divider />

          {/* API Specification */}
          <Stack spacing={1.5}>
            <Typography variant="overline" color="text.secondary">
              API Specification
            </Typography>
            {versionData.apiSpecs ? (
              <SwaggerSpecViewer
                spec={versionData.apiSpecs as Record<string, unknown>}
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
      )}
    </PageLayout>
  );
};

export default CatalogKindDetails;
