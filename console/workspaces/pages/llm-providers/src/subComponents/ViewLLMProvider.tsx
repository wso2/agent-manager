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

import React, { useMemo, useState } from "react";
import {
  resolveProviderStatusColor,
  resolveProviderStatusIcon,
  resolveProviderStatusLabel,
} from "../utils/providerStatus";
import {
  useGetLLMProvider,
  useListLLMDeployments,
  useListLLMProviderTemplates,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { PageLayout } from "@agent-management-platform/views";
import {
  Box,
  Card,
  Chip,
  CircularProgress,
  Divider,
  Stack,
  Tab,
  Tabs,
  Typography,
} from "@wso2/oxygen-ui";
import { ServerCog } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useParams } from "react-router-dom";
import { LLMProviderAccessControlTab } from "./LLMProviderAccessControlTab";
import { LLMProviderConnectionTab } from "./LLMProviderConnectionTab";
import { LLMProviderOverviewTab } from "./LLMProviderOverviewTab";

const TABS = [
  "Overview",
  "Connection",
  "Access Control",
  "Models",
  "Deployments",
] as const;

type TabPanelProps = {
  value: number;
  index: number;
  children: React.ReactNode;
};

function TabPanel({ value, index, children }: TabPanelProps) {
  return (
    <Box role="tabpanel" hidden={value !== index} sx={{ pt: 2 }}>
      {value === index ? children : null}
    </Box>
  );
}

export const ViewLLMProvider: React.FC = () => {
  const [tabIndex, setTabIndex] = useState(0);

  const { providerId, orgId } = useParams<{
    providerId: string;
    orgId: string;
  }>();

  const { data: providerData, isLoading } = useGetLLMProvider({
    orgName: orgId,
    providerId,
  });

  const { data: templatesData } = useListLLMProviderTemplates({
    orgName: orgId,
  });

  const { data: deploymentsData, isLoading: isDeploymentsLoading } =
    useListLLMDeployments({ orgName: orgId, providerId });

  const templateLogoUrl = useMemo(() => {
    const handle = providerData?.template;
    if (!handle || !templatesData?.templates) return undefined;
    const tpl = templatesData.templates.find(
      (t) => t.name === handle || t.id === handle,
    );
    return tpl?.metadata?.logoUrl;
  }, [providerData?.template, templatesData?.templates]);

  const templateDisplayName = useMemo(() => {
    const handle = providerData?.template;
    if (!handle || !templatesData?.templates) return handle ?? "";
    const tpl = templatesData.templates.find(
      (t) => t.name === handle || t.id === handle,
    );
    return tpl?.name ?? handle;
  }, [providerData?.template, templatesData?.templates]);

  const openapiSpecUrl = useMemo(() => {
    const handle = providerData?.template;
    if (!handle || !templatesData?.templates) return undefined;
    const tpl = templatesData.templates.find(
      (t) => t.name === handle || t.id === handle,
    );
    return tpl?.metadata?.openapiSpecUrl;
  }, [providerData?.template, templatesData?.templates]);

  const authValuePrefix = useMemo(() => {
    const handle = providerData?.template;
    if (!handle || !templatesData?.templates) return "";
    const tpl = templatesData.templates.find(
      (t) => t.name === handle || t.id === handle,
    );
    return tpl?.metadata?.auth?.valuePrefix ?? "";
  }, [providerData?.template, templatesData?.templates]);

  const providerName = providerData?.name ?? providerId ?? "";
  const version = providerData?.version;
  const description = providerData?.description?.trim();

  const models = useMemo(
    () =>
      (providerData?.modelProviders ?? []).flatMap((mp) =>
        (mp.models ?? []).map((model) => ({
          model,
          groupName: mp.name ?? mp.id,
        })),
      ),
    [providerData?.modelProviders],
  );

  const deployments = deploymentsData ?? [];

  return (
    <PageLayout
      title={providerName}
      description={description}
      backHref={generatePath(
        absoluteRouteMap.children.org.children.llmProviders.path,
        { orgId: orgId ?? "" },
      )}
      backLabel="Back to LLM Providers"
      isLoading={isLoading}
      disableIcon
      titleTail={
        <Stack direction="row" spacing={1} alignItems="center" sx={{ ml: 1 }}>
          {templateDisplayName && (
            <Chip
              label={templateDisplayName}
              icon={
                <Box
                  component="img"
                  src={templateLogoUrl}
                  sx={{
                    width: 14,
                    height: 14,
                  }}
                />
              }
              size="small"
            />
          )}
          {version && <Chip label={version} size="small" variant="outlined" />}
          {providerData?.status && (
            <Chip
              label={resolveProviderStatusLabel(providerData.status)}
              size="small"
              color={resolveProviderStatusColor(providerData.status)}
              icon={resolveProviderStatusIcon(providerData.status)}
            />
          )}
        </Stack>
      }
    >
      <Stack spacing={3}>
        {/* Tabbed content card */}
        <Card variant="outlined">
          <Tabs
            value={tabIndex}
            onChange={(_, v: number) => setTabIndex(v)}
            variant="scrollable"
            allowScrollButtonsMobile
          >
            {TABS.map((label) => (
              <Tab key={label} label={label} />
            ))}
          </Tabs>
          <Divider />

          <Box sx={{ px: 3, pb: 3 }}>
            {/* Overview tab */}
            <TabPanel value={tabIndex} index={0}>
              <LLMProviderOverviewTab
                providerData={providerData}
                openapiSpecUrl={openapiSpecUrl}
                isLoading={isLoading}
              />
            </TabPanel>

            {/* Connection tab */}
            <TabPanel value={tabIndex} index={1}>
              <LLMProviderConnectionTab
                providerData={providerData}
                valuePrefix={authValuePrefix}
                isLoading={isLoading}
              />
            </TabPanel>

            {/* Access Control tab */}
            <TabPanel value={tabIndex} index={2}>
              <LLMProviderAccessControlTab
                providerData={providerData}
                openapiSpecUrl={openapiSpecUrl}
                isLoading={isLoading}
              />
            </TabPanel>

            {/* Models tab */}
            <TabPanel value={tabIndex} index={3}>
              {models.length > 0 ? (
                <Box
                  sx={{
                    maxHeight: 320,
                    overflowY: "auto",
                    p: 1.5,
                    border: "1px solid",
                    borderColor: "divider",
                    borderRadius: 1,
                    bgcolor: "background.paper",
                  }}
                >
                  <Stack
                    direction="row"
                    spacing={1}
                    sx={{ flexWrap: "wrap", gap: 1 }}
                  >
                    {models.map(({ model, groupName }) => (
                      <Box
                        key={`${groupName}:${model.id}`}
                        sx={{
                          border: "1px solid",
                          borderColor: "divider",
                          borderRadius: 0.5,
                          px: 1.25,
                          py: 0.75,
                          display: "inline-flex",
                          alignItems: "center",
                          bgcolor: "background.paper",
                        }}
                      >
                        <Typography variant="body2" color="primary.main">
                          {model.name ?? model.id}
                        </Typography>
                      </Box>
                    ))}
                  </Stack>
                </Box>
              ) : (
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    minHeight: 120,
                    border: "1px dashed",
                    borderColor: "divider",
                    borderRadius: 1,
                    bgcolor: "background.paper",
                  }}
                >
                  <Typography variant="body2" color="text.secondary">
                    No models configured
                  </Typography>
                </Box>
              )}
            </TabPanel>

            {/* Deployments tab */}
            <TabPanel value={tabIndex} index={4}>
              {isDeploymentsLoading ? (
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <CircularProgress size={16} />
                  <Typography variant="caption" color="text.secondary">
                    Loading deployments...
                  </Typography>
                </Box>
              ) : deployments.length > 0 ? (
                <Stack spacing={1.5}>
                  {deployments.map((dep, index) => (
                    <Box
                      key={`${dep.environment}-${dep.imageId}-${index}`}
                      sx={{
                        p: 2,
                        border: "1px solid",
                        borderColor: "divider",
                        borderRadius: 1,
                        bgcolor: "background.paper",
                      }}
                    >
                      <Stack
                        direction="row"
                        justifyContent="space-between"
                        alignItems="center"
                      >
                        <Stack spacing={0.25}>
                          <Stack
                            direction="row"
                            spacing={1}
                            alignItems="center"
                          >
                            <ServerCog size={16} />
                            <Typography
                              variant="body2"
                              sx={{ fontWeight: 500 }}
                            >
                              {dep.environment}
                            </Typography>
                          </Stack>
                          <Typography
                            variant="caption"
                            color="text.secondary"
                            sx={{ fontFamily: "monospace" }}
                          >
                            {dep.imageId}
                          </Typography>
                        </Stack>
                        <Chip
                          label={dep.projectName}
                          size="small"
                          variant="outlined"
                        />
                      </Stack>
                    </Box>
                  ))}
                </Stack>
              ) : (
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    minHeight: 120,
                    border: "1px dashed",
                    borderColor: "divider",
                    borderRadius: 1,
                    bgcolor: "background.paper",
                  }}
                >
                  <Typography variant="body2" color="text.secondary">
                    No deployments found
                  </Typography>
                </Box>
              )}
            </TabPanel>
          </Box>
        </Card>
      </Stack>
    </PageLayout>
  );
};

export default ViewLLMProvider;
