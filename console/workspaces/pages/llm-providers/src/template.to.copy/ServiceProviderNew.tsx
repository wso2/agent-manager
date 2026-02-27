/*
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

import React, { useState, useEffect, useRef } from 'react';
import { Link as RouterLink, useNavigate } from 'react-router-dom';
import {
  Box,
  Button,
  Grid,
  PageContent,
  PageTitle,
  Stack,
  Typography,
} from '@wso2/oxygen-ui';
import {
  useLLMProviders,
  useProviderTemplates,
  ProviderTemplateProvider,
  useProviderTemplate,
} from '../../../../contexts/llmProvider';
import { useGuardrails } from '../../../../contexts/GuardrailsContext';
import { ChevronLeft } from '@wso2/oxygen-ui-icons-react';
import { useRole } from '../../../../contexts/RoleContext';
import { useAppShell } from '../../../../contexts/AppShellContext';
import {
  buildOrgPath,
  buildProjectPath,
} from '../../../../utils/projectRouting';
import { logger } from '../../../../utils/logger';
import ProviderTemplateSelector from './AddNewProvider/ProviderTemplateSelector';
import ProviderTemplateFormFields from './AddNewProvider/ProviderTemplateFormFields';
import GuardrailsSection from './AddNewProvider/GuardrailsSection';
import type {
  FormState,
  GuardrailSelection,
} from './AddNewProvider/serviceProviderTypes';
import { FormattedMessage } from 'react-intl';

const VERSION_PATTERN = /^v\d+\.\d+$/;

type TemplateBasedFormFieldsContainerProps = {
  formState: FormState;
  setFormState: React.Dispatch<React.SetStateAction<FormState>>;
  showCredential: boolean;
  setShowCredential: React.Dispatch<React.SetStateAction<boolean>>;
  setOpenapiSpec: React.Dispatch<React.SetStateAction<string>>;
};

function TemplateBasedFormFieldsContainer({
  formState,
  setFormState,
  showCredential,
  setShowCredential,
  setOpenapiSpec,
}: TemplateBasedFormFieldsContainerProps) {
  const { template, isLoading, error } = useProviderTemplate();
  const lastTemplateIdRef = useRef<string | null>(null);

  // Apply template metadata values to form when template loads
  useEffect(() => {
    const currentTemplateId = template?.id ?? null;
    const templateChanged = lastTemplateIdRef.current !== currentTemplateId;
    lastTemplateIdRef.current = currentTemplateId;

    setFormState((prev) => ({
      ...prev,
      upstreamUrl: templateChanged
        ? template?.metadata?.endpointUrl || ''
        : prev.upstreamUrl,
      upstreamAuthType: templateChanged
        ? template?.metadata?.auth?.type || 'api-key'
        : prev.upstreamAuthType,
      upstreamAuthHeader: templateChanged
        ? template?.metadata?.auth?.header || 'Authorization'
        : prev.upstreamAuthHeader,
      upstreamAuthValue: templateChanged ? '' : prev.upstreamAuthValue,
      valuePrefix: template?.metadata?.auth?.valuePrefix || '',
    }));

    // Fetch OpenAPI spec
    if (templateChanged) {
      const specUrl = template?.metadata?.openapiSpecUrl;
      if (specUrl) {
        fetch(specUrl)
          .then((res) => res.text())
          .then((text) => {
            setOpenapiSpec(text);
          })
          .catch((err) => {
            logger.error('Failed to fetch OpenAPI spec:', err);
            setOpenapiSpec('');
          });
      } else {
        setOpenapiSpec('');
      }
    }
  }, [template, setFormState, setOpenapiSpec]);

  return (
    <ProviderTemplateFormFields
      formState={formState}
      setFormState={setFormState}
      showCredential={showCredential}
      setShowCredential={setShowCredential}
      template={template}
      isLoading={isLoading}
      error={error}
    />
  );
}

export default function ServiceProviderNew() {
  const navigate = useNavigate();
  const { role } = useRole();
  const { createProvider } = useLLMProviders();
  const {
    templatesResponse,
    isLoading: templatesLoading,
    error: templatesError,
  } = useProviderTemplates();
  const { currentProject, currentOrganization, setCurrentProject } =
    useAppShell();
  const isProjectLevel = Boolean(currentProject?.id);
  const providersPath = isProjectLevel
    ? buildProjectPath(currentOrganization, currentProject, '/service-provider')
    : buildOrgPath(currentOrganization, '/service-provider');

  const [selectedTemplateId, setSelectedTemplateId] = useState<string | null>(
    null
  );
  const [openapiSpec, setOpenapiSpec] = useState<string>('');

  const [formState, setFormState] = useState<FormState>({
    name: '',
    description: '',
    version: 'v1.0',
    context: '/',
    providerType: '',
    upstreamUrl: '',
    upstreamAuthType: 'api-key',
    upstreamAuthHeader: 'Authorization',
    upstreamAuthValue: '',
    valuePrefix: '',
  });
  const isVersionValid = VERSION_PATTERN.test(formState.version.trim());
  const [showCredential, setShowCredential] = useState(false);
  const [guardrails, setGuardrails] = useState<GuardrailSelection[]>([]);
  const [guardrailDrawerOpen, setGuardrailDrawerOpen] = useState(false);
  const [selectedGuardrail, setSelectedGuardrail] = useState<string | null>(
    null
  );
  const [guardrailSettings, setGuardrailSettings] = useState<
    Record<string, unknown>
  >({});

  const { guardrails: availableGuardrails = [] } = useGuardrails();

  useEffect(() => {
    if (isProjectLevel && role === 'admin') {
      setCurrentProject(null);
      navigate(buildOrgPath(currentOrganization, '/service-provider/new'), {
        replace: true,
      });
    }
  }, [isProjectLevel, role, currentOrganization, navigate, setCurrentProject]);

  if (role === 'developer') {
    return (
      <PageContent fullWidth>
        <Stack spacing={1}>
          <Typography variant="h6">
            <FormattedMessage
              id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.ServiceProviderNew.service.provider.creation.is.unavailable"
              defaultMessage={'Service provider creation is unavailable.'}
            />
          </Typography>
          <Typography variant="body2" color="text.secondary">
            <FormattedMessage
              id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.ServiceProviderNew.switch.to.the.admin.role.to.add.new.service.providers"
              defaultMessage={
                'Switch to the admin role to add new service providers.'
              }
            />
          </Typography>
          {/* <Button component={RouterLink} to={providersPath}>
            Back to list
          </Button> */}
        </Stack>
      </PageContent>
    );
  }

  const selectedGuardrailPolicy = availableGuardrails.find(
    (policy) => policy.name === selectedGuardrail
  );

  const handleOpenGuardrailDrawer = () => {
    setSelectedGuardrail(null);
    setGuardrailSettings({});
    setGuardrailDrawerOpen(true);
  };

  const handleSelectGuardrail = (guardrailName: string) => {
    setSelectedGuardrail(guardrailName);
    const existing = guardrails.find((item) => item.name === guardrailName);
    setGuardrailSettings(existing?.settings ?? {});
  };

  const handleAddGuardrail = (settings: Record<string, unknown>) => {
    if (!selectedGuardrail || !selectedGuardrailPolicy) return;
    setGuardrailSettings(settings);
    setGuardrails((prev) => {
      const existingIndex = prev.findIndex(
        (item) => item.name === selectedGuardrail
      );
      const configurationSummary = Object.entries(settings)
        .filter(([, value]) => Boolean(value))
        .map(([key, value]) => {
          if (
            typeof value === 'string' ||
            typeof value === 'number' ||
            typeof value === 'boolean'
          ) {
            return `${key}: ${value}`;
          }
          if (value === null || value === undefined) {
            return `${key}:`;
          }
          return `${key}: ${JSON.stringify(value)}`;
        })
        .join(', ');
      if (existingIndex === -1) {
        return [
          ...prev,
          {
            name: selectedGuardrail,
            version: selectedGuardrailPolicy.version || '1.0.0',
            configuration: configurationSummary,
            settings,
          },
        ];
      }
      const next = [...prev];
      next[existingIndex] = {
        name: selectedGuardrail,
        version: selectedGuardrailPolicy.version || '1.0.0',
        configuration: configurationSummary,
        settings,
      };
      return next;
    });
  };

  const isFormValid = Boolean(
    formState.name.trim() &&
      formState.version.trim() &&
      isVersionValid &&
      selectedTemplateId &&
      formState.upstreamUrl.trim() &&
      formState.upstreamAuthType &&
      formState.upstreamAuthValue.trim()
  );

  const toProviderId = (name: string): string =>
    name
      .toLowerCase()
      .trim()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '');

  const handleCreateProvider = async () => {
    if (!isFormValid || !selectedTemplateId) return;

    try {
      const selectedTemplate = templatesResponse?.list?.find(
        (t) => t.id === selectedTemplateId
      );
      const providerId = toProviderId(formState.name);

      const upstream = {
        main: {
          url: selectedTemplate?.metadata?.endpointUrl || formState.upstreamUrl,
          ref: '',
          auth: {
            type:
              selectedTemplate?.metadata?.auth?.type ||
              formState.upstreamAuthType,
            header:
              selectedTemplate?.metadata?.auth?.header ||
              formState.upstreamAuthHeader,
            value: formState.valuePrefix
              ? `${formState.valuePrefix}${formState.upstreamAuthValue}`
              : formState.upstreamAuthValue,
          },
        },
      };

      const payload = {
        id: providerId,
        name: formState.name.trim(),
        description: formState.description.trim(),
        version: formState.version.trim(),
        context: formState.context.trim() || '/',
        template: selectedTemplateId,
        openapi: openapiSpec,
        upstream,
        policies: guardrails.map((guardrail) => ({
          name: guardrail.name,
          version: guardrail.version,
          paths: [
            {
              path: '/*',
              methods: ['*'],
              params: guardrail.settings ?? {},
            },
          ],
        })),
        accessControl: {
          mode: 'allow_all' as const,
          exceptions: [],
        },
      };

      const createdProvider = await createProvider(payload);

      const createdProviderId = createdProvider.id ?? providerId;
      navigate(
        isProjectLevel
          ? buildProjectPath(
              currentOrganization,
              currentProject,
              `/service-provider/${createdProviderId}`
            )
          : buildOrgPath(
              currentOrganization,
              `/service-provider/${createdProviderId}`
            ),
        {
          state: { providerAdded: true },
        }
      );
    } catch (error) {
      logger.error('Failed to create LLM provider:', error);
    }
  };

  return (
    <PageContent fullWidth>
      <Button
        component={RouterLink}
        to={providersPath}
        size="small"
        startIcon={<ChevronLeft size={24} />}
      >
        <FormattedMessage
          id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.ServiceProviderNew.back.to.list"
          defaultMessage={'Back to list'}
        />
      </Button>

      <Stack spacing={2} mt={2}>
        <PageTitle>
          <PageTitle.Header>
            <FormattedMessage
              id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.ServiceProviderNew.add.llm.service.provider"
              defaultMessage={'Add LLM Service Provider'}
            />
          </PageTitle.Header>
        </PageTitle>
      </Stack>

      <Box sx={{ mt: 2, maxWidth: 820 }}>
        {/* MAIN FORM */}
        <Grid container spacing={2}>
          <ProviderTemplateSelector
            templatesLoading={templatesLoading}
            templatesError={templatesError}
            templatesResponse={templatesResponse}
            selectedTemplateId={selectedTemplateId}
            onSelectTemplate={(template) => {
              setSelectedTemplateId(template.id ?? null);
              setFormState((prev) => ({
                ...prev,
                providerType: template.name,
              }));
              setOpenapiSpec('');
            }}
          />

          {selectedTemplateId && (
            <ProviderTemplateProvider templateId={selectedTemplateId}>
              <TemplateBasedFormFieldsContainer
                formState={formState}
                setFormState={setFormState}
                showCredential={showCredential}
                setShowCredential={setShowCredential}
                setOpenapiSpec={setOpenapiSpec}
              />
            </ProviderTemplateProvider>
          )}

          {selectedTemplateId && (
            <GuardrailsSection
              guardrails={guardrails}
              selectedGuardrail={selectedGuardrail}
              guardrailSettings={guardrailSettings}
              guardrailDrawerOpen={guardrailDrawerOpen}
              onOpenDrawer={handleOpenGuardrailDrawer}
              onCloseDrawer={() => setGuardrailDrawerOpen(false)}
              onSelectGuardrail={handleSelectGuardrail}
              onAddGuardrail={handleAddGuardrail}
            />
          )}
        </Grid>
        {selectedTemplateId && (
          <Box
            sx={{
              mt: 3,
              display: 'flex',
              justifyContent: 'flex-start',
              gap: 1,
            }}
          >
            <Button
              variant="contained"
              component={RouterLink}
              to={providersPath}
              color="secondary"
            >
              <FormattedMessage
                id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.ServiceProviderNew.cancel"
                defaultMessage={'Cancel'}
              />
            </Button>
            <Button
              variant="contained"
              onClick={handleCreateProvider}
              disabled={!isFormValid}
            >
              <FormattedMessage
                id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.ServiceProviderNew.add.provider"
                defaultMessage={'Add Provider'}
              />
            </Button>
          </Box>
        )}
      </Box>
    </PageContent>
  );
}
