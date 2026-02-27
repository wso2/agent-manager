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

import React from 'react';
import {
  Card,
  CardContent,
  Chip,
  Divider,
  FormControl,
  FormLabel,
  Grid,
  IconButton,
  InputAdornment,
  MenuItem,
  Select,
  Stack,
  TextField,
  Typography,
  Box,
  CircularProgress,
} from '@wso2/oxygen-ui';
import { Eye, EyeOff } from '@wso2/oxygen-ui-icons-react';
import type { ProviderTemplate } from '../../../../../utils/types';
import type { FormState } from './serviceProviderTypes';
import { FormattedMessage } from 'react-intl';

type ProviderTemplateFormFieldsProps = {
  formState: FormState;
  setFormState: React.Dispatch<React.SetStateAction<FormState>>;
  showCredential: boolean;
  setShowCredential: React.Dispatch<React.SetStateAction<boolean>>;
  template?: ProviderTemplate | null;
  isLoading: boolean;
  error: Error | null;
};

const VERSION_PATTERN = /^v\d+\.\d+$/;

export default function ProviderTemplateFormFields({
  formState,
  setFormState,
  showCredential,
  setShowCredential,
  template,
  isLoading,
  error,
}: ProviderTemplateFormFieldsProps) {
  const [versionTouched, setVersionTouched] = React.useState(false);

  if (isLoading) {
    return (
      <Grid size={{ xs: 12 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, py: 2 }}>
          <CircularProgress size={20} />
          <Typography variant="body2" color="text.secondary">
            <FormattedMessage
              id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.AddNewProvider.ProviderTemplateFormFields.loading.template.details"
              defaultMessage={'Loading template details...'}
            />
          </Typography>
        </Box>
      </Grid>
    );
  }

  if (error) {
    return (
      <Grid size={{ xs: 12 }}>
        <Typography variant="body2" color="error">
          <FormattedMessage
            id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.AddNewProvider.ProviderTemplateFormFields.failed.to.load.template"
            defaultMessage={'Failed to load template:'}
          />{' '}
          {error.message}
        </Typography>
      </Grid>
    );
  }

  const hasTemplateUrl = Boolean(template?.metadata?.endpointUrl);
  const hasTemplateAuthType = Boolean(template?.metadata?.auth?.type);
  const hasTemplateAuthHeader = Boolean(template?.metadata?.auth?.header);
  const trimmedVersion = formState.version.trim();
  const versionErrorMessage = !versionTouched
    ? ''
    : !trimmedVersion
      ? 'Version is required.'
      : !VERSION_PATTERN.test(trimmedVersion)
        ? 'Version must match v<major>.<minor> (e.g., v1.0).'
        : '';

  return (
    <>
      {/* Row 2: Name + Version */}
      <Grid size={{ xs: 12, md: 8 }}>
        <FormControl fullWidth>
          <FormLabel required>
            <FormattedMessage
              id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.AddNewProvider.ProviderTemplateFormFields.name"
              defaultMessage={'Name'}
            />
          </FormLabel>
          <TextField
            fullWidth
            value={formState.name}
            onChange={(e) =>
              setFormState((prev) => ({ ...prev, name: e.target.value }))
            }
            placeholder={`WSO2 ${template?.name || ''} Provider`}
          />
        </FormControl>
      </Grid>

      <Grid size={{ xs: 12, md: 4 }}>
        <FormControl fullWidth>
          <FormLabel required>
            <FormattedMessage
              id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.AddNewProvider.ProviderTemplateFormFields.version"
              defaultMessage={'Version'}
            />
          </FormLabel>
          <TextField
            fullWidth
            value={formState.version}
            onChange={(e) =>
              setFormState((prev) => ({ ...prev, version: e.target.value }))
            }
            onBlur={() => setVersionTouched(true)}
            placeholder="v1.0"
            error={Boolean(versionErrorMessage)}
            helperText={versionErrorMessage || undefined}
          />
        </FormControl>
      </Grid>

      {/* Row 4: Description */}
      <Grid size={{ xs: 12 }}>
        <FormControl fullWidth>
          <FormLabel>
            <FormattedMessage
              id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.AddNewProvider.ProviderTemplateFormFields.description"
              defaultMessage={'Description'}
            />
          </FormLabel>
          <TextField
            fullWidth
            multiline
            minRows={2}
            value={formState.description}
            onChange={(e) =>
              setFormState((prev) => ({ ...prev, description: e.target.value }))
            }
            placeholder={`Primary ${template?.name || ''} provider`}
          />
        </FormControl>
      </Grid>

      {/* Row 5: Context */}
      <Grid size={{ xs: 12 }}>
        <FormControl fullWidth>
          <FormLabel>
            <FormattedMessage
              id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.AddNewProvider.ProviderTemplateFormFields.context"
              defaultMessage={'Context'}
            />
          </FormLabel>
          <TextField
            fullWidth
            value={formState.context}
            onChange={(e) =>
              setFormState((prev) => ({ ...prev, context: e.target.value }))
            }
            placeholder="/"
          />
        </FormControl>
      </Grid>

      {/* Upstream URL - only show if not provided by template */}
      {!hasTemplateUrl && (
        <Grid size={{ xs: 12 }}>
          <FormControl fullWidth>
            <FormLabel required>
              <FormattedMessage
                id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.AddNewProvider.ProviderTemplateFormFields.upstream.url"
                defaultMessage={'Upstream URL'}
              />
            </FormLabel>
            <TextField
              fullWidth
              value={formState.upstreamUrl}
              onChange={(e) =>
                setFormState((prev) => ({
                  ...prev,
                  upstreamUrl: e.target.value,
                }))
              }
              placeholder="https://api.openai.com/v1"
              helperText="The base URL of the upstream LLM provider"
            />
          </FormControl>
        </Grid>
      )}

      {/* Auth Type - only show if not provided by template */}
      {!hasTemplateAuthType && (
        <Grid size={{ xs: 12, md: 6 }}>
          <FormControl fullWidth>
            <FormLabel required>
              <FormattedMessage
                id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.AddNewProvider.ProviderTemplateFormFields.authentication.type"
                defaultMessage={'Authentication Type'}
              />
            </FormLabel>
            <Select
              value={formState.upstreamAuthType}
              onChange={(e) =>
                setFormState((prev) => ({
                  ...prev,
                  upstreamAuthType: e.target.value,
                }))
              }
            >
              {['api-key'].map((type) => (
                <MenuItem key={type} value={type}>
                  {type}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Grid>
      )}

      {/* Auth Header - only show if not provided by template */}
      {!hasTemplateAuthHeader && (
        <Grid size={{ xs: 12, md: 6 }}>
          <FormControl fullWidth>
            <FormLabel>
              <FormattedMessage
                id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.AddNewProvider.ProviderTemplateFormFields.auth.header.name"
                defaultMessage={'Auth Header Name'}
              />
            </FormLabel>
            <TextField
              fullWidth
              value={formState.upstreamAuthHeader}
              onChange={(e) =>
                setFormState((prev) => ({
                  ...prev,
                  upstreamAuthHeader: e.target.value,
                }))
              }
              placeholder="Authorization"
            />
          </FormControl>
        </Grid>
      )}

      {/* Auth Value - always show, user must provide their API key */}
      <Grid size={{ xs: 12 }}>
        <FormControl fullWidth>
          <FormLabel required>
            <FormattedMessage
              id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.AddNewProvider.ProviderTemplateFormFields.api.key"
              defaultMessage={'API Key'}
            />
          </FormLabel>
          <TextField
            fullWidth
            value={formState.upstreamAuthValue}
            onChange={(e) =>
              setFormState((prev) => ({
                ...prev,
                upstreamAuthValue: e.target.value,
              }))
            }
            type={showCredential ? 'text' : 'password'}
            slotProps={{
              input: {
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      size="small"
                      onClick={() => setShowCredential((prev) => !prev)}
                      aria-label={
                        showCredential ? 'Hide credentials' : 'Show credentials'
                      }
                    >
                      {showCredential ? (
                        <EyeOff size={18} />
                      ) : (
                        <Eye size={18} />
                      )}
                    </IconButton>
                  </InputAdornment>
                ),
              },
            }}
            placeholder="Enter your API key or token"
            // helperText={
            //   template?.metadata?.auth?.valuePrefix
            //     ? `Will be prefixed with: ${template.metadata.auth.valuePrefix}`
            //     : 'Your authentication credential for the upstream provider'
            // }
          />
        </FormControl>
      </Grid>

      {/* Show what values are being used from template */}
      {/* {(hasTemplateUrl || hasTemplateAuthType || hasTemplateAuthHeader) && (
        <Grid size={{ xs: 12 }}>
          <Card variant="outlined" sx={{ backgroundColor: 'action.hover' }}>
            <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ display: 'block', mb: 1 }}
              >
                Values from template:
              </Typography>
              <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                {hasTemplateUrl && (
                  <Chip
                    size="small"
                    label={`URL: ${template?.metadata?.endpointUrl}`}
                    variant="outlined"
                  />
                )}
                {hasTemplateAuthType && (
                  <Chip
                    size="small"
                    label={`Auth: ${template?.metadata?.auth?.type}`}
                    variant="outlined"
                  />
                )}
                {hasTemplateAuthHeader && (
                  <Chip
                    size="small"
                    label={`Header: ${template?.metadata?.auth?.header}`}
                    variant="outlined"
                  />
                )}
              </Stack>
            </CardContent>
          </Card>
        </Grid>
      )} */}
    </>
  );
}
