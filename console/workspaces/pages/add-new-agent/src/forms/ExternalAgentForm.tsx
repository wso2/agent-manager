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

import { CircularProgress, Form, TextField, useTheme } from "@wso2/oxygen-ui";
import { useCallback, useEffect, useMemo } from "react";
import { useParams } from "react-router-dom";
import { debounce } from "lodash";
import { useGenerateResourceName } from "@agent-management-platform/api-client";
import { ConnectAgentFormValues } from "../form/schema";
import { Check } from "@wso2/oxygen-ui-icons-react";

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
  const theme = useTheme();
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
    if (formData.displayName) {
      debouncedGenerateName(formData.displayName);
    } else if (!formData.displayName) {
      debouncedGenerateName.cancel();
      handleFieldChange("name", "");
    }
  }, [formData.displayName, debouncedGenerateName, handleFieldChange]);

  return (
    <Form.Stack spacing={3}>
      <Form.Section>
        <Form.Subheader>Agent Details</Form.Subheader>
        <Form.Stack spacing={2}>
          <Form.ElementWrapper label="Name" name="displayName">
            <TextField
              id="displayName"
              placeholder="e.g., Customer Support"
              value={formData.displayName}
              onChange={(e) => handleFieldChange('displayName', e.target.value)}
              error={!!errors.displayName}
              helperText={errors.displayName}
              fullWidth
              slotProps={{
                input: {
                  endAdornment: isGeneratingName ?
                    <CircularProgress size={20} /> :
                    (!!formData.name &&
                      <Check size={20} color={theme.vars?.palette.success.main} />),
                },
              }}
            />
          </Form.ElementWrapper>
          <Form.ElementWrapper label="Description (optional)" name="description">
            <TextField
              id="description"
              placeholder="Short description of what this agent does"
              multiline
              minRows={2}
              maxRows={6}
              value={formData.description || ''}
              onChange={(e) => handleFieldChange('description', e.target.value)}
              error={!!errors.description}
              helperText={errors.description}
              fullWidth
            />
          </Form.ElementWrapper>
        </Form.Stack>
      </Form.Section>
    </Form.Stack>
  );
};
