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

import { CircularProgress, Form, Grid, Stack, Typography } from "@wso2/oxygen-ui";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { debounce } from "lodash";
import { useGenerateResourceName } from "@agent-management-platform/api-client";
import { LabelsEditor, MarkdownEditor } from "@agent-management-platform/shared-component";
import { AgentHandleField } from "../components/AgentHandleField";
import { ConnectAgentFormValues } from "../form/schema";
import { INPUT_LIMITS } from "@agent-management-platform/types";
import { TextInput } from "@agent-management-platform/views";

interface ExternalAgentFormProps {
  formData: ConnectAgentFormValues;
  setFormData: React.Dispatch<React.SetStateAction<ConnectAgentFormValues>>;
  errors: Record<string, string | undefined>;
  setFieldError: (
    field: keyof ConnectAgentFormValues,
    error: string | undefined
  ) => void;
  validateField: (
    field: keyof ConnectAgentFormValues,
    value: unknown,
    fullData?: ConnectAgentFormValues
  ) => string | undefined;
}

export const ExternalAgentForm = ({
  formData,
  setFormData,
  errors,
  setFieldError,
  validateField,
}: ExternalAgentFormProps) => {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();
  const [isHandleEditable, setIsHandleEditable] = useState(false);
  const { mutate: generateName, isPending: isGeneratingName } = useGenerateResourceName({
    orgName: orgId,
  });

  const handleFieldChange = useCallback(
    (field: keyof ConnectAgentFormValues, value: unknown) => {
      setFormData((prevData) => {
        const newData = { ...prevData, [field]: value };
        const error = validateField(field, value, newData);
        setFieldError(field, error);
        return newData;
      });
    },
    [setFormData, validateField, setFieldError]
  );

  // Create debounced function for name generation
  const debouncedGenerateName = useMemo(
    () =>
      debounce((name: string) => {
        if(name.length < 3) {
          handleFieldChange("name", "");
          return;
        }
        generateName({
          displayName: name,
          resourceType: 'agent',
          projectName: projectId,
        }, {
          onSuccess: (data: { name: string }) => {
            handleFieldChange("name", data.name);
          },
          onError: (error: unknown) => {
            // eslint-disable-next-line no-console
            console.error('Failed to generate name:', error);
          }
        });
      }, 500),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [generateName, projectId, orgId, handleFieldChange]
  );

  // Cleanup debounce on unmount
  useEffect(() => {
    return () => {
      debouncedGenerateName.cancel();
    };
  }, [debouncedGenerateName]);

  // Auto-generate name from display name using API with debounce
  useEffect(() => {
    if (isHandleEditable) {
      debouncedGenerateName.cancel();
      return;
    }
    if (formData.displayName && formData.displayName.length >= 3) {
      debouncedGenerateName(formData.displayName);
    } else {
      debouncedGenerateName.cancel();
      handleFieldChange("name", "");
    }
  }, [formData.displayName, debouncedGenerateName, handleFieldChange, isHandleEditable]);

  return (
    <Form.Stack spacing={3}>
      <Stack spacing={2}>
        <Typography variant="subtitle1">Agent Details</Typography>
        <Grid container spacing={2}>
          <Grid size={{ xs: 12, md: 6, xl: 4 }}>
            <TextInput
              label="Name"
              id="displayName"
              maxLength={INPUT_LIMITS.NAME}
              hideCharacterCount={formData.displayName.length < INPUT_LIMITS.NAME}
              placeholder="e.g., Customer Support"
              value={formData.displayName}
              onChange={(e) => handleFieldChange('displayName', e.target.value)}
              error={!!errors.displayName}
              helperText={
                isGeneratingName ? (
                  <Stack component="span" direction="row" alignItems="center" gap={1}>
                    <CircularProgress size={12} />
                    <Typography variant="caption">Validating name...</Typography>
                  </Stack>
                ) : (
                  errors.displayName
                )
              }
            />
          </Grid>
          <Grid size={{ xs: 12, md: 6, xl: 4 }}>
            <AgentHandleField
              value={formData.name}
              editable={isHandleEditable}
              onEdit={() => setIsHandleEditable(true)}
              onChange={(value) => handleFieldChange("name", value)}
              error={errors.name}
            />
          </Grid>
          <Grid size={{ xl: 4 }} sx={{ display: { xs: 'none', xl: 'block' } }} />
          <Grid size={{ xs: 12, md: 12, xl: 8 }}>
            <Form.ElementWrapper label="Description (optional)" name="description">
              <MarkdownEditor
                id="description"
                placeholder="Short description of what this agent does. Markdown is supported."
                value={formData.description || ''}
                onChange={(value) => handleFieldChange('description', value)}
                error={!!errors.description}
                helperText={errors.description}
              />
            </Form.ElementWrapper>
          </Grid>
          <Grid size={{ xs: 12, md: 12, xl: 8 }}>
            <Form.ElementWrapper label="Labels (optional)" name="labels">
              <LabelsEditor
                hideTitle
                description="Attach key/value labels to organize and filter agents."
                value={formData.labels ?? {}}
                onChange={(labels) => handleFieldChange("labels", labels)}
              />
            </Form.ElementWrapper>
          </Grid>
        </Grid>
      </Stack>
    </Form.Stack>
  );
};
