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

import React, { useMemo, useState } from 'react';
import {
  useGetLLMProvider,
  useListLLMDeployments,
  useListLLMProviderTemplates,
} from '@agent-management-platform/api-client';
import { absoluteRouteMap } from '@agent-management-platform/types';
import { PageLayout } from '@agent-management-platform/views';
import {
  Avatar,
  Box,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Divider,
  Grid,
  Stack,
  Tab,
  Tabs,
  Typography,
} from '@wso2/oxygen-ui';
import { Clock, ServerCog } from '@wso2/oxygen-ui-icons-react';
import { generatePath, useParams } from 'react-router-dom';

function getInitials(name: string): string {
  const words = name.trim().split(/\s+/);
  if (words.length === 0) return '?';
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase();
  return `${words[0][0]}${words[1][0]}`.toUpperCase();
}

function formatRelativeTime(dateString: string): string {
  const date = new Date(dateString);
  if (isNaN(date.getTime())) return dateString;
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSecs = Math.floor(diffMs / 1000);
  if (diffSecs < 60) return 'just now';
  const diffMins = Math.floor(diffSecs / 60);
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffHours / 24);
  return `${diffDays}d ago`;
}

type StatusColor = 'success' | 'warning' | 'error' | 'default';

function resolveStatusColor(status: string): StatusColor {
  switch (status?.toLowerCase()) {
    case 'active':
    case 'deployed':
      return 'success';
    case 'pending':
      return 'warning';
    case 'failed':
      return 'error';
    default:
      return 'default';
  }
}

const TABS = ['Overview', 'Models', 'Deployments'] as const;

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

  const { data: templatesData } = useListLLMProviderTemplates({ orgName: orgId });

  const { data: deploymentsData, isLoading: isDeploymentsLoading } =
    useListLLMDeployments({ orgName: orgId, providerId });

  const templateLogoUrl = useMemo(() => {
    const handle = providerData?.templateHandle;
    if (!handle || !templatesData?.templates) return undefined;
    const tpl = templatesData.templates.find(
      (t) => t.name === handle || t.id === handle,
    );
    return (tpl?.metadata as { logoUrl?: string } | undefined)?.logoUrl;
  }, [providerData?.templateHandle, templatesData?.templates]);

  const templateDisplayName = useMemo(() => {
    const handle = providerData?.templateHandle;
    if (!handle || !templatesData?.templates) return handle ?? '';
    const tpl = templatesData.templates.find(
      (t) => t.name === handle || t.id === handle,
    );
    return (
      (tpl?.metadata as { displayName?: string } | undefined)?.displayName ??
      handle
    );
  }, [providerData?.templateHandle, templatesData?.templates]);

  const providerName =
    providerData?.artifact?.displayName ??
    providerData?.configuration?.name ??
    providerId ??
    '';

  const version = providerData?.configuration?.version;
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

  const deployments = deploymentsData?.deployments ?? [];

  return (
    <PageLayout
      title={providerName}
      description={description}
      backHref={generatePath(
        absoluteRouteMap.children.org.children.llmProviders.path,
        { orgId: orgId ?? '' },
      )}
      disableIcon
      backLabel="Back to LLM Providers"
      isLoading={isLoading}
      titleTail={
        <Stack direction="row" spacing={1} alignItems="center" sx={{ ml: 1 }}>
          {version && (
            <Chip
              label={`v${version}`}
              size="small"
              variant="outlined"
              color="primary"
            />
          )}
          {providerData?.status && (
            <Chip
              label={providerData.status}
              size="small"
              color={resolveStatusColor(providerData.status)}
            />
          )}
        </Stack>
      }
    >
      <Stack spacing={3}>
        {/* Provider header card */}
        <Card variant="outlined">
          <CardContent>
            <Box
              sx={{
                display: 'flex',
                alignItems: 'flex-start',
                gap: 2,
                flexWrap: 'wrap',
              }}
            >
              <Avatar
                src={templateLogoUrl}
                sx={{
                  width: 64,
                  height: 64,
                  fontWeight: 600,
                  fontSize: 22,
                  bgcolor: templateLogoUrl ? 'common.white' : 'primary.light',
                  color: templateLogoUrl
                    ? 'text.primary'
                    : 'primary.contrastText',
                  border: templateLogoUrl ? '1px solid' : 'none',
                  borderColor: 'divider',
                  p: templateLogoUrl ? 0.5 : 0,
                  '& img': { objectFit: 'contain' },
                }}
              >
                {!templateLogoUrl ? getInitials(providerName) : null}
              </Avatar>

              <Stack spacing={0.75} flex={1} sx={{ minWidth: 0 }}>
                <Stack
                  direction="row"
                  spacing={1}
                  alignItems="center"
                  flexWrap="wrap"
                >
                  {templateDisplayName && (
                    <Chip
                      label={templateDisplayName}
                      size="small"
                      variant="outlined"
                      color="primary"
                      icon={
                        templateLogoUrl ? (
                          <Avatar
                            src={templateLogoUrl}
                            sx={{
                              width: 14,
                              height: 14,
                              '& img': { objectFit: 'contain' },
                            }}
                          />
                        ) : undefined
                      }
                      sx={{ borderRadius: 0.5 }}
                    />
                  )}
                  {version && (
                    <Chip
                      label={`v${version}`}
                      size="small"
                      variant="outlined"
                    />
                  )}
                  {providerData?.status && (
                    <Chip
                      label={providerData.status}
                      size="small"
                      color={resolveStatusColor(providerData.status)}
                    />
                  )}
                </Stack>

                <Typography variant="h5" sx={{ fontWeight: 600 }}>
                  {providerName}
                </Typography>

                {description && (
                  <Typography variant="body2" color="text.secondary">
                    {description.length > 200
                      ? `${description.slice(0, 200).trim()}…`
                      : description}
                  </Typography>
                )}

                {providerData?.createdBy && (
                  <Stack direction="row" spacing={0.5} alignItems="center">
                    <Typography variant="caption" color="text.secondary">
                      Created by
                    </Typography>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ fontWeight: 500 }}
                    >
                      {providerData.createdBy}
                    </Typography>
                  </Stack>
                )}
              </Stack>
            </Box>
          </CardContent>
        </Card>

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
              {providerData ? (
                <Grid container spacing={3}>
                  {providerData.configuration?.context && (
                    <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                      <Stack spacing={0.5}>
                        <Typography
                          variant="caption"
                          color="text.secondary"
                          sx={{ fontWeight: 500 }}
                        >
                          Context
                        </Typography>
                        <Typography
                          variant="body2"
                          sx={{ fontFamily: 'monospace' }}
                        >
                          {providerData.configuration.context}
                        </Typography>
                      </Stack>
                    </Grid>
                  )}
                  {providerData.configuration?.vhost && (
                    <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                      <Stack spacing={0.5}>
                        <Typography
                          variant="caption"
                          color="text.secondary"
                          sx={{ fontWeight: 500 }}
                        >
                          Virtual Host
                        </Typography>
                        <Typography
                          variant="body2"
                          sx={{ fontFamily: 'monospace' }}
                        >
                          {providerData.configuration.vhost}
                        </Typography>
                      </Stack>
                    </Grid>
                  )}
                  {providerData.configuration?.upstream?.main?.url && (
                    <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                      <Stack spacing={0.5}>
                        <Typography
                          variant="caption"
                          color="text.secondary"
                          sx={{ fontWeight: 500 }}
                        >
                          Upstream URL
                        </Typography>
                        <Typography
                          variant="body2"
                          sx={{
                            fontFamily: 'monospace',
                            wordBreak: 'break-all',
                          }}
                        >
                          {providerData.configuration.upstream.main.url}
                        </Typography>
                      </Stack>
                    </Grid>
                  )}
                  {providerData.configuration?.upstream?.main?.auth?.type && (
                    <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                      <Stack spacing={0.5}>
                        <Typography
                          variant="caption"
                          color="text.secondary"
                          sx={{ fontWeight: 500 }}
                        >
                          Auth Type
                        </Typography>
                        <Typography variant="body2">
                          {providerData.configuration.upstream.main.auth.type}
                        </Typography>
                      </Stack>
                    </Grid>
                  )}
                  {providerData.configuration?.accessControl?.mode && (
                    <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                      <Stack spacing={0.5}>
                        <Typography
                          variant="caption"
                          color="text.secondary"
                          sx={{ fontWeight: 500 }}
                        >
                          Access Control
                        </Typography>
                        <Chip
                          label={providerData.configuration.accessControl.mode}
                          size="small"
                          variant="outlined"
                          sx={{ width: 'fit-content', textTransform: 'capitalize' }}
                        />
                      </Stack>
                    </Grid>
                  )}
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                    <Stack spacing={0.5}>
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{ fontWeight: 500 }}
                      >
                        In Catalog
                      </Typography>
                      <Chip
                        label={providerData.inCatalog ? 'Yes' : 'No'}
                        size="small"
                        color={providerData.inCatalog ? 'success' : 'default'}
                        variant="outlined"
                        sx={{ width: 'fit-content' }}
                      />
                    </Stack>
                  </Grid>
                </Grid>
              ) : null}
            </TabPanel>

            {/* Models tab */}
            <TabPanel value={tabIndex} index={1}>
              {models.length > 0 ? (
                <Box
                  sx={{
                    maxHeight: 320,
                    overflowY: 'auto',
                    p: 1.5,
                    border: '1px solid',
                    borderColor: 'divider',
                    borderRadius: 1,
                    bgcolor: 'background.paper',
                  }}
                >
                  <Stack
                    direction="row"
                    spacing={1}
                    sx={{ flexWrap: 'wrap', gap: 1 }}
                  >
                    {models.map(({ model, groupName }) => (
                      <Box
                        key={`${groupName}:${model.id}`}
                        sx={{
                          border: '1px solid',
                          borderColor: 'divider',
                          borderRadius: 0.5,
                          px: 1.25,
                          py: 0.75,
                          display: 'inline-flex',
                          alignItems: 'center',
                          bgcolor: 'background.paper',
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
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    minHeight: 120,
                    border: '1px dashed',
                    borderColor: 'divider',
                    borderRadius: 1,
                    bgcolor: 'background.paper',
                  }}
                >
                  <Typography variant="body2" color="text.secondary">
                    No models configured
                  </Typography>
                </Box>
              )}
            </TabPanel>

            {/* Deployments tab */}
            <TabPanel value={tabIndex} index={2}>
              {isDeploymentsLoading ? (
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <CircularProgress size={16} />
                  <Typography variant="caption" color="text.secondary">
                    Loading deployments...
                  </Typography>
                </Box>
              ) : deployments.length > 0 ? (
                <Stack spacing={1.5}>
                  {deployments.map((dep) => (
                    <Box
                      key={dep.id}
                      sx={{
                        p: 2,
                        border: '1px solid',
                        borderColor: 'divider',
                        borderRadius: 1,
                        bgcolor: 'background.paper',
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
                              {dep.environmentId}
                            </Typography>
                          </Stack>
                          <Stack
                            direction="row"
                            spacing={0.5}
                            alignItems="center"
                          >
                            <Clock size={12} />
                            <Typography
                              variant="caption"
                              color="text.secondary"
                            >
                              {dep.updatedAt
                                ? formatRelativeTime(dep.updatedAt)
                                : '—'}
                            </Typography>
                          </Stack>
                        </Stack>
                        <Chip
                          label={dep.status}
                          size="small"
                          color={resolveStatusColor(dep.status)}
                        />
                      </Stack>
                    </Box>
                  ))}
                </Stack>
              ) : (
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    minHeight: 120,
                    border: '1px dashed',
                    borderColor: 'divider',
                    borderRadius: 1,
                    bgcolor: 'background.paper',
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
