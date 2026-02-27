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
  Avatar,
  Box,
  Chip,
  CircularProgress,
  FormControl,
  FormLabel,
  Grid,
  Stack,
  Typography,
} from '@wso2/oxygen-ui';
import type {
  ProviderTemplate,
  ProviderTemplatesResponse,
} from '../../../../../utils/types';
import awsBedrockLogo from '../../../../../assets/brands/AWSBedrock.webp';
import openAiLogo from '../../../../../assets/brands/openAI.png';
import anthropicLogo from '../../../../../assets/brands/Anthropic.jpg';
import azureLogo from '../../../../../assets/brands/Azure.png';
import googleVertexLogo from '../../../../../assets/brands/GoogleVertex.png';
import googleGeminiLogo from '../../../../../assets/brands/googlegemini.png';
import mistralAiLogo from '../../../../../assets/brands/mistralai.png';
import { FormattedMessage } from 'react-intl';

// Logo mapping for provider templates by name (case-insensitive partial match)
const getLogoForTemplate = (templateName: string): string | null => {
  const lowerName = templateName.toLowerCase();
  if (lowerName.includes('bedrock') || lowerName.includes('aws'))
    return awsBedrockLogo;
  if (lowerName.includes('openai') && !lowerName.includes('azure'))
    return openAiLogo;
  if (lowerName.includes('anthropic') || lowerName.includes('claude'))
    return anthropicLogo;
  if (lowerName.includes('mistral')) return mistralAiLogo;
  if (lowerName.includes('azure')) return azureLogo;
  if (lowerName.includes('gemini')) return googleGeminiLogo;
  if (lowerName.includes('google') || lowerName.includes('vertex'))
    return googleVertexLogo;
  return null;
};

const getShortNameForTemplate = (templateName: string): string => {
  const words = templateName.split(/[\s-_]+/);
  if (words.length >= 2) {
    return (words[0][0] + words[1][0]).toUpperCase();
  }
  return templateName.substring(0, 2).toUpperCase();
};

const COMING_SOON_TEMPLATE_IDS = new Set(['awsbedrock', 'aws-bedrock']);

type ProviderTemplateSelectorProps = {
  templatesLoading: boolean;
  templatesError: Error | null;
  templatesResponse: ProviderTemplatesResponse;
  selectedTemplateId: string | null;
  onSelectTemplate: (template: ProviderTemplate) => void;
};

export default function ProviderTemplateSelector({
  templatesLoading,
  templatesError,
  templatesResponse,
  selectedTemplateId,
  onSelectTemplate,
}: ProviderTemplateSelectorProps) {
  return (
    <Grid size={{ xs: 12 }}>
      <FormControl fullWidth>
        <Stack spacing={0.5}>
          <FormLabel>Select the provider</FormLabel>
        </Stack>
        {templatesLoading ? (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mt: 1.5 }}>
            <CircularProgress size={20} />
            <Typography variant="body2" color="text.secondary">
              <FormattedMessage
                id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.AddNewProvider.ProviderTemplateSelector.loading.provider.templates"
                defaultMessage={'Loading provider templates...'}
              />
            </Typography>
          </Box>
        ) : templatesError ? (
          <Typography variant="body2" color="error" sx={{ mt: 1.5 }}>
            <FormattedMessage
              id="aiWorkspace.pages.appShell.appShellPages.serviceProvider.AddNewProvider.ProviderTemplateSelector.failed.to.load.provider.templates"
              defaultMessage={'Failed to load provider templates:'}
            />{' '}
            {templatesError.message}
          </Typography>
        ) : (
          <Box
            sx={{
              mt: 1.5,
              display: 'grid',
              gap: 1.5,
              gridTemplateColumns: {
                xs: '1fr',
                sm: 'repeat(2, 1fr)',
                md: 'repeat(3, 1fr)',
              },
            }}
          >
            {templatesResponse?.list?.map((template) => {
              const templateId = (template.id ?? '').toLowerCase();
              const isComingSoon = COMING_SOON_TEMPLATE_IDS.has(templateId);
              const isSelected = !isComingSoon && selectedTemplateId === template.id;
              const logo = getLogoForTemplate(template.name);
              const shortName = getShortNameForTemplate(template.name);
              return (
                <Box
                  key={template.id}
                  role="button"
                  aria-disabled={isComingSoon}
                  tabIndex={isComingSoon ? -1 : 0}
                  onClick={() => {
                    if (isComingSoon) return;
                    onSelectTemplate(template);
                  }}
                  onKeyDown={(event) => {
                    if (isComingSoon) return;
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault();
                      onSelectTemplate(template);
                    }
                  }}
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 1.5,
                    py: 1,
                    paddingLeft: 1,
                    borderRadius: 1,
                    border: '1px solid',
                    borderColor: isSelected ? 'primary.main' : 'divider',
                    backgroundColor: isSelected
                      ? 'rgba(0, 0, 0, 0.04)'
                      : 'background.paper',
                    boxShadow: isSelected
                      ? '0 0 0 2px rgba(0, 0, 0, 0.08)'
                      : 'none',
                    transition:
                      'border-color 0.2s ease, box-shadow 0.2s ease, transform 0.2s ease',
                    cursor: isComingSoon ? 'not-allowed' : 'pointer',
                    ...(isComingSoon
                      ? null
                      : {
                          '&:hover': {
                            borderColor: 'primary.main',
                            boxShadow: '0 6px 16px rgba(0, 0, 0, 0.08)',
                            transform: 'translateY(-1px)',
                          },
                        }),
                  }}
                >
                  <Box
                    sx={{
                      width: 40,
                      height: 40,
                      borderRadius: 1,
                      border: '1px solid',
                      borderColor: 'divider',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      backgroundColor: 'background.default',
                      overflow: 'hidden',
                    }}
                  >
                    {logo ? (
                      <Box
                        component="img"
                        src={logo}
                        alt={`${template.name} logo`}
                        sx={{
                          width: '90%',
                          height: '90%',
                          borderRadius: 1,
                          objectFit: 'contain',
                        }}
                      />
                    ) : (
                      <Avatar sx={{ width: 36, height: 36, fontSize: 14 }}>
                        {shortName}
                      </Avatar>
                    )}
                  </Box>
                  <Box sx={{ minWidth: 0, flex: 1 }}>
                    <Typography variant="subtitle2">{template.name}</Typography>
                    {template.description && (
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        noWrap
                      >
                        {template.description}
                      </Typography>
                    )}
                  </Box>
                  {isComingSoon ? (
                    <Chip
                      label="Coming soon"
                      size="small"
                      sx={{
                        flexShrink: 0,
                        fontSize: 10,
                        marginRight: 1,
                        bgcolor: '#EA6A33',
                        color: '#FFFFFF',
                        '& .MuiChip-label': {
                          px: 1,
                        },
                      }}
                    />
                  ) : null}
                </Box>
              );
            })}
          </Box>
        )}
      </FormControl>
    </Grid>
  );
}
