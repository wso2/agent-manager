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
  useGetLLMProvider,
  useListLLMProviderTemplates,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { PageLayout } from "@agent-management-platform/views";
import {
  Box,
  Button,
  Card,
  Chip,
  Divider,
  Stack,
  Tab,
  Tabs,
} from "@wso2/oxygen-ui";
import { Rocket } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useParams } from "react-router-dom";
import { LLMProviderAccessControlTab } from "./LLMProviderAccessControlTab";
import { LLMProviderConnectionTab } from "./LLMProviderConnectionTab";
import { LLMProviderGuardrailsTab } from "./LLMProviderGuardrailsTab";
import { LLMProviderModelsTab } from "./LLMProviderModelsTab";
import { LLMProviderOverviewTab } from "./LLMProviderOverviewTab";
import { LLMProviderRateLimitingTab } from "./LLMProviderRateLimitingTab";
import { LLMProviderSecurityTab } from "./LLMProviderSecurityTab";

const TABS = [
  "Overview",
  "Connection",
  "Access Control",
  "Security",
  "Rate Limiting",
  "Guardrails",
  "Models",
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

  const {
    data: providerData,
    isLoading,
    error: providerError,
  } = useGetLLMProvider({
    orgName: orgId,
    providerId,
  });

  const { data: templatesData } = useListLLMProviderTemplates({
    orgName: orgId,
  });

  const template = useMemo(() => {
    const handle = providerData?.template;
    if (!handle || !templatesData?.templates) return null;
    return (
      templatesData.templates.find(
        (t) => t.name === handle || t.id === handle,
      ) ?? null
    );
  }, [providerData?.template, templatesData?.templates]);

  const templateLogoUrl = template?.metadata?.logoUrl;
  const templateDisplayName = template?.name ?? providerData?.template ?? "";
  const openapiSpecUrl = template?.metadata?.openapiSpecUrl;
  const authValuePrefix = template?.metadata?.auth?.valuePrefix ?? "";

  const providerName = providerData?.name ?? providerId ?? "";
  const version = providerData?.version;
  const description = providerData?.description?.trim();

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
      actions={
        <Button
          component={Link}
          to={generatePath(
            absoluteRouteMap.children.org.children.llmProviders.children.view
              .children.deploy.path,
            { orgId: orgId ?? "", providerId: providerId ?? "" },
          )}
          variant="contained"
          size="small"
          startIcon={<Rocket size={16} />}
        >
          Deploy to Gateway
        </Button>
      }
      titleTail={
        <Stack direction="row" spacing={1} alignItems="center" sx={{ ml: 1 }}>
          {templateDisplayName && (
            <Chip
              label={templateDisplayName}
              icon={
                templateLogoUrl ? (
                  <Box
                    component="img"
                    src={templateLogoUrl}
                    alt={templateDisplayName}
                    sx={{
                      width: 14,
                      height: 14,
                    }}
                  />
                ) : undefined
              }
              size="small"
            />
          )}
          {version && <Chip label={version} size="small" variant="outlined" />}
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
                error={providerError}
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

            {/* Security tab */}
            <TabPanel value={tabIndex} index={3}>
              <LLMProviderSecurityTab
                providerData={providerData}
                isLoading={isLoading}
              />
            </TabPanel>

            {/* Rate Limiting tab */}
            <TabPanel value={tabIndex} index={4}>
              <LLMProviderRateLimitingTab
                providerData={providerData}
                openapiSpecUrl={openapiSpecUrl}
                isLoading={isLoading}
              />
            </TabPanel>

            {/* Guardrails tab */}
            <TabPanel value={tabIndex} index={5}>
              <LLMProviderGuardrailsTab
                providerData={providerData}
                openapiSpecUrl={openapiSpecUrl}
                isLoading={isLoading}
                error={providerError}
              />
            </TabPanel>

            {/* Models tab */}
            <TabPanel value={tabIndex} index={6}>
              <LLMProviderModelsTab
                providerData={providerData}
                isLoading={isLoading}
                error={providerError}
              />
            </TabPanel>
          </Box>
        </Card>
      </Stack>
    </PageLayout>
  );
};

export default ViewLLMProvider;
